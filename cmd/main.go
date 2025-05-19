package main

import (
	"crypto/tls"
	"flag"
	"os"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/antimetal/agent/internal/intake"
	k8sagent "github.com/antimetal/agent/internal/kubernetes/agent"
	"github.com/antimetal/agent/internal/kubernetes/cluster"
	"github.com/antimetal/agent/internal/kubernetes/scheme"
	"github.com/antimetal/agent/pkg/resource/store"
)

var (
	setupLog logr.Logger

	// CLI Options
	intakeAddr           string
	intakeAPIKey         string
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	secureMetrics        bool
	enableHTTP2          bool
	enableK8sController  bool
	kubernetesProvider   string
	eksAccountID         string
	eksRegion            string
	eksClusterName       string
	eksAutodiscover      bool
)

func init() {
	flag.StringVar(&intakeAddr, "intake-address", "localhost:50051",
		"The address of the cloud inventory intake service")
	flag.StringVar(&intakeAPIKey, "intake-api-key", "",
		"The API key to use upload resources",
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080",
		"The address the metric endpoint binds to. Set this to '0' to disable the metrics server")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to. Set this to '0' to disable the metrics server")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&enableK8sController, "enable-kubernetes-controller", true,
		"Enable Kubernetes cluster snapshot collector")
	flag.StringVar(&kubernetesProvider, "kubernetes-provider", "kind", "The Kubernetes provider")
	flag.StringVar(&eksAccountID, "kubernetes-provider-eks-account-id", "",
		"The AWS account ID the EKS cluster is deployed in")
	flag.StringVar(&eksRegion, "kubernetes-provider-eks-region", "",
		"The AWS region the EKS cluster is deployed in")
	flag.StringVar(&eksClusterName, "kubernetes-provider-eks-cluster-name", "",
		"The name of the EKS cluster")
	flag.BoolVar(&eksAutodiscover, "kubernetes-provider-eks-autodiscover", true,
		"Autodiscover EKS cluster name")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog = ctrl.Log.WithName("setup")
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Get(),
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "4927b366.antimetal.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Shared resources
	rsrcStore, err := store.New()
	if err != nil {
		setupLog.Error(err, "unable to create resource inventory")
		os.Exit(1)
	}
	if err := mgr.Add(rsrcStore); err != nil {
		setupLog.Error(err, "unable to register resource inventory")
		os.Exit(1)
	}

	intakeConn, err := grpc.NewClient(intakeAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		setupLog.Error(err, "unable to connect to cloud inventory service")
		os.Exit(1)
	}

	// Setup Intake Worker
	intakeWorker, err := intake.NewWorker(rsrcStore,
		intake.WithLogger(mgr.GetLogger().WithName("intake-worker")),
		intake.WithGRPCConn(intakeConn),
		intake.WithAPIKey(intakeAPIKey),
	)
	if err != nil {
		setupLog.Error(err, "unable to create intake worker")
		os.Exit(1)
	}
	if err := mgr.Add(intakeWorker); err != nil {
		setupLog.Error(err, "unable to register intake worker")
		os.Exit(1)
	}

	// Setup Kubernetes Collector Controller
	if enableK8sController {
		providerOpts := getProviderOptions(setupLog.WithName("cluster-provider"))
		provider, err := cluster.GetProvider(ctx, kubernetesProvider, providerOpts)
		if err != nil {
			setupLog.Error(err, "unable to determine cluster provider")
			os.Exit(1)
		}
		ctrl := &k8sagent.Controller{
			Provider: provider,
			Store:    rsrcStore,
		}
		if err := ctrl.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "K8sCollector")
			os.Exit(1)
		}
	}

	// Final setup and start Manager
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getProviderOptions(logger logr.Logger) cluster.ProviderOptions {
	return cluster.ProviderOptions{
		Logger: logger,
		EKS: cluster.EKSOptions{
			Autodiscover: eksAutodiscover,
			AccountID:    eksAccountID,
			Region:       eksRegion,
			ClusterName:  eksClusterName,
		},
	}
}

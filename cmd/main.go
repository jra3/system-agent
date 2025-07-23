// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	zapcore "go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/antimetal/agent/internal/intake"
	k8sagent "github.com/antimetal/agent/internal/kubernetes/agent"
	"github.com/antimetal/agent/internal/kubernetes/cluster"
	"github.com/antimetal/agent/internal/kubernetes/scheme"
	"github.com/antimetal/agent/pkg/performance"
	"github.com/antimetal/agent/pkg/resource/store"
)

var (
	setupLog logr.Logger

	// CLI Options
	intakeAddr           string
	intakeAPIKey         string
	intakeSecure         bool
	metricsAddr          string
	metricsSecure        bool
	metricsCertDir       string
	metricsCertName      string
	metricsKeyName       string
	enableLeaderElection bool
	probeAddr            string
	enableHTTP2          bool
	enableK8sController  bool
	kubernetesProvider   string
	eksAccountID         string
	eksRegion            string
	eksClusterName       string
	eksAutodiscover      bool
	maxStreamAge         time.Duration
	pprofAddr            string
)

func init() {
	flag.StringVar(&intakeAddr, "intake-address", "intake.antimetal.com:443",
		"The address of the cloud inventory intake service")
	flag.StringVar(&intakeAPIKey, "intake-api-key", "",
		"The API key to use upload resources",
	)
	flag.BoolVar(&intakeSecure, "intake-secure", true,
		"Use secure connection to the Antimetal intake service",
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080",
		"The address the metric endpoint binds to. Set this to '0' to disable the metrics server")
	flag.BoolVar(&metricsSecure, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.StringVar(&metricsCertDir, "metrics-cert-dir", "",
		"The directory where the metrics server TLS certificates are stored.",
	)
	flag.StringVar(&metricsCertName, "metrics-cert-name", "",
		"The name of the TLS certificate file for the metrics server.",
	)
	flag.StringVar(&metricsKeyName, "metrics-key-name", "",
		"The name of the TLS key file for the metrics server.",
	)
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to. Set this to '0' to disable the metrics server")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
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
	flag.DurationVar(&maxStreamAge, "max-stream-age", 10*time.Minute,
		"Maximum age of the intake stream before it is reset")
	flag.StringVar(&pprofAddr, "pprof-address", "0",
		"The address the pprof server binds to. Set this to '0' to disable the pprof server")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog = ctrl.Log.WithName("setup")
}

func main() {
	// Check for subcommands
	if len(os.Args) > 1 && os.Args[1] == "test-collectors" {
		runCollectorTest()
		return
	}

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

	metricsServerOpts := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: metricsSecure,
		TLSOpts:       tlsOpts,
	}
	if metricsSecure {
		metricsServerOpts.FilterProvider = filters.WithAuthenticationAndAuthorization

		// NOTE: If CertDir, CertName, and KeyName are empty, controller-runtime will
		// automatically generate self-signed certificates for the metrics server. While convenient for
		// development and testing, this setup is not recommended for production.
		metricsServerOpts.CertDir = metricsCertDir
		metricsServerOpts.CertName = metricsCertName
		metricsServerOpts.KeyName = metricsKeyName
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme.Get(),
		Metrics:                metricsServerOpts,
		HealthProbeBindAddress: probeAddr,
		PprofBindAddress:       pprofAddr,
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

	var creds credentials.TransportCredentials
	if intakeSecure {
		creds = credentials.NewTLS(&tls.Config{})
	} else {
		creds = insecure.NewCredentials()
	}
	intakeConn, err := grpc.NewClient(intakeAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 5 * time.Minute,
		}),
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
		intake.WithMaxStreamAge(maxStreamAge),
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

func runCollectorTest() {
	// Parse test-collectors specific flags
	testFlags := flag.NewFlagSet("test-collectors", flag.ExitOnError)
	interval := testFlags.Duration("interval", 5*time.Second, "Collection interval")
	verbose := testFlags.Bool("verbose", false, "Enable verbose logging")
	prettyPrint := testFlags.Bool("pretty", true, "Pretty print JSON output")
	metricTypes := testFlags.String("metrics", "", "Comma-separated list of metric types to collect (empty for all)")

	testFlags.Parse(os.Args[2:])

	// Setup logger
	var logger logr.Logger
	if *verbose {
		zapLog, _ := zapcore.NewDevelopment()
		logger = zapr.NewLogger(zapLog)
	} else {
		logger = logr.Discard()
	}

	// Use environment variables from container
	config := performance.CollectionConfig{
		HostProcPath: getEnvOrDefault("HOST_PROC", "/proc"),
		HostSysPath:  getEnvOrDefault("HOST_SYS", "/sys"),
		HostDevPath:  getEnvOrDefault("HOST_DEV", "/dev"),
	}

	// Get available metric types from registry
	availableTypes := []performance.MetricType{
		performance.MetricTypeLoad,
		performance.MetricTypeMemory,
		performance.MetricTypeCPU,
		performance.MetricTypeProcess,
		performance.MetricTypeDisk,
		performance.MetricTypeNetwork,
		performance.MetricTypeTCP,
		performance.MetricTypeKernel,
		performance.MetricTypeCPUInfo,
		performance.MetricTypeMemoryInfo,
		performance.MetricTypeDiskInfo,
		performance.MetricTypeNetworkInfo,
	}

	// Filter metric types if specified
	if *metricTypes != "" {
		requestedTypes := strings.Split(*metricTypes, ",")
		var filteredTypes []performance.MetricType
		for _, requested := range requestedTypes {
			requested = strings.TrimSpace(requested)
			for _, available := range availableTypes {
				if string(available) == requested {
					filteredTypes = append(filteredTypes, available)
					break
				}
			}
		}
		availableTypes = filteredTypes
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Collection ticker
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	fmt.Printf("Starting collector test (interval: %v)\n", *interval)
	fmt.Printf("Testing metric types: %v\n", availableTypes)
	fmt.Printf("Using paths: proc=%s, sys=%s, dev=%s\n", config.HostProcPath, config.HostSysPath, config.HostDevPath)
	fmt.Printf("Press Ctrl+C to stop\n\n")

	for {
		select {
		case <-ticker.C:
			collectAndPrintTest(availableTypes, config, logger, *prettyPrint)
		case <-sigChan:
			fmt.Println("\nStopping collector test...")
			return
		case <-ctx.Done():
			return
		}
	}
}

func collectAndPrintTest(metricTypes []performance.MetricType, config performance.CollectionConfig, logger logr.Logger, prettyPrint bool) {
	fmt.Printf("=== Collection at %s ===\n", time.Now().Format(time.RFC3339))

	for _, metricType := range metricTypes {
		fmt.Printf("\n--- %s ---\n", metricType)

		// Get collector factory from registry
		factory, err := performance.GetCollector(metricType)
		if err != nil {
			fmt.Printf("Error getting collector: %v\n", err)
			continue
		}

		// Create collector instance
		collector, err := factory(logger, config)
		if err != nil {
			fmt.Printf("Error creating collector: %v\n", err)
			continue
		}

		// Start the collector and get one data point
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		dataChan, err := collector.Start(ctx)
		if err != nil {
			fmt.Printf("Error starting collector: %v\n", err)
			cancel()
			continue
		}

		// Get first data point with timeout
		select {
		case data := <-dataChan:
			if data != nil {
				var output []byte
				var marshalErr error

				if prettyPrint {
					output, marshalErr = json.MarshalIndent(data, "", "  ")
				} else {
					output, marshalErr = json.Marshal(data)
				}

				if marshalErr != nil {
					fmt.Printf("Error marshaling data: %v\n", marshalErr)
				} else {
					fmt.Printf("%s\n", output)
				}
			} else {
				fmt.Printf("No data received\n")
			}
		case <-time.After(5 * time.Second):
			fmt.Printf("Timeout waiting for data\n")
		}

		// Stop the collector
		collector.Stop()
		cancel()
	}

	fmt.Printf("\n%s\n\n", strings.Repeat("=", 50))
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

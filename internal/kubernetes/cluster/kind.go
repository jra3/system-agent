// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package cluster

import (
	"context"
	"fmt"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	ctrl "sigs.k8s.io/controller-runtime"
)

type KIND struct{}

var _ Provider = &KIND{}

func (k *KIND) Name() string {
	return ProviderKIND
}

func (p *KIND) ClusterName(ctx context.Context) (string, error) {
	return p.getClusterName(ctx)
}

func (p *KIND) Region(ctx context.Context) (string, error) {
	return "", nil
}

func (p *KIND) getClusterName(ctx context.Context) (string, error) {
	cfg := ctrl.GetConfigOrDie()

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}

	cm, err := clientset.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	data, ok := cm.Data[bootstrapapi.KubeConfigKey]
	if !ok {
		return "", fmt.Errorf("kubeconfig not found in cluster-info ConfigMap: %w", err)
	}

	kubecfg, err := clientcmd.Load([]byte(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse kubeconfig from cluster-info ConfigMap: %w", err)
	}

	if len(kubecfg.Clusters) == 0 {
		return "", fmt.Errorf("no clusters found in cluster-info configmap")
	} else if len(kubecfg.Clusters) > 1 {
		return "", fmt.Errorf("multiple clusters found in cluster-info configmap, expected only one")
	}

	var clusterName string

	for name, cluster := range kubecfg.Clusters {
		clusterName = name
		if clusterName == "" {
			u, err := url.ParseRequestURI(cluster.Server)
			if err != nil {
				return "", fmt.Errorf("failed to parse server URL: %w", err)
			}
			clusterName = u.Hostname()
		}
	}

	return clusterName, nil
}

// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package cluster

import (
	"context"

	"github.com/antimetal/agent/pkg/aws"
)

type EKS struct {
	awsClient aws.Client
}

var _ Provider = &EKS{}

func (p *EKS) Name() string {
	return ProviderEKS
}

func (p *EKS) ClusterName(ctx context.Context) (string, error) {
	return p.awsClient.GetEKSClusterName(ctx)
}

func (p *EKS) Region(ctx context.Context) (string, error) {
	return p.awsClient.GetRegion(ctx)
}

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

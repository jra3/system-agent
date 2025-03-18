package cluster

import (
	"context"
	"fmt"

	"github.com/antimetal/agent/pkg/aws"
	"github.com/go-logr/logr"
)

const (
	ProviderEKS  = "eks"
	ProviderGKE  = "gke"
	ProviderAKS  = "aks"
	ProviderKIND = "kind"
)

type Provider interface {
	Name() string
	ClusterName(ctx context.Context) (string, error)
	Region(ctx context.Context) (string, error)
}

type ProviderOptions struct {
	Logger logr.Logger
	EKS    EKSOptions
}

type EKSOptions struct {
	Autodiscover bool
	AccountID    string
	Region       string
	ClusterName  string
}

func GetProvider(ctx context.Context, provider string, opts ProviderOptions) (Provider, error) {
	switch provider {
	case ProviderEKS:
		awsClient, err := aws.NewClient(constructAwsClientOpts(ctx, opts)...)
		if err != nil {
			return nil, fmt.Errorf("error creating EKS provider: failed to create AWS client: %w", err)
		}
		return &EKS{
			awsClient: awsClient,
		}, nil
	case ProviderGKE:
		return nil, fmt.Errorf("provider %s not implemented", provider)
	case ProviderAKS:
		return nil, fmt.Errorf("provider %s not implemented", provider)
	case ProviderKIND:
		return &KIND{}, nil
	default:
		return nil, fmt.Errorf("unrecognized provider: %s", provider)
	}
}

func constructAwsClientOpts(ctx context.Context, opts ProviderOptions) []aws.ClientOption {
	clientOpts := []aws.ClientOption{
		aws.WithLogger(opts.Logger),
	}
	if opts.EKS.AccountID != "" {
		clientOpts = append(clientOpts, aws.WithAccountID(opts.EKS.AccountID))
	}
	if opts.EKS.Region != "" {
		clientOpts = append(clientOpts, aws.WithRegion(opts.EKS.Region))
	}
	if opts.EKS.ClusterName != "" {
		clientOpts = append(clientOpts, aws.WithEKSClusterName(opts.EKS.ClusterName))
	}
	if opts.EKS.Autodiscover {
		clientOpts = append(clientOpts, aws.WithAutoDiscovery(ctx))
	}
	return clientOpts
}

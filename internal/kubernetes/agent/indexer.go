package agent

import (
	"context"
	"fmt"

	"github.com/antimetal/agent/internal/kubernetes/cluster"
	"github.com/antimetal/agent/pkg/errors"
	"github.com/antimetal/agent/pkg/resource"
	k8sv1 "github.com/antimetal/apis/gengo/kubernetes/v1"
	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/api/meta"
)

type indexer struct {
	provider cluster.Provider
	store    resource.Store
	mapper   meta.RESTMapper
}

func (i *indexer) LoadClusterInfo(ctx context.Context, major string, minor string) error {
	clusterName, err := i.provider.ClusterName(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster name: %w", err)
	}
	region, err := i.provider.Region(ctx)
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	}
	cluster := &k8sv1.Cluster{
		MajorVersion: major,
		MinorVersion: minor,
		Provider:     getProvider(i.provider),
	}
	clusterAny, err := anypb.New(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}

	return i.store.AddResource(resource.KubeCluster, &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Kind: resource.KindResource,
			Type: string(cluster.ProtoReflect().Descriptor().FullName()),
		},
		Metadata: &resourcev1.ResourceMeta{
			Provider:   resourcev1.Provider_PROVIDER_KUBERNETES,
			ProviderId: clusterName,
			Name:       clusterName,
			Region:     region,
		},
		Spec: clusterAny,
	})
}

func (i *indexer) Add(ctx context.Context, obj object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	rsrc, rels, err := i.generate(obj)
	if err != nil {
		return fmt.Errorf("failed to generate resource and relationships: %w", err)
	}
	mapping, err := i.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping: %w", err)
	}
	if err := i.store.AddResource(resource.KubeObject(mapping.Resource, obj), rsrc); err != nil {
		return fmt.Errorf("failed to add resource to inventory: %w", err)
	}
	if err := i.store.AddRelationships(rels...); err != nil {
		return fmt.Errorf("failed to add relationships for resource to inventory: %w", err)
	}
	return nil
}

func (i *indexer) Update(ctx context.Context, obj object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	rsrc, rels, err := i.generate(obj)
	if err != nil {
		return fmt.Errorf("failed to generate resource: %w", err)
	}
	mapping, err := i.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping: %w", err)
	}
	if err := i.store.UpdateResource(resource.KubeObject(mapping.Resource, obj), rsrc); err != nil {
		return fmt.Errorf("failed to update resource to inventory: %w", err)
	}

	for _, rel := range rels {
		pred, err := anypb.UnmarshalNew(rel.Predicate, proto.UnmarshalOptions{})
		if err != nil {
			return fmt.Errorf("failed to unmarshal predicate: %w", err)
		}
		rels, err := i.store.GetRelationships(string(rel.Subject), string(rel.Object), pred)
		if err != nil {
			err = fmt.Errorf("failed to find existing relationships: %w", err)
			return errors.NewRetryable(err.Error())
		}
		if len(rels) == 0 {
			if err := i.store.AddRelationships(rel); err != nil {
				err = fmt.Errorf("failed to add relationship: %w", err)
				return errors.NewRetryable(err.Error())
			}
		}
	}
	return nil
}

func (i *indexer) Delete(ctx context.Context, obj object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	mapping, err := i.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping: %w", err)
	}
	return i.store.DeleteResource(resource.KubeObject(mapping.Resource, obj))
}

func getProvider(prov cluster.Provider) k8sv1.ClusterProvider {
	switch prov.Name() {
	case cluster.ProviderEKS:
		return k8sv1.ClusterProvider_CLUSTER_PROVIDER_EKS
	case cluster.ProviderGKE:
		return k8sv1.ClusterProvider_CLUSTER_PROVIDER_GKE
	case cluster.ProviderAKS:
		return k8sv1.ClusterProvider_CLUSTER_PROVIDER_AKS
	case cluster.ProviderKIND:
		return k8sv1.ClusterProvider_CLUSTER_PROVIDER_KIND
	default:
		return k8sv1.ClusterProvider_CLUSTER_PROVIDER_OTHER
	}
}

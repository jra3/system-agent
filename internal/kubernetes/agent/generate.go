package agent

import (
	"fmt"

	"github.com/antimetal/agent/internal/kubernetes/scheme"
	"github.com/antimetal/agent/pkg/errors"
	"github.com/antimetal/agent/pkg/resource"
	k8sv1 "github.com/antimetal/apis/gengo/kubernetes/v1"
	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (i *indexer) generate(obj object) (rsrc *resourcev1.Resource, rels []*resourcev1.Relationship, err error) {
	clusterRsrc, err := i.store.GetResource(resource.KubeCluster)
	if err != nil {
		err = fmt.Errorf("failed to get cluster resource, will retry: %w", err)
		return nil, nil, errors.NewRetryable(err.Error())
	}
	clusterName := clusterRsrc.GetMetadata().GetName()

	gvk := obj.GetObjectKind().GroupVersionKind()
	mapping, err := i.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get REST mapping for %s: %w", gvk.String(), err)
	}
	k8sRsrc := k8sResource{
		gvr: mapping.Resource,
		obj: obj,
	}

	var owners []k8sResource
	if obj.GetOwnerReferences() != nil {
		owners = make([]k8sResource, len(obj.GetOwnerReferences()))
		for idx, ownerRef := range obj.GetOwnerReferences() {
			ownerGvk := schema.FromAPIVersionAndKind(ownerRef.APIVersion, ownerRef.Kind)
			ownerMapping, err := i.mapper.RESTMapping(ownerGvk.GroupKind(), ownerGvk.Version)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get REST mapping for %s: %w", ownerGvk.String(), err)
			}
			ownerRuntimeObj, err := scheme.Get().New(ownerGvk)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create runtime object for %s: %w", ownerGvk.String(), err)
			}
			ownerObj, ok := ownerRuntimeObj.(object)
			if !ok {
				return nil, nil, fmt.Errorf("object is not a Kubernetes object: %T", ownerRuntimeObj)
			}
			ownerObj.SetName(ownerRef.Name)
			ownerObj.SetNamespace(obj.GetNamespace())
			ownerObj.SetUID(ownerRef.UID)
			owners[idx] = k8sResource{
				gvr: ownerMapping.Resource,
				obj: ownerObj,
			}
		}
	}

	switch obj := obj.(type) {
	case *corev1.Pod:
		rsrc, rels, err = genPod(i.store, clusterName, k8sRsrc, owners...)
	case *corev1.Node:
		rsrc, rels, err = genNode(clusterName, k8sRsrc, owners...)
	case *corev1.PersistentVolume:
		rsrc, rels, err = genPersistentVolume(clusterName, k8sRsrc, owners...)
	case *corev1.PersistentVolumeClaim:
		rsrc, rels, err = genPersistentVolumeClaim(clusterName, k8sRsrc, owners...)
	case *corev1.Service:
		rsrc, rels, err = genService(clusterName, k8sRsrc, owners...)
	case *appsv1.DaemonSet:
		rsrc, rels, err = genDaemonSet(clusterName, k8sRsrc, owners...)
	case *appsv1.Deployment:
		rsrc, rels, err = genDeployment(clusterName, k8sRsrc, owners...)
	case *appsv1.ReplicaSet:
		rsrc, rels, err = genReplicaSet(clusterName, k8sRsrc, owners...)
	case *appsv1.StatefulSet:
		rsrc, rels, err = genStatefulSet(clusterName, k8sRsrc, owners...)
	case *batchv1.Job:
		rsrc, rels, err = genJob(clusterName, k8sRsrc, owners...)
	default:
		err = fmt.Errorf(
			"no generator found for %s %s/%s", obj.GetObjectKind().GroupVersionKind().String(),
			obj.GetNamespace(), obj.GetName(),
		)
	}

	return
}

func genPod(store resource.Store, clusterName string, k8sRsrc k8sResource, owners ...k8sResource,
) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	podObj, ok := k8sRsrc.obj.(*corev1.Pod)
	if !ok {
		return nil, nil, fmt.Errorf("object is not a Pod; got %s", k8sRsrc.obj.GetObjectKind().GroupVersionKind().String())
	}

	rsrc, rels, err := genBase(clusterName, k8sRsrc, owners...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource and base relationships: %w", err)
	}

	if podObj.Spec.NodeName != "" {
		nodeRsrc, err := store.GetResource(resource.KubeObject(
			schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "nodes",
			},
			&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: podObj.Spec.NodeName,
				},
			},
		))
		if err != nil {
			err = fmt.Errorf("failed to get node resource: %w", err)
			return nil, nil, errors.NewRetryable(err.Error())
		}
		rsrc.GetMetadata().Region = nodeRsrc.GetMetadata().Region
		rsrc.GetMetadata().Zone = nodeRsrc.GetMetadata().Zone
	}

	for _, volume := range podObj.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvc := k8sResource{
				gvr: schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "persistentvolumeclaims",
				},
				obj: &corev1.PersistentVolumeClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "PersistentVolumeClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      volume.PersistentVolumeClaim.ClaimName,
						Namespace: podObj.GetNamespace(),
					},
				},
			}
			volumeMount := &k8sv1.VolumeMount{}
			volumeMountAny, err := anypb.New(volumeMount)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
			}
			attachedTo := &k8sv1.AttachedTo{}
			attachedToAny, err := anypb.New(attachedTo)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
			}
			rels = append(rels,
				&resourcev1.Relationship{
					Type: &resourcev1.TypeDescriptor{
						Kind: resource.KindRelationship,
						Type: proto.MessageName(volumeMount),
					},
					Subject:   []byte(resource.KubeObject(pvc.gvr, pvc.obj)),
					Object:    []byte(resource.KubeObject(k8sRsrc.gvr, podObj)),
					Predicate: attachedToAny,
				},
				&resourcev1.Relationship{
					Type: &resourcev1.TypeDescriptor{
						Kind: resource.KindRelationship,
						Type: proto.MessageName(attachedTo),
					},
					Subject:   []byte(resource.KubeObject(k8sRsrc.gvr, podObj)),
					Object:    []byte(resource.KubeObject(pvc.gvr, pvc.obj)),
					Predicate: volumeMountAny,
				},
			)
		}
	}

	return rsrc, rels, nil
}

func genNode(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	rsrc, rels, err := genBase(clusterName, k8sRsrc, owners...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource and base relationships: %w", err)
	}
	rsrc.GetMetadata().Region = k8sRsrc.obj.GetLabels()["topology.kubernetes.io/region"]
	rsrc.GetMetadata().Zone = k8sRsrc.obj.GetLabels()["topology.kubernetes.io/zone"]
	return rsrc, rels, nil
}

func genPersistentVolume(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	rsrc, rels, err := genBase(clusterName, k8sRsrc, owners...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource and base relationships: %w", err)
	}
	rsrc.GetMetadata().Region = k8sRsrc.obj.GetLabels()["topology.kubernetes.io/region"]
	rsrc.GetMetadata().Zone = k8sRsrc.obj.GetLabels()["topology.kubernetes.io/zone"]
	return rsrc, rels, nil
}

func genPersistentVolumeClaim(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	pvcObj, ok := k8sRsrc.obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil, nil, fmt.Errorf("object is not a PersistentVolumeClaim; got %s", k8sRsrc.obj.GetObjectKind().GroupVersionKind().String())
	}

	rsrc, rels, err := genBase(clusterName, k8sRsrc, owners...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource and base relationships: %w", err)
	}

	if pvcObj.Spec.VolumeName != "" {
		pv := k8sResource{
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "persistentvolumes",
			},
			obj: &corev1.PersistentVolume{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "PersistentVolume",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: pvcObj.Spec.VolumeName,
				},
			},
		}

		boundBy := &k8sv1.BoundBy{}
		boundByAny, err := anypb.New(boundBy)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
		}
		claimsFrom := &k8sv1.ClaimsFrom{}
		claimsFromAny, err := anypb.New(claimsFrom)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
		}
		rels = append(rels,
			&resourcev1.Relationship{
				Type: &resourcev1.TypeDescriptor{
					Kind: resource.KindRelationship,
					Type: proto.MessageName(claimsFrom),
				},
				Subject:   []byte(resource.KubeObject(k8sRsrc.gvr, pvcObj)),
				Object:    []byte(resource.KubeObject(pv.gvr, pv.obj)),
				Predicate: claimsFromAny,
			},
			&resourcev1.Relationship{
				Type: &resourcev1.TypeDescriptor{
					Kind: resource.KindRelationship,
					Type: proto.MessageName(boundBy),
				},
				Subject:   []byte(resource.KubeObject(pv.gvr, pv.obj)),
				Object:    []byte(resource.KubeObject(k8sRsrc.gvr, pvcObj)),
				Predicate: boundByAny,
			},
		)
	}

	return rsrc, rels, nil
}

func genService(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genDaemonSet(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genDeployment(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genReplicaSet(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genStatefulSet(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genJob(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	return genBase(clusterName, k8sRsrc, owners...)
}

func genBase(clusterName string, k8sRsrc k8sResource, owners ...k8sResource) (*resourcev1.Resource, []*resourcev1.Relationship, error) {
	data, err := k8sRsrc.obj.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Kind: resource.KindResource,
			Type: proto.MessageName(k8sRsrc.obj),
		},
		Metadata: &resourcev1.ResourceMeta{
			Provider:   resourcev1.Provider_PROVIDER_KUBERNETES,
			ProviderId: string(k8sRsrc.obj.GetUID()),
			Name:       k8sRsrc.obj.GetName(),
			Namespace: &resourcev1.Namespace{
				Namespace: &resourcev1.Namespace_Kube{
					Kube: &resourcev1.KubernetesNamespace{
						Cluster:   clusterName,
						Namespace: k8sRsrc.obj.GetNamespace(),
					},
				},
			},
			Tags: labelsToTags(k8sRsrc.obj.GetLabels()),
		},
		Spec: &anypb.Any{
			TypeUrl: proto.MessageName(k8sRsrc.obj),
			Value:   data,
		},
	}

	// Add relationships to the cluster and the object.
	rels := make([]*resourcev1.Relationship, 0, len(owners)+2)
	contains := &k8sv1.Contains{}
	containsAny, err := anypb.New(contains)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
	}
	containedBy := &k8sv1.ContainedBy{}
	containedByAny, err := anypb.New(containedBy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create predicate: %w", err)
	}
	rels = append(rels,
		&resourcev1.Relationship{
			Type: &resourcev1.TypeDescriptor{
				Kind: resource.KindRelationship,
				Type: proto.MessageName(contains),
			},
			Subject:   []byte(resource.KubeCluster),
			Object:    []byte(resource.KubeObject(k8sRsrc.gvr, k8sRsrc.obj)),
			Predicate: containsAny,
		},
		&resourcev1.Relationship{
			Type: &resourcev1.TypeDescriptor{
				Kind: resource.KindRelationship,
				Type: proto.MessageName(containedBy),
			},
			Subject:   []byte(resource.KubeObject(k8sRsrc.gvr, k8sRsrc.obj)),
			Object:    []byte(resource.KubeCluster),
			Predicate: containedByAny,
		},
	)

	// Add relationships to the resource owners if any.
	for _, owner := range owners {
		owns := &k8sv1.Owns{}
		ownsAny, err := anypb.New(owns)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create owns predicate: %w", err)
		}
		ownedBy := &k8sv1.OwnedBy{}
		ownedByAny, err := anypb.New(ownedBy)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create ownedBy predicate: %w", err)
		}
		rels = append(rels,
			&resourcev1.Relationship{
				Type: &resourcev1.TypeDescriptor{
					Kind: resource.KindRelationship,
					Type: proto.MessageName(owns),
				},
				Subject:   []byte(resource.KubeObject(owner.gvr, owner.obj)),
				Object:    []byte(resource.KubeObject(k8sRsrc.gvr, k8sRsrc.obj)),
				Predicate: ownsAny,
			},
			&resourcev1.Relationship{
				Type: &resourcev1.TypeDescriptor{
					Kind: resource.KindRelationship,
					Type: proto.MessageName(ownedBy),
				},
				Subject:   []byte(resource.KubeObject(k8sRsrc.gvr, k8sRsrc.obj)),
				Object:    []byte(resource.KubeObject(owner.gvr, owner.obj)),
				Predicate: ownedByAny,
			},
		)
	}

	return rsrc, rels, nil
}

func labelsToTags(labels map[string]string) []*resourcev1.Tag {
	tags := make([]*resourcev1.Tag, len(labels))
	for k, v := range labels {
		tags = append(tags, &resourcev1.Tag{
			Key:   k,
			Value: v,
		})
	}
	return tags
}

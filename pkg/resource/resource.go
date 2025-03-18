package resource

import (
	"strings"

	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type provider = string

const (
	k8s provider = "kubernetes"
)

var (
	KindResource     = string((&resourcev1.Resource{}).ProtoReflect().Descriptor().FullName())
	KindRelationship = string((&resourcev1.Relationship{}).ProtoReflect().Descriptor().FullName())

	KubeCluster = resourceKey(k8s, "cluster")
)

func KubeObject(gvr schema.GroupVersionResource, obj metav1.Object) string {
	s := strings.Builder{}
	s.WriteString(k8s)

	ns := obj.GetNamespace()
	if len(ns) > 0 {
		return resourceKey(k8s, gvr.GroupVersion().String(), gvr.Resource, ns, obj.GetName())
	}
	return resourceKey(k8s, gvr.GroupVersion().String(), gvr.Resource, obj.GetName())
}

func resourceKey(parts ...string) string {
	return strings.Join(parts, "/")
}

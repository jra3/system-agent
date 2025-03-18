package agent

import (
	gogoproto "github.com/gogo/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type object interface {
	gogoproto.Message
	gogoproto.Marshaler
	gogoproto.Unmarshaler
	metav1.Object
	runtime.Object
}

type k8sResource struct {
	gvr schema.GroupVersionResource
	obj object
}

package scheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	// scheme contains all the API types necessary for the K8s dynamic clients
	scheme = runtime.NewScheme()
)

func init() {
	runtimeutil.Must(clientgoscheme.AddToScheme(scheme))
}

// Get returns a scheme with default types registered.
func Get() *runtime.Scheme {
	return scheme
}

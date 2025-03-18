package resource

import (
	"github.com/antimetal/agent/pkg/errors"
	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	"google.golang.org/protobuf/proto"
)

var (
	ErrResourceNotFound      = errors.New("resource not found")
	ErrRelationshipsNotFound = errors.New("relationships not found")
)

// Store persists Resources and their Relationships. Resources are objects that represent a type
// of workload running on the system or cloud resource (e.g. Kubernetes Pod, AWS EC2 instance, etc).
// Resources are identified by a unique name path.
//
// Resources can also have relationships with other resources.
// Relationships are represented by a RDF triplet with a subject, predicate, and object.
type Store interface {
	// GetResource returns the resource located by name.
	// If the resource does not exist, it will return ErrResourceNotFound.
	GetResource(name string) (*resourcev1.Resource, error)

	// AddResource adds rsrc to the inventory located by name.
	// If a resource already exists with the same name, it will return an error.
	AddResource(name string, resource *resourcev1.Resource) error

	// UpdateResource updates a resource located by name with rsrc.
	// If a resource already exists with the same name, it will be merged with rsrc,
	// otherwise a new resource will be add at name.
	UpdateResource(name string, resource *resourcev1.Resource) error

	// DeleteResource deletes the resource located by name.
	// It also cascade deletes all relationships where the resource is the subject
	// or object.
	DeleteResource(name string) error

	// GetRelationships returns all relationships that match the combination subject, object,
	// and predicate with the following invariants:
	//
	// subject == "" matches any subject
	//
	// object == "" matches any object
	//
	// predicateT == nil matches any predicate
	//
	// If there are no matching relationships then it will return ErrRelationshipsNotFound.
	//
	// Examples:
	//
	//   - GetRelationships("foo", "", nil) returns all relationships where subject is "foo".
	//
	//   - GetRelationships("", "", &ConnectedTo{}) returns all relationships where predicate
	//     has a protobuf message type of ConnectedTo between any subject and object.
	//
	//   - GetRelationships("foo", "bar", &ConnectedTo{}) returns all ConnectedTo relationships
	//     between subject "foo" and object "bar".
	GetRelationships(subject, object string, predicateT proto.Message) ([]*resourcev1.Relationship, error)

	// AddRelationships adds rels to the inventory.
	AddRelationships(rels ...*resourcev1.Relationship) error

	// Close closes the inventory store.
	// It should be idempotent - calling Close multiple times will close only once.
	Close() error
}

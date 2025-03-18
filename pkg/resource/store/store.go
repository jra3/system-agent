package store

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"sync"

	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	badger "github.com/dgraph-io/badger/v4"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/antimetal/agent/pkg/errors"
	"github.com/antimetal/agent/pkg/resource"
)

const (
	objKeySize = sha256.Size
)

type keyPart = []byte
type indexKey = []byte
type indexVal = []byte
type objKey = []byte

var (
	resourceKey     = keyPart("rsrc")
	relationshipKey = keyPart("rel")
	index           = keyPart("idx")
	subjectIdx      = keyPart("rel-subj")
	objectIdx       = keyPart("rel-obj")
	predicateIdx    = keyPart("rel-predicate")
)

// Store is a simple store for resources and their relationships.
// Resources are objects that represent a type of workload running on the system
// or cloud resource (e.g. Kubernetes Pod, AWS EC2 instance, etc).
// Resources are identified by a unique name path.
//
// Resources can also have relationships with other resources. Relationships are
// represented by a RDF triplet with a subject, predicate, and object.
type Store struct {
	mu sync.RWMutex

	store *badger.DB
}

// New creates a new Store.
func New() (*Store, error) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		return nil, err
	}
	inv := &Store{
		store: db,
	}
	return inv, nil
}

// AddResource adds rsrc to the resource store located by name.
// If a resource already exists with the same name, it will return an error.
func (s *Store) AddResource(name string, rsrc *resourcev1.Resource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := buildKey(resourceKey, []byte(name))

	return s.store.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			return fmt.Errorf("resource already exists")
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("failed to read resource: %w", err)
		}
		rsrc = rsrc.DeepCopy()
		now := timestamppb.Now()
		rsrc.GetMetadata().CreatedAt = now
		rsrc.GetMetadata().UpdatedAt = now
		data, err := proto.Marshal(rsrc)
		if err != nil {
			return fmt.Errorf("failed to marshal resource: %w", err)
		}

		return txn.Set(key, data)
	})
}

// UpdateResource updates a resource located by name with rsrc.
// If a resource already exists with the same name, it will be merged with rsrc,
// otherwise a new resource will be add at name.
func (s *Store) UpdateResource(name string, rsrc *resourcev1.Resource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := buildKey(resourceKey, []byte(name))
	return s.store.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		// If the resource does not exist, create it
		if errors.Is(err, badger.ErrKeyNotFound) {
			now := timestamppb.Now()
			rsrc.GetMetadata().CreatedAt = now
			rsrc.GetMetadata().UpdatedAt = now
			data, err := proto.Marshal(rsrc)
			if err != nil {
				return fmt.Errorf("failed to marshal resource: %w", err)
			}
			return txn.Set(key, data)
		}
		if err != nil {
			return fmt.Errorf("failed to read resource: %w", err)
		}
		rsrc.GetMetadata().UpdatedAt = timestamppb.Now()
		err = item.Value(func(val []byte) error {
			r := &resourcev1.Resource{}
			err := proto.Unmarshal(val, r)
			if err != nil {
				return fmt.Errorf("failed to unmarshal resource: %w", err)
			}
			rsrc := rsrc.DeepCopy()
			r.Type = rsrc.Type
			r.Metadata = rsrc.Metadata
			r.Spec = rsrc.Spec
			r.GetMetadata().UpdatedAt = timestamppb.Now()
			data, err := proto.Marshal(r)
			if err != nil {
				return fmt.Errorf("failed to marshal resource: %w", err)
			}
			return txn.Set(key, data)
		})
		if err != nil {
			return fmt.Errorf("failed to update resource: %w", err)
		}
		return nil
	})
}

// GetResource returns the resource located by name.
// If the resource does not exist, it will return ErrResourceNotFound.
func (s *Store) GetResource(name string) (*resourcev1.Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := buildKey(resourceKey, []byte(name))
	var val []byte
	err := s.store.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(val)
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, resource.ErrResourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find resource: %w", err)
	}
	rsrc := &resourcev1.Resource{}
	err = proto.Unmarshal(val, rsrc)
	return rsrc, err
}

// DeleteResource deletes the resource located by name.
// It also cascade deletes all relationships where the resource is the subject
// or object.
func (s *Store) DeleteResource(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Update(func(txn *badger.Txn) error {
		delObjs := make([]objKey, 0)

		// 1. Delete all relationships where resource is the subject
		subjectIdxLookup := buildKey(index, subjectIdx, keyPart(name))
		delSubjectObjs, err := deleteIndexedObjects(txn, subjectIdxLookup)
		if err != nil {
			return fmt.Errorf("failed to delete subject relationships: %w", err)
		}
		delObjs = append(delObjs, delSubjectObjs...)

		// 2. Delete all relationships where resource is the object
		objectIdxLookup := buildKey(index, objectIdx, keyPart(name))
		delObjectObjs, err := deleteIndexedObjects(txn, objectIdxLookup)
		if err != nil {
			return fmt.Errorf("failed to delete object relationships: %w", err)
		}
		delObjs = append(delObjs, delObjectObjs...)

		// 3. Update relationship indexes
		if err := txn.Delete(subjectIdxLookup); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("failed to delete subject relationship index: %w", err)
		}
		if err := txn.Delete(objectIdxLookup); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("failed to delete object relationship index: %w", err)
		}
		// TODO: This is pretty expensive - O(delObjs*numPredicateIndexes)
		// An optimization would be to use bloom filters to check whether the index
		// contains the object. That we can only read the index if we know there's an
		// object there saving us KV lookups
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(buildKey(index, predicateIdx)); it.ValidForPrefix(buildKey(index, predicateIdx)); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				for _, obj := range delObjs {
					if err := deleteObjKeyFromIndex(txn, item.Key(), obj); err != nil {
						return fmt.Errorf("failed to update index: %w", err)
					}
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to update predicate index value: %w", err)
			}
		}
		// 4. Finally delete the actual resource
		return txn.Delete(buildKey(resourceKey, []byte(name)))
	})
}

// AddRelationships adds rels to the inventory.
func (s *Store) AddRelationships(rels ...*resourcev1.Relationship) error {
	for _, rel := range rels {
		if bytes.Equal(rel.GetSubject(), rel.GetObject()) {
			return fmt.Errorf(
				"[%s;%s;%s]: subject and object cannot be equal",
				rel.GetSubject(),
				rel.GetPredicate().GetTypeUrl(),
				rel.GetObject(),
			)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Update(func(txn *badger.Txn) error {
		for _, rel := range rels {
			// 1. Write the relationship object
			data, err := proto.Marshal(rel)
			if err != nil {
				return fmt.Errorf("failed to marshal relationship: %w", err)
			}
			h := sha256.Sum256(data)
			if err := txn.Set(buildKey(relationshipKey, h[:]), data); err != nil {
				return fmt.Errorf("failed to write relationship: %w", err)
			}

			// 2. Update the indexes
			predicate := keyPart(strings.TrimPrefix(rel.Predicate.GetTypeUrl(), "type.googleapis.com/"))
			predicateIdxKey := buildKey(index, predicateIdx, predicate)
			if err := addObjKeyToIndex(txn, predicateIdxKey, h[:]); err != nil {
				return fmt.Errorf("failed to update predicate index: %w", err)
			}

			objectIdxKey := buildKey(index, objectIdx, rel.Object)
			if err := addObjKeyToIndex(txn, objectIdxKey, h[:]); err != nil {
				return fmt.Errorf("failed to update object index: %w", err)
			}

			subjectIdxKey := buildKey(index, subjectIdx, rel.Subject)
			if err := addObjKeyToIndex(txn, subjectIdxKey, h[:]); err != nil {
				return fmt.Errorf("failed to update subject index: %w", err)
			}
		}
		return nil
	})
}

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
func (s *Store) GetRelationships(subject, object string, predicateT proto.Message) ([]*resourcev1.Relationship, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rels []*resourcev1.Relationship

	err := s.store.View(func(txn *badger.Txn) error {
		// 1. Decide which indexes to use
		indexes := make([]indexKey, 0)
		if subject != "" {
			indexes = append(indexes, buildKey(index, subjectIdx, keyPart(subject)))
		}
		if object != "" {
			indexes = append(indexes, buildKey(index, objectIdx, keyPart(object)))
		}
		if predicateT != nil {
			predicate := []byte(predicateT.ProtoReflect().Descriptor().FullName())
			indexes = append(indexes, buildKey(index, predicateIdx, predicate))
		}
		if len(indexes) == 0 {
			return resource.ErrRelationshipsNotFound
		}

		// 2. Read the objects keys from the index
		objs, err := readObjKeysFromIndexes(txn, indexes...)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return resource.ErrRelationshipsNotFound
			}
			return fmt.Errorf("failed to read indexed objects: %w", err)
		}

		// 3. Get the relationships objects
		for _, obj := range objs {
			item, err := txn.Get(buildKey(relationshipKey, obj[:]))
			if err != nil {
				return fmt.Errorf("failed to get relationship %x: %w", obj, err)
			}
			rel := &resourcev1.Relationship{}
			err = item.Value(func(val []byte) error {
				return proto.Unmarshal(val, rel)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal relationship %x: %w", obj, err)
			}
			rels = append(rels, rel)
		}
		return nil
	})

	if len(rels) == 0 {
		return nil, resource.ErrRelationshipsNotFound
	}

	return rels, err
}

// Close closes the inventory store.
// It is idempotent - calling Close multiple times will close only once.
func (s *Store) Close() error {
	return s.store.Close()
}

func buildKey(parts ...keyPart) []byte {
	b := bytes.Buffer{}
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		b.WriteByte('/')
		b.Write(p)
	}
	return b.Bytes()
}

func splitObjects(b indexVal) []objKey {
	if len(b) == 0 {
		return nil
	}

	// This assumes that len(b) % chunkSize == 0
	numChunks := len(b) / objKeySize
	chunks := make([]objKey, 0, numChunks)

	for i := 0; i < len(b); i += objKeySize {
		chunks = append(chunks, objKey(b[i:i+objKeySize]))
	}
	return chunks
}

func numObjects(b []byte) int {
	return len(b) / objKeySize
}

func addObjKeyToIndex(txn *badger.Txn, key indexKey, value objKey) error {
	item, err := txn.Get(key)
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return txn.Set(key, value[:])
	}
	return item.Value(func(val []byte) error {
		val = append(val, value[:]...)
		objs := splitObjects(val)
		slices.SortFunc(objs, func(a, b objKey) int {
			return bytes.Compare(a[:], b[:])
		})
		val = bytes.Join(objs, []byte(""))
		return txn.Set(key, val)
	})
}

func deleteObjKeyFromIndex(txn *badger.Txn, key indexKey, value []byte) error {
	item, err := txn.Get(key)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	}
	return item.Value(func(val []byte) error {
		// use binary search to find and remove the objKey
		n := numObjects(val)
		l, h := 0, n
		for l < h {
			m := (l + h) >> 1 // avoids overflow
			r := bytes.Compare(val[m*objKeySize:(m+1)*objKeySize], value)
			if r < 0 {
				l = m + 1
			} else if r > 0 {
				h = m - 1
			} else {
				// found the objKey, remove it
				newVal := append(val[:m*objKeySize], val[(m+1)*objKeySize:]...)
				return txn.Set(key, newVal)
			}
		}
		return nil
	})
}

func readObjKeysFromIndexes(txn *badger.Txn, indexes ...indexKey) ([]objKey, error) {
	if len(indexes) == 0 {
		return nil, nil
	}

	// fast path if there is only one index
	if len(indexes) == 1 {
		item, err := txn.Get(indexes[0])
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var objs [][]byte
		err = item.Value(func(val []byte) error {
			objs = splitObjects(val)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return objs, nil
	}

	// read the objects from the indexes
	indexValues := make([][]byte, len(indexes))
	for i, idx := range indexes {
		item, err := txn.Get(idx)
		if err != nil {
			return nil, err
		}
		err = item.Value(func(val []byte) error {
			indexValues[i] = val
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// find the intersection of the objects from the indexes
	return intersectIndexes(indexValues...), nil
}

func deleteIndexedObjects(txn *badger.Txn, idxPrefix []byte) ([]objKey, error) {
	objs, err := readObjKeysFromIndexes(txn, idxPrefix)
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		if err := txn.Delete(buildKey(relationshipKey, obj[:])); err != nil {
			return nil, fmt.Errorf("failed to delete relationship %x: %w", obj, err)
		}
	}
	return objs, nil
}

func intersectIndexes(indexVals ...indexVal) []objKey {
	if len(indexVals) == 0 {
		return nil
	}

	type e = [objKeySize]byte
	type set = map[e]struct{}
	convertToSet := func(indexval []byte) set {
		s := make(set)
		for i := 0; i < len(indexval); i += objKeySize {
			s[e(indexval[i:i+objKeySize])] = struct{}{}
		}
		return s
	}
	cmp := convertToSet(indexVals[0])

	for _, idxVal := range indexVals[1:] {
		s := convertToSet(idxVal)
		for k := range cmp {
			if _, ok := s[k]; !ok {
				delete(cmp, k)
			}
		}
	}

	idxObjs := make([]objKey, 0, len(cmp))
	for k := range cmp {
		idxObjs = append(idxObjs, k[:])
	}
	return idxObjs
}

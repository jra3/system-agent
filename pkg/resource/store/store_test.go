package store_test

import (
	"testing"

	"github.com/antimetal/agent/pkg/errors"
	"github.com/antimetal/agent/pkg/resource"
	"github.com/antimetal/agent/pkg/resource/store"
	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestInventory_AddResource(t *testing.T) {
	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Type: "foo",
		},
		Metadata: &resourcev1.ResourceMeta{
			Name: "test",
		},
	}

	if err := inv.AddResource("test", rsrc); err != nil {
		t.Fatalf("failed to add resource: %v", err)
	}

	r, err := inv.GetResource("test")
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	if r.Metadata.Name != rsrc.Metadata.Name {
		t.Fatalf("expected name %q, got %q", rsrc.Metadata.Name, r.Metadata.Name)
	}
	if r.Type.Type != rsrc.Type.Type {
		t.Fatalf("expected type %q, got %q", rsrc.Type.Type, r.Type.Type)
	}
	if r.Metadata.CreatedAt == nil {
		t.Fatalf("expected creation time to be set")
	}
	if r.Metadata.UpdatedAt == nil {
		t.Fatalf("expected update time to be set")
	}

	_, err = inv.GetResource("notexist")
	if !errors.Is(err, resource.ErrResourceNotFound) {
		t.Fatalf("expected error %v, got %v", resource.ErrResourceNotFound, err)
	}
}

func TestInventory_UpdateResourceNewResource(t *testing.T) {
	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Type: "foo",
		},
		Metadata: &resourcev1.ResourceMeta{
			Name: "test",
		},
	}

	if err := inv.UpdateResource("test", rsrc); err != nil {
		t.Fatalf("failed to add resource: %v", err)
	}

	r, err := inv.GetResource("test")
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	if r.Metadata.Name != rsrc.Metadata.Name {
		t.Fatalf("expected name %q, got %q", rsrc.Metadata.Name, r.Metadata.Name)
	}
	if r.Type.Type != rsrc.Type.Type {
		t.Fatalf("expected type %q, got %q", rsrc.Type.Type, r.Type.Type)
	}
	if r.Metadata.CreatedAt == nil {
		t.Fatalf("expected creation time to be set")
	}
	if r.Metadata.UpdatedAt == nil {
		t.Fatalf("expected update time to be set")
	}

	_, err = inv.GetResource("notexist")
	if !errors.Is(err, resource.ErrResourceNotFound) {
		t.Fatalf("expected error %v, got %v", resource.ErrResourceNotFound, err)
	}
}

func TestInventory_UpdateResource(t *testing.T) {
	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Type: "foo",
		},
		Metadata: &resourcev1.ResourceMeta{
			Name: "test",
		},
	}

	if err := inv.AddResource("test", rsrc); err != nil {
		t.Fatalf("failed to add resource: %v", err)
	}

	r, err := inv.GetResource("test")
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	r.Metadata.Name = "test2"
	if err := inv.UpdateResource("test", r); err != nil {
		t.Fatalf("failed to update resource: %v", err)
	}

	r2, err := inv.GetResource("test")
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	if r2.Metadata.Name != "test2" {
		t.Fatalf("expected name test2, got %q", r.Metadata.Name)
	}
	if r.Type.Type != r2.Type.Type {
		t.Fatalf("expected type %q, got %q", rsrc.Type.Type, r.Type.Type)
	}
	if !r2.Metadata.UpdatedAt.AsTime().After(r.Metadata.UpdatedAt.AsTime()) {
		t.Fatalf("expected update time to be updated: r: %v, r2: %v",
			r.Metadata.UpdatedAt.AsTime(), r2.Metadata.UpdatedAt.AsTime(),
		)
	}
}

func TestInventory_GetRelationships(t *testing.T) {
	type testCase struct {
		name              string
		subject           string
		object            string
		predicate         proto.Message
		expectedNumResult int
	}

	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrcs := []*resourcev1.Resource{
		{
			Type: &resourcev1.TypeDescriptor{
				Type: "foo",
			},
			Metadata: &resourcev1.ResourceMeta{
				Name: "test",
			},
		},
		{
			Type: &resourcev1.TypeDescriptor{
				Type: "bar",
			},
			Metadata: &resourcev1.ResourceMeta{
				Name: "test2",
			},
		},
	}
	for _, rsrc := range rsrcs {
		if err := inv.AddResource(rsrc.GetMetadata().GetName(), rsrc); err != nil {
			t.Fatalf("failed to add resource: %v", err)
		}
	}

	predicate, err := anypb.New(&resourcev1.Resource{})
	if err != nil {
		t.Fatalf("failed to create predicate: %v", err)
	}
	predicate2, err := anypb.New(&resourcev1.Relationship{})
	if err != nil {
		t.Fatalf("failed to create predicate 2: %v", err)
	}

	rels := []*resourcev1.Relationship{
		{
			Subject:   []byte("test"),
			Object:    []byte("test2"),
			Predicate: predicate,
		},
		{
			Subject:   []byte("test2"),
			Object:    []byte("test"),
			Predicate: predicate2,
		},
		{
			Subject:   []byte("test"),
			Object:    []byte("test2"),
			Predicate: predicate2,
		},
		{
			Subject:   []byte("test2"),
			Object:    []byte("test3"),
			Predicate: predicate,
		},
	}
	if err := inv.AddRelationships(rels...); err != nil {
		t.Fatalf("failed to add relationships: %v", err)
	}

	testCases := []testCase{
		{
			name:              "empty",
			subject:           "notexist",
			object:            "test",
			predicate:         &resourcev1.Resource{},
			expectedNumResult: 0,
		},
		{
			name:              "subject",
			subject:           "test",
			expectedNumResult: 2,
		},
		{
			name:              "subject-2",
			subject:           "test2",
			expectedNumResult: 2,
		},
		{
			name:              "subject-3",
			subject:           "test3",
			expectedNumResult: 0,
		},
		{
			name:              "object",
			object:            "test2",
			expectedNumResult: 2,
		},
		{
			name:              "object-2",
			object:            "test3",
			expectedNumResult: 1,
		},
		{
			name:              "predicate",
			predicate:         &resourcev1.Resource{},
			expectedNumResult: 2,
		},
		{
			name:              "predicate-2",
			predicate:         &resourcev1.Relationship{},
			expectedNumResult: 2,
		},
		{
			name:              "subject-object-predicate",
			subject:           "test",
			object:            "test2",
			predicate:         &resourcev1.Resource{},
			expectedNumResult: 1,
		},
		{
			name:              "subject-object",
			subject:           "test",
			object:            "test2",
			expectedNumResult: 2,
		},
		{
			name:              "subject-object-2",
			subject:           "test2",
			object:            "test3",
			expectedNumResult: 1,
		},
		{
			name:              "subject-predicate",
			subject:           "test2",
			predicate:         &resourcev1.Relationship{},
			expectedNumResult: 1,
		},
		{
			name:              "object-predicate",
			object:            "test",
			predicate:         &resourcev1.Relationship{},
			expectedNumResult: 1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rels, err := inv.GetRelationships(tc.subject, tc.object, tc.predicate)
			if err != nil && !errors.Is(err, resource.ErrRelationshipsNotFound) {
				t.Fatalf("failed to get relationships: %v\n", err)
			}

			if tc.expectedNumResult == 0 && !errors.Is(err, resource.ErrRelationshipsNotFound) {
				t.Fatalf("expected error %v, got %v\n", resource.ErrRelationshipsNotFound, err)
			}

			if len(rels) != tc.expectedNumResult {
				t.Fatalf("expected %d relationships, got %d\n%+v", tc.expectedNumResult, len(rels), rels)
			}
		})
	}
}

func TestInventory_DeleteResource_CascadeDelete(t *testing.T) {
	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Type: "foo",
		},
		Metadata: &resourcev1.ResourceMeta{
			Name: "foo",
		},
	}
	if err := inv.AddResource("foo", rsrc); err != nil {
		t.Fatalf("failed to add resource: %v", err)
	}

	rels := []*resourcev1.Relationship{
		{
			Subject: []byte("foo"),
			Object:  []byte("bar"),
			Predicate: &anypb.Any{
				TypeUrl: "foo",
			},
		},
		{
			Subject: []byte("bar"),
			Object:  []byte("foo"),
			Predicate: &anypb.Any{
				TypeUrl: "bar",
			},
		},
		{
			Subject: []byte("bar"),
			Object:  []byte("baz"),
			Predicate: &anypb.Any{
				TypeUrl: "baz",
			},
		},
	}
	if err := inv.AddRelationships(rels...); err != nil {
		t.Fatalf("failed to add relationships: %v", err)
	}

	if err := inv.DeleteResource("foo"); err != nil {
		t.Fatalf("failed to delete resource: %v", err)
	}

	if rsrc, err := inv.GetResource("foo"); !errors.Is(err, resource.ErrResourceNotFound) {
		t.Fatalf("expected error %v, got %v; rsrc: %+v", resource.ErrResourceNotFound, err, rsrc)
	}
	if rel, err := inv.GetRelationships("foo", "bar", nil); !errors.Is(err, resource.ErrRelationshipsNotFound) {
		t.Fatalf("expected error %v, got %v; rel: %+v", resource.ErrRelationshipsNotFound, err, rel)
	}
	if rel, err := inv.GetRelationships("bar", "foo", nil); !errors.Is(err, resource.ErrRelationshipsNotFound) {
		t.Fatalf("expected error %v, got %v; rel: %+v", resource.ErrRelationshipsNotFound, err, rel)
	}
	if _, err := inv.GetRelationships("bar", "baz", nil); err != nil {
		t.Fatalf("expected bar->baz relationship to exist, got %v", err)
	}
}

func TestInventory_DeleteResource_NoRelationships(t *testing.T) {
	inv, err := store.New()
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	defer inv.Close()

	rsrc := &resourcev1.Resource{
		Type: &resourcev1.TypeDescriptor{
			Type: "foo",
		},
		Metadata: &resourcev1.ResourceMeta{
			Name: "foo",
		},
	}
	if err := inv.AddResource("test", rsrc); err != nil {
		t.Fatalf("failed to add resource: %v", err)
	}

	if err := inv.DeleteResource("test"); err != nil {
		t.Fatalf("failed to delete resource: %v", err)
	}

	if rsrc, err := inv.GetResource("test"); !errors.Is(err, resource.ErrResourceNotFound) {
		t.Fatalf("expected error %v, got %v; rsrc: %+v", resource.ErrResourceNotFound, err, rsrc)
	}
}

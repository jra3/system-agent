// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package agent

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
)

type eventType int

const (
	EventAdd eventType = iota
	EventUpdate
	EventDelete
)

type event struct {
	typ eventType
	obj object
}

type k8sCollectorHandler struct {
	logger logr.Logger
	scheme *runtime.Scheme
	queue  workqueue.TypedRateLimitingInterface[event]
}

func (h k8sCollectorHandler) OnAdd(obj any, _ bool) {
	h.handle(EventAdd, obj)
}

func (h k8sCollectorHandler) OnUpdate(_, newObj any) {
	h.handle(EventUpdate, newObj)
}

func (h k8sCollectorHandler) OnDelete(obj any) {
	h.handle(EventDelete, obj)
}

func (h k8sCollectorHandler) handle(ev eventType, obj any) {
	k8sObj, ok := obj.(object)
	if !ok {
		h.logger.Error(fmt.Errorf("invalid object: %T", obj), "received invalid object", "object", obj)
		return
	}
	// Reset the GroupVersionKind because TypeMeta gets cleared.
	gvks, unversioned, err := h.scheme.ObjectKinds(k8sObj)
	if len(gvks) == 0 || err != nil || unversioned {
		h.logger.Error(err, "object kind not found or is not versioned", "object", k8sObj)
		return
	}
	k8sObj.GetObjectKind().SetGroupVersionKind(gvks[0])
	k8sObj.SetManagedFields(nil)

	h.queue.AddRateLimited(event{typ: ev, obj: k8sObj})
}

func eventStr(e eventType) string {
	switch e {
	case EventAdd:
		return "add"
	case EventUpdate:
		return "update"
	case EventDelete:
		return "delete"
	default:
		return "unknown"
	}
}

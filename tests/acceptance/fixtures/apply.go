package fixtures

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ApplyAdapter returns a dynamic client whose apply operations are emulated over
// the fake tracker. The tracker cannot server-side apply an unstructured object:
// it runs the field ownership merge of a typed model, so an apply of a plain
// object fails before the reconciler's write is stored. This adapter plays the
// API server's part instead, creating a missing object and merging an existing
// one by field, so a case can drive the real reconciler over the fake cluster
// (KOSD-C-04). It changes no controller behaviour: admission still runs on the
// write path before the object reaches the store.
func ApplyAdapter(cluster dynamic.Interface) dynamic.Interface {
	return applyCluster{Interface: cluster}
}

// applyCluster redirects every resource view to the apply emulation.
type applyCluster struct {
	dynamic.Interface
}

// Resource wraps the namespaced view the reconciler writes through.
func (c applyCluster) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return applyNamespaceable{NamespaceableResourceInterface: c.Interface.Resource(resource)}
}

// applyNamespaceable wraps the namespace-scoped views of one resource.
type applyNamespaceable struct {
	dynamic.NamespaceableResourceInterface
}

// Namespace wraps the scoped view.
func (n applyNamespaceable) Namespace(namespace string) dynamic.ResourceInterface {
	return applyResource{ResourceInterface: n.NamespaceableResourceInterface.Namespace(namespace)}
}

// applyResource emulates the API server's apply for one resource.
type applyResource struct {
	dynamic.ResourceInterface
}

// Apply stores the desired object: a missing one is created and an existing one
// is merged, with labels and annotations joined as the API server joins them.
func (r applyResource) Apply(ctx context.Context, name string, object *unstructured.Unstructured, options metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if len(subresources) > 0 && subresources[0] == "status" {
		return r.applyStatus(ctx, name, object)
	}
	current, err := r.ResourceInterface.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return r.ResourceInterface.Create(ctx, object, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	merged := current.DeepCopy()
	mergeKeys(merged, object.GetLabels(), true)
	mergeKeys(merged, object.GetAnnotations(), false)
	for key, value := range object.Object {
		if key == "metadata" {
			continue
		}
		merged.Object[key] = value
	}
	return r.ResourceInterface.Update(ctx, merged, metav1.UpdateOptions{})
}

// ApplyStatus stores only the status part of the desired object, as the status
// subresource does (KOSD-C-03).
func (r applyResource) ApplyStatus(ctx context.Context, name string, object *unstructured.Unstructured, options metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return r.applyStatus(ctx, name, object)
}

// applyStatus writes the desired status over the stored object.
func (r applyResource) applyStatus(ctx context.Context, name string, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	current, err := r.ResourceInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	merged := current.DeepCopy()
	if status, ok := object.Object["status"]; ok {
		merged.Object["status"] = status
	}
	return r.ResourceInterface.UpdateStatus(ctx, merged, metav1.UpdateOptions{})
}

// mergeKeys joins the desired labels or annotations into the current object,
// keeping the keys the desired set does not name.
func mergeKeys(current *unstructured.Unstructured, desired map[string]string, labels bool) {
	if len(desired) == 0 {
		return
	}
	merged := map[string]string{}
	if labels {
		for key, value := range current.GetLabels() {
			merged[key] = value
		}
	} else {
		for key, value := range current.GetAnnotations() {
			merged[key] = value
		}
	}
	for key, value := range desired {
		merged[key] = value
	}
	if labels {
		current.SetLabels(merged)
	} else {
		current.SetAnnotations(merged)
	}
}

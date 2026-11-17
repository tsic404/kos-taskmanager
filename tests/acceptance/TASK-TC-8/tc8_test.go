// Package tasktc8 checks TASK-TC-8: deleting one Pod from its application's
// Pod runtime page makes that Pod leave the list and leaves the application's
// desired state untouched; the replacement instance the runtime's workload
// controller creates is the one that appears next, and no drift record is left
// on the application (KOSD-C-77, KOSD-C-35).
package tasktc8

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/identity"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application, its volume and its Pods live.
const namespace = "home"

// application is the application the page deletes a Pod of.
const application = "shop"

// deleted is the Pod the case removes from the page.
const deleted = "shop-1"

// replacement is the instance the runtime's workload controller creates after
// the delete; KOS itself never creates one (KOSD-C-77).
const replacement = "shop-3"

// world is one running application with two replicas and the product that
// serves its Pod page and the control-plane write.
type world struct {
	cluster dynamic.Interface
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Volume(t, cluster, namespace, "data", v1beta1.VolumeMobilityNodeLocal, "node-a")
	replicas := int32(2)
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin",
		Volumes: []string{"data"}, Replicas: &replicas,
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: application}},
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	for index, name := range []string{deleted, "shop-2"} {
		fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
			Namespace: namespace, Name: name, Labels: map[string]string{"app": application},
			Node: "node-a", Phase: "Running", Ready: true,
			Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
			StartedAt:  fixtures.Now.Add(-time.Duration(index+1) * time.Minute),
		})
	}
	return &world{cluster: cluster, product: fixtures.NewProductWith(t, cluster, fixtures.NewTrail(), nil, nil)}
}

// projection reads the advertised application's Pod list through the address
// the page row names.
func (w *world) projection(t *testing.T, session identity.Session) []map[string]any {
	t.Helper()
	page := w.product.ReadPage(t, session, app.PagePods)
	rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPods), "applications")
	var path string
	for _, row := range rows {
		if row["namespace"] == namespace && row["name"] == application {
			path, _ = row["path"].(string)
		}
	}
	if path == "" {
		t.Fatalf("the Pod page carries no row for %s/%s", namespace, application)
	}
	status, body := w.product.Call(t, session, http.MethodGet, path, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("read the Pod projection = %d %v, want 200", status, body)
	}
	return fixtures.ObjectsOf(t, body, "items")
}

func TestTASKTC8DeletingOnePodFromThePageKeepsTheDesiredState(t *testing.T) {
	world := newWorld(t)
	before := podNames(world.projection(t, fixtures.OwningMember))
	if want := []string{"shop-1", "shop-2"}; !reflect.DeepEqual(before, want) {
		t.Fatalf("initial Pods = %v, want %v", before, want)
	}
	specBefore := specOf(t, fixtures.ApplicationObject(t, world.cluster, namespace, application))

	method, path := fixtures.PageActionOf(t, world.product, app.PagePods, app.ActionDeletePod,
		map[string]string{"namespace": namespace, "name": application, "pod": deleted})
	if method != http.MethodDelete {
		t.Fatalf("Pod delete entry = %s %s, want a DELETE", method, path)
	}
	status, answer := world.product.Call(t, fixtures.OwningMember, method, path, nil, nil)
	if status != http.StatusAccepted {
		t.Fatalf("delete %s = %d %v, want 202", deleted, status, answer)
	}
	if answer["accepted"] != true {
		t.Fatalf("delete answer = %v, want the accepted mark", answer)
	}
	target, _ := answer["ref"].(map[string]any)
	if target["kind"] != "Pod" || target["namespace"] != namespace || target["name"] != deleted {
		t.Fatalf("delete reference = %v, want the one Pod it removed", answer["ref"])
	}

	// The deleted Pod is gone from the list; the other replica is still there.
	after := podNames(world.projection(t, fixtures.OwningMember))
	if want := []string{"shop-2"}; !reflect.DeepEqual(after, want) {
		t.Fatalf("Pods after the delete = %v, want %v", after, want)
	}

	// The replacement instance is the runtime's own decision. Once it exists,
	// the same page lists it in place of the deleted Pod.
	fixtures.RuntimePod(t, world.cluster, fixtures.PodOptions{
		Namespace: namespace, Name: replacement, Labels: map[string]string{"app": application},
		Node: "node-b", Phase: "ContainerCreating", Ready: false,
		Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
	})
	replaced := podNames(world.projection(t, fixtures.OwningMember))
	if want := []string{"shop-2", replacement}; !reflect.DeepEqual(replaced, want) {
		t.Fatalf("Pods after the replacement = %v, want %v", replaced, want)
	}

	// The desired state is untouched: the delete removes one runtime fact and
	// never writes the application spec, its replicas, its volumes or its drift
	// record (KOSD-C-77, KOSD-C-35).
	afterObject := fixtures.ApplicationObject(t, world.cluster, namespace, application)
	specAfter := specOf(t, afterObject)
	if !reflect.DeepEqual(specBefore, specAfter) {
		t.Fatalf("application spec changed across the Pod delete:\nbefore %v\nafter  %v", specBefore, specAfter)
	}
	if replicas, _, _ := unstructured.NestedInt64(afterObject.Object, "spec", "replicas"); replicas != 2 {
		t.Fatalf("application replicas = %v after the delete, want the declared 2", replicas)
	}
	if drift, found, _ := unstructured.NestedSlice(afterObject.Object, "status", "drift"); found && len(drift) != 0 {
		t.Fatalf("application carries drift %v after a Pod delete, want none", drift)
	}
}

func TestTASKTC8ARepeatedDeleteOfTheSamePodHasOneEffect(t *testing.T) {
	world := newWorld(t)
	method, path := fixtures.PageActionOf(t, world.product, app.PagePods, app.ActionDeletePod,
		map[string]string{"namespace": namespace, "name": application, "pod": deleted})
	for attempt := 1; attempt <= 2; attempt++ {
		status, answer := world.product.Call(t, fixtures.OwningMember, method, path, nil, nil)
		if status != http.StatusAccepted {
			t.Fatalf("delete attempt %d = %d %v, want 202", attempt, status, answer)
		}
	}
	if got, want := podNames(world.projection(t, fixtures.OwningMember)), []string{"shop-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pods after two identical deletes = %v, want %v", got, want)
	}
}

func TestTASKTC8NoPodCreateSpecWriteTerminalOrPortForwardInterfaceExists(t *testing.T) {
	world := newWorld(t)
	before := podNames(world.projection(t, fixtures.OwningMember))
	for _, attempt := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "Pod create", method: http.MethodPost, path: "/api/v1/pods/" + namespace + "/" + application,
			body: map[string]any{"name": "shop-9"}},
		{name: "Pod spec write", method: http.MethodPut,
			path: "/api/v1/pods/" + namespace + "/" + application + "/" + deleted,
			body: map[string]any{"spec": map[string]any{"containers": []any{}}}},
		{name: "terminal exec", method: http.MethodPost,
			path: "/api/v1/pods/" + namespace + "/" + application + "/" + deleted + "/exec"},
		{name: "port forward", method: http.MethodPost,
			path: "/api/v1/pods/" + namespace + "/" + application + "/" + deleted + "/portforward"},
	} {
		status, answer := world.product.Call(t, fixtures.OwningMember, attempt.method, attempt.path, attempt.body, nil)
		if status >= 200 && status < 300 {
			t.Fatalf("%s = %d %v, want no such interface", attempt.name, status, answer)
		}
	}
	if after := podNames(world.projection(t, fixtures.OwningMember)); !reflect.DeepEqual(before, after) {
		t.Fatalf("the refused Pod writes changed the Pods: %v then %v", before, after)
	}
}

// specOf reads one application's spec document.
func specOf(t *testing.T, object *unstructured.Unstructured) map[string]any {
	t.Helper()
	spec, found, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil || !found {
		t.Fatalf("read the application spec: found = %v, err = %v", found, err)
	}
	return spec
}

// podNames reads the names of projected Pods, ordered.
func podNames(items []map[string]any) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

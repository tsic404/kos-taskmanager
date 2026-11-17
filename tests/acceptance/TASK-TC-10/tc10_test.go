// Package tasktc10 checks TASK-TC-10: a stopped application's Pod page states
// an empty list together with the reason and offers no delete entry. The
// control plane's own answer is an empty projection, so the empty state is an
// answer rather than a failed read (KOSD-C-76).
package tasktc10

import (
	"net/http"
	"testing"

	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api/v1beta1"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application lives.
const namespace = "home"

// application is the stopped application the page is read for.
const application = "shop"

// world is one application stopped through spec.paused, with the workload it
// rendered before it stopped.
type world struct {
	cluster dynamic.Interface
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin", Paused: true,
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: application}},
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	return &world{cluster: cluster, product: fixtures.NewProductWith(t, cluster, fixtures.NewTrail(), nil, nil)}
}

func TestTASKTC10AStoppedApplicationAnswersAnEmptyListWithItsReason(t *testing.T) {
	world := newWorld(t)
	page := world.product.ReadPage(t, fixtures.Owner, app.PagePods)
	var row map[string]any
	for _, candidate := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPods), "applications") {
		if candidate["namespace"] == namespace && candidate["name"] == application {
			row = candidate
		}
	}
	if row == nil {
		t.Fatalf("the Pod page carries no row for the stopped application %s/%s", namespace, application)
	}
	if row["paused"] != true {
		t.Fatalf("stopped application row = %v, want the stop marker", row)
	}
	if reason, _ := row["reason"].(string); reason == "" {
		t.Fatalf("stopped application row = %v, want the reason its list is empty", row)
	}

	// The control plane's own answer is an empty list, not a failure: an empty
	// projection is what a stopped application has (KOSD-C-76).
	path, _ := row["path"].(string)
	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, path, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("Pod projection of the stopped application = %d %v, want 200", status, body)
	}
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("Pod projection of the stopped application = %v, want an empty list", body)
	}
	if len(items) != 0 {
		t.Fatalf("Pod projection of the stopped application = %v, want no Pods", items)
	}
}

func TestTASKTC10AStoppedApplicationOffersNoDeleteEntry(t *testing.T) {
	world := newWorld(t)
	page := world.product.ReadPage(t, fixtures.Owner, app.PagePods)
	for _, row := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPodDelete), "applications") {
		if row["namespace"] == namespace && row["name"] == application {
			t.Fatalf("the stopped application still carries a delete entry: %v", row)
		}
	}
	// The page's delete item exists, but it names no target here, so the window
	// renders no delete form for the stopped application.
}

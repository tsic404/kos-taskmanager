// Package tasktc7 checks TASK-TC-7: the Pod runtime page declares one
// application's control-plane projection, its per-Pod log read and the single
// Pod delete; the projection lists only the addressed application's Pods, each
// with the facts KOSD-C-76 names, the source type and the observation moment.
// A second application's Pods in the same namespace never appear.
package tasktc7

import (
	"net/http"
	"sort"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where both applications and their Pods live.
const namespace = "home"

// application is the application whose Pod runtime page is read.
const application = "shop"

// other is a second application in the same namespace whose Pods never appear.
const other = "reports"

// world is one cluster shaped for the Pod runtime page: two applications, each
// with its own workload and replicas, and the product that serves the page and
// the control-plane API.
type world struct {
	cluster dynamic.Interface
	product *fixtures.Product
	logs    *fixtures.LogSource
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin", Members: []string{"mei"},
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: application}},
	})
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: other, Owner: "lin",
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: other}},
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	fixtures.Workload(t, cluster, namespace, other, map[string]string{"app": other})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: "shop-1", Labels: map[string]string{"app": application},
		Node: "node-a", Phase: "Running", Ready: true, Restarts: 3,
		Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
		StartedAt:  fixtures.Now.Add(-30 * time.Minute),
	})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: "shop-2", Labels: map[string]string{"app": application},
		Node: "node-b", Phase: "Pending", Ready: false,
		Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
	})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: "reports-1", Labels: map[string]string{"app": other},
		Node: "node-a", Phase: "Running", Ready: true,
		Containers: []fixtures.PodContainer{{Name: "worker", Image: "registry.test/reports:1"}},
	})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: "reports-2", Labels: map[string]string{"app": other},
		Node: "node-b", Phase: "Running", Ready: true,
		Containers: []fixtures.PodContainer{{Name: "worker", Image: "registry.test/reports:1"}},
	})
	logs := &fixtures.LogSource{ByContainer: map[string][]controlplane.LogLine{
		fixtures.LogKey(namespace, "shop-1", "web"): {
			{Timestamp: fixtures.Now.Add(-2 * time.Minute), Pod: "shop-1", Container: "web", Message: "shop-1 的行"},
		},
		fixtures.LogKey(namespace, "shop-2", "web"): {
			{Timestamp: fixtures.Now.Add(-1 * time.Minute), Pod: "shop-2", Container: "web", Message: "shop-2 的行"},
		},
		fixtures.LogKey(namespace, "reports-1", "worker"): {
			{Timestamp: fixtures.Now.Add(-1 * time.Minute), Pod: "reports-1", Container: "worker", Message: "reports 的行"},
		},
	}}
	return &world{cluster: cluster, logs: logs, product: fixtures.NewProductWith(t, cluster, fixtures.NewTrail(), nil, logs)}
}

// pageRows reads one item's per-application rows.
func pageRows(t *testing.T, product *fixtures.Product, session identity.Session, item string) []map[string]any {
	t.Helper()
	page := product.ReadPage(t, session, app.PagePods)
	return fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, item), "applications")
}

// rowOf picks one application's row.
func rowOf(rows []map[string]any, name string) (map[string]any, bool) {
	for _, row := range rows {
		if row["namespace"] == namespace && row["name"] == name {
			return row, true
		}
	}
	return nil, false
}

func TestTASKTC7ThePodPageDeclaresTheProjectionLogAndDelete(t *testing.T) {
	world := newWorld(t)
	declared, found := world.product.Registry.PageFor(app.PagePods, write.RoleOwner)
	if !found {
		t.Fatal("the installed package declares no Pod runtime page")
	}
	if declared.ReadOnly {
		t.Fatal("the Pod page is read-only, want the single-Pod delete entry it declares")
	}
	items := map[string]extension.PageItem{}
	for _, item := range declared.Items {
		items[item.ID] = item
	}
	if len(items) != 3 {
		t.Fatalf("the Pod page declares %d items, want the projection, the log read and the delete", len(items))
	}
	if item := items[app.ItemPods]; item.Source != extension.PageSourceObject {
		t.Fatalf("Pod list item source = %q, want the object source", item.Source)
	}
	if item := items[app.ItemPodLogs]; item.Source != extension.PageSourceRead {
		t.Fatalf("Pod log item source = %q, want the read source", item.Source)
	}
	if item := items[app.ItemPodDelete]; item.Source != extension.PageSourceObject || item.Action != app.ActionDeletePod {
		t.Fatalf("Pod delete item = %+v, want an object item naming the delete action", item)
	}
	// The page's one write is the single-Pod delete: no Pod create, no Pod spec
	// write, no terminal and no port forward (KOSD-F-20).
	if len(declared.Actions) != 1 {
		t.Fatalf("the Pod page declares actions %v, want only the single-Pod delete", declared.Actions)
	}
	action := declared.Actions[0]
	if action.ID != app.ActionDeletePod || action.Method != http.MethodDelete {
		t.Fatalf("Pod page action = %+v, want the single-Pod DELETE", action)
	}
	if action.Path != app.PathPodDelete {
		t.Fatalf("Pod delete path = %q, want %q", action.Path, app.PathPodDelete)
	}

	// The page the session reads carries the same entry on the delete item, so
	// the window never assembles a write address of its own (KOSD-C-22).
	page := world.product.ReadPage(t, fixtures.Owner, app.PagePods)
	deleteItem, found := fixtures.ItemsByID(page)[app.ItemPodDelete]
	if !found || deleteItem.Entry == nil {
		t.Fatalf("the served Pod page carries no delete entry: %+v", deleteItem)
	}
	if deleteItem.Entry.Method != http.MethodDelete || deleteItem.Entry.Path != app.PathPodDelete {
		t.Fatalf("served delete entry = %+v, want the declared DELETE", *deleteItem.Entry)
	}
	logsValue := fixtures.ValueOf(t, page, app.ItemPodLogs)
	if rule, _ := logsValue["rule"].(string); rule == "" {
		t.Fatalf("the served Pod log item carries no rule: %v", logsValue)
	}
}

func TestTASKTC7TheProjectionListsOnlyTheAddressedApplicationsPods(t *testing.T) {
	world := newWorld(t)
	row, found := rowOf(pageRows(t, world.product, fixtures.Owner, app.ItemPods), application)
	if !found {
		t.Fatalf("the Pod page carries no row for %s/%s", namespace, application)
	}
	path, _ := row["path"].(string)
	if want := "/api/v1/pods/" + namespace + "/" + application; path != want {
		t.Fatalf("Pod projection path = %q, want %q", path, want)
	}

	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, path, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("Pod projection = %d %v, want 200", status, body)
	}
	reference, _ := body["application"].(map[string]any)
	if reference["namespace"] != namespace || reference["name"] != application || reference["kind"] != "Application" {
		t.Fatalf("projection application = %v, want this application's reference", body["application"])
	}
	items := fixtures.ObjectsOf(t, body, "items")
	if got, want := podNames(items), []string{"shop-1", "shop-2"}; !equalStrings(got, want) {
		t.Fatalf("projected Pods = %v, want only this application's %v", got, want)
	}
	for _, item := range items {
		if item["sourceType"] != "object" {
			t.Fatalf("Pod %v source type = %v, want the object source", item["name"], item["sourceType"])
		}
		if observed, _ := item["observedAt"].(string); observed == "" {
			t.Fatalf("Pod %v carries no observation time", item["name"])
		}
		if item["stale"] != false {
			t.Fatalf("Pod %v stale = %v, want a fresh read", item["name"], item["stale"])
		}
	}
	first := items[0]
	if first["node"] != "node-a" || first["phase"] != "Running" || first["ready"] != true {
		t.Fatalf("Pod shop-1 facts = %v, want the runtime facts the projection lists", first)
	}
	if restarts, _ := first["restarts"].(float64); restarts != 3 {
		t.Fatalf("Pod shop-1 restarts = %v, want 3", first["restarts"])
	}
	if started, _ := first["startedAt"].(string); started == "" {
		t.Fatalf("Pod shop-1 carries no start time: %v", first)
	}
	containers := fixtures.ObjectsOf(t, first, "containers")
	if len(containers) != 1 || containers[0]["name"] != "web" || containers[0]["image"] != "registry.test/shop:2" {
		t.Fatalf("Pod shop-1 containers = %v, want the container and image", containers)
	}

	// Nothing of the second application's Pods appears in the read.
	for _, name := range podNames(items) {
		if name == "reports-1" || name == "reports-2" {
			t.Fatalf("another application's Pod leaked into the projection: %s", name)
		}
	}
	// The second application keeps its own row and its own address.
	otherRow, found := rowOf(pageRows(t, world.product, fixtures.Owner, app.ItemPods), other)
	if !found {
		t.Fatal("the page does not carry the second application's own row")
	}
	if otherRow["path"] == path {
		t.Fatalf("both applications name the same projection path %q", path)
	}
}

func TestTASKTC7ThePerPodLogReadCoversOnlyThatPodsContainers(t *testing.T) {
	world := newWorld(t)
	row, found := rowOf(pageRows(t, world.product, fixtures.Owner, app.ItemPodLogs), application)
	if !found {
		t.Fatalf("the Pod log item carries no row for %s/%s", namespace, application)
	}
	path, _ := row["path"].(string)
	if path != "/api/v1/logs/"+namespace+"/"+application {
		t.Fatalf("Pod log path = %q, want the application's own log read", path)
	}
	// The read is narrowed to one Pod by the page's own control (KOSD-C-64).
	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, path+"?pod=shop-1", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("per-Pod log read = %d %v, want 200", status, body)
	}
	lines, ok := body["lines"].([]any)
	if !ok {
		t.Fatalf("per-Pod log read = %v, want a line list", body)
	}
	if len(lines) != 1 {
		t.Fatalf("per-Pod log read = %v, want only shop-1's line", lines)
	}
	line, _ := lines[0].(map[string]any)
	if line["pod"] != "shop-1" || line["message"] != "shop-1 的行" {
		t.Fatalf("per-Pod log line = %v, want shop-1's own line", line)
	}
}

// TestTASKTC7AnOverLimitPerPodLogReadStatesItsWindowBoundAndCut drives the real
// control-plane log route past its bound: the answer keeps the Pod narrowing,
// reports the window and limit that applied, marks the read as truncated and
// names the oldest line it kept (KOSD-C-64).
func TestTASKTC7AnOverLimitPerPodLogReadStatesItsWindowBoundAndCut(t *testing.T) {
	world := newWorld(t)
	world.logs.ByContainer[fixtures.LogKey(namespace, "shop-1", "web")] = []controlplane.LogLine{
		{Timestamp: fixtures.Now.Add(-6 * time.Minute), Pod: "shop-1", Container: "web", Message: "较早的行"},
		{Timestamp: fixtures.Now.Add(-4 * time.Minute), Pod: "shop-1", Container: "web", Message: "次新的行"},
		{Timestamp: fixtures.Now.Add(-2 * time.Minute), Pod: "shop-1", Container: "web", Message: "最新的行"},
	}
	row, found := rowOf(pageRows(t, world.product, fixtures.Owner, app.ItemPodLogs), application)
	if !found {
		t.Fatalf("the Pod log item carries no row for %s/%s", namespace, application)
	}
	path, _ := row["path"].(string)
	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, path+"?pod=shop-1&limit=2&window=15m", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("bounded per-Pod log read = %d %v, want 200", status, body)
	}
	if body["window"] != "15m0s" || body["limit"] != float64(2) {
		t.Fatalf("log read bounds = window %v limit %v, want 15m0s and 2", body["window"], body["limit"])
	}
	if body["truncated"] != true {
		t.Fatalf("truncated = %v, want true past the line bound", body["truncated"])
	}
	wantCut := fixtures.Now.Add(-4 * time.Minute).UTC().Format(time.RFC3339)
	if body["truncatedAt"] != wantCut {
		t.Fatalf("truncatedAt = %v, want the oldest kept line %s", body["truncatedAt"], wantCut)
	}
	lines, ok := body["lines"].([]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("lines = %v, want the two newest lines", body["lines"])
	}
	for _, raw := range lines {
		line, _ := raw.(map[string]any)
		if line["pod"] != "shop-1" {
			t.Fatalf("line = %v, want only shop-1's own lines", line)
		}
	}
	last, _ := lines[len(lines)-1].(map[string]any)
	if last["message"] != "最新的行" {
		t.Fatalf("last line = %v, want the newest kept line", last)
	}
}

// podNames reads the names of projected Pods.
func podNames(items []map[string]any) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

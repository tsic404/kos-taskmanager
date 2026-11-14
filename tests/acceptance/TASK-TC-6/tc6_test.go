// Package tasktc6 checks TASK-TC-6: the resource-specification entry submits a
// CPU limit and then reads the value the application's requests and limits
// carry; the uninstall entry leaves the application listed while the uninstall
// gate holds it and gone once the object is removed (KOSD-C-33, KOSD-C-67,
// KOSD-AC-61).
package tasktc6

import (
	"net/http"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/api/v1beta1"

	taskapp "github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application lives.
const namespace = "home"

// entryItems are the six item identities that carry a modification entry.
var entryItems = []string{
	taskapp.ItemStop, taskapp.ItemStart, taskapp.ItemRestart,
	taskapp.ItemScale, taskapp.ItemResourceSpec, taskapp.ItemUninstall,
}

// newWorld installs the package over one application whose resource defaults
// the package materialized into its spec (KOSD-C-67).
func newWorld(t *testing.T) *fixtures.Product {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.ValidDefinition(t, cluster, "shop", "1.0.0", "kind: ConfigMap\nmetadata: {name: shop}\n")
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: "lin", Members: []string{"mei"},
		DefinitionRef: "shop", UID: "uid-shop",
		Resources: &v1beta1.ResourceRequirements{
			Requests: v1beta1.ResourceList{CPU: "250m", Memory: "256Mi"},
			Limits:   v1beta1.ResourceList{CPU: "500m", Memory: "512Mi"},
		},
	})
	return fixtures.NewProduct(t, cluster, fixtures.NewTrail(), nil)
}

// specResources reads the requests and limits one stored application carries.
func specResources(t *testing.T, product *fixtures.Product) map[string]any {
	t.Helper()
	application := fixtures.ApplicationObject(t, product.Cluster, namespace, "shop")
	resources, found, err := unstructured.NestedMap(application.Object, "spec", "resources")
	if err != nil || !found {
		t.Fatalf("read the application resources: found = %v, err = %v", found, err)
	}
	return resources
}

func TestTASKTC6TheResourceEntryWritesTheApplicationSpec(t *testing.T) {
	product := newWorld(t)
	method, path := fixtures.PageAction(t, product, taskapp.ActionResourceSpec, "shop", namespace)
	if method != http.MethodPut {
		t.Fatalf("resource entry = %s %s, want a PUT", method, path)
	}

	// The form submits the specification it holds, changing only the CPU limit
	// (KOSD-C-67).
	application := fixtures.ApplicationObject(t, product.Cluster, namespace, "shop")
	spec, found, err := unstructured.NestedMap(application.Object, "spec")
	if err != nil || !found {
		t.Fatalf("read the application spec: found = %v, err = %v", found, err)
	}
	spec["resources"] = map[string]any{"limits": map[string]any{"cpu": "1"}}
	status, answer := product.Call(t, fixtures.OwningMember, method, path, map[string]any{
		"apiVersion": "kos.tsic.top/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": "shop", "namespace": namespace},
		"spec":       spec,
	}, map[string]string{"If-Match": application.GetResourceVersion()})
	if status != http.StatusAccepted {
		t.Fatalf("resource update = %d %v, want 202", status, answer)
	}

	// The override only writes the fields it carries: the CPU limit is the new
	// value and every other field keeps the default it had (KOSD-C-67).
	stored := specResources(t, product)
	if limits := nested(t, stored, "limits"); limits["cpu"] != "1" {
		t.Fatalf("stored limits = %v, want the submitted CPU limit", limits)
	}
	if limits := nested(t, stored, "limits"); limits["memory"] != "512Mi" {
		t.Fatalf("stored limits = %v, want the memory limit kept", limits)
	}
	if requests := nested(t, stored, "requests"); requests["cpu"] != "250m" || requests["memory"] != "256Mi" {
		t.Fatalf("stored requests = %v, want both requests kept", requests)
	}

	// The entry now gives the value the application carries: the page and the
	// object agree (KOSD-AC-61).
	page := product.ReadPage(t, fixtures.OwningMember, taskapp.PageApplications)
	targets := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemResourceSpec), "applications")
	if len(targets) != 1 || targets[0]["name"] != "shop" {
		t.Fatalf("resource entry targets = %v, want the application", targets)
	}
	given, ok := targets[0]["resources"].(map[string]any)
	if !ok {
		t.Fatalf("resource entry gives %#v, want the application's requests and limits", targets[0]["resources"])
	}
	if !reflect.DeepEqual(given, stored) {
		t.Fatalf("resource entry gives %v, want the spec's %v", given, stored)
	}
	if targets[0]["unlimited"] == true {
		t.Fatalf("resource entry = %v, want a constrained application", targets[0])
	}
}

func TestTASKTC6TheUninstallEntryKeepsTheApplicationUntilItIsGone(t *testing.T) {
	product := newWorld(t)

	// While the uninstall gate holds the application, the object stays visible
	// and the page lists it with the phase and the blocked objects it carries
	// (KOSD-C-33, KOSD-C-69).
	fixtures.BeginUninstall(t, product.Cluster, namespace, "shop")
	page := product.ReadPage(t, fixtures.OwningMember, taskapp.PageApplications)
	rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstalled), "applications")
	if len(rows) != 1 || rows[0]["name"] != "shop" {
		t.Fatalf("installed applications = %v, want the uninstalling application to stay listed", rows)
	}
	if rows[0]["phase"] != "Uninstalling" {
		t.Fatalf("uninstalling phase = %v, want the object's phase", rows[0]["phase"])
	}
	uninstall, ok := rows[0]["uninstall"].(map[string]any)
	if !ok {
		t.Fatalf("uninstalling application carries no uninstall state: %v", rows[0])
	}
	blocked, _ := uninstall["blocked"].([]any)
	if len(blocked) != 1 {
		t.Fatalf("blocked objects = %v, want the object that holds the gate", uninstall["blocked"])
	}
	// No entry applies while the application is going away.
	for _, item := range entryItems {
		if targets := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, item), "applications"); len(targets) != 0 {
			t.Fatalf("entry %q offers %v while the application is uninstalling", item, targets)
		}
	}

	// The uninstall entry is the control-plane delete behind the version
	// precondition (KOSD-C-12). Once the object is removed, the page drops it.
	method, path := fixtures.PageAction(t, product, taskapp.ActionUninstall, "shop", namespace)
	if method != http.MethodDelete {
		t.Fatalf("uninstall entry = %s %s, want a DELETE", method, path)
	}
	current := fixtures.ApplicationObject(t, product.Cluster, namespace, "shop")
	status, answer := product.Call(t, fixtures.OwningMember, method, path, nil,
		map[string]string{"If-Match": current.GetResourceVersion()})
	if status != http.StatusAccepted {
		t.Fatalf("uninstall = %d %v, want 202", status, answer)
	}
	page = product.ReadPage(t, fixtures.OwningMember, taskapp.PageApplications)
	if rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstalled), "applications"); len(rows) != 0 {
		t.Fatalf("installed applications = %v after the uninstall, want none", rows)
	}
}

// nested reads one field of a resource document.
func nested(t *testing.T, document map[string]any, field string) map[string]any {
	t.Helper()
	value, ok := document[field].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want an object", field, document[field])
	}
	return value
}

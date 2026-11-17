// Package tasktc9 checks TASK-TC-9: the Pod page's delete entry follows the
// role and ownership rule of KOSD-C-59 — the global Owner and the Member that
// owns the application see it, a Member listed on the application but not
// owning it and a Guest do not, and an identity that cannot see the application
// gets neither the page's facts nor the entry. A direct delete by an identity
// without the write role is refused and leaves the Pod in place (KOSD-C-77).
package tasktc9

import (
	"net/http"
	"testing"

	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/identity"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application and its Pods live.
const namespace = "home"

// application is the application the Pod page is read for.
const application = "shop"

// pod is the Pod a direct delete names.
const pod = "shop-1"

// world is one application owned by lin, naming mei, open to guests.
type world struct {
	cluster dynamic.Interface
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin",
		Members: []string{"mei"}, AllowGuests: true,
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: application}},
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: pod, Labels: map[string]string{"app": application},
		Node: "node-a", Phase: "Running", Ready: true,
		Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
	})
	return &world{cluster: cluster, product: fixtures.NewProductWith(t, cluster, fixtures.NewTrail(), nil, nil)}
}

// saw reports whether one item names the application for one session.
func (w *world) saw(t *testing.T, session identity.Session, item string) bool {
	t.Helper()
	page := w.product.ReadPage(t, session, app.PagePods)
	for _, row := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, item), "applications") {
		if row["namespace"] == namespace && row["name"] == application {
			return true
		}
	}
	return false
}

func TestTASKTC9TheDeleteEntryFollowsTheApplicationsWriteRole(t *testing.T) {
	world := newWorld(t)
	for _, step := range []struct {
		session identity.Session
		name    string
		visible bool
		entry   bool
	}{
		{session: fixtures.Owner, name: "Owner", visible: true, entry: true},
		{session: fixtures.OwningMember, name: "owning Member", visible: true, entry: true},
		{session: fixtures.NamedMember, name: "named non-owning Member", visible: true, entry: false},
		{session: fixtures.Guest, name: "Guest", visible: true, entry: false},
		{session: fixtures.OtherMember, name: "identity that cannot see the application", visible: false, entry: false},
	} {
		if got := world.saw(t, step.session, app.ItemPods); got != step.visible {
			t.Fatalf("%s sees the application's Pod page = %v, want %v", step.name, got, step.visible)
		}
		if got := world.saw(t, step.session, app.ItemPodDelete); got != step.entry {
			t.Fatalf("%s sees the delete entry = %v, want %v", step.name, got, step.entry)
		}
	}
}

func TestTASKTC9ADirectDeleteWithoutTheWriteRoleIsRefusedAndKeepsThePod(t *testing.T) {
	world := newWorld(t)
	method, path := fixtures.PageActionOf(t, world.product, app.PagePods, app.ActionDeletePod,
		map[string]string{"namespace": namespace, "name": application, "pod": pod})
	for _, step := range []struct {
		session identity.Session
		name    string
	}{
		{session: fixtures.NamedMember, name: "named non-owning Member"},
		{session: fixtures.Guest, name: "Guest"},
	} {
		status, answer := world.product.Call(t, step.session, method, path, nil, nil)
		if status != http.StatusForbidden {
			t.Fatalf("%s delete = %d %v, want 403", step.name, status, answer)
		}
		if failure := fixtures.Failure(t, answer); failure["code"] != "permission_denied" {
			t.Fatalf("%s delete failure = %v, want the permission refusal", step.name, failure)
		}
		if !world.saw(t, fixtures.Owner, app.ItemPods) {
			t.Fatalf("%s's refused delete removed the Pod", step.name)
		}
		ids := podNames(t, world, fixtures.Owner)
		if len(ids) != 1 || ids[0] != pod {
			t.Fatalf("%s's refused delete changed the Pods: %v", step.name, ids)
		}
	}

	// The owning Member's delete is accepted and the Pod leaves the list.
	status, answer := world.product.Call(t, fixtures.OwningMember, method, path, nil, nil)
	if status != http.StatusAccepted {
		t.Fatalf("owning Member delete = %d %v, want 202", status, answer)
	}
	if ids := podNames(t, world, fixtures.OwningMember); len(ids) != 0 {
		t.Fatalf("Pods after the owning Member's delete = %v, want none", ids)
	}
}

// podNames reads the Pod list through the application's own projection address.
func podNames(t *testing.T, world *world, session identity.Session) []string {
	t.Helper()
	page := world.product.ReadPage(t, session, app.PagePods)
	for _, row := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPods), "applications") {
		if row["namespace"] != namespace || row["name"] != application {
			continue
		}
		path, _ := row["path"].(string)
		status, body := world.product.Call(t, session, http.MethodGet, path, nil, nil)
		if status != http.StatusOK {
			t.Fatalf("read the Pod projection = %d %v, want 200", status, body)
		}
		names := make([]string, 0)
		for _, item := range fixtures.ObjectsOf(t, body, "items") {
			name, _ := item["name"].(string)
			names = append(names, name)
		}
		return names
	}
	return nil
}

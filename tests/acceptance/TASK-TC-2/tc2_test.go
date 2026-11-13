// Package tasktc2 checks TASK-TC-2: the overview is read-only. A write method
// submitted to its read interface is refused before anything runs, the page
// offers no modification entry at all, and both the objects and the projection
// are the same before and after the refused requests.
package tasktc2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// overviewPath is the read interface of the page (KOSD-C-12).
const overviewPath = "/api/v1/pages/" + app.PageOverview

// newWorld shapes one cluster whose overview has every item readable, so a
// modification entry would have something to act on if the page carried one.
func newWorld(t *testing.T) *fixtures.Shell {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Node(t, cluster, "node-a", true, false)
	fixtures.Node(t, cluster, "node-b", true, true)
	fixtures.Volume(t, cluster, "home", "data", v1beta1.VolumeMobilityNodeLocal, "node-b")
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: "home", Name: "shop", Owner: "lin", Volumes: []string{"data"},
	})
	trail := fixtures.NewTrail()
	trail.Record(fixtures.ObjectRefOf("node-b"), "maintain", "accepted", fixtures.Now)
	return fixtures.NewShell(t, cluster, trail)
}

func TestTASKTC2TheOverviewOffersNoModificationEntry(t *testing.T) {
	shell := newWorld(t)

	declared, found := shell.Registry.PageFor(app.PageOverview, write.RoleGuest)
	if !found {
		t.Fatal("the installed package declares no overview page")
	}
	if !declared.ReadOnly {
		t.Fatal("the overview is not declared read-only")
	}
	if declared.ReadOnlyReason == "" {
		t.Fatal("the read-only overview states no reason")
	}
	if len(declared.Actions) != 0 {
		t.Fatalf("the overview declares actions %v, want none", declared.Actions)
	}

	page := fixtures.ReadPage(t, shell, fixtures.Owner)
	if !page.ReadOnly || page.ReadOnlyReason == "" {
		t.Fatalf("read page read-only = %v reason = %q, want the declared read-only page",
			page.ReadOnly, page.ReadOnlyReason)
	}
	if len(page.Actions) != 0 {
		t.Fatalf("the read overview carries actions %v, want none", page.Actions)
	}
	for _, item := range page.Items {
		if item.Action != "" {
			t.Fatalf("item %q names action %q on a read-only page", item.ID, item.Action)
		}
		if item.Entry != nil {
			t.Fatalf("item %q carries a modification entry %+v", item.ID, item.Entry)
		}
	}
}

func TestTASKTC2WriteMethodsAreRefusedOnTheReadInterface(t *testing.T) {
	shell := newWorld(t)

	// The path is a read route: a GET without a session is refused as
	// unauthenticated, so the 405 below is the method rule and not a missing
	// route.
	unsessioned := httptest.NewRecorder()
	shell.API.ServeHTTP(unsessioned, httptest.NewRequest(http.MethodGet, overviewPath, nil))
	if unsessioned.Code != http.StatusUnauthorized {
		t.Fatalf("GET %s = %d, want the identity refusal that proves the route exists",
			overviewPath, unsessioned.Code)
	}

	// A path no component declares answers 404, so every 405 below names the
	// read interface rather than a catch-all.
	missing := httptest.NewRecorder()
	shell.API.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/pages/absent", nil))
	if missing.Code == http.StatusMethodNotAllowed {
		t.Fatal("a path no component declares answered a method refusal")
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, overviewPath, strings.NewReader(`{"nodesReady":true}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		shell.API.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, want %d", method, overviewPath, recorder.Code, http.StatusMethodNotAllowed)
		}
		if allow := recorder.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Fatalf("%s %s Allow = %q, want GET", method, overviewPath, allow)
		}
	}
}

func TestTASKTC2TheObjectsAndProjectionStayUnchanged(t *testing.T) {
	shell := newWorld(t)
	before := fixtures.Snapshot(t, shell.Cluster)
	beforePage := fixtures.PageJSON(t, fixtures.ReadPage(t, shell, fixtures.Owner))

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		shell.API.ServeHTTP(recorder, httptest.NewRequest(method, overviewPath, nil))
	}
	for _, session := range []identity.Session{fixtures.Owner, fixtures.OwningMember, fixtures.Guest} {
		page := fixtures.ReadPage(t, shell, session)
		if !page.ReadOnly || len(page.Actions) != 0 {
			t.Fatalf("%s sees a writable overview", session.Username)
		}
	}

	after := fixtures.Snapshot(t, shell.Cluster)
	if len(after) != len(before) {
		t.Fatalf("the object set changed: %d objects before, %d after", len(before), len(after))
	}
	for index := range before {
		if before[index] != after[index] {
			t.Fatalf("object changed by a refused write:\nbefore: %s\nafter:  %s", before[index], after[index])
		}
	}
	afterPage := fixtures.PageJSON(t, fixtures.ReadPage(t, shell, fixtures.Owner))
	if afterPage != beforePage {
		t.Fatalf("the projection changed by a refused write:\nbefore: %s\nafter:  %s", beforePage, afterPage)
	}

	// No modification entry appeared anywhere in what the page read returned.
	if strings.Contains(afterPage, `"entry"`) || strings.Contains(afterPage, `"actions"`) {
		t.Fatalf("the projection carries a modification entry: %s", afterPage)
	}
}

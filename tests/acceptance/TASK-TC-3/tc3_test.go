// Package tasktc3 checks TASK-TC-3: the application page reads its three
// display items and its six modification entries with the source type each
// declares; four identities each receive the entries KOSD-C-59 gives them; a
// direct call to the stop entry is judged by the control plane instead of the
// page; and an accepted action is read back from the application object's phase
// and conditions, never from a task record (KOSD-AC-16, KOSD-AC-61).
package tasktc3

import (
	"net/http"
	"sort"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"

	taskapp "github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the world's application lives.
const namespace = "home"

// owner is the account the application belongs to: the owning Member.
const owner = "lin"

// world is one cluster shaped for the application page: a healthy application
// with a volume, an endpoint and a backup policy, a catalogue entry it was
// installed from, and one named member plus guests who may see it.
type world struct {
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Definition(t, cluster, "shop", "sha256:shop-v1", "1.0.0")
	fixtures.Volume(t, cluster, namespace, "data", "cluster-movable", "")
	fixtures.Endpoint(t, cluster, namespace, "shop-web", "shop", "shop.test")
	fixtures.BackupPolicy(t, cluster, fixtures.BackupPolicyOptions{Namespace: namespace, Name: "nightly", Selector: "app=shop"})
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: owner,
		Members: []string{"mei"}, AllowGuests: true,
		DefinitionRef: "shop", Volumes: []string{"data"}, UID: "uid-shop",
		Conditions: []metav1.Condition{{
			Type: "Reconciled", Status: metav1.ConditionTrue,
			Reason: "Reconciled", Message: "下游对象已收敛",
		}},
	})
	return &world{product: fixtures.NewProduct(t, cluster, fixtures.NewTrail(), nil)}
}

// itemSources is the source type every item must declare (KOSD-C-53).
func itemSources() map[string]string {
	return map[string]string{
		taskapp.ItemInstalled:     extension.PageSourceObject,
		taskapp.ItemInstallable:   extension.PageSourceComputed,
		taskapp.ItemResourceUsage: extension.PageSourceMetric,
		taskapp.ItemStop:          extension.PageSourceObject,
		taskapp.ItemStart:         extension.PageSourceObject,
		taskapp.ItemRestart:       extension.PageSourceObject,
		taskapp.ItemScale:         extension.PageSourceObject,
		taskapp.ItemResourceSpec:  extension.PageSourceObject,
		taskapp.ItemUninstall:     extension.PageSourceObject,
	}
}

// entry is the one action a modification item names and the write it lands on
// (KOSD-C-12, KOSD-C-22).
type entry struct {
	action string
	method string
	path   string
}

// declaredEntries is the entry set the page must declare.
func declaredEntries() map[string]entry {
	return map[string]entry{
		taskapp.ItemStop:         {taskapp.ActionStop, http.MethodPost, taskapp.PathActionRequests},
		taskapp.ItemStart:        {taskapp.ActionStart, http.MethodPost, taskapp.PathActionRequests},
		taskapp.ItemRestart:      {taskapp.ActionRestart, http.MethodPost, taskapp.PathActionRequests},
		taskapp.ItemScale:        {taskapp.ActionScale, http.MethodPost, taskapp.PathActionRequests},
		taskapp.ItemResourceSpec: {taskapp.ActionResourceSpec, http.MethodPut, taskapp.PathApplications + "/{name}?namespace={namespace}"},
		taskapp.ItemUninstall:    {taskapp.ActionUninstall, http.MethodDelete, taskapp.PathApplications + "/{name}?namespace={namespace}"},
	}
}

func TestTASKTC3ThePageDeclaresThreeReadsAndSixEntries(t *testing.T) {
	world := newWorld(t)

	declared, found := world.product.Registry.PageFor(taskapp.PageApplications, write.RoleGuest)
	if !found {
		t.Fatal("the installed package declares no application page a guest may see")
	}
	if declared.ReadOnly {
		t.Fatal("the application page is declared read-only")
	}
	entries := declaredEntries()
	if len(declared.Items) != len(itemSources()) {
		t.Fatalf("the page declares %d items, want %d", len(declared.Items), len(itemSources()))
	}
	if len(declared.Actions) != len(entries) {
		t.Fatalf("the page declares %d actions, want the %d entries", len(declared.Actions), len(entries))
	}
	actions := map[string]extension.Action{}
	for _, action := range declared.Actions {
		actions[action.ID] = action
	}
	for item, want := range entries {
		action, known := actions[want.action]
		if !known {
			t.Fatalf("entry %q names action %q, which the page does not declare", item, want.action)
		}
		if action.Method != want.method || action.Path != want.path {
			t.Fatalf("action %q = %s %s, want %s %s",
				want.action, action.Method, action.Path, want.method, want.path)
		}
	}
	// Every lifecycle entry names an action value the control plane accepts, so
	// the page never offers a value the endpoint would refuse (KOSD-C-66).
	for item, want := range entries {
		if item == taskapp.ItemResourceSpec || item == taskapp.ItemUninstall {
			continue
		}
		value, known := taskapp.ControlPlaneAction(want.action)
		if !known {
			t.Fatalf("entry %q names no control-plane action", item)
		}
		if !controlplane.AcceptsAction(value) {
			t.Fatalf("entry %q names control-plane action %q, which the endpoint does not accept", item, value)
		}
	}

	for _, item := range declared.Items {
		want, known := itemSources()[item.ID]
		if !known {
			t.Fatalf("item %q is not one of the contracted items", item.ID)
		}
		if item.Source != want {
			t.Fatalf("item %q source = %q, want %q", item.ID, item.Source, want)
		}
		if item.Reference != taskapp.Identifier+"/"+item.ID {
			t.Fatalf("item %q reference = %q, want this product's own source", item.ID, item.Reference)
		}
		if item.Label == "" {
			t.Fatalf("item %q carries no label", item.ID)
		}
		wanted, isEntry := entries[item.ID]
		switch {
		case isEntry && item.Action != wanted.action:
			t.Fatalf("item %q action = %q, want %q", item.ID, item.Action, wanted.action)
		case isEntry && item.Reason != "":
			t.Fatalf("entry %q also declares a reason %q", item.ID, item.Reason)
		case !isEntry && item.Action != "":
			t.Fatalf("display item %q declares action %q", item.ID, item.Action)
		case !isEntry && item.Reason == "":
			t.Fatalf("display item %q states no reason for having no entry", item.ID)
		}
	}
}

func TestTASKTC3TheFourIdentitiesReceiveTheirOwnEntries(t *testing.T) {
	world := newWorld(t)
	for _, reader := range []struct {
		session identity.Session
		writes  bool
	}{
		{session: fixtures.Owner, writes: true},
		{session: fixtures.OwningMember, writes: true},
		{session: fixtures.NamedMember, writes: false},
		{session: fixtures.Guest, writes: false},
	} {
		page := world.product.ReadPage(t, reader.session, taskapp.PageApplications)
		for _, item := range page.Items {
			if item.ObservedAt == nil {
				t.Fatalf("%s: item %q carries no observation time", reader.session.Username, item.ID)
			}
			if item.Source != itemSources()[item.ID] {
				t.Fatalf("%s: item %q source = %q, want %q",
					reader.session.Username, item.ID, item.Source, itemSources()[item.ID])
			}
		}

		installed := fixtures.ValueOf(t, page, taskapp.ItemInstalled)
		rows := fixtures.ObjectsOf(t, installed, "applications")
		if len(rows) != 1 || rows[0]["name"] != "shop" {
			t.Fatalf("%s sees installed applications %v, want the one application",
				reader.session.Username, rows)
		}
		row := rows[0]
		if row["owner"] != owner || row["phase"] != "Ready" || row["health"] != "Healthy" {
			t.Fatalf("%s reads application %v, want the object's own owner, phase and health",
				reader.session.Username, row)
		}
		conditions, _ := row["conditions"].([]any)
		if len(conditions) != 1 {
			t.Fatalf("%s reads conditions %v, want the condition the object carries",
				reader.session.Username, row["conditions"])
		}

		// The installable list is computed from the catalogue, not filtered by
		// the application's visibility (APPSTORE-C-31, APPSTORE-C-78).
		installable := fixtures.ValueOf(t, page, taskapp.ItemInstallable)
		catalogue := fixtures.ObjectsOf(t, installable, "entries")
		if len(catalogue) != 1 || catalogue[0]["name"] != "shop" || catalogue[0]["installable"] != true {
			t.Fatalf("%s reads installable entries %v, want the installable catalogue entry",
				reader.session.Username, catalogue)
		}
		if rule, _ := installable["rule"].(string); rule == "" {
			t.Fatalf("%s reads the installable list without its rule", reader.session.Username)
		}

		// The metric source is absent in this world: the item states that and
		// carries no usage (TASK-C-2).
		usage := fixtures.ValueOf(t, page, taskapp.ItemResourceUsage)
		if usage["available"] != false {
			t.Fatalf("%s reads usage %v with no metric source", reader.session.Username, usage)
		}
		if reason, _ := usage["reason"].(string); reason == "" {
			t.Fatalf("%s reads an unavailable usage item without a reason", reader.session.Username)
		}

		for item := range declaredEntries() {
			value := fixtures.ValueOf(t, page, item)
			targets := fixtures.ObjectsOf(t, value, "applications")
			offered := len(targets) == 1 && targets[0]["name"] == "shop"
			// A running application offers every entry but start: start changes
			// a paused application only (KOSD-C-66).
			want := reader.writes && item != taskapp.ItemStart
			if offered != want {
				t.Fatalf("%s: entry %q offers the application = %v, want %v",
					reader.session.Username, item, offered, want)
			}
		}
	}
}

func TestTASKTC3DirectCallsAreJudgedByTheControlPlane(t *testing.T) {
	world := newWorld(t)
	method, path := fixtures.PageAction(t, world.product, taskapp.ActionStop, "shop", namespace)
	if method != http.MethodPost {
		t.Fatalf("stop entry = %s %s, want a POST", method, path)
	}
	stop := map[string]any{"kind": "Application", "namespace": namespace, "name": "shop", "action": "stop"}

	// The identities the page shows no entry to are refused by the API itself,
	// and the object stays as it was (KOSD-AC-16).
	for _, session := range []identity.Session{fixtures.NamedMember, fixtures.Guest} {
		status, body := world.product.Call(t, session, method, path, stop, nil)
		if status != http.StatusForbidden {
			t.Fatalf("%s stop = %d %v, want 403", session.Username, status, body)
		}
		if code, _ := fixtures.Failure(t, body)["code"].(string); code != controlplane.CodePermissionDenied {
			t.Fatalf("%s stop = %d %v, want code %q", session.Username, status, body, controlplane.CodePermissionDenied)
		}
		stored := fixtures.ApplicationObject(t, world.product.Cluster, namespace, "shop")
		if paused, _, _ := unstructured.NestedBool(stored.Object, "spec", "paused"); paused {
			t.Fatalf("%s wrote the stop marker of an application it does not own", session.Username)
		}
	}

	// The two writing identities are accepted; the second one is idempotent
	// (KOSD-C-12).
	for _, session := range []identity.Session{fixtures.OwningMember, fixtures.Owner} {
		status, body := world.product.Call(t, session, method, path, stop, nil)
		if status != http.StatusAccepted {
			t.Fatalf("%s stop = %d %v, want 202", session.Username, status, body)
		}
		if accepted, _ := body["accepted"].(bool); !accepted {
			t.Fatalf("%s stop = %v, want an accepted action", session.Username, body)
		}
	}

	// The result is the application object's: the pause marker, the phase it
	// kept and the conditions it carries, with no task record anywhere
	// (KOSD-AC-61).
	stored := fixtures.ApplicationObject(t, world.product.Cluster, namespace, "shop")
	phase, _, _ := unstructured.NestedString(stored.Object, "status", "phase")
	paused, _, _ := unstructured.NestedBool(stored.Object, "spec", "paused")
	if !paused || phase != "Ready" {
		t.Fatalf("stored application paused = %v phase = %q, want the stop marker and the phase it kept", paused, phase)
	}
	page := world.product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)
	rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstalled), "applications")
	if len(rows) != 1 {
		t.Fatalf("the page lists %d applications after the stop, want the one", len(rows))
	}
	row := rows[0]
	if row["paused"] != true {
		t.Fatalf("page paused = %v, want the object's stop marker", row["paused"])
	}
	if row["phase"] != phase {
		t.Fatalf("page phase = %v, want the object's phase %q", row["phase"], phase)
	}
	// Stop no longer applies and start does: the entries follow the state the
	// object now carries (KOSD-C-66).
	if targets := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemStop), "applications"); len(targets) != 0 {
		t.Fatalf("stop offers %v after the application paused", targets)
	}
	if targets := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemStart), "applications"); len(targets) != 1 {
		t.Fatalf("start offers %v after the application paused, want the application", targets)
	}
}

func TestTASKTC3TheFourActionsLeaveDistinctAuditRecords(t *testing.T) {
	world := newWorld(t)
	submit := func(body map[string]any) {
		t.Helper()
		status, answer := world.product.Call(t, fixtures.OwningMember, http.MethodPost, taskapp.PathActionRequests, body, nil)
		if status != http.StatusAccepted {
			t.Fatalf("%v = %d %v, want an accepted action", body, status, answer)
		}
	}
	submit(map[string]any{"kind": "Application", "namespace": namespace, "name": "shop", "action": "stop"})
	submit(map[string]any{"kind": "Application", "namespace": namespace, "name": "shop", "action": "start"})
	submit(map[string]any{"kind": "Application", "namespace": namespace, "name": "shop", "action": "restart"})
	submit(map[string]any{"kind": "Application", "namespace": namespace, "name": "shop", "action": "scale", "replicas": 3})

	accepted := map[string]controlplane.AuditRecord{}
	for _, record := range world.product.Trail.Records() {
		if record.Object.Kind != "Application" || record.Object.Namespace != namespace || record.Object.Name != "shop" {
			continue
		}
		if record.Result != controlplane.ResultAccepted {
			continue
		}
		// Each action leaves one record under the account that submitted it, so
		// the four are distinguishable (KOSD-C-14).
		if record.Actor != fixtures.OwningMember.Username {
			t.Fatalf("audit actor = %q, want %q", record.Actor, fixtures.OwningMember.Username)
		}
		accepted[record.Verb] = record
	}
	for _, verb := range []string{"stop", "start", "restart", "scale"} {
		if _, found := accepted[verb]; !found {
			t.Fatalf("audit verbs %v miss %q", keysOf(accepted), verb)
		}
	}
	if len(accepted) != 4 {
		t.Fatalf("audit verbs = %v, want the four lifecycle actions only", keysOf(accepted))
	}

	// The scale entry's own value is the submitted count, and the object
	// carries it (KOSD-AC-61).
	page := world.product.ReadPage(t, fixtures.OwningMember, taskapp.PageApplications)
	scale := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemScale), "applications")
	if len(scale) != 1 || scale[0]["replicas"] != float64(3) {
		t.Fatalf("scale entry = %v, want the count the application now carries", scale)
	}
}

func keysOf(records map[string]controlplane.AuditRecord) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Package tasktc1 checks TASK-TC-1: the overview reads its five display groups
// item by item, each with the source type it declares and the moment it was
// observed; the aggregate items follow the rules KOSD-C-53 fixes; the recent
// events are the hour that ends at the read, newest first (KOSD-AC-59); and a
// node taken out of scheduling carries the origin KOSD-C-82 judges together
// with the node-local applications it affects. The same page is read as four
// identities, and what it shows is trimmed to what each may see (KOSD-C-59).
package tasktc1

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the world's applications, volumes and events live.
const namespace = "home"

// owner is the account the applications belong to.
const owner = "lin"

// world is one cluster shaped for the overview: a healthy application with a
// volume, an endpoint and a backup policy, a failing application behind its
// definition, one maintained node with a node-local volume, a second node
// cordoned outside KOS, and four events inside and outside the window.
type world struct {
	cluster dynamic.Interface
	trail   *fixtures.Trail
	shell   *fixtures.Shell
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Node(t, cluster, "node-a", true, false)
	fixtures.Node(t, cluster, "node-b", true, true)
	fixtures.Node(t, cluster, "node-c", false, true)
	fixtures.Node(t, cluster, "node-d", true, true)

	fixtures.Definition(t, cluster, "shop", "sha256:shop-v1", "1.0.0")
	fixtures.Definition(t, cluster, "reports", "sha256:reports-v2", "2.0.0")
	fixtures.Volume(t, cluster, namespace, "data", v1beta1.VolumeMobilityNodeLocal, "node-b")

	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: owner, Members: []string{"mei"},
		DefinitionRef: "shop", Volumes: []string{"data"},
		AppliedDigest: "sha256:shop-v1",
	})
	fixtures.Endpoint(t, cluster, namespace, "shop-web", "shop", "shop.test")
	fixtures.BackupPolicy(t, cluster, fixtures.BackupPolicyOptions{Namespace: namespace, Name: "nightly", Selector: "app=shop"})
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "reports", Owner: owner, DefinitionRef: "reports",
		Phase: "Failed", Health: string(v1beta1.HealthStateDegraded),
		Conditions: []metav1.Condition{{
			Type: "Reconciled", Status: metav1.ConditionFalse,
			Reason: "RenderFailed", Message: "渲染失败",
		}},
		AppliedDigest: "sha256:reports-v1",
	})
	// gateway is healthy and still abnormal: the rule counts an unmet condition
	// on its own (KOSD-C-53).
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "gateway", Owner: owner,
		Conditions: []metav1.Condition{{
			Type: "Reconciled", Status: metav1.ConditionUnknown,
			Reason: "Waiting", Message: "等待入口就绪",
		}},
	})
	// search is already upgrading, which the version discovery reports as
	// pending without a definition change (TASK-C-1).
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "search", Owner: owner,
		Phase: "Upgrading", AppliedDigest: "sha256:search-v1",
		UpgradeTarget: "sha256:search-v2",
	})

	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "shop-backoff", Namespace: namespace,
		SubjectGroup: api.Group, SubjectKind: "Application",
		SubjectNamespace: namespace, SubjectName: "shop",
		Reason: "BackOff", Message: "容器反复退出",
		LastSeen: fixtures.Now.Add(-10 * time.Minute),
	})
	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "node-b-drain", Namespace: namespace,
		SubjectKind: "Node", SubjectName: "node-b",
		Reason: "Drain", Message: "节点进入维护",
		LastSeen: fixtures.Now.Add(-20 * time.Minute),
	})
	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "reports-failed", Namespace: namespace,
		SubjectGroup: api.Group, SubjectKind: "Application",
		SubjectNamespace: namespace, SubjectName: "reports",
		Reason: "RenderFailed", Message: "渲染失败",
		LastSeen: fixtures.Now.Add(-50 * time.Minute),
	})
	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "shop-old", Namespace: namespace,
		SubjectGroup: api.Group, SubjectKind: "Application",
		SubjectNamespace: namespace, SubjectName: "shop",
		Reason: "Old", Message: "窗口之前的事件",
		LastSeen: fixtures.Now.Add(-2 * time.Hour),
	})

	trail := fixtures.NewTrail()
	trail.Record(fixtures.ObjectRefOf("node-b"), controlplane.VerbMaintain,
		controlplane.ResultAccepted, fixtures.Now.Add(-5*time.Minute))
	// node-d's newest effective record is the restore, so its cordon is a
	// second, unpaired write the rule does not count as planned (KOSD-C-82).
	trail.Record(fixtures.ObjectRefOf("node-d"), controlplane.VerbMaintain,
		controlplane.ResultAccepted, fixtures.Now.Add(-30*time.Minute))
	trail.Record(fixtures.ObjectRefOf("node-d"), controlplane.VerbRestore,
		controlplane.ResultAccepted, fixtures.Now.Add(-20*time.Minute))
	// node-a is back in scheduling: its unpaired record produces no
	// maintenance entry at all.
	trail.Record(fixtures.ObjectRefOf("node-a"), controlplane.VerbMaintain,
		controlplane.ResultAccepted, fixtures.Now.Add(-40*time.Minute))

	return &world{cluster: cluster, trail: trail, shell: fixtures.NewShell(t, cluster, trail)}
}

// read reads the overview as one session.
func (w *world) read(t *testing.T, session identity.Session) pages.Page {
	t.Helper()
	return fixtures.ReadPage(t, w.shell, session)
}

// itemSources is the source type every item must declare (KOSD-C-53).
func itemSources() map[string]string {
	return map[string]string{
		app.ItemNodesReady:          extension.PageSourceObject,
		app.ItemAbnormalObjects:     extension.PageSourceComputed,
		app.ItemPendingUpgrades:     extension.PageSourceComputed,
		app.ItemRecentEvents:        extension.PageSourceObject,
		app.ItemMaintenanceNodes:    extension.PageSourceObject,
		app.ItemMaintenanceOrigin:   extension.PageSourceDatabase,
		app.ItemMaintenanceAffected: extension.PageSourceComputed,
	}
}

func TestTASKTC1TheOverviewDeclaresOneItemPerSource(t *testing.T) {
	world := newWorld(t)
	declared, found := world.shell.Registry.PageFor(app.PageOverview, write.RoleGuest)
	if !found {
		t.Fatal("the installed package declares no overview page a guest may read")
	}
	if !declared.ReadOnly || declared.ReadOnlyReason == "" {
		t.Fatalf("overview read-only = %v reason %q, want a read-only page that states why",
			declared.ReadOnly, declared.ReadOnlyReason)
	}
	if len(declared.Actions) != 0 {
		t.Fatalf("the overview declares actions %v, want none", declared.Actions)
	}
	sources := itemSources()
	if len(declared.Items) != len(sources) {
		t.Fatalf("the overview declares %d items, want %d", len(declared.Items), len(sources))
	}
	for _, item := range declared.Items {
		want, known := sources[item.ID]
		if !known {
			t.Fatalf("item %q is not one of the contracted display items", item.ID)
		}
		if item.Source != want {
			t.Fatalf("item %q source = %q, want %q", item.ID, item.Source, want)
		}
		if want == extension.PageSourceLocal {
			continue
		}
		if item.Reference != app.Identifier+"/"+item.ID {
			t.Fatalf("item %q reference = %q, want this product's own source", item.ID, item.Reference)
		}
		if item.Label == "" {
			t.Fatalf("item %q carries no label", item.ID)
		}
	}
}

func TestTASKTC1EveryItemCarriesItsObservationAndFreshness(t *testing.T) {
	world := newWorld(t)

	fresh := world.read(t, fixtures.Owner)
	if fresh.ID != app.PageOverview || len(fresh.Items) != len(itemSources()) {
		t.Fatalf("overview = %q with %d items, want %q with %d",
			fresh.ID, len(fresh.Items), app.PageOverview, len(itemSources()))
	}
	for _, item := range fresh.Items {
		if item.ObservedAt == nil {
			t.Fatalf("item %q carries no observation time", item.ID)
		}
		if !item.ObservedAt.Equal(fixtures.Now) {
			t.Fatalf("item %q observed at %s, want the read clock %s", item.ID, item.ObservedAt, fixtures.Now)
		}
		if item.Stale {
			t.Fatalf("item %q is marked stale at the observation it carries", item.ID)
		}
	}

	// The bound is the one KOSD-C-13 fixes: at the threshold the value is fresh,
	// one second past it every item is marked stale.
	for _, step := range []struct {
		offset time.Duration
		stale  bool
	}{
		{offset: controlplane.StaleThreshold, stale: false},
		{offset: controlplane.StaleThreshold + time.Second, stale: true},
	} {
		shell := fixtures.NewShellAt(t, world.cluster, world.trail, fixtures.Now.Add(step.offset))
		page := fixtures.ReadPage(t, shell, fixtures.Owner)
		for _, item := range page.Items {
			if item.Stale != step.stale {
				t.Fatalf("item %q stale = %v after %s, want %v", item.ID, item.Stale, step.offset, step.stale)
			}
		}
	}
}

func TestTASKTC1AggregatesFollowTheDeclaredRules(t *testing.T) {
	world := newWorld(t)
	page := world.read(t, fixtures.Owner)

	// Node readiness is the object source: four nodes, one not ready.
	nodes := fixtures.ValueOf(t, page, app.ItemNodesReady)
	readiness := fixtures.ObjectsOf(t, nodes, "nodes")
	if len(readiness) != 4 {
		t.Fatalf("nodes = %d, want the four nodes", len(readiness))
	}
	if ready, _ := nodes["ready"].(float64); ready != 3 {
		t.Fatalf("ready = %v, want 3 of 4", nodes["ready"])
	}
	notReady := namesWhere(readiness, "ready", false)
	if len(notReady) != 1 || notReady[0] != "node-c" {
		t.Fatalf("nodes not ready = %v, want node-c", notReady)
	}

	// Abnormal objects: a degraded health or an unmet condition each count, and
	// nothing else does (KOSD-C-53).
	abnormal := fixtures.ValueOf(t, page, app.ItemAbnormalObjects)
	rule, _ := abnormal["rule"].(string)
	if rule == "" || !strings.Contains(rule, "Healthy") || !strings.Contains(rule, "条件") {
		t.Fatalf("the abnormal item states the rule as %q, want health and conditions named", rule)
	}
	objects := fixtures.ObjectsOf(t, abnormal, "objects")
	if got := keysOf(objects); !equalStrings(got, []string{"home/gateway", "home/reports"}) {
		t.Fatalf("abnormal objects = %v, want the two the rule finds", got)
	}
	byName := map[string]map[string]any{}
	for _, object := range objects {
		byName[object["name"].(string)] = object
	}
	if reports := byName["reports"]; reports["health"] != string(v1beta1.HealthStateDegraded) {
		t.Fatalf("reports health = %v, want the object's own degraded health", reports["health"])
	}
	conditions, _ := byName["gateway"]["conditions"].([]any)
	if len(conditions) != 1 {
		t.Fatalf("gateway conditions = %v, want the unmet condition that makes it abnormal", byName["gateway"]["conditions"])
	}

	// Pending upgrades: the version discovery — an application behind its
	// definition, and one the reconciler is already upgrading (TASK-C-1).
	upgrades := fixtures.ValueOf(t, page, app.ItemPendingUpgrades)
	pending := fixtures.ObjectsOf(t, upgrades, "pending")
	if got := keysOf(pending); !equalStrings(got, []string{"home/reports", "home/search"}) {
		t.Fatalf("pending upgrades = %v, want the two the discovery finds", got)
	}
	states := map[string]map[string]any{}
	for _, entry := range pending {
		states[entry["name"].(string)] = entry
	}
	if entry := states["reports"]; entry["state"] != "definition-changed" ||
		entry["target"] != "sha256:reports-v2" || entry["version"] != "2.0.0" {
		t.Fatalf("reports pending = %v, want the digest the definition offers", entry)
	}
	if entry := states["search"]; entry["state"] != "in-progress" || entry["target"] != "sha256:search-v2" {
		t.Fatalf("search pending = %v, want the running upgrade target", entry)
	}
}

func TestTASKTC1TheRecentEventsAreTheWindowNewestFirst(t *testing.T) {
	world := newWorld(t)
	page := world.read(t, fixtures.Owner)

	recent := fixtures.ValueOf(t, page, app.ItemRecentEvents)
	if window, _ := recent["window"].(string); window != controlplane.EventWindow.String() {
		t.Fatalf("window = %q, want %q", recent["window"], controlplane.EventWindow)
	}
	events := fixtures.ObjectsOf(t, recent, "events")
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event["name"].(string))
	}
	// The three events inside the hour, newest first; the two-hour-old event is
	// not readable (KOSD-C-64, KOSD-AC-59).
	want := []string{"shop-backoff", "node-b-drain", "reports-failed"}
	if len(names) != len(want) {
		t.Fatalf("events = %v, want %v inside the window", names, want)
	}
	for index, name := range want {
		if names[index] != name {
			t.Fatalf("events = %v, want %v newest first", names, want)
		}
	}
	for _, name := range names {
		if name == "shop-old" {
			t.Fatal("an event older than the window is readable")
		}
	}
}

func TestTASKTC1MaintenanceCarriesOriginAndAffectedApplications(t *testing.T) {
	world := newWorld(t)
	page := world.read(t, fixtures.Owner)

	maintained := fixtures.ValueOf(t, page, app.ItemMaintenanceNodes)
	nodes := fixtures.ObjectsOf(t, maintained, "nodes")
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node["name"].(string))
	}
	sort.Strings(names)
	if !equalStrings(names, []string{"node-b", "node-c", "node-d"}) {
		t.Fatalf("maintenance nodes = %v, want the unschedulable nodes and only them", names)
	}

	origins := fixtures.ValueOf(t, page, app.ItemMaintenanceOrigin)
	judged := map[string]string{}
	for _, entry := range fixtures.ObjectsOf(t, origins, "nodes") {
		judged[entry["name"].(string)] = entry["origin"].(string)
	}
	if judged["node-b"] != "planned" {
		t.Fatalf("node-b origin = %q, want planned from its accepted cordon", judged["node-b"])
	}
	if judged["node-c"] != "unplanned" {
		t.Fatalf("node-c origin = %q, want unplanned without a KOS record", judged["node-c"])
	}
	if judged["node-d"] != "unplanned" {
		t.Fatalf("node-d origin = %q, want unplanned when the newest record is a restore", judged["node-d"])
	}
	if _, listed := judged["node-a"]; listed {
		t.Fatal("a node back in scheduling still produces a maintenance origin")
	}

	affected := fixtures.ValueOf(t, page, app.ItemMaintenanceAffected)
	apps := fixtures.ObjectsOf(t, affected, "nodes")
	if len(apps) != 1 {
		t.Fatalf("affected nodes = %v, want the node holding a node-local volume", apps)
	}
	if apps[0]["node"] != "node-b" {
		t.Fatalf("affected node = %v, want node-b", apps[0]["node"])
	}
	applications := stringList(t, apps[0]["applications"])
	if len(applications) != 1 || applications[0] != namespace+"/shop" {
		t.Fatalf("affected applications = %v, want %s/shop", applications, namespace)
	}

	// An audit trail that cannot be read is unknown, never planned (KOSD-C-82).
	world.trail.Err = errors.New("audit is not answering")
	blind := fixtures.ValueOf(t, world.read(t, fixtures.Owner), app.ItemMaintenanceOrigin)
	for _, entry := range fixtures.ObjectsOf(t, blind, "nodes") {
		if entry["origin"] != "unknown" {
			t.Fatalf("node %v origin = %v while the trail is down, want unknown", entry["name"], entry["origin"])
		}
	}
}

func TestTASKTC1VisibilityTrimsWhatEachIdentityReads(t *testing.T) {
	world := newWorld(t)

	// Each identity's expectation: what the aggregates and events show it. The
	// named member sees the application that names it, the member without a
	// name sees neither, and the guest sees neither because neither allows
	// guests (KOSD-C-59).
	cases := []struct {
		session      identity.Session
		abnormal     []string
		pending      []string
		events       []string
		affectedApps []string
	}{
		{session: fixtures.Owner,
			abnormal: []string{"home/gateway", "home/reports"}, pending: []string{"home/reports", "home/search"},
			events:       []string{"shop-backoff", "node-b-drain", "reports-failed"},
			affectedApps: []string{"home/shop"}},
		{session: fixtures.OwningMember,
			abnormal: []string{"home/gateway", "home/reports"}, pending: []string{"home/reports", "home/search"},
			events:       []string{"shop-backoff", "reports-failed"},
			affectedApps: []string{"home/shop"}},
		{session: fixtures.NamedMember,
			abnormal: []string{}, pending: []string{},
			events:       []string{"shop-backoff"},
			affectedApps: []string{"home/shop"}},
		{session: fixtures.OtherMember,
			abnormal: []string{}, pending: []string{},
			events: []string{}, affectedApps: []string{}},
		{session: fixtures.Guest,
			abnormal: []string{}, pending: []string{},
			events: []string{}, affectedApps: []string{}},
	}
	for _, testCase := range cases {
		page := world.read(t, testCase.session)

		abnormal := fixtures.ValueOf(t, page, app.ItemAbnormalObjects)
		if got := keysOf(fixtures.ObjectsOf(t, abnormal, "objects")); !equalStrings(got, testCase.abnormal) {
			t.Fatalf("%s abnormal = %v, want %v", testCase.session.Username, got, testCase.abnormal)
		}
		pending := fixtures.ValueOf(t, page, app.ItemPendingUpgrades)
		if got := keysOf(fixtures.ObjectsOf(t, pending, "pending")); !equalStrings(got, testCase.pending) {
			t.Fatalf("%s pending = %v, want %v", testCase.session.Username, got, testCase.pending)
		}
		recent := fixtures.ValueOf(t, page, app.ItemRecentEvents)
		names := make([]string, 0)
		for _, event := range fixtures.ObjectsOf(t, recent, "events") {
			names = append(names, event["name"].(string))
		}
		if !equalStrings(names, testCase.events) {
			t.Fatalf("%s events = %v, want %v", testCase.session.Username, names, testCase.events)
		}
		affected := fixtures.ValueOf(t, page, app.ItemMaintenanceAffected)
		applications := make([]string, 0)
		for _, entry := range fixtures.ObjectsOf(t, affected, "nodes") {
			applications = append(applications, stringList(t, entry["applications"])...)
		}
		if !equalStrings(applications, testCase.affectedApps) {
			t.Fatalf("%s affected = %v, want %v", testCase.session.Username, applications, testCase.affectedApps)
		}

		// Nodes and their origin are not application facts: every signed-in
		// identity reads the same maintenance state (KOSD-C-59).
		maintained := fixtures.ValueOf(t, page, app.ItemMaintenanceNodes)
		if entries := fixtures.ObjectsOf(t, maintained, "nodes"); len(entries) != 3 {
			t.Fatalf("%s maintenance nodes = %v, want the unschedulable nodes", testCase.session.Username, entries)
		}
	}
}

// keysOf renders abnormal or pending entries as kind-less identity keys.
func keysOf(entries []map[string]any) []string {
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		namespace, _ := entry["namespace"].(string)
		name, _ := entry["name"].(string)
		keys = append(keys, namespace+"/"+name)
	}
	sort.Strings(keys)
	return keys
}

// namesWhere lists the entries whose boolean field carries want.
func namesWhere(entries []map[string]any, field string, want bool) []string {
	names := make([]string, 0)
	for _, entry := range entries {
		value, _ := entry[field].(bool)
		if value == want {
			names = append(names, entry["name"].(string))
		}
	}
	sort.Strings(names)
	return names
}

func stringList(t *testing.T, value any) []string {
	t.Helper()
	entries, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want a list", value)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.(string))
	}
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

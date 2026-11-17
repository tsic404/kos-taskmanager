// Package tasktc12 checks TASK-TC-12: the application detail page resolves the
// health, volume, endpoint, backup, drift and event items DASH-C-52 lists, each
// with the source type and observation time the page shell stamps, and the log
// item names the per-application read KOSD-C-64 bounds. It reads the page as
// identities that may and may not see the application, reads one application's
// logs through the address the page declares, and disables the component to
// prove the page and its entry leave together while another component's page
// stays served (KOSD-C-59, KOSD-AC-16, KOSD-AC-54).
package tasktc12

import (
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

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

// namespace is where the application, its volume and its endpoints live.
const namespace = "home"

// application is the application the detail page is read for.
const application = "shop"

// other is a second application whose facts and logs never appear on shop's rows.
const other = "reports"

// world is one cluster shaped for the detail page: an application with a volume,
// an endpoint, a cover/ingest-and-drift record and one event inside the window,
// a second application carrying its own event, and the Pods and logs a log read
// collects.
type world struct {
	cluster dynamic.Interface
	trail   *fixtures.Trail
	logs    *fixtures.LogSource
	shell   *fixtures.Shell
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)

	fixtures.Volume(t, cluster, namespace, "data", v1beta1.VolumeMobilityNodeLocal, "node-a")
	fixtures.Volume(t, cluster, namespace, "media", v1beta1.VolumeMobilitySharedRWX, "")
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin", Members: []string{"mei"},
		DefinitionRef: "shop", Volumes: []string{"data", "gone"},
		Labels:      map[string]string{"app": application, "tier": "web"},
		DriftPolicy: v1beta1.DriftPolicyAdopt,
		Drift: []v1beta1.DriftEntry{{
			Object:   v1beta1.ObjectRef{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: namespace, Name: application},
			Path:     "spec.replicas",
			External: "5",
			Expected: "2",
		}},
		Rendered: []v1beta1.ObjectRef{{
			Group: "apps", Version: "v1", Kind: "Deployment", Namespace: namespace, Name: application,
		}},
	})
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: other, Owner: "lin",
		Labels: map[string]string{"app": other},
	})
	fixtures.Endpoint(t, cluster, namespace, "shop-web", application, "shop.test")
	fixtures.Endpoint(t, cluster, namespace, "reports-web", other, "reports.test")
	fixtures.BackupPolicy(t, cluster, fixtures.BackupPolicyOptions{
		Namespace: namespace, Name: "nightly", Selector: "app=shop",
	})
	fixtures.BackupPolicy(t, cluster, fixtures.BackupPolicyOptions{
		Namespace: namespace, Name: "reports-only", Selector: "app=reports",
	})
	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "shop-backoff", Namespace: namespace,
		SubjectGroup: api.Group, SubjectKind: "Application",
		SubjectNamespace: namespace, SubjectName: application,
		Reason: "BackOff", Message: "容器反复退出",
		LastSeen: fixtures.Now.Add(-10 * time.Minute),
	})
	fixtures.Event(t, cluster, fixtures.EventOptions{
		Name: "reports-backoff", Namespace: namespace,
		SubjectGroup: api.Group, SubjectKind: "Application",
		SubjectNamespace: namespace, SubjectName: other,
		Reason: "BackOff", Message: "另一个应用的事件",
		LastSeen: fixtures.Now.Add(-5 * time.Minute),
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	fixtures.Pod(t, cluster, namespace, "shop-1", map[string]string{"app": application}, []string{"web"})
	fixtures.Pod(t, cluster, namespace, "shop-2", map[string]string{"app": application}, []string{"web"})
	fixtures.Pod(t, cluster, namespace, "reports-1", map[string]string{"app": other}, []string{"web"})

	logs := &fixtures.LogSource{ByContainer: map[string][]controlplane.LogLine{
		fixtures.LogKey(namespace, "shop-1", "web"): {
			{Timestamp: fixtures.Now.Add(-20 * time.Minute), Pod: "shop-1", Container: "web", Message: "窗口之前的行"},
			{Timestamp: fixtures.Now.Add(-3 * time.Minute), Pod: "shop-1", Container: "web", Message: "shop 的行"},
		},
		fixtures.LogKey(namespace, "shop-2", "web"): {
			{Timestamp: fixtures.Now.Add(-2 * time.Minute), Pod: "shop-2", Container: "web", Message: "shop 的另一个副本"},
		},
		fixtures.LogKey(namespace, "reports-1", "web"): {
			{Timestamp: fixtures.Now.Add(-1 * time.Minute), Pod: "reports-1", Container: "web", Message: "reports 的行"},
		},
	}}
	trail := fixtures.NewTrail()
	return &world{
		cluster: cluster, trail: trail, logs: logs,
		shell:   fixtures.NewShell(t, cluster, trail),
		product: fixtures.NewProductWith(t, cluster, trail, nil, logs),
	}
}

// readDetail reads the detail page as one session.
func (w *world) readDetail(t *testing.T, session identity.Session) pages.Page {
	t.Helper()
	page, failure := w.shell.Pages.Read(t.Context(), session, app.PageDetail)
	if failure != nil {
		t.Fatalf("read the detail page as %q: %s (%s)", session.Username, failure.Message, failure.Code)
	}
	return page
}

// itemSources is the source type every detail item must declare (DASH-C-52).
func itemSources() map[string]string {
	return map[string]string{
		app.ItemDetailHealth:  extension.PageSourceObject,
		app.ItemDetailVolumes: extension.PageSourceObject,
		app.ItemDetailIngress: extension.PageSourceObject,
		app.ItemDetailBackup:  extension.PageSourceObject,
		app.ItemDetailDrift:   extension.PageSourceObject,
		app.ItemDetailEvents:  extension.PageSourceObject,
		app.ItemDetailLogs:    extension.PageSourceRead,
	}
}

// rowsOf reads one item's per-application rows.
func rowsOf(t *testing.T, page pages.Page, id string) []map[string]any {
	t.Helper()
	return fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, id), "applications")
}

// rowOf picks the row one application's facts are on; a page that carries no row
// for the application shows none of its facts (KOSD-C-59).
func rowOf(t *testing.T, page pages.Page, id, name string) (map[string]any, bool) {
	t.Helper()
	for _, row := range rowsOf(t, page, id) {
		if row["name"] == name && row["namespace"] == namespace {
			return row, true
		}
	}
	return nil, false
}

func TestTASKTC12TheDetailPageDeclaresTheItemsDashC52Lists(t *testing.T) {
	world := newWorld(t)
	declared, found := world.shell.Registry.PageFor(app.PageDetail, write.RoleGuest)
	if !found {
		t.Fatal("the installed package declares no detail page a guest may read")
	}
	if !declared.ReadOnly || declared.ReadOnlyReason == "" {
		t.Fatalf("detail read-only = %v reason %q, want read-only with a reason",
			declared.ReadOnly, declared.ReadOnlyReason)
	}
	if len(declared.Actions) != 0 {
		t.Fatalf("the detail page declares actions %v, want none", declared.Actions)
	}
	sources := itemSources()
	if len(declared.Items) != len(sources) {
		t.Fatalf("the detail page declares %d items, want %d", len(declared.Items), len(sources))
	}
	for _, item := range declared.Items {
		want, known := sources[item.ID]
		if !known {
			t.Fatalf("item %q is not one of the detail items DASH-C-52 lists", item.ID)
		}
		if item.Source != want {
			t.Fatalf("item %q source = %q, want %q", item.ID, item.Source, want)
		}
		if item.Label == "" {
			t.Fatalf("item %q carries no label", item.ID)
		}
		if item.Reference != app.Identifier+"/"+item.ID {
			t.Fatalf("item %q reference = %q, want this product's own source", item.ID, item.Reference)
		}
		// A log read resolves per application, so the item states where the read
		// happens rather than claiming to have read every application (KOSD-C-64).
		if item.ID == app.ItemDetailLogs && item.Action != "" {
			t.Fatalf("the log item declares the action %q, want no modification entry", item.Action)
		}
	}
}

func TestTASKTC12EachItemResolvesForTheAddressedApplication(t *testing.T) {
	world := newWorld(t)
	page := world.readDetail(t, fixtures.OwningMember)

	if page.ID != app.PageDetail || len(page.Items) != len(itemSources()) {
		t.Fatalf("detail page = %q with %d items, want %q with %d",
			page.ID, len(page.Items), app.PageDetail, len(itemSources()))
	}
	for _, item := range page.Items {
		if item.ObservedAt == nil {
			t.Fatalf("item %q carries no observation time", item.ID)
		}
		if !item.ObservedAt.Equal(fixtures.Now) {
			t.Fatalf("item %q observed at %s, want the read clock %s", item.ID, item.ObservedAt, fixtures.Now)
		}
		if item.Stale {
			t.Fatalf("item %q is marked stale at the observation it carries", item.ID)
		}
		if _, found := rowOf(t, page, item.ID, application); !found {
			t.Fatalf("item %q carries no row for %s/%s", item.ID, namespace, application)
		}
	}

	// Health is the application object's own state.
	health, _ := rowOf(t, page, app.ItemDetailHealth, application)
	if health["phase"] != "Ready" || health["health"] != string(v1beta1.HealthStateHealthy) {
		t.Fatalf("health row = %v, want the application's phase and health", health)
	}

	// Volumes: the declared and resolvable one carries the volume's own facts,
	// and a declaration the cluster cannot resolve stays visible as unresolved.
	volumes, _ := rowOf(t, page, app.ItemDetailVolumes, application)
	declaredVolumes := fixtures.ObjectsOf(t, volumes, "volumes")
	if got := namesOf(declaredVolumes); !equalStrings(got, []string{"data", "gone"}) {
		t.Fatalf("volumes = %v, want the two the application declares", got)
	}
	data := declaredVolumes[0]
	if data["resolved"] != true || data["mobility"] != string(v1beta1.VolumeMobilityNodeLocal) || data["node"] != "node-a" {
		t.Fatalf("volume data = %v, want the node-local volume bound to node-a", data)
	}
	if declaredVolumes[1]["resolved"] != false {
		t.Fatalf("volume gone = %v, want an unresolved declaration", declaredVolumes[1])
	}

	// Endpoints: only the one whose target is this application.
	ingress, _ := rowOf(t, page, app.ItemDetailIngress, application)
	endpoints := fixtures.ObjectsOf(t, ingress, "endpoints")
	if got := namesOf(endpoints); !equalStrings(got, []string{"shop-web"}) {
		t.Fatalf("endpoints = %v, want the one exposing shop", got)
	}
	if endpoints[0]["host"] != "shop.test" || endpoints[0]["kind"] != "http.host" {
		t.Fatalf("endpoint = %v, want the declared host and shape", endpoints[0])
	}

	// Backup: only the policy whose selector covers this application.
	backup, _ := rowOf(t, page, app.ItemDetailBackup, application)
	policies := fixtures.ObjectsOf(t, backup, "policies")
	if got := namesOf(policies); !equalStrings(got, []string{"nightly"}) {
		t.Fatalf("backup policies = %v, want the one covering shop", got)
	}
	if policies[0]["schedule"] != "0 3 * * *" || policies[0]["drill"] != true {
		t.Fatalf("policy = %v, want the schedule and drill it declares", policies[0])
	}

	// Drift: the record the application carries, with the arbitration policy.
	drift, _ := rowOf(t, page, app.ItemDetailDrift, application)
	if drift["policy"] != string(v1beta1.DriftPolicyAdopt) {
		t.Fatalf("drift policy = %v, want adopt", drift["policy"])
	}
	entries := fixtures.ObjectsOf(t, drift, "entries")
	if len(entries) != 1 || entries[0]["path"] != "spec.replicas" {
		t.Fatalf("drift entries = %v, want the recorded downstream path", entries)
	}

	// Events: the window the control plane fixes, newest first, only this
	// application's (KOSD-C-64, KOSD-AC-59).
	eventsValue := fixtures.ValueOf(t, page, app.ItemDetailEvents)
	if window, _ := eventsValue["window"].(string); window != controlplane.EventWindow.String() {
		t.Fatalf("event window = %v, want %s", eventsValue["window"], controlplane.EventWindow)
	}
	events, _ := rowOf(t, page, app.ItemDetailEvents, application)
	listed := fixtures.ObjectsOf(t, events, "events")
	if len(listed) != 1 || listed[0]["name"] != "shop-backoff" {
		t.Fatalf("events = %v, want the one about shop", listed)
	}

	// Logs: the row names this application's own read (KOSD-C-64).
	logsRow, _ := rowOf(t, page, app.ItemDetailLogs, application)
	want := "/api/v1/logs/" + namespace + "/" + application
	if logsRow["path"] != want {
		t.Fatalf("log read path = %v, want %q", logsRow["path"], want)
	}
}

func TestTASKTC12TheStaleMarkFollowsTheFreshnessBound(t *testing.T) {
	world := newWorld(t)
	for _, step := range []struct {
		offset time.Duration
		stale  bool
	}{
		{offset: controlplane.StaleThreshold, stale: false},
		{offset: controlplane.StaleThreshold + time.Second, stale: true},
	} {
		shell := fixtures.NewShellAt(t, world.cluster, world.trail, fixtures.Now.Add(step.offset))
		page, failure := shell.Pages.Read(t.Context(), fixtures.Owner, app.PageDetail)
		if failure != nil {
			t.Fatalf("read the detail page %s after the observation: %s", step.offset, failure.Message)
		}
		for _, item := range page.Items {
			if item.Stale != step.stale {
				t.Fatalf("item %q stale = %v after %s, want %v", item.ID, item.Stale, step.offset, step.stale)
			}
		}
	}
}

func TestTASKTC12AnIdentityThatCannotSeeTheApplicationGetsNoRow(t *testing.T) {
	world := newWorld(t)
	// mei is named on the application, kai is not (KOSD-C-59).
	for _, step := range []struct {
		session identity.Session
		visible bool
	}{
		{session: fixtures.OwningMember, visible: true},
		{session: fixtures.Owner, visible: true},
		{session: fixtures.NamedMember, visible: true},
		{session: fixtures.OtherMember, visible: false},
		{session: fixtures.Guest, visible: false},
	} {
		page := world.readDetail(t, step.session)
		_, found := rowOf(t, page, app.ItemDetailHealth, application)
		if found != step.visible {
			t.Fatalf("%s sees shop on the detail page = %v, want %v", step.session.Username, found, step.visible)
		}
		if step.visible {
			continue
		}
		// An identity that cannot see the application sees none of its facts,
		// on any item: the page and its entry are as absent as the application.
		for _, item := range page.Items {
			if _, found := rowOf(t, page, item.ID, application); found {
				t.Fatalf("%s reads item %q of an application it may not see", step.session.Username, item.ID)
			}
		}
	}
}

// logRead reads one application's logs through the control plane as one account.
func (w *world) logRead(t *testing.T, session identity.Session, path, query string) (int, map[string]any) {
	t.Helper()
	if query != "" {
		path += "?" + query
	}
	return w.product.Call(t, session, http.MethodGet, path, nil, nil)
}

func TestTASKTC12LogsAreReadPerApplicationWithinTheBounds(t *testing.T) {
	world := newWorld(t)
	page := world.readDetail(t, fixtures.OwningMember)
	logsRow, found := rowOf(t, page, app.ItemDetailLogs, application)
	if !found {
		t.Fatalf("the log item carries no row for %s", application)
	}
	path, _ := logsRow["path"].(string)
	if path == "" {
		t.Fatalf("the log row = %v, want the read address", logsRow)
	}

	// The read returns only this application's workload lines, and only inside
	// the window the read ran on (KOSD-C-64).
	status, body := world.logRead(t, fixtures.OwningMember, path, "")
	if status != http.StatusOK {
		t.Fatalf("log read = %d %v, want 200", status, body)
	}
	lines := linesOf(t, body)
	if got := messagesOf(lines); !equalStrings(got, []string{"shop 的另一个副本", "shop 的行"}) {
		t.Fatalf("log lines = %v, want shop's two lines inside the window", got)
	}
	if body["truncated"] == true {
		t.Fatalf("log read = %v, want no truncation at the default bound", body)
	}

	// A request above the line bound is clamped, and the bound that kept the
	// newest lines marks where the read was cut.
	status, body = world.logRead(t, fixtures.OwningMember, path, "limit=100000&window=24h")
	if status != http.StatusOK {
		t.Fatalf("bounded log read = %d %v, want 200", status, body)
	}
	if limit, _ := body["limit"].(float64); limit != 1000 {
		t.Fatalf("clamped limit = %v, want the 1000-line bound", body["limit"])
	}
	if window, _ := body["window"].(string); window != time.Hour.String() {
		t.Fatalf("clamped window = %v, want %s", body["window"], time.Hour)
	}

	// Truncation keeps the newest lines and names the position the read starts
	// at; a full download is not on offer.
	status, body = world.logRead(t, fixtures.OwningMember, path, "limit=1")
	if status != http.StatusOK {
		t.Fatalf("truncated log read = %d %v, want 200", status, body)
	}
	if body["truncated"] != true {
		t.Fatalf("truncated read = %v, want the truncation mark", body)
	}
	if kept := linesOf(t, body); len(kept) != 1 || kept[0]["message"] != "shop 的另一个副本" {
		t.Fatalf("kept lines = %v, want the newest line", kept)
	}
	if cut, _ := body["truncatedAt"].(string); cut == "" {
		t.Fatalf("truncated read = %v, want the position the read was cut at", body)
	}
	if _, found := body["download"]; found {
		t.Fatalf("log read = %v, want no full-download offer", body)
	}
}

func TestTASKTC12AnUnreadableLogSourceStatesTheReason(t *testing.T) {
	world := newWorld(t)
	world.logs.Err = errors.New("kubelet 不可达")
	page := world.readDetail(t, fixtures.OwningMember)
	logsRow, _ := rowOf(t, page, app.ItemDetailLogs, application)
	path, _ := logsRow["path"].(string)

	status, body := world.logRead(t, fixtures.OwningMember, path, "")
	if status == http.StatusOK {
		t.Fatalf("log read of an unreachable source = 200 %v, want a failure", body)
	}
	failure := fixtures.Failure(t, body)
	if message, _ := failure["message"].(string); message == "" {
		t.Fatalf("log failure = %v, want a reason instead of an empty read", failure)
	}
	if _, found := body["lines"]; found {
		t.Fatalf("log failure = %v, want no empty line list", body)
	}
}

func TestTASKTC12AnInvisibleApplicationReadsNoLogs(t *testing.T) {
	world := newWorld(t)
	page := world.readDetail(t, fixtures.OwningMember)
	logsRow, _ := rowOf(t, page, app.ItemDetailLogs, application)
	path, _ := logsRow["path"].(string)
	_, hidden := world.logRead(t, fixtures.Guest, path, "")
	_, missing := world.logRead(t, fixtures.Guest, "/api/v1/logs/"+namespace+"/absent", "")
	hiddenFailure, missingFailure := fixtures.Failure(t, hidden), fixtures.Failure(t, missing)
	if hiddenFailure["code"] != missingFailure["code"] {
		t.Fatalf("invisible application reads %v, an absent one %v, want the same answer",
			hiddenFailure, missingFailure)
	}
}

func TestTASKTC12DisablingTheComponentWithdrawsThePageOnly(t *testing.T) {
	others := []extension.Request{fixtures.OtherComponent(t, "kos-notes", "notes", "记录")}

	enabled := extension.Evaluate(append(others, fixtures.OurPackage(t)))
	served := pages.New(enabled.Registry, pages.Config{})
	if !hasPage(served.List(fixtures.Owner), app.PageDetail) {
		t.Fatal("the enabled component's detail page is not listed")
	}
	if !hasPage(served.List(fixtures.Owner), "notes") {
		t.Fatal("the other component's page is not listed beside it")
	}

	disabled := fixtures.OurPackage(t)
	disabled.IsDisabled = true
	withdrawn := extension.Evaluate(append(others, disabled))
	shell := pages.New(withdrawn.Registry, pages.Config{Now: func() time.Time { return fixtures.Now }})
	if hasPage(shell.List(fixtures.Owner), app.PageDetail) {
		t.Fatal("the disabled component's detail page is still listed")
	}
	if _, failure := shell.Read(t.Context(), fixtures.Owner, app.PageDetail); failure == nil {
		t.Fatal("the disabled component's detail page is still readable")
	} else if failure.Code != controlplane.CodeNotFound {
		t.Fatalf("detail read after disable = %s, want the same answer as an unknown page", failure.Code)
	}
	// Another component's surface is unaffected by this component's disable
	// (KOSD-AC-54).
	if !hasPage(shell.List(fixtures.Owner), "notes") {
		t.Fatal("disabling this component withdrew another component's page")
	}
	// The disable is a declaration change: the object set the component reads
	// is untouched by it (KOSD-AC-54).
	world := newWorld(t)
	before := fixtures.Snapshot(t, world.cluster)
	if _, failure := shell.Read(t.Context(), fixtures.Owner, app.PageDetail); failure == nil {
		t.Fatal("the disabled detail page was readable")
	}
	if after := fixtures.Snapshot(t, world.cluster); !equalStrings(before, after) {
		t.Fatalf("the object set changed across the disable: %v then %v", before, after)
	}
}

// hasPage reports whether one page id is in a page list.
func hasPage(list []pages.Summary, id string) bool {
	for _, page := range list {
		if page.ID == id {
			return true
		}
	}
	return false
}

// namesOf reads the name field of a list of rows.
func namesOf(rows []map[string]any) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name, _ := row["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// linesOf reads the lines of a log read.
func linesOf(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["lines"].([]any)
	if !ok {
		t.Fatalf("log read = %v, want a line list", body)
	}
	lines := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		line, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("log line = %v, want an object", entry)
		}
		lines = append(lines, line)
	}
	return lines
}

// messagesOf reads the message field of a list of log lines.
func messagesOf(lines []map[string]any) []string {
	messages := make([]string, 0, len(lines))
	for _, line := range lines {
		message, _ := line["message"].(string)
		messages = append(messages, message)
	}
	sort.Strings(messages)
	return messages
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

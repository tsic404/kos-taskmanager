// Package fixtures boots the pieces the overview cases need: the packaged
// artifact installed into a throwaway cluster object set, the audit trail
// stand-in the maintenance origin is judged by, and the page shell that renders
// the overview for one session. Nothing here reaches a real cluster or database.
package fixtures

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	kosapp "github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/internal/artifact"
)

// Now is the read clock every case runs on.
var Now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// The accounts the overview is read as (KOSD-C-59).
var (
	Owner        = identity.Session{Username: "owner", Role: write.RoleOwner}
	OwningMember = identity.Session{Username: "lin", Role: write.RoleMember}
	NamedMember  = identity.Session{Username: "mei", Role: write.RoleMember}
	OtherMember  = identity.Session{Username: "kai", Role: write.RoleMember}
	Guest        = identity.Session{Username: "guest", Role: write.RoleGuest}
)

// Cluster builds a fake dynamic client holding the objects the cases shape. The
// scheme registers every kernel kind and the two native classes the page reads,
// so a source that lists one is served the stored JSON without a typed round
// trip.
func Cluster(t *testing.T) dynamic.Interface {
	t.Helper()
	scheme := runtime.NewScheme()
	lists := map[schema.GroupVersionResource]string{}
	add := func(gvk schema.GroupVersionKind, resource string) {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
		lists[gvk.GroupVersion().WithResource(resource)] = gvk.Kind + "List"
	}
	add(schema.GroupVersionKind{Version: "v1", Kind: "Node"}, "nodes")
	add(schema.GroupVersionKind{Version: "v1", Kind: "Event"}, "events")
	// Restart and the other one-shot actions travel as ConfigMap records in the
	// KOS namespace, so the fake cluster serves that class too (KOSD-C-33).
	add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, "configmaps")
	// A log read resolves the application's rendered workloads and their Pods,
	// so the classes KOSD-C-64 reads through are served here (KOSD-C-76).
	add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, "deployments")
	add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "pods")
	for _, kind := range api.Kinds() {
		resource, ok := api.ResourceName(kind)
		if !ok {
			t.Fatalf("kernel kind %q has no resource name", kind)
		}
		add(schema.GroupVersionKind{
			Group: api.Group, Version: v1beta1.GroupVersion.Version, Kind: kind,
		}, resource)
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, lists)
}

// Node writes one node with the status the overview reads.
func Node(t *testing.T, cluster dynamic.Interface, name string, ready, unschedulable bool) {
	t.Helper()
	condition := string(corev1.ConditionFalse)
	if ready {
		condition = string(corev1.ConditionTrue)
	}
	create(t, cluster, map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"unschedulable": unschedulable},
		"status": map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": condition}},
		},
	})
}

// ApplicationOptions is one application as a case shapes it.
type ApplicationOptions struct {
	// Namespace is the object's namespace.
	Namespace string
	// Name is the object's name.
	Name string
	// Owner is the local username spec.owner carries (KOSD-C-60).
	Owner string
	// Members are the usernames spec.visibility.members names (KOSD-C-59).
	Members []string
	// AllowGuests is spec.visibility.allowGuests.
	AllowGuests bool
	// NamespaceOverride is spec.namespace, the namespace the render writes in.
	NamespaceOverride string
	// DefinitionRef names the application definition the application came from.
	DefinitionRef string
	// Volumes are the volume object names spec.volumes references.
	Volumes []string
	// Phase is status.phase; empty reads as Ready.
	Phase string
	// Health is status.health.state; empty reads as Healthy.
	Health string
	// Conditions are the failure conditions status.conditions carries.
	Conditions []metav1.Condition
	// AppliedDigest is the package digest status.applied records.
	AppliedDigest string
	// UpgradeTarget is status.upgrade.target, when an upgrade is requested.
	UpgradeTarget string
	// UpgradeRolledBack marks a requested upgrade the preflight rolled back.
	UpgradeRolledBack bool
	// UID is metadata.uid; a one-shot action record is named after it.
	UID string
	// Replicas is spec.replicas; nil leaves the count to the package.
	Replicas *int32
	// Paused is the stop marker spec.paused (KOSD-C-66).
	Paused bool
	// Resources is the CPU and memory specification spec.resources carries
	// (KOSD-C-67).
	Resources *v1beta1.ResourceRequirements
	// ResourceStatus is the projection status.resources carries (KOSD-C-67).
	ResourceStatus *v1beta1.ResourceStatus
	// Finalizers are metadata.finalizers; the uninstall gate is one of them.
	Finalizers []string
	// Deleting marks an object the API server is removing: metadata carries a
	// deletion timestamp and the uninstall gate keeps it observable (KOSD-C-69).
	Deleting  bool
	Uninstall *v1beta1.UninstallStatus
	// Labels are metadata.labels, which a backup policy's selector judges
	// (KOSD-C-41).
	Labels map[string]string
	// DriftPolicy is spec.driftPolicy, the arbitration the external-change
	// record answers (KOSD-C-35).
	DriftPolicy v1beta1.DriftPolicy
	// Drift is status.drift: the downstream values an external write changed
	// (KOSD-C-35).
	Drift []v1beta1.DriftEntry
	// Rendered is status.rendered: the objects the last applied render wrote,
	// which a log read resolves the application's Pods through (KOSD-C-33).
	Rendered []v1beta1.ObjectRef
}

// Application writes one application object.
func Application(t *testing.T, cluster dynamic.Interface, options ApplicationOptions) {
	t.Helper()
	phase, health := options.Phase, options.Health
	if phase == "" {
		phase = "Ready"
	}
	if health == "" {
		health = string(v1beta1.HealthStateHealthy)
	}
	conditions := make([]any, 0, len(options.Conditions))
	for _, condition := range options.Conditions {
		conditions = append(conditions, documentOf(t, condition))
	}
	members := make([]any, 0, len(options.Members))
	for _, member := range options.Members {
		members = append(members, member)
	}
	volumes := make([]any, 0, len(options.Volumes))
	for _, volume := range options.Volumes {
		volumes = append(volumes, volume)
	}
	status := map[string]any{
		"phase":      phase,
		"health":     map[string]any{"state": health},
		"conditions": conditions,
	}
	if options.AppliedDigest != "" {
		status["applied"] = map[string]any{"definitionDigest": options.AppliedDigest}
	}
	if options.UpgradeTarget != "" {
		status["upgrade"] = map[string]any{
			"target": options.UpgradeTarget, "rolledBack": options.UpgradeRolledBack,
		}
	}
	if options.ResourceStatus != nil {
		status["resources"] = documentOf(t, options.ResourceStatus)
	}
	if options.Uninstall != nil {
		status["uninstall"] = documentOf(t, options.Uninstall)
	}
	if options.Drift != nil {
		entries := make([]any, 0, len(options.Drift))
		for _, entry := range options.Drift {
			entries = append(entries, documentOf(t, entry))
		}
		status["drift"] = entries
	}
	if options.Rendered != nil {
		rendered := make([]any, 0, len(options.Rendered))
		for _, ref := range options.Rendered {
			rendered = append(rendered, documentOf(t, ref))
		}
		status["rendered"] = rendered
	}
	spec := map[string]any{
		"owner":         options.Owner,
		"visibility":    map[string]any{"members": members, "allowGuests": options.AllowGuests},
		"volumes":       volumes,
		"definitionRef": options.DefinitionRef,
	}
	if options.NamespaceOverride != "" {
		spec["namespace"] = options.NamespaceOverride
	}
	if options.Replicas != nil {
		spec["replicas"] = *options.Replicas
	}
	if options.Paused {
		spec["paused"] = true
	}
	if options.Resources != nil {
		spec["resources"] = documentOf(t, options.Resources)
	}
	if options.DriftPolicy != "" {
		spec["driftPolicy"] = string(options.DriftPolicy)
	}
	metadata := map[string]any{"namespace": options.Namespace, "name": options.Name}
	if options.UID != "" {
		metadata["uid"] = options.UID
	}
	if len(options.Finalizers) > 0 {
		finalizers := make([]any, 0, len(options.Finalizers))
		for _, finalizer := range options.Finalizers {
			finalizers = append(finalizers, finalizer)
		}
		metadata["finalizers"] = finalizers
	}
	if options.Deleting {
		metadata["deletionTimestamp"] = Now.UTC().Format(time.RFC3339)
	}
	if len(options.Labels) > 0 {
		labels := make(map[string]any, len(options.Labels))
		for key, value := range options.Labels {
			labels[key] = value
		}
		metadata["labels"] = labels
	}
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "Application",
		"metadata":   metadata,
		"spec":       spec,
		"status":     status,
	})
}

// Definition writes one application definition offering a package.
func Definition(t *testing.T, cluster dynamic.Interface, name, digest, version string) {
	t.Helper()
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "ApplicationDefinition",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"package": map[string]any{"digest": digest, "version": version}},
	})
}

// PublishDefinition republishes one catalogue entry with a new package: a first
// publish creates it and a later one replaces the version it offers (KOSD-C-32,
// APPSTORE-C-31).
func PublishDefinition(t *testing.T, cluster dynamic.Interface, name, digest, version string) {
	t.Helper()
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "ApplicationDefinition",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"package": map[string]any{"digest": digest, "version": version}},
	}}
	resource := cluster.Resource(schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: "applicationdefinitions",
	})
	if _, err := resource.Create(context.Background(), object, metav1.CreateOptions{}); err == nil {
		return
	} else if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("publish definition %q: %v", name, err)
	}
	current, err := resource.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read definition %q: %v", name, err)
	}
	updated := current.DeepCopy()
	if err := unstructured.SetNestedMap(updated.Object, map[string]any{"package": map[string]any{"digest": digest, "version": version}}, "spec"); err != nil {
		t.Fatalf("shape definition %q: %v", name, err)
	}
	if _, err := resource.Update(context.Background(), updated, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("republish definition %q: %v", name, err)
	}
}

// ValidDefinition publishes one definition the kernel's own validation accepts:
// the package digest is computed from the content and the translation template
// is present (KOSD-C-32). A case that writes an application needs one, because
// the write validates the definition the application names.
func ValidDefinition(t *testing.T, cluster dynamic.Interface, name, version, template string) {
	t.Helper()
	spec := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: version},
		Template: template,
	}
	digest, err := kosapp.DefinitionDigest(spec)
	if err != nil {
		t.Fatalf("digest definition %q: %v", name, err)
	}
	spec.Package.Digest = digest
	if err := kosapp.ValidateDefinition(spec); err != nil {
		t.Fatalf("definition %q is invalid: %v", name, err)
	}
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "ApplicationDefinition",
		"metadata":   map[string]any{"name": name},
		"spec":       documentOf(t, spec),
	})
}

// DefinitionObject reads one catalogue entry.
func DefinitionObject(t *testing.T, cluster dynamic.Interface, name string) *unstructured.Unstructured {
	t.Helper()
	object, err := cluster.Resource(schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: "applicationdefinitions",
	}).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read definition %q: %v", name, err)
	}
	return object
}

// ApplicationObject reads one stored application.
func ApplicationObject(t *testing.T, cluster dynamic.Interface, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	object, err := cluster.Resource(schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: "applications",
	}).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read application %s/%s: %v", namespace, name, err)
	}
	return object
}

// Update writes one stored object back, which is how a case shapes the state a
// reconciler or the API server would leave behind.
func Update(t *testing.T, cluster dynamic.Interface, object *unstructured.Unstructured) {
	t.Helper()
	resource, err := resourceOf(object)
	if err != nil {
		t.Fatalf("resolve %s: %v", object.GetKind(), err)
	}
	if _, err := cluster.Resource(resource).Namespace(object.GetNamespace()).Update(
		context.Background(), object, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update %s/%s: %v", object.GetKind(), object.GetName(), err)
	}
}

// BeginUninstall shapes the state the API server and the application reconciler
// produce while the uninstall gate holds one application: a deletion timestamp,
// the gate finalizer and the Uninstalling phase with its blocked objects
// (KOSD-C-33, KOSD-C-69). The fake cluster removes a deleted object at once, so
// a case stages the gate the real server keeps observable.
func BeginUninstall(t *testing.T, cluster dynamic.Interface, namespace, name string) {
	t.Helper()
	application := ApplicationObject(t, cluster, namespace, name)
	application.SetDeletionTimestamp(&metav1.Time{Time: Now})
	application.SetFinalizers(append(application.GetFinalizers(), kosapp.Finalizer))
	if err := unstructured.SetNestedField(application.Object, string(kosapp.PhaseUninstalling), "status", "phase"); err != nil {
		t.Fatalf("set the uninstalling phase: %v", err)
	}
	if err := unstructured.SetNestedField(application.Object, string(v1beta1.HealthStateUnavailable), "status", "health", "state"); err != nil {
		t.Fatalf("set the uninstalling health: %v", err)
	}
	blocked := []any{map[string]any{
		"group": "apps", "version": "v1", "kind": "Deployment",
		"namespace": namespace, "name": name,
	}}
	if err := unstructured.SetNestedSlice(application.Object, blocked, "status", "uninstall", "blocked"); err != nil {
		t.Fatalf("set the uninstall blocked list: %v", err)
	}
	Update(t, cluster, application)
}

// UsageSamples is a metric source stand-in: the samples the case configured, or
// the failure that disconnects it (TASK-C-2).
type UsageSamples struct {
	// Samples are the rows the source answers with.
	Samples []app.ApplicationUsage
	// Err makes every read fail, the way a disconnected metric source does.
	Err error
}

// Usage implements app.UsageSource.
func (s UsageSamples) Usage(context.Context, identity.Session) ([]app.ApplicationUsage, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Samples, nil
}

// Failure reads the stable error envelope a control-plane write answers with
// (KOSD-C-12): the body carries it under "error".
func Failure(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	failure, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("the answer %v carries no error envelope", body)
	}
	return failure
}

// Volume writes one volume, bound to node when the mobility is node-local.
func Volume(t *testing.T, cluster dynamic.Interface, namespace, name string, mobility v1beta1.VolumeMobility, node string) {
	t.Helper()
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "Volume",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"mobility": string(mobility), "capacity": "1Gi"},
		"status":     map[string]any{"node": node},
	})
}

// Endpoint writes one endpoint exposing an application.
func Endpoint(t *testing.T, cluster dynamic.Interface, namespace, name, targetApp, host string) {
	t.Helper()
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "Endpoint",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"kind": "http.host", "targetApp": targetApp, "host": host},
		"status":     map[string]any{"phase": "Ready", "health": map[string]any{"state": "Healthy"}},
	})
}

// BackupPolicyOptions is one backup policy as a case shapes it.
type BackupPolicyOptions struct {
	// Namespace is the policy's namespace.
	Namespace string
	// Name is the policy's name.
	Name string
	// Selector is the label selector that decides the backup range
	// (KOSD-C-41).
	Selector string
}

// BackupPolicy writes one backup policy over the range its selector covers.
func BackupPolicy(t *testing.T, cluster dynamic.Interface, options BackupPolicyOptions) {
	t.Helper()
	create(t, cluster, map[string]any{
		"apiVersion": api.Group + "/" + v1beta1.GroupVersion.Version,
		"kind":       "BackupPolicy",
		"metadata":   map[string]any{"namespace": options.Namespace, "name": options.Name},
		"spec": map[string]any{
			"selector": options.Selector, "schedule": "0 3 * * *",
			"consistency": "crash-consistent", "retention": map[string]any{"keep": int64(7)},
			"drill": map[string]any{"enabled": true},
		},
		"status": map[string]any{"phase": "Ready", "health": map[string]any{"state": "Healthy"}},
	})
}

// EventOptions is one cluster event as a case shapes it.
type EventOptions struct {
	// Name is the event's name.
	Name string
	// Namespace is where the event is stored.
	Namespace string
	// SubjectGroup is the involved object's API group; empty for a core object.
	SubjectGroup string
	// SubjectKind is the involved object's kind.
	SubjectKind string
	// SubjectNamespace is the involved object's namespace.
	SubjectNamespace string
	// SubjectName is the involved object's name.
	SubjectName string
	// Reason is the short cause.
	Reason string
	// Message is the human-readable detail.
	Message string
	// LastSeen is when the event last occurred.
	LastSeen time.Time
}

// Workload writes one Deployment whose selector matches the Pods a log read
// resolves through it (KOSD-C-64).
func Workload(t *testing.T, cluster dynamic.Interface, namespace, name string, selector map[string]string) {
	t.Helper()
	jsonSelector := make(map[string]any, len(selector))
	for key, value := range selector {
		jsonSelector[key] = value
	}
	create(t, cluster, map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec": map[string]any{
			"selector": map[string]any{"matchLabels": jsonSelector},
			"template": map[string]any{"metadata": map[string]any{"labels": jsonSelector}},
		},
	})
}

// Pod writes one Pod with the containers a log read collects (KOSD-C-76).
func Pod(t *testing.T, cluster dynamic.Interface, namespace, name string, labels map[string]string, containers []string) {
	t.Helper()
	declared := make([]any, 0, len(containers))
	for _, container := range containers {
		declared = append(declared, map[string]any{"name": container, "image": "busybox"})
	}
	jsonLabels := make(map[string]any, len(labels))
	for key, value := range labels {
		jsonLabels[key] = value
	}
	create(t, cluster, map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": namespace, "name": name, "labels": jsonLabels},
		"spec":       map[string]any{"containers": declared},
	})
}

// Event writes one cluster event.
func Event(t *testing.T, cluster dynamic.Interface, options EventOptions) {
	t.Helper()
	apiVersion := "v1"
	if options.SubjectGroup != "" {
		apiVersion = options.SubjectGroup + "/" + v1beta1.GroupVersion.Version
	}
	create(t, cluster, map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata":   map[string]any{"namespace": options.Namespace, "name": options.Name},
		"involvedObject": map[string]any{
			"apiVersion": apiVersion,
			"kind":       options.SubjectKind,
			"namespace":  options.SubjectNamespace,
			"name":       options.SubjectName,
		},
		"type":          "Warning",
		"reason":        options.Reason,
		"message":       options.Message,
		"count":         int64(1),
		"lastTimestamp": options.LastSeen.UTC().Format(time.RFC3339),
	})
}

// Trail is the audit store stand-in (KOSD-C-14). Every record is kept per
// object; a read filters by the verbs and outcomes it asks for and returns the
// newest first, which is how the origin rule reads a node's own history.
type Trail struct {
	entries map[string][]identity.AuditEntry
	// Err makes every read fail the way a stopped database does (KOSD-C-82).
	Err error
}

// NewTrail returns an empty trail.
func NewTrail() *Trail { return &Trail{entries: map[string][]identity.AuditEntry{}} }

// Record appends one maintenance record for a node.
func (t *Trail) Record(object, verb, result string, at time.Time) {
	t.entries[object] = append(t.entries[object], identity.AuditEntry{
		Actor: "owner", Object: object, Verb: verb, Result: result, OccurredAt: at,
	})
}

// RecentAuditEntriesForObject implements app.AuditReader.
func (t *Trail) RecentAuditEntriesForObject(_ context.Context, object string, verbs, results []string, limit int) ([]identity.AuditEntry, error) {
	if t.Err != nil {
		return nil, t.Err
	}
	matched := make([]identity.AuditEntry, 0)
	for _, entry := range t.entries[object] {
		if !contains(verbs, entry.Verb) || !contains(results, entry.Result) {
			continue
		}
		matched = append(matched, entry)
	}
	sort.SliceStable(matched, func(left, right int) bool {
		return matched[left].OccurredAt.After(matched[right].OccurredAt)
	})
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// Shell is one installed product's page projection over one object set.
type Shell struct {
	// Cluster is the object set the page sources read.
	Cluster dynamic.Interface
	// Trail is the audit stand-in the origin item reads.
	Trail *Trail
	// Registry is the contribution projection the installed package produces.
	Registry *extension.Registry
	// Pages is the shell that renders the declared pages (KOSD-C-53).
	Pages *pages.Service
	// API is the page read interface over HTTP (KOSD-C-12).
	API http.Handler
}

// NewShell installs the packaged artifact and wires its reader into the page
// shell. The package the cases install is the artifact this repository builds,
// so the page the shell serves is the page the package declares.
func NewShell(t *testing.T, cluster dynamic.Interface, trail *Trail) *Shell {
	t.Helper()
	return NewShellAt(t, cluster, trail, Now)
}

// NewShellAt is NewShell with the shell's read clock moved, which is how the
// freshness bound of KOSD-C-13 is exercised: the observation stays at Now.
func NewShellAt(t *testing.T, cluster dynamic.Interface, trail *Trail, readAt time.Time) *Shell {
	t.Helper()
	registry := install(t)
	readings := &app.Readings{Dynamic: cluster, Audit: trail, Now: func() time.Time { return Now }}
	service := pages.New(registry, pages.Config{
		Readings: pages.Router{app.Identifier: readings},
		Now:      func() time.Time { return readAt },
	})
	return &Shell{
		Cluster: cluster, Trail: trail, Registry: registry,
		Pages: service, API: pages.NewAPI(service).Handler(),
	}
}

// OurPackage is this repository's install request (KOSD-C-17).
func OurPackage(t *testing.T) extension.Request {
	t.Helper()
	return product(t)
}

// product is this repository's install request.
func product(t *testing.T) extension.Request {
	t.Helper()
	pkg, archive, err := artifact.Build(artifact.Options{Root: repoRoot(t)})
	if err != nil {
		t.Fatalf("build the artifact: %v", err)
	}
	return extension.Request{
		Name:    app.Identifier,
		Created: Now,
		Package: &extension.Package{
			Reference: app.Identifier + "-package",
			Digest:    pkg.Digest.String(),
			Content:   archive,
		},
	}
}

// install evaluates the install request with the kernel's own rules and returns
// the contribution projection it produces (KOSD-C-17, KOSD-C-28).
func install(t *testing.T) *extension.Registry {
	t.Helper()
	return extension.Evaluate([]extension.Request{product(t)}).Registry
}

// repoRoot resolves the repository root from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	return root
}

// ReadPage reads the overview as one session.
func ReadPage(t *testing.T, shell *Shell, session identity.Session) pages.Page {
	t.Helper()
	page, failure := shell.Pages.Read(context.Background(), session, app.PageOverview)
	if failure != nil {
		t.Fatalf("read the overview as %q: %s (%s)", session.Username, failure.Message, failure.Code)
	}
	return page
}

// ItemsByID indexes a rendered page's items.
func ItemsByID(page pages.Page) map[string]pages.Item {
	indexed := make(map[string]pages.Item, len(page.Items))
	for _, item := range page.Items {
		indexed[item.ID] = item
	}
	return indexed
}

// ValueOf reads one item's value as the JSON document a page consumer sees; the
// item must exist and carry a value.
func ValueOf(t *testing.T, page pages.Page, id string) map[string]any {
	t.Helper()
	item, found := ItemsByID(page)[id]
	if !found {
		t.Fatalf("the page does not carry item %q", id)
	}
	value, ok := documentOf(t, item.Value).(map[string]any)
	if !ok {
		t.Fatalf("item %q value = %#v, want an object", id, item.Value)
	}
	return value
}

// ObjectsOf reads one list field of an item value.
func ObjectsOf(t *testing.T, value map[string]any, field string) []map[string]any {
	t.Helper()
	raw, ok := value[field].([]any)
	if !ok {
		t.Fatalf("field %q = %#v, want a list", field, value[field])
	}
	objects := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		object, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("field %q entry = %#v, want an object", field, entry)
		}
		objects = append(objects, object)
	}
	return objects
}

// Snapshot renders every stored object, ordered, so a case can prove two
// requests left the object set unchanged.
func Snapshot(t *testing.T, cluster dynamic.Interface) []string {
	t.Helper()
	classes := []schema.GroupVersionResource{
		{Version: "v1", Resource: "nodes"},
		{Version: "v1", Resource: "events"},
	}
	for _, kind := range api.Kinds() {
		resource, _ := api.ResourceName(kind)
		classes = append(classes, schema.GroupVersionResource{
			Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource,
		})
	}
	lines := make([]string, 0)
	for _, class := range classes {
		list, err := cluster.Resource(class).Namespace("").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			encoded, err := json.Marshal(item.Object)
			if err != nil {
				t.Fatalf("encode %s/%s: %v", item.GetKind(), item.GetName(), err)
			}
			lines = append(lines, string(encoded))
		}
	}
	sort.Strings(lines)
	return lines
}

// PageJSON renders a page as the consumer receives it.
func PageJSON(t *testing.T, page pages.Page) string {
	t.Helper()
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("encode the page: %v", err)
	}
	return string(encoded)
}

// ObjectRefOf is the audit object name the origin rule asks for.
func ObjectRefOf(node string) string { return api.Group + "/Node/" + node }

func create(t *testing.T, cluster dynamic.Interface, object map[string]any) {
	t.Helper()
	stored := &unstructured.Unstructured{Object: object}
	// The API server assigns a resource version to every stored object; the fake
	// tracker does not, so the fixture assigns the one a precondition compares
	// against (KOSD-C-12).
	if stored.GetResourceVersion() == "" {
		stored.SetResourceVersion("1")
	}
	resource, err := resourceOf(stored)
	if err != nil {
		t.Fatalf("resolve %s: %v", stored.GetKind(), err)
	}
	_, err = cluster.Resource(resource).Namespace(stored.GetNamespace()).Create(
		context.Background(), stored, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create %s/%s: %v", stored.GetKind(), stored.GetName(), err)
	}
}

func resourceOf(object *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	switch object.GetKind() {
	case "Node":
		return schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, nil
	case "Event":
		return schema.GroupVersionResource{Version: "v1", Resource: "events"}, nil
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case "Pod":
		return schema.GroupVersionResource{Version: "v1", Resource: "pods"}, nil
	default:
		resource, _ := api.ResourceName(object.GetKind())
		return schema.GroupVersionResource{
			Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: resource,
		}, nil
	}
}

func documentOf(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %#v: %v", value, err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode %#v: %v", value, err)
	}
	return document
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

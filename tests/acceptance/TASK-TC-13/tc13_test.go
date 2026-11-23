// Package tasktc13 checks TASK-TC-13: a crashed provider leaves an installed
// application Degraded without reinstalling it; the application page offers a
// reinstall entry the control plane accepts, and the entry is offered to the
// roles KOSD-C-59 gives it; the reinstall is executed by the application
// reconciler, which consumes the action record and applies the current spec
// again, while the user spec, the existing volumes and the existing endpoints
// keep their values and identities and their status may still move
// (KOSD-C-56, KOSD-C-03, KOSD-C-07).
package tasktc13

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	kosapp "github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/reconcile"
	"github.com/tsic404/kosd/pkg/write"

	taskapp "github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the dependent application lives.
const namespace = "home"

// owner is the account that owns the application: the owning Member.
const owner = "lin"

// The image the definition renders and the value the case drifts it to. The
// reconciler writing the rendered value back is what makes "re-applied from the
// current spec" observable (KOSD-C-56).
const (
	renderedImage = "busybox"
	driftedImage  = "drifted"
)

// deploymentTemplate renders one Deployment, so a replay is a real downstream
// write and health comes from observed replicas.
const deploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Name }}
  template:
    metadata:
      labels:
        app: {{ .Name }}
    spec:
      containers:
        - name: app
          image: ` + renderedImage + `
`

// crashedProvider reports the capability whose provider has crashed. The
// definition requires it, so the reconciler keeps the application Degraded
// instead of reinstalling it on its own (KOSD-C-56).
type crashedProvider struct{}

func (crashedProvider) UnavailableCapabilities(context.Context) ([]string, error) {
	return []string{"storage.volume"}, nil
}

// newWorld installs the package over one Ready application whose provider
// crashed, carrying a volume and an endpoint the reinstall must keep, and
// returns the product surface the page cases read plus the real application
// reconciler over the same object set.
func newWorld(t *testing.T) (*fixtures.Product, *kosapp.Controller) {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Namespace(t, cluster, namespace)
	createDefinition(t, cluster)
	fixtures.Volume(t, cluster, namespace, "data", v1beta1.VolumeMobilityClusterMovable, "")
	fixtures.Endpoint(t, cluster, namespace, "shop-web", "shop", "shop.test")
	// The workload the completed install rendered already exists, so the
	// reinstall re-applies it rather than reporting a first-time creation.
	createRenderedWorkload(t, cluster)
	volume := fixtures.Object(t, cluster, "Volume", namespace, "data")
	volume.SetUID("uid-volume")
	fixtures.Update(t, cluster, volume)
	endpoint := fixtures.Object(t, cluster, "Endpoint", namespace, "shop-web")
	endpoint.SetUID("uid-endpoint")
	fixtures.Update(t, cluster, endpoint)
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: owner,
		Members: []string{"mei"}, AllowGuests: true,
		DefinitionRef: "shop", Volumes: []string{"data"}, UID: "uid-shop",
		NamespaceOverride: namespace,
		Health:            string(v1beta1.HealthStateDegraded),
		Conditions: []metav1.Condition{{
			Type: "CapabilityUnsatisfied", Status: metav1.ConditionTrue,
			Reason: "CapabilityUnsatisfied", Message: "Provider storage.volume 不可用",
		}},
		Labels: map[string]string{"managed-by": "kosd"},
	})
	product := fixtures.NewProduct(t, cluster, fixtures.NewTrail(), nil)
	controller := kosapp.NewController(kosapp.NewWriterStore(&write.Writer{
		Dynamic: fixtures.ApplyAdapter(cluster), Resources: fixtures.ClusterResources(),
	}), kosapp.Options{
		Capabilities: kosapp.StaticCapabilities{{
			Name: "storage.volume", Version: "v1",
			Scope: v1beta1.CapabilityScopeCluster, Verified: true,
		}},
		Providers: crashedProvider{},
		Clock:     func() time.Time { return fixtures.Now },
	})
	return product, controller
}

// createDefinition writes a definition whose package digest matches its content
// and whose template renders the workload the case replays.
func createDefinition(t *testing.T, cluster dynamic.Interface) {
	t.Helper()
	definition := &v1beta1.ApplicationDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: api.Group + "/v1beta1", Kind: "ApplicationDefinition"},
		ObjectMeta: metav1.ObjectMeta{Name: "shop"},
		Spec: v1beta1.ApplicationDefinitionSpec{
			Package:  v1beta1.PackageIdentity{Version: "1.0.0"},
			Template: deploymentTemplate,
			Requires: []v1beta1.CapabilityRequirement{{Name: "storage.volume", Version: "v1"}},
		},
	}
	digest, err := kosapp.DefinitionDigest(definition.Spec)
	if err != nil {
		t.Fatalf("digest the definition: %v", err)
	}
	definition.Spec.Package.Digest = digest
	if err := kosapp.ValidateDefinition(definition.Spec); err != nil {
		t.Fatalf("the definition is invalid: %v", err)
	}
	document, err := runtime.DefaultUnstructuredConverter.ToUnstructured(definition)
	if err != nil {
		t.Fatalf("encode the definition: %v", err)
	}
	resource := cluster.Resource(schema.GroupVersionResource{
		Group: api.Group, Version: "v1beta1", Resource: "applicationdefinitions",
	})
	if _, err := resource.Create(context.Background(), &unstructured.Unstructured{Object: document}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create the definition: %v", err)
	}
}

// createRenderedWorkload writes the Deployment the install rendered, carrying
// the attribution markers the reconciler writes over and the observed replicas
// health reads (KOSD-C-02, KOSD-C-08).
func createRenderedWorkload(t *testing.T, cluster dynamic.Interface) {
	t.Helper()
	workload := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"namespace": namespace, "name": "shop",
			"labels": map[string]any{"managed-by": "kosd", "owner-uid": "uid-shop"},
		},
		"spec": map[string]any{
			"replicas": int64(1),
			"selector": map[string]any{"matchLabels": map[string]any{"app": "shop"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": "shop"}},
				"spec": map[string]any{"containers": []any{
					map[string]any{"name": "app", "image": renderedImage},
				}},
			},
		},
		"status": map[string]any{"replicas": int64(1), "readyReplicas": int64(1)},
	}}
	resource := cluster.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	if _, err := resource.Namespace(namespace).Create(context.Background(), workload, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create the rendered workload: %v", err)
	}
}

// round runs one application reconciliation, so the action the page submitted
// is executed before the case asserts on its effects (KOSD-C-06).
func round(t *testing.T, controller *kosapp.Controller) {
	t.Helper()
	if _, err := controller.Reconcile(context.Background(), reconcile.Request{
		Kind: "Application", Namespace: namespace, Name: "shop",
	}); err != nil {
		t.Fatalf("reconcile the application: %v", err)
	}
}

// targetNames reads one entry item's application rows.
func targetNames(t *testing.T, product *fixtures.Product, session identity.Session, item string) []string {
	t.Helper()
	page := product.ReadPage(t, session, taskapp.PageApplications)
	rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, item), "applications")
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row["name"].(string))
	}
	return names
}

func TestTASKTC13TheReinstallEntrySubmitsTheCurrentSpec(t *testing.T) {
	product, controller := newWorld(t)

	// The entry is offered exactly to the identities KOSD-C-59 gives write
	// access to the application, and the target list is empty for the others.
	for _, reader := range []struct {
		session identity.Session
		offers  bool
	}{
		{session: fixtures.Owner, offers: true},
		{session: fixtures.OwningMember, offers: true},
		{session: fixtures.NamedMember, offers: false},
		{session: fixtures.Guest, offers: false},
	} {
		names := targetNames(t, product, reader.session, taskapp.ItemReinstall)
		if reader.offers && (len(names) != 1 || names[0] != "shop") {
			t.Fatalf("%s: reinstall targets = %v, want the owned application",
				reader.session.Username, names)
		}
		if !reader.offers && len(names) != 0 {
			t.Fatalf("%s: reinstall targets = %v, want none", reader.session.Username, names)
		}
	}

	// One reconciliation of a Ready application with its provider down leaves
	// it Ready but Degraded and consumes no action: nothing reinstalls it on its
	// own (KOSD-C-56).
	round(t, controller)
	if got := healthOf(t, product, "shop"); got != string(v1beta1.HealthStateDegraded) {
		t.Fatalf("health with the provider down = %s, want Degraded", got)
	}
	if phase := phaseOf(t, product); phase != kosapp.PhaseReady {
		t.Fatalf("phase with the provider down = %s, want Ready", phase)
	}
	if record := actionRecord(t, product); record != "" {
		t.Fatalf("the application was reinstalled on its own: record %s exists", record)
	}

	// The user spec, the volume and the endpoint are the values and identities
	// the reinstall must keep (KOSD-C-56, KOSD-C-03).
	beforeSpec := specOf(t, product)
	beforeVolume := fixtures.Object(t, product.Cluster, "Volume", namespace, "data").GetUID()
	beforeEndpoint := fixtures.Object(t, product.Cluster, "Endpoint", namespace, "shop-web").GetUID()

	// Drift the live workload so the replay is observable: only a round that
	// really renders and applies the current spec writes the value back.
	driftImage(t, product)

	// The submission comes from the page declaration, never from a value this
	// case invents (KOSD-C-12, KOSD-C-22).
	method, path := fixtures.PageAction(t, product, taskapp.ActionReinstall, "shop", namespace)
	if method != http.MethodPost {
		t.Fatalf("reinstall entry = %s %s, want the declared POST", method, path)
	}
	value, known := taskapp.ControlPlaneAction(taskapp.ActionReinstall)
	if !known {
		t.Fatalf("the reinstall entry names no control-plane action")
	}
	status, answer := product.Call(t, fixtures.OwningMember, method, path, map[string]any{
		"kind": "Application", "namespace": namespace, "name": "shop",
		"action": value,
	}, nil)
	if status != http.StatusAccepted {
		t.Fatalf("reinstall = %d %v, want 202", status, answer)
	}
	if accepted, _ := answer["accepted"].(bool); !accepted {
		t.Fatalf("reinstall answer = %v, want an accepted action", answer)
	}
	record := actionRecord(t, product)
	if record == "" {
		t.Fatalf("the accepted reinstall carries no action record: %v", answer)
	}

	// The reconciler consumes the record and applies the current spec again.
	round(t, controller)
	if leftover := recordNamed(t, product, record); leftover != nil {
		t.Fatalf("the reinstall record %s was not consumed", record)
	}
	if !reinstallRequested(t, product) {
		t.Fatalf("the reinstall produced no %s event", kosapp.EventReinstallRequested)
	}
	if image := workloadImage(t, product); image != renderedImage {
		t.Fatalf("the reinstall did not re-apply the current spec: image = %q, want %q", image, renderedImage)
	}

	// The user spec keeps its values and the volume and the endpoint keep their
	// identities; only status, conditions and observations may move
	// (KOSD-C-56, KOSD-C-03, KOSD-C-07).
	if afterSpec := specOf(t, product); !reflect.DeepEqual(beforeSpec, afterSpec) {
		t.Fatalf("the reinstall changed the user spec:\nbefore %v\nafter  %v", beforeSpec, afterSpec)
	}
	if uid := fixtures.Object(t, product.Cluster, "Volume", namespace, "data").GetUID(); uid != beforeVolume {
		t.Fatalf("the reinstall recreated the volume: uid %s became %s", beforeVolume, uid)
	}
	if uid := fixtures.Object(t, product.Cluster, "Endpoint", namespace, "shop-web").GetUID(); uid != beforeEndpoint {
		t.Fatalf("the reinstall recreated the endpoint: uid %s became %s", beforeEndpoint, uid)
	}
	if got := healthOf(t, product, "shop"); got != string(v1beta1.HealthStateDegraded) {
		t.Fatalf("health after the reinstall = %s, want Degraded while the provider is down", got)
	}
	if phase := phaseOf(t, product); phase != kosapp.PhaseReady {
		t.Fatalf("phase after the reinstall = %s, want Ready", phase)
	}

	// The control plane accepts the value the entry declares, so the page never
	// offers a submission the endpoint refuses (KOSD-C-66).
	if !known || !controlplane.AcceptsAction(value) {
		t.Fatalf("the reinstall entry names %q, which the endpoint does not accept", value)
	}
}

// specOf reads the stored application spec.
func specOf(t *testing.T, product *fixtures.Product) map[string]any {
	t.Helper()
	spec, found, err := unstructured.NestedMap(
		fixtures.ApplicationObject(t, product.Cluster, namespace, "shop").Object, "spec")
	if err != nil || !found {
		t.Fatalf("read the application spec: found = %v, err = %v", found, err)
	}
	return spec
}

// phaseOf reads the stored application phase.
func phaseOf(t *testing.T, product *fixtures.Product) string {
	t.Helper()
	phase, found, err := unstructured.NestedString(
		fixtures.ApplicationObject(t, product.Cluster, namespace, "shop").Object, "status", "phase")
	if err != nil || !found {
		t.Fatalf("read the application phase: found = %v, err = %v", found, err)
	}
	return phase
}

// healthOf reads one stored application's health state.
func healthOf(t *testing.T, product *fixtures.Product, name string) string {
	t.Helper()
	health, found, err := unstructured.NestedString(
		fixtures.ApplicationObject(t, product.Cluster, namespace, name).Object, "status", "health", "state")
	if err != nil || !found {
		t.Fatalf("read application %s health: found = %v, err = %v", name, found, err)
	}
	return health
}

// actionRecord reads the one-shot record of this application, if any.
func actionRecord(t *testing.T, product *fixtures.Product) string {
	t.Helper()
	records, err := product.Cluster.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).
		Namespace(api.SystemNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: kosapp.ActionOwnerLabel + "=uid-shop",
	})
	if err != nil {
		t.Fatalf("list the action records: %v", err)
	}
	for _, record := range records.Items {
		if record.GetLabels()[write.ActionRecordLabel] != "" {
			return record.GetName()
		}
	}
	return ""
}

// recordNamed reads one action record; nil when it is gone.
func recordNamed(t *testing.T, product *fixtures.Product, name string) *unstructured.Unstructured {
	t.Helper()
	record, err := product.Cluster.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).
		Namespace(api.SystemNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the action record %s: %v", name, err)
	}
	return record
}

// reinstallRequested reports whether the reinstall event was recorded.
func reinstallRequested(t *testing.T, product *fixtures.Product) bool {
	t.Helper()
	events, err := product.Cluster.Resource(schema.GroupVersionResource{Version: "v1", Resource: "events"}).
		Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list the events: %v", err)
	}
	for _, event := range events.Items {
		if event.GetName() != "" && reasonOf(event) == kosapp.EventReinstallRequested {
			return true
		}
	}
	return false
}

// reasonOf reads an event's reason.
func reasonOf(event unstructured.Unstructured) string {
	reason, _, _ := unstructured.NestedString(event.Object, "reason")
	return reason
}

// driftImage changes the live workload's image behind the reconciler.
func driftImage(t *testing.T, product *fixtures.Product) {
	t.Helper()
	resource := product.Cluster.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	stored, err := resource.Namespace(namespace).Get(context.Background(), "shop", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the workload: %v", err)
	}
	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		t.Fatalf("read the workload containers: found = %v, err = %v", found, err)
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("the workload container is not a map: %T", containers[0])
	}
	container["image"] = driftedImage
	containers[0] = container
	if err := unstructured.SetNestedSlice(stored.Object, containers, "spec", "template", "spec", "containers"); err != nil {
		t.Fatalf("drift the workload: %v", err)
	}
	if _, err := resource.Namespace(namespace).Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("write the drift: %v", err)
	}
}

// workloadImage reads the live workload's container image.
func workloadImage(t *testing.T, product *fixtures.Product) string {
	t.Helper()
	resource := product.Cluster.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	stored, err := resource.Namespace(namespace).Get(context.Background(), "shop", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the workload: %v", err)
	}
	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		t.Fatalf("read the workload containers: found = %v, err = %v", found, err)
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("the workload container is not a map: %T", containers[0])
	}
	image, _ := container["image"].(string)
	return image
}

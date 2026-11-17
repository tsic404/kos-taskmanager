// Package tasktc11 checks TASK-TC-11: with the runtime unreachable the Pod
// projection and the per-Pod log read are unreadable and carry a reason
// instead of an empty list or an empty log, and both recover once the runtime
// answers again (KOSD-C-76, KOSD-C-64).
package tasktc11

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application and its Pod live.
const namespace = "home"

// application is the application whose Pod page is read.
const application = "shop"

// pod is the Pod the log read narrows to.
const pod = "shop-1"

// world is one application with a reachable-to-unreachable runtime source.
type world struct {
	source  *fixtures.SwitchableSource
	product *fixtures.Product
}

func newWorld(t *testing.T) *world {
	t.Helper()
	cluster := fixtures.Cluster(t)
	trail := fixtures.NewTrail()
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: application, Owner: "lin",
		Rendered: []v1beta1.ObjectRef{{Kind: "Deployment", Namespace: namespace, Name: application}},
	})
	fixtures.Workload(t, cluster, namespace, application, map[string]string{"app": application})
	fixtures.RuntimePod(t, cluster, fixtures.PodOptions{
		Namespace: namespace, Name: pod, Labels: map[string]string{"app": application},
		Node: "node-a", Phase: "Running", Ready: true,
		Containers: []fixtures.PodContainer{{Name: "web", Image: "registry.test/shop:2"}},
	})
	logs := &fixtures.LogSource{ByContainer: map[string][]controlplane.LogLine{
		fixtures.LogKey(namespace, pod, "web"): {
			{Timestamp: fixtures.Now.Add(-time.Minute), Pod: pod, Container: "web", Message: "shop-1 的行"},
		},
	}}
	source := &fixtures.SwitchableSource{Live: controlplane.NewClusterSource(cluster)}
	return &world{
		source:  source,
		product: fixtures.NewProductWithSource(t, cluster, trail, nil, logs, source),
	}
}

// addresses reads the two control-plane addresses the page row names.
func (w *world) addresses(t *testing.T) (string, string) {
	t.Helper()
	page := w.product.ReadPage(t, fixtures.Owner, app.PagePods)
	var podsPath, logsPath string
	for _, row := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPods), "applications") {
		if row["namespace"] == namespace && row["name"] == application {
			podsPath, _ = row["path"].(string)
		}
	}
	for _, row := range fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, app.ItemPodLogs), "applications") {
		if row["namespace"] == namespace && row["name"] == application {
			logsPath, _ = row["path"].(string)
		}
	}
	if podsPath == "" || logsPath == "" {
		t.Fatalf("the Pod page carries no addresses for %s/%s: pods %q logs %q",
			namespace, application, podsPath, logsPath)
	}
	return podsPath, logsPath
}

func TestTASKTC11AnUnreachableRuntimeIsUnreadableWithAReason(t *testing.T) {
	world := newWorld(t)
	podsPath, logsPath := world.addresses(t)

	// Before the outage both reads answer with facts.
	if status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, podsPath, nil, nil); status != http.StatusOK {
		t.Fatalf("Pod projection before the outage = %d %v, want 200", status, body)
	}

	world.source.Fail(errors.New("集群 API 不可达"))
	for _, step := range []struct {
		name string
		path string
		key  string
	}{
		{name: "Pod projection", path: podsPath, key: "items"},
		{name: "per-Pod log read", path: logsPath + "?pod=" + pod, key: "lines"},
	} {
		status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, step.path, nil, nil)
		if status != http.StatusBadGateway {
			t.Fatalf("%s during the outage = %d %v, want 502", step.name, status, body)
		}
		failure := fixtures.Failure(t, body)
		if failure["code"] != controlplane.CodeClusterUnavailable {
			t.Fatalf("%s failure = %v, want the cluster-unavailable code", step.name, failure)
		}
		if message, _ := failure["message"].(string); message == "" {
			t.Fatalf("%s failure = %v, want a reason", step.name, failure)
		}
		// An unreadable read is never presented as an empty answer.
		if _, found := body[step.key]; found {
			t.Fatalf("%s during the outage carries %q, want no empty answer: %v", step.name, step.key, body)
		}
	}

	// Recovery restores both reads on the same addresses.
	world.source.Recover()
	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, podsPath, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("Pod projection after recovery = %d %v, want 200", status, body)
	}
	if items := fixtures.ObjectsOf(t, body, "items"); len(items) != 1 || items[0]["name"] != pod {
		t.Fatalf("Pod projection after recovery = %v, want the Pod again", items)
	}
	status, body = world.product.Call(t, fixtures.Owner, http.MethodGet, logsPath+"?pod="+pod, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("per-Pod log read after recovery = %d %v, want 200", status, body)
	}
	if lines, _ := body["lines"].([]any); len(lines) != 1 {
		t.Fatalf("per-Pod log read after recovery = %v, want the Pod's line again", body)
	}
}

func TestTASKTC11ARejectedCredentialIsItsOwnReason(t *testing.T) {
	world := newWorld(t)
	podsPath, _ := world.addresses(t)
	world.source.Fail(apierrors.NewUnauthorized("集群拒绝了这个凭据"))

	status, body := world.product.Call(t, fixtures.Owner, http.MethodGet, podsPath, nil, nil)
	if status != http.StatusBadGateway {
		t.Fatalf("Pod projection with a rejected credential = %d %v, want 502", status, body)
	}
	if failure := fixtures.Failure(t, body); failure["code"] != controlplane.CodeClusterCredentialsInvalid {
		t.Fatalf("Pod projection failure = %v, want the credential reason", failure)
	}
}

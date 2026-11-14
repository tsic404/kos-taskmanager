// Package tasktc5 checks TASK-TC-5: with the metric source disconnected the
// application page states that resource usage is unmeasurable and why, keeps the
// application list, health and entries readable, and never presents zero usage
// as a measurement (TASK-C-2). The connected case is read beside it, so the
// absence cases are not vacuously true.
package tasktc5

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	taskapp "github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application lives.
const namespace = "home"

// newWorld installs the package over one cluster and wires the metric source the
// case names; a nil source is the distribution without metrics.
func newWorld(t *testing.T, usage taskapp.UsageSource) *fixtures.Product {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Definition(t, cluster, "shop", "sha256:shop-v1", "1.0.0")
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: "lin", Members: []string{"mei"},
		DefinitionRef: "shop", UID: "uid-shop",
	})
	return fixtures.NewProduct(t, cluster, fixtures.NewTrail(), usage)
}

// usageValue reads the resource-usage item and fails when it carries a usage
// number while the source cannot measure one.
func usageValue(t *testing.T, product *fixtures.Product) map[string]any {
	t.Helper()
	page := product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)
	value := fixtures.ValueOf(t, page, taskapp.ItemResourceUsage)
	if value["available"] != false {
		t.Fatalf("usage item = %v, want an unmeasurable value", value)
	}
	reason, _ := value["reason"].(string)
	if reason == "" {
		t.Fatalf("usage item = %v, want the reason it is unmeasurable", value)
	}
	if _, present := value["applications"]; present {
		t.Fatalf("usage item = %v, want no usage at any level", value)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode the usage item: %v", err)
	}
	for _, word := range []string{"cpu", "memory"} {
		if strings.Contains(string(encoded), word) {
			t.Fatalf("usage item %s names %q", encoded, word)
		}
	}
	return value
}

func TestTASKTC5NoMetricSourceIsUnmeasurable(t *testing.T) {
	product := newWorld(t, nil)
	usageValue(t, product)

	// The rest of the page is unaffected: the list, the health and every entry
	// the owning Member has remain readable (TASK-C-2).
	page := product.ReadPage(t, fixtures.OwningMember, taskapp.PageApplications)
	rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstalled), "applications")
	if len(rows) != 1 || rows[0]["name"] != "shop" || rows[0]["health"] != "Healthy" {
		t.Fatalf("installed applications = %v, want the readable list and health", rows)
	}
	targets := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemStop), "applications")
	if len(targets) != 1 || targets[0]["name"] != "shop" {
		t.Fatalf("stop entry = %v, want the running application", targets)
	}
}

func TestTASKTC5AFailingSourceIsUnmeasurable(t *testing.T) {
	product := newWorld(t, fixtures.UsageSamples{Err: errors.New("指标来源已断开")})
	usageValue(t, product)

	// The application list still reads: an unreachable metric source is not an
	// unreachable cluster (TASK-C-2).
	page := product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)
	if rows := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstalled), "applications"); len(rows) != 1 {
		t.Fatalf("installed applications = %v, want the list to stay readable", rows)
	}
}

func TestTASKTC5AConnectedSourceReportsItsSamples(t *testing.T) {
	product := newWorld(t, fixtures.UsageSamples{Samples: []taskapp.ApplicationUsage{
		{Namespace: namespace, Name: "shop", CPU: "25m", Memory: "64Mi"},
	}})
	page := product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)
	value := fixtures.ValueOf(t, page, taskapp.ItemResourceUsage)
	if value["available"] != true {
		t.Fatalf("usage item = %v, want a measurable value", value)
	}
	rows := fixtures.ObjectsOf(t, value, "applications")
	if len(rows) != 1 || rows[0]["name"] != "shop" || rows[0]["cpu"] != "25m" || rows[0]["memory"] != "64Mi" {
		t.Fatalf("usage rows = %v, want the sample the source answered with", rows)
	}
}

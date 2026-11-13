// Package tasktc4 checks TASK-TC-4: after the catalogue republishes a higher
// version of the entry an application was installed from, the application page
// offers no upgrade entry and no update prompt (APPSTORE-C-79, APPSTORE-C-80).
// The application center's half of the case — the target version and the update
// available notice — is APPSTORE-TC-168 and APPSTORE-TC-169 in the app-center
// product, which reads the same definition;
// this case proves that the definition is there and that this page shows none of
// it.
package tasktc4

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/pkg/write"

	taskapp "github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/tests/acceptance/fixtures"
)

// namespace is where the application lives.
const namespace = "home"

// forbidden are the identities an upgrade entry or an update notice would have
// to carry, in the vocabulary the page declares and the vocabulary it renders.
var forbidden = []string{
	"upgrade", "Upgrade", "update", "Update",
	"升级", "更新", "targetVersion", "updateAvailable", "availableVersion",
}

func newWorld(t *testing.T) *fixtures.Product {
	t.Helper()
	cluster := fixtures.Cluster(t)
	fixtures.Definition(t, cluster, "shop", "sha256:shop-v1", "1.0.0")
	fixtures.Application(t, cluster, fixtures.ApplicationOptions{
		Namespace: namespace, Name: "shop", Owner: "lin", Members: []string{"mei"},
		DefinitionRef: "shop", AppliedDigest: "sha256:shop-v1",
	})
	// The catalogue publishes a higher version with a new digest, which is the
	// input the app center turns into an upgrade entry (APPSTORE-C-31).
	fixtures.PublishDefinition(t, cluster, "shop", "sha256:shop-v2", "2.0.0")
	return fixtures.NewProduct(t, cluster, fixtures.NewTrail(), nil)
}

func TestTASKTC4ThePageDeclaresNoUpgradeEntry(t *testing.T) {
	product := newWorld(t)
	declared, found := product.Registry.PageFor(taskapp.PageApplications, write.RoleGuest)
	if !found {
		t.Fatal("the installed package declares no application page")
	}
	if declared.ReadOnly {
		t.Fatal("the application page is read-only, so the case cannot tell an absent entry from a page that has none")
	}
	// Exactly the entries TASK-C-2 names: an upgrade entry would be one more.
	if len(declared.Actions) != 6 {
		t.Fatalf("the page declares %d actions, want the six entries of TASK-C-2", len(declared.Actions))
	}
	for _, action := range declared.Actions {
		for _, word := range forbidden {
			if strings.Contains(action.ID, word) || strings.Contains(action.Path, word) {
				t.Fatalf("action %q %s names %q", action.ID, action.Path, word)
			}
		}
	}
	for _, item := range declared.Items {
		for _, word := range forbidden {
			if strings.Contains(item.ID, word) || strings.Contains(item.Label, word) || strings.Contains(item.Action, word) {
				t.Fatalf("item %q (%s, action %q) names %q", item.ID, item.Label, item.Action, word)
			}
			if strings.Contains(item.Reason, word) {
				t.Fatalf("item %q reason %q names %q", item.ID, item.Reason, word)
			}
		}
	}
}

func TestTASKTC4TheServedPageCarriesNoHigherVersion(t *testing.T) {
	product := newWorld(t)
	page := product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)

	// The page the control plane serves has no upgrade entry and no update
	// notice, although the catalogue now offers one (APPSTORE-C-79,
	// APPSTORE-C-80).
	rendered := fixtures.PageJSON(t, page)
	for _, word := range forbidden {
		if strings.Contains(rendered, word) {
			t.Fatalf("the application page carries %q: %s", word, rendered)
		}
	}

	// The installed application's item stays the object's own facts: the higher
	// version the catalogue offers never reaches it.
	installed := fixtures.ValueOf(t, page, taskapp.ItemInstalled)
	encoded, err := json.Marshal(installed)
	if err != nil {
		t.Fatalf("encode the installed item: %v", err)
	}
	for _, offer := range []string{"2.0.0", "sha256:shop-v2"} {
		if strings.Contains(string(encoded), offer) {
			t.Fatalf("the installed item reads the catalogue's %q: %s", offer, encoded)
		}
	}
	rows := fixtures.ObjectsOf(t, installed, "applications")
	if len(rows) != 1 {
		t.Fatalf("the page lists %d applications, want the installed one", len(rows))
	}
	for _, key := range []string{"version", "target", "upgrade", "update", "available"} {
		if _, present := rows[0][key]; present {
			t.Fatalf("the application row carries %q: %v", key, rows[0])
		}
	}

	// The catalogue version is visible where it belongs: the installable list,
	// which is the entry list and not an installed application (APPSTORE-C-31).
	catalogue := fixtures.ObjectsOf(t, fixtures.ValueOf(t, page, taskapp.ItemInstallable), "entries")
	if len(catalogue) != 1 || catalogue[0]["version"] != "2.0.0" {
		t.Fatalf("installable entries = %v, want the catalogue's own version", catalogue)
	}
}

func TestTASKTC4TheCatalogueStillOffersTheHigherVersion(t *testing.T) {
	product := newWorld(t)
	// The app center's input is the definition, and it does offer the higher
	// version; this page never reads it (APPSTORE-C-31, APPSTORE-C-79).
	definition := fixtures.DefinitionObject(t, product.Cluster, "shop")
	version, _, _ := unstructured.NestedString(definition.Object, "spec", "package", "version")
	digest, _, _ := unstructured.NestedString(definition.Object, "spec", "package", "digest")
	if version != "2.0.0" || digest != "sha256:shop-v2" {
		t.Fatalf("the definition offers %s/%s, want the higher version the app center projects", version, digest)
	}

	// The installed application is still the one it applied; the application
	// page reports that object and shows no second, upgradeable state.
	application := fixtures.ApplicationObject(t, product.Cluster, namespace, "shop")
	applied, _, _ := unstructured.NestedString(application.Object, "status", "applied", "definitionDigest")
	if applied != "sha256:shop-v1" {
		t.Fatalf("the application applied %q, want the baseline it installed", applied)
	}
	page := product.ReadPage(t, fixtures.Owner, taskapp.PageApplications)
	if strings.Contains(fixtures.PageJSON(t, page), "sha256:shop-v2") {
		t.Fatal("the application page reads the definition's package digest")
	}
}

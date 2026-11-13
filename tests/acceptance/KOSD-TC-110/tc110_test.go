// Package tc110 checks the disable-and-enable half of KOSD-AC-54 from this
// package's side: disabling the component withdraws the application's window and
// its page entries, enabling it brings back the same declaration, and nothing
// the package does not own is touched.
package tc110_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tsic404/kos-dashboard/pkg/apphost"
	"github.com/tsic404/kos-sdk/packaging"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/internal/artifact"
)

const consoleOrigin = "https://console.kos.tsic.top"

func build(t *testing.T) (packaging.Package, []byte) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	pkg, archive, err := artifact.Build(artifact.Options{Root: root})
	if err != nil {
		t.Fatalf("build the artifact: %v", err)
	}
	return pkg, archive
}

func contribution(pkg packaging.Package, phase string) apphost.Contribution {
	raw, err := json.Marshal(pkg.Manifest.Contributions.Apps[0])
	if err != nil {
		panic(err)
	}
	return apphost.Contribution{
		Extension: app.Identifier, Component: app.Identifier,
		Phase: phase, Digest: pkg.Digest.String(), Manifest: raw,
	}
}

func otherContribution() apphost.Contribution {
	raw, err := json.Marshal(map[string]any{
		"id": "notes", "name": "Notes", "version": "1.0.0",
		"icon":  "assets/icon.svg",
		"entry": "https://notes.apps.kos.tsic.top/assets/index.html",
		"window": map[string]any{
			"width": 900, "height": 600, "minWidth": 640, "minHeight": 480,
			"resizable": true, "mode": "multi", "defaultState": "normal",
		},
		"permissions": []any{},
	})
	if err != nil {
		panic(err)
	}
	return apphost.Contribution{
		Extension: "kos-notes", Component: "kos-notes",
		Phase: apphost.PhaseReady, Digest: "sha256:other", Manifest: raw,
	}
}

func load(t *testing.T, ours apphost.Contribution) *apphost.Registry {
	t.Helper()
	return apphost.Load([]apphost.Contribution{ours, otherContribution()},
		apphost.ValidationOptions{ConsoleOrigin: consoleOrigin})
}

func TestKOSDTC110DisableWithdrawsTheApplicationAndEnableRestoresIt(t *testing.T) {
	pkg, archive := build(t)

	enabled := load(t, contribution(pkg, apphost.PhaseReady))
	served, ok := enabled.App(app.AppID)
	if !ok {
		t.Fatalf("the enabled application is not served: %v", enabled.Refusals())
	}

	// Disable: the window and its page entries disappear while the other product
	// stays served (KOSD-AC-54).
	disabled := load(t, contribution(pkg, apphost.PhaseDisabled))
	if _, ok := disabled.App(app.AppID); ok {
		t.Fatal("the disabled application is still served")
	}
	for _, origin := range disabled.Origins() {
		if origin == app.EntryOrigin {
			t.Fatalf("the disabled application origin %q is still served", origin)
		}
	}
	if _, ok := disabled.App("notes"); !ok {
		t.Fatal("disabling this product removed the other one")
	}
	if len(disabled.Refusals()) != 1 || disabled.Refusals()[0].Reason == "" {
		t.Fatalf("refusals = %+v, want one reasoned refusal", disabled.Refusals())
	}

	// Enable: the same declaration returns, with the same digest and origin.
	reEnabled := load(t, contribution(pkg, apphost.PhaseReady))
	restored, ok := reEnabled.App(app.AppID)
	if !ok {
		t.Fatalf("the re-enabled application is not served: %v", reEnabled.Refusals())
	}
	if !reflect.DeepEqual(restored, served) {
		t.Fatalf("re-enabled entry = %+v, want %+v", restored, served)
	}

	// An uninstall in progress withdraws it the same way, so a terminating
	// component never leaves a window behind (KOSD-C-28).
	terminating := load(t, contribution(pkg, apphost.PhaseTerminating))
	if _, ok := terminating.App(app.AppID); ok {
		t.Fatal("a terminating application is still served")
	}

	// Rebuilding after the cycle yields the same bytes: disabling or enabling
	// changes no package data (KOSD-AC-54).
	_, rebuilt := build(t)
	if !bytes.Equal(archive, rebuilt) {
		t.Fatal("the package bytes changed across the disable and enable cycle")
	}
}

func TestKOSDTC110ThePackageOwnsNothingADisableCouldStop(t *testing.T) {
	pkg, _ := build(t)
	encoded, err := json.Marshal(pkg.Manifest.Contributions)
	if err != nil {
		t.Fatalf("encode contributions: %v", err)
	}
	var classes map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &classes); err != nil {
		t.Fatalf("decode contributions: %v", err)
	}
	keys := make([]string, 0, len(classes))
	for key := range classes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"apps", "pages"}) {
		t.Fatalf("contributions = %v, want the applications and pages classes", keys)
	}
	keepsApplicationData, declared := true, false
	for _, entry := range pkg.Manifest.Retention {
		if entry.ExternalReference != "" {
			t.Fatalf("retention entry %q names external data %q, want none",
				entry.Resource, entry.ExternalReference)
		}
		if entry.Resource == "apps" {
			keepsApplicationData, declared = entry.Keep, true
		}
	}
	if !declared {
		t.Fatal("the declaration names no retention for the application contribution")
	}
	if keepsApplicationData {
		t.Fatal("the application contribution is kept on uninstall, want removal")
	}
}

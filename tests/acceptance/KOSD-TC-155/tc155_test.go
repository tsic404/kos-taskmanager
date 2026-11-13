// Package tc155 checks KOSD-TC-155 from the package's side: the artifact this
// repository builds installs as one system application whose window and four
// page surfaces appear, disappears on uninstall and returns on reinstall, while
// another installed product and the package bytes stay untouched.
package tc155_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tsic404/kos-dashboard/pkg/apphost"
	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kos-sdk/packaging"

	"github.com/tsic404/kosd/pkg/extension"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/internal/artifact"
)

// consoleOrigin is the origin the console is served from: an application entry
// may never share it (KOSD-C-73).
const consoleOrigin = "https://console.kos.tsic.top"

// otherApplication is a second installed product, which the cases read before
// and after this one is installed, uninstalled or disabled.
func otherApplication() json.RawMessage {
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
	return raw
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	return root
}

func build(t *testing.T) (packaging.Package, []byte) {
	t.Helper()
	pkg, archive, err := artifact.Build(artifact.Options{Root: repoRoot(t)})
	if err != nil {
		t.Fatalf("build the artifact: %v", err)
	}
	return pkg, archive
}

func validationOptions() apphost.ValidationOptions {
	return apphost.ValidationOptions{ConsoleOrigin: consoleOrigin}
}

func ourContribution(pkg packaging.Package) apphost.Contribution {
	raw, err := json.Marshal(pkg.Manifest.Contributions.Apps[0])
	if err != nil {
		panic(err)
	}
	return apphost.Contribution{
		Extension: app.Identifier,
		Component: app.Identifier,
		Phase:     apphost.PhaseReady,
		Digest:    pkg.Digest.String(),
		Manifest:  raw,
	}
}

func TestKOSDTC155ThePackageInstallsAsOneApplication(t *testing.T) {
	pkg, archive := build(t)

	// The install path's own check: structure, digest and declaration.
	verified, err := packaging.Verify(archive, pkg.Digest)
	if err != nil {
		t.Fatalf("the packaged artifact fails the install-path check: %v", err)
	}

	// The packed field set is the framework's, not the packer's: re-checking the
	// produced manifest reports nothing missing and nothing unknown (SDK-C-3).
	encoded, err := json.Marshal(verified.Manifest)
	if err != nil {
		t.Fatalf("encode the packed manifest: %v", err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode the packed manifest: %v", err)
	}
	if checked := manifest.ValidateExtensionManifest(document); !checked.OK() {
		t.Fatalf("the packed declaration is invalid: %v", checked.Issues)
	}

	apps := verified.Manifest.Contributions.Apps
	if len(apps) != 1 {
		t.Fatalf("contributions declare %d applications, want 1", len(apps))
	}
	declared := apps[0]
	if declared.ID != app.AppID || declared.Name != app.AppName || declared.Version != app.Version {
		t.Fatalf("application identity = %s/%s/%s, want %s/%s/%s",
			declared.ID, declared.Name, declared.Version, app.AppID, app.AppName, app.Version)
	}
	if declared.Entry != app.EntryOrigin+"/"+app.DocumentAsset {
		t.Fatalf("entry = %q, want the document on the application origin", declared.Entry)
	}
	if !strings.HasPrefix(declared.Entry, "https://") {
		t.Fatalf("entry %q is not an HTTPS address", declared.Entry)
	}
	if declared.Entry == consoleOrigin || strings.HasPrefix(declared.Entry, consoleOrigin+"/") {
		t.Fatalf("entry %q shares the console origin", declared.Entry)
	}
	if !reflect.DeepEqual(declared.Permissions, []manifest.Permission{}) {
		t.Fatalf("permissions = %v, want none declared", declared.Permissions)
	}
	// Every field KOSD-C-73 requires is present, and the optional fields nobody
	// declared stay absent: the packer fills no default (SDK-C-3).
	appFields := fieldsOf(t, json.RawMessage(mustEncode(t, declared)))
	for _, field := range []string{"id", "name", "version", "icon", "entry", "window", "permissions"} {
		if _, ok := appFields[field]; !ok {
			t.Fatalf("the packed application declares no %s", field)
		}
	}
	for _, field := range []string{"backend", "deepLinks"} {
		if _, ok := appFields[field]; ok {
			t.Fatalf("the packed application declares %s, which the declaration omits", field)
		}
	}
	windowFields := fieldsOf(t, appFields["window"])
	wantWindow := append([]string(nil), manifest.AppWindowFields...)
	sort.Strings(wantWindow)
	if got := sortedKeys(windowFields); !reflect.DeepEqual(got, wantWindow) {
		t.Fatalf("window fields = %v, want %v", got, wantWindow)
	}

	// The document the window loads and the icon it shows travel with the
	// package: an entry naming an absent asset installs into a window that never
	// loads (KOSD-C-73).
	carried := make(map[string]bool, len(verified.Assets))
	for _, asset := range verified.Assets {
		carried[asset.Path] = true
	}
	for _, required := range []string{app.DocumentAsset, app.IconAsset} {
		if !carried[required] {
			t.Fatalf("the package does not carry %s", required)
		}
	}

	// The package contributes the window application and the overview page and
	// nothing that owns a cluster surface, so installing or removing it cannot
	// touch another product's objects (KOSD-C-18, KOSD-AC-72).
	contributions, err := json.Marshal(verified.Manifest.Contributions)
	if err != nil {
		t.Fatalf("encode contributions: %v", err)
	}
	var classNames map[string]json.RawMessage
	if err := json.Unmarshal(contributions, &classNames); err != nil {
		t.Fatalf("decode contributions: %v", err)
	}
	if got := sortedKeys(classNames); !reflect.DeepEqual(got, []string{"apps", "pages"}) {
		t.Fatalf("contributions = %v, want the applications and pages classes", got)
	}
}

// TestKOSDTC155TheKernelLoadsThePackedArtifact holds the artifact to the
// kernel's own reader: the install path parses manifest.json with a closed field
// set, so a declaration the SDK accepts on its own is still not installable.
func TestKOSDTC155TheKernelLoadsThePackedArtifact(t *testing.T) {
	pkg, archive := build(t)

	kernel, err := extension.Read(archive)
	if err != nil {
		t.Fatalf("the kernel refuses the packed artifact: %v", err)
	}
	if kernel.Digest != pkg.Digest.String() {
		t.Fatalf("kernel digest = %q, want %q", kernel.Digest, pkg.Digest)
	}
	loaded, err := kernel.Declaration.Load()
	if err != nil {
		t.Fatalf("the kernel cannot load the declaration: %v", err)
	}
	if len(loaded.Problems) > 0 {
		t.Fatalf("the kernel withdrew contributions: %v", loaded.Problems)
	}
	if loaded.Identifier != app.Identifier || loaded.Version != app.Version {
		t.Fatalf("kernel identity = %s/%s, want %s/%s",
			loaded.Identifier, loaded.Version, app.Identifier, app.Version)
	}

	apps := loaded.Contributions.Apps
	if len(apps) != 1 {
		t.Fatalf("the kernel loads %d applications, want 1", len(apps))
	}
	declared := apps[0]
	if declared.ID != app.AppID || declared.Name != app.AppName || declared.Version != app.Version {
		t.Fatalf("kernel application = %s/%s/%s, want %s/%s/%s",
			declared.ID, declared.Name, declared.Version, app.AppID, app.AppName, app.Version)
	}
	if declared.Entry != app.Manifest().Entry {
		t.Fatalf("kernel entry = %q, want %q", declared.Entry, app.Manifest().Entry)
	}
	if len(declared.Permissions) != 0 {
		t.Fatalf("kernel application declares permissions %v, want none", declared.Permissions)
	}

	// The kernel loads the overview page the package contributes: one read-only
	// page whose items every session may read (KOSD-C-18, KOSD-F-16).
	pages := loaded.Contributions.Pages
	if len(pages) != 1 {
		t.Fatalf("the kernel loads %d pages, want the overview page", len(pages))
	}
	if pages[0].ID != app.PageOverview || !pages[0].ReadOnly || pages[0].ReadOnlyReason == "" {
		t.Fatalf("kernel page = %+v, want the read-only overview page", pages[0])
	}
	if len(pages[0].Items) == 0 {
		t.Fatal("the loaded overview page declares no item")
	}

	// Uninstall withdraws the window and the page: neither contribution keeps
	// data (KOSD-C-18, KOSD-AC-72).
	kept := map[string]bool{}
	for _, entry := range loaded.Retention {
		kept[entry.Resource] = entry.Keep
	}
	for _, resource := range []string{"apps", "pages"} {
		keep, declared := kept[resource]
		if !declared {
			t.Fatalf("kernel retention = %+v, want %s removed on uninstall", loaded.Retention, resource)
		}
		if keep {
			t.Fatalf("kernel retention keeps %s on uninstall, want removal", resource)
		}
	}
}

// TestKOSDTC155TheShellRefusesOnlyTheManifestsPermissions keeps the browser's
// copy of the permission set tied to the packed App contribution: the document
// cannot read the manifest, so a drift would silently widen what the handshake
// accepts (KOSD-C-73, KOSD-C-75).
func TestKOSDTC155TheShellRefusesOnlyTheManifestsPermissions(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(repoRoot(t), "assets", "app.js"))
	if err != nil {
		t.Fatalf("read the shell entry: %v", err)
	}
	declared := shellPermissions(t, string(source))
	want := make([]string, 0, len(app.Manifest().Permissions))
	for _, permission := range app.Manifest().Permissions {
		want = append(want, string(permission))
	}
	if !reflect.DeepEqual(declared, want) {
		t.Fatalf("the shell declares permissions %v, the packed application %v", declared, want)
	}
}

// shellPermissions reads the DECLARED_PERMISSIONS literal of assets/app.js,
// which is the only permission declaration the browser sees.
func shellPermissions(t *testing.T, source string) []string {
	t.Helper()
	literal := regexp.MustCompile(`(?s)DECLARED_PERMISSIONS\s*=\s*\[([^\]]*)\]`).
		FindStringSubmatch(source)
	if literal == nil {
		t.Fatal("assets/app.js declares no DECLARED_PERMISSIONS list")
	}
	permissions := []string{}
	for _, match := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(literal[1], -1) {
		permissions = append(permissions, match[1])
	}
	return permissions
}

func TestKOSDTC155InstallUninstallReinstall(t *testing.T) {
	pkg, archive := build(t)
	ours := ourContribution(pkg)
	other := apphost.Contribution{
		Extension: "kos-notes", Component: "kos-notes",
		Phase: apphost.PhaseReady, Digest: "sha256:other", Manifest: otherApplication(),
	}

	// Install: the window and its page surfaces are served.
	installed := apphost.Load([]apphost.Contribution{ours, other}, validationOptions())
	entry, ok := installed.App(app.AppID)
	if !ok {
		t.Fatalf("the installed application is not served: %v", installed.Refusals())
	}
	if entry.Origin() != app.EntryOrigin {
		t.Fatalf("served origin = %q, want %q", entry.Origin(), app.EntryOrigin)
	}
	if entry.Digest != pkg.Digest.String() {
		t.Fatalf("served digest = %q, want %q", entry.Digest, pkg.Digest)
	}
	if !reflect.DeepEqual(entry.Window, windowDefaults()) {
		t.Fatalf("served window = %+v, want the declared defaults", entry.Window)
	}
	if len(installed.Apps()) != 2 {
		t.Fatalf("installed applications = %d, want this one and the other product", len(installed.Apps()))
	}

	// The signed asset address the console issues for this window names this
	// package's digest and serves the document the package carries (KOSD-C-73).
	secret := []byte("0123456789abcdef0123456789abcdef")
	expires := time.Now().Add(5 * time.Minute)
	asset := apphost.Asset{
		Extension: app.Identifier, AppID: app.AppID, Version: app.Version,
		Digest: pkg.Digest.String(), Path: app.DocumentAsset,
	}
	address, err := apphost.SignedURL(secret, app.EntryOrigin, asset, expires)
	if err != nil {
		t.Fatalf("sign the document address: %v", err)
	}
	if !strings.HasPrefix(address, app.EntryOrigin+apphost.AssetAddressPrefix) {
		t.Fatalf("signed address %q is not on the application origin", address)
	}
	served := readServedDocument(t, pkg, asset)
	if !bytes.Contains(served, []byte("./app.js")) {
		t.Fatalf("served document does not load the shell entry: %s", served)
	}

	// Uninstall: the application is gone, the other product is not.
	uninstalled := apphost.Load([]apphost.Contribution{other}, validationOptions())
	if _, ok := uninstalled.App(app.AppID); ok {
		t.Fatal("the uninstalled application is still served")
	}
	if _, ok := uninstalled.App("notes"); !ok {
		t.Fatal("uninstalling this product removed the other one")
	}
	for _, origin := range uninstalled.Origins() {
		if origin == app.EntryOrigin {
			t.Fatalf("the uninstalled application origin %q is still served", origin)
		}
	}

	// Reinstall: the same package comes back through the same path, with the
	// same digest, origin and window.
	reinstalled := apphost.Load([]apphost.Contribution{ours, other}, validationOptions())
	again, ok := reinstalled.App(app.AppID)
	if !ok {
		t.Fatalf("the reinstalled application is not served: %v", reinstalled.Refusals())
	}
	if !reflect.DeepEqual(again, entry) {
		t.Fatalf("reinstalled entry = %+v, want %+v", again, entry)
	}
	if !bytes.Equal(archive, rebuild(t)) {
		t.Fatal("the package bytes changed between install and reinstall")
	}
}

func TestKOSDTC155DigestCoversTheManifestAndEveryAsset(t *testing.T) {
	assets, err := artifact.Assets(artifact.Options{Root: repoRoot(t)})
	if err != nil {
		t.Fatalf("read the assets: %v", err)
	}
	baseline, _, err := artifact.BuildFiles(assets)
	if err != nil {
		t.Fatalf("pack the baseline: %v", err)
	}

	// Every UI asset is covered: changing one byte of any asset changes the
	// digest, and so does renaming one (SDK-C-4, KOSD-C-17).
	for index := range assets {
		changed := make([]packaging.Asset, len(assets))
		copy(changed, assets)
		content := append([]byte(nil), changed[index].Content...)
		changed[index].Content = append(content, '\n')
		packed, _, err := artifact.BuildFiles(changed)
		if err != nil {
			t.Fatalf("pack with %s changed: %v", changed[index].Path, err)
		}
		if packed.Digest == baseline.Digest {
			t.Fatalf("changing %s left the digest unchanged", changed[index].Path)
		}
	}

	renamed := make([]packaging.Asset, len(assets))
	copy(renamed, assets)
	didRename := false
	for index := range renamed {
		if renamed[index].Path == app.DocumentAsset || renamed[index].Path == app.IconAsset {
			continue
		}
		renamed[index].Path = renamed[index].Path + ".renamed"
		didRename = true
		break
	}
	if !didRename {
		t.Fatal("the package carries no asset that could be renamed")
	}
	packed, _, err := artifact.BuildFiles(renamed)
	if err != nil {
		t.Fatalf("pack with a renamed asset: %v", err)
	}
	if packed.Digest == baseline.Digest {
		t.Fatal("renaming an asset left the digest unchanged")
	}

	// The archive carries exactly the declaration, its digest and the assets the
	// repository holds: no entry the manifest does not name, no asset left out.
	_, archive := build(t)
	entries := archiveEntries(t, archive)
	names, err := artifact.AssetPaths(artifact.Options{Root: repoRoot(t)})
	if err != nil {
		t.Fatalf("list the asset names: %v", err)
	}
	want := append([]string{packaging.ManifestEntry, packaging.DigestEntry}, names...)
	sort.Strings(want)
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("archive entries = %v, want %v", entries, want)
	}
}

func TestKOSDTC155ReadBudgetRejectsTheWholePackage(t *testing.T) {
	pkg, archive := build(t)

	if _, err := packaging.Verify(archive, pkg.Digest); err != nil {
		t.Fatalf("the artifact exceeds the default read budget: %v", err)
	}

	// An entry budget below the archive's entry count stops at the boundary and
	// names the dimension; no partial content is returned.
	byEntries, err := packaging.VerifyWithLimits(archive, pkg.Digest, packaging.Limits{Entries: 2})
	if err == nil {
		t.Fatal("the entry budget was not enforced")
	}
	if !strings.Contains(err.Error(), "entries") {
		t.Fatalf("entry-budget refusal = %q, want the entry dimension", err)
	}
	if !errors.Is(err, packaging.ErrMalformedPackage) {
		t.Fatalf("entry-budget refusal = %v, want the package marked unreadable", err)
	}
	if byEntries.Manifest.Identifier != "" || len(byEntries.Assets) != 0 {
		t.Fatal("an over-budget read returned partial content")
	}

	// A byte budget below the first entry's size is decided from the header,
	// before any content is materialized.
	byBytes, err := packaging.VerifyWithLimits(archive, pkg.Digest, packaging.Limits{UncompressedBytes: 1})
	if err == nil {
		t.Fatal("the byte budget was not enforced")
	}
	if !strings.Contains(err.Error(), "byte budget") {
		t.Fatalf("byte-budget refusal = %q, want the content dimension", err)
	}
	if byBytes.Manifest.Identifier != "" || len(byBytes.Assets) != 0 {
		t.Fatal("an over-budget read returned partial content")
	}
}

func rebuild(t *testing.T) []byte {
	t.Helper()
	_, archive := build(t)
	return archive
}

func windowDefaults() apphost.WindowDefaults {
	window := app.Window()
	return apphost.WindowDefaults{
		Width: int(window.Width), Height: int(window.Height),
		MinWidth: int(window.MinWidth), MinHeight: int(window.MinHeight),
		Resizable: window.Resizable, Mode: window.Mode, DefaultState: window.DefaultState,
	}
}

// readServedDocument reads the document through the console's asset store,
// which serves a verified package from its unpacked layout.
func readServedDocument(t *testing.T, pkg packaging.Package, asset apphost.Asset) []byte {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, asset.Extension, asset.Version, asset.Digest)
	for _, carried := range pkg.Assets {
		name := filepath.Join(base, filepath.FromSlash(carried.Path))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatalf("create the asset directory: %v", err)
		}
		if err := os.WriteFile(name, carried.Content, 0o644); err != nil {
			t.Fatalf("write the unpacked asset: %v", err)
		}
	}
	content, err := (&apphost.DirAssets{Root: root}).Read(context.Background(), asset)
	if err != nil {
		t.Fatalf("read the served document: %v", err)
	}
	return content
}

func archiveEntries(t *testing.T, archive []byte) []string {
	t.Helper()
	decompressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("open the archive: %v", err)
	}
	defer decompressed.Close()
	var names []string
	reader := tar.NewReader(decompressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read the archive: %v", err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustEncode(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return encoded
}

func fieldsOf(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	return fields
}

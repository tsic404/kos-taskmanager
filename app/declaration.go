// Package app declares the kos-taskmanager extension package: one window
// application whose document and UI assets ship with the package (KOSD-C-17).
//
// The declaration is the only source of the packed manifest. The packer, the
// acceptance cases and the shell's navigation all read it here, so the artifact
// cannot declare one field set while the repository states another.
package app

import "github.com/tsic404/kos-sdk/manifest"

const (
	// Identifier is the extension identity across versions (KOSD-C-17).
	Identifier = "kos-taskmanager"
	// Version is the extension and application version this package ships.
	Version = "0.1.0"
	// FrameworkVersion is the framework API range the package targets.
	FrameworkVersion = ">=1.0 <2.0"
	// AppID is the application identity inside the extension (KOSD-C-73).
	AppID = "kos-taskmanager"
	// AppName is the name the launcher and the window title show.
	AppName = "运行视图"
	// EntryOrigin is the application origin the deployment maps to this
	// package's assets. It is its own origin, never the console's (KOSD-C-73).
	EntryOrigin = "https://taskmanager.apps.kos.tsic.top"
	// DocumentAsset is the application document inside the package.
	DocumentAsset = "assets/index.html"
	// IconAsset is the icon inside the package (KOSD-C-73).
	IconAsset = "assets/icon.svg"
)

// Window is the window default the launcher opens the application with.
func Window() manifest.AppWindow {
	return manifest.AppWindow{
		Width:        1200,
		Height:       800,
		MinWidth:     960,
		MinHeight:    640,
		Resizable:    true,
		Mode:         "single",
		DefaultState: "normal",
	}
}

// Manifest is the `App` contribution the package declares (KOSD-C-73). It is a
// UI-only application: no backend, no deep links, no permissions yet.
func Manifest() manifest.AppManifest {
	return manifest.AppManifest{
		ID:          AppID,
		Name:        AppName,
		Version:     Version,
		Icon:        IconAsset,
		Entry:       EntryOrigin + "/" + DocumentAsset,
		Window:      Window(),
		Permissions: []manifest.Permission{},
	}
}

// Declaration is the extension declaration the packer writes (KOSD-C-18). It
// contributes one App and nothing else, so installing, disabling or removing the
// package touches no other contribution and no cluster object.
func Declaration() manifest.ExtensionManifest {
	return manifest.ExtensionManifest{
		Identifier:       Identifier,
		Version:          Version,
		FrameworkVersion: FrameworkVersion,
		Contributions: manifest.Contributions{
			Apps: []manifest.AppManifest{Manifest()},
		},
		Dependencies:  manifest.Dependencies{},
		Permissions:   []manifest.ExtensionPermission{},
		Configuration: manifest.Configuration{},
		// An App is a declaration, not data: uninstall withdraws the window, so
		// the application contribution keeps nothing (KOSD-C-18, KOSD-C-29).
		Retention: []manifest.Retention{
			{Resource: "apps", Keep: false},
		},
	}
}

// Package appcenter declares the app-center surface the package contributes:
// the page with the catalogue reads and their install and upgrade entries
// (APPSTORE-C-31), the window application that hosts the same surface
// (KOSD-C-73), and the readings that resolve the page's items.
package appcenter

import (
	"net/http"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/write"
)

// The page and the write entries it offers (APPSTORE-C-31).
const (
	// PageID names the app-center page.
	PageID = "app-center"
	// InstallAction and UpgradeAction are the page's write entries.
	InstallAction = "install"
	UpgradeAction = "upgrade"
	// EntriesItem and UpgradesItem are the two catalogue reads the page shows.
	EntriesItem  = "entries"
	UpgradesItem = "upgrades"
	// EntriesRef and UpgradesRef are the catalogue reads the items resolve.
	EntriesRef  = "catalogue/entries"
	UpgradesRef = "catalogue/upgrades"
	// ApplicationPath is the control-plane write installing and upgrading.
	ApplicationPath = "/api/v1/resources/Application"
)

// The window application the package contributes (KOSD-C-73).
const (
	// AppID is the application identity, unique inside the installed extension.
	AppID = "app-center"
	// AppName is the name the launcher and the dock show.
	AppName = "应用中心"
	// AppOrigin is the application's own HTTPS origin; the package serves its
	// assets there and shares it with neither the console nor another app.
	AppOrigin = "https://app-center.kos.tsic.top/"
	// IconAsset is the package asset the launcher renders.
	IconAsset = "assets/icon.svg"
)

// Page returns the app-center page (APPSTORE-C-31): the catalogue entries with
// the install entry and the upgrade paths with the upgrade entry. The install
// parameters are not a fixed item: the config structure travels inside each
// entry, and the page read lays the form fields out from it (KOSD-C-32).
func Page() extension.Page {
	return extension.Page{
		ID:           PageID,
		Title:        "应用中心",
		Location:     "panel",
		RequiredRole: string(write.RoleMember),
		Items: []extension.PageItem{
			{ID: EntriesItem, Label: "可安装应用", Source: extension.PageSourceComputed, Reference: EntriesRef, Action: InstallAction},
			{ID: UpgradesItem, Label: "可升级应用", Source: extension.PageSourceComputed, Reference: UpgradesRef, Action: UpgradeAction},
		},
		Actions: []extension.Action{
			{ID: InstallAction, Method: http.MethodPost, Path: ApplicationPath},
			{ID: UpgradeAction, Method: http.MethodPut, Path: ApplicationPath},
		},
	}
}

// App returns the window application declaration (KOSD-C-73). It is UI-only:
// no backend, no deep links and no permissions, so the window reads the
// control-plane API with its own app token and nothing else. version is the
// package version the application ships with.
func App(version string) manifest.AppManifest {
	return manifest.AppManifest{
		ID:      AppID,
		Name:    AppName,
		Version: version,
		Icon:    IconAsset,
		Entry:   AppOrigin,
		Window: manifest.AppWindow{
			Width:        1120,
			Height:       760,
			MinWidth:     880,
			MinHeight:    600,
			Resizable:    true,
			Mode:         "single",
			DefaultState: "normal",
		},
		Permissions: []manifest.Permission{},
	}
}

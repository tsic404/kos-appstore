package appcenter

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/pkg/extension"
)

// TestPageCarriesTheInstallAndUpgradeEntries checks the app-center page: both
// catalogue reads are shown, and each names the control-plane write that carries
// it out (APPSTORE-C-31).
func TestPageCarriesTheInstallAndUpgradeEntries(t *testing.T) {
	page := Page()
	if page.ID != PageID || page.Title == "" || page.Location == "" {
		t.Fatalf("page = %+v, want an identified, titled page in a location", page)
	}
	if page.ReadOnly {
		t.Fatalf("page = %+v, want a page with modification entries", page)
	}
	actions := map[string]extension.Action{}
	for _, action := range page.Actions {
		actions[action.ID] = action
	}
	for id, method := range map[string]string{
		InstallAction: http.MethodPost,
		UpgradeAction: http.MethodPut,
	} {
		action, found := actions[id]
		if !found {
			t.Fatalf("page declares no %s action: %+v", id, page.Actions)
		}
		if action.Method != method || action.Path != ApplicationPath {
			t.Errorf("%s action = %s %s, want %s %s", id, action.Method, action.Path, method, ApplicationPath)
		}
	}
	items := map[string]extension.PageItem{}
	for _, item := range page.Items {
		items[item.ID] = item
	}
	for itemID, actionID := range map[string]string{
		EntriesItem:  InstallAction,
		UpgradesItem: UpgradeAction,
	} {
		item, found := items[itemID]
		if !found {
			t.Fatalf("page declares no %s item: %+v", itemID, page.Items)
		}
		if item.Source != extension.PageSourceComputed || item.Reference == "" {
			t.Errorf("item %s = %s/%s, want a computed read with a reference", itemID, item.Source, item.Reference)
		}
		if item.Action != actionID {
			t.Errorf("item %s action = %q, want %q", itemID, item.Action, actionID)
		}
	}
}

// TestAppSatisfiesTheRequiredManifestSet checks the window declaration against
// the mandatory field set of KOSD-C-73: a missing field, a permission outside
// the fixed set or an entry that is not an https origin would be invalid there.
func TestAppSatisfiesTheRequiredManifestSet(t *testing.T) {
	declared := App("1.2.3")
	encoded, err := json.Marshal(declared)
	if err != nil {
		t.Fatalf("encode the app manifest: %v", err)
	}
	checked := manifest.ValidateAppManifest(asDocument(t, encoded))
	if !checked.OK() {
		t.Fatalf("app manifest is invalid: %v", checked.Issues)
	}
	app := checked.Manifest
	if app.ID != AppID || app.Name != AppName || app.Version != "1.2.3" {
		t.Fatalf("app = %+v, want the app-center identity at the package version", app)
	}
	if app.Icon != IconAsset {
		t.Fatalf("icon = %q, want the package asset %q", app.Icon, IconAsset)
	}
	if app.Entry != AppOrigin {
		t.Fatalf("entry = %q, want the app's own origin %q", app.Entry, AppOrigin)
	}
	if app.Backend != nil || len(app.DeepLinks) != 0 {
		t.Fatalf("app = %+v, want a UI-only app without deep links", app)
	}
	if app.Permissions == nil || len(app.Permissions) != 0 {
		t.Fatalf("permissions = %v, want the empty declared set", app.Permissions)
	}
	if app.Window.Width <= 0 || app.Window.Height <= 0 || app.Window.MinWidth <= 0 || app.Window.MinHeight <= 0 {
		t.Fatalf("window = %+v, want positive pixel sizes", app.Window)
	}
	if app.Window.Mode != "single" || app.Window.DefaultState != "normal" {
		t.Fatalf("window = %+v, want a single-instance window opening normal", app.Window)
	}
}

// asDocument decodes one encoded JSON document into the shape the manifest
// checks read.
func asDocument(t *testing.T, encoded []byte) map[string]any {
	t.Helper()
	document := map[string]any{}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode the manifest document: %v", err)
	}
	return document
}

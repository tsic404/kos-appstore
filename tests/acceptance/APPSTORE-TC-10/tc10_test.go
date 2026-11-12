// Package appstoretc10 verifies that removing the appstore component removes its
// entries and its page while the application installed from one of them stays,
// and that the application's upgrade path is marked as missing its source
// (APPSTORE-AC-4, APPSTORE-TC-10).
package appstoretc10

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tsic404/kosd/pkg/catalogue"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/appcenter"
	"github.com/tsic404/kos-appstore/declaration"
)

const (
	ownerName   = "owner"
	ownerPass   = "owner-password"
	appName     = "cache"
	definition  = "redis"
	component   = "appstore"
	appCenterID = "app-center"
)

// TestTC10SourceRemovalKeepsInstalledApplication removes the source and checks
// the entries and the page are gone, the application is untouched and its upgrade
// path names the missing source (APPSTORE-TC-10).
func TestTC10SourceRemovalKeepsInstalledApplication(t *testing.T) {
	cluster := harness.Start(t)
	shell := cluster.StartShell(t)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	artifacts := extension.MemoryArtifacts{}
	installed := cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention())
	shell.Use(installed)
	shell.SyncCatalogue(t)

	// The page appears with its component, and the entry installs an
	// application (KOSD-AC-72).
	if !pageListed(t, shell, token) {
		t.Fatal("the app-center page is missing while its component is installed")
	}
	status, body := shell.Call(t, http.MethodPost, "/api/v1/resources/Application",
		harness.ApplicationManifest(appName, harness.Namespace, definition, harness.Namespace, nil, nil), token)
	if status != http.StatusAccepted {
		t.Fatalf("install the entry = %d %v", status, body)
	}
	before := cluster.Get(t, "v1beta1", "Application", appName)

	// Removing the component withdraws the source: the entry and the page
	// disappear and the definition is no longer offered.
	cluster.DeleteComponent(t, component)
	shell.Use(cluster.ExtensionStateWith(t, artifacts, shell.Retention()))
	shell.SyncCatalogue(t)

	status, body = shell.Call(t, http.MethodGet, "/api/v1/catalogue/entries", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list entries after the removal = %d %v", status, body)
	}
	if entries := harness.Items(t, body); len(entries) != 0 {
		t.Fatalf("entries after the removal = %v, want none", entries)
	}
	if pageListed(t, shell, token) {
		t.Fatal("the app-center page outlived its component")
	}

	// The application is unchanged: same spec and same stored object.
	after := cluster.Get(t, "v1beta1", "Application", appName)
	if after.GetResourceVersion() != before.GetResourceVersion() {
		t.Fatalf("the application changed when its source was removed: %s -> %s",
			before.GetResourceVersion(), after.GetResourceVersion())
	}

	status, body = shell.Call(t, http.MethodGet, "/api/v1/catalogue/upgrades", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list upgrade paths = %d %v", status, body)
	}
	upgrades := harness.Items(t, body)
	if len(upgrades) != 1 {
		t.Fatalf("upgrade paths = %v, want the one installed application", upgrades)
	}
	if state := harness.String(upgrades[0], "state"); state != catalogue.UpgradeSourceMissing {
		t.Fatalf("upgrade state = %q, want %q", state, catalogue.UpgradeSourceMissing)
	}
	if ref := harness.String(upgrades[0], "definition"); ref != definition {
		t.Fatalf("upgrade definition = %q, want %q", ref, definition)
	}
	if name := harness.String(upgrades[0], "name"); name != appName {
		t.Fatalf("upgrade application = %q, want %q", name, appName)
	}

	// The page item the window renders carries the same judgement: the upgrade
	// path exists and names the missing source (APPSTORE-C-31).
	if state := pageUpgradeState(t, shell); state != catalogue.UpgradeSourceMissing {
		t.Fatalf("page upgrade state = %q, want %q", state, catalogue.UpgradeSourceMissing)
	}

	// Reinstalling goes through the channel that installed it: the same package
	// declaration and artifact source, no dedicated entry point (KOSD-AC-72,
	// KOSD-TC-157).
	reinstalled := cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention())
	shell.Use(reinstalled)
	shell.SyncCatalogue(t)
	if !pageListed(t, shell, token) {
		t.Fatal("the app-center page did not return after the reinstall")
	}
	status, body = shell.Call(t, http.MethodGet, "/api/v1/catalogue/entries", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list entries after the reinstall = %d %v", status, body)
	}
	if entries := harness.Items(t, body); len(entries) == 0 {
		t.Fatalf("entries after the reinstall = %v, want the source's entries back", entries)
	}
	restored := cluster.Get(t, "v1beta1", "Application", appName)
	if restored.GetResourceVersion() != before.GetResourceVersion() {
		t.Fatalf("the reinstall changed the installed application: %s -> %s",
			before.GetResourceVersion(), restored.GetResourceVersion())
	}
}

// shipped returns the declaration the appstore package ships.
func shipped(t *testing.T) extension.Declaration {
	t.Helper()
	declared, err := declaration.Declaration()
	if err != nil {
		t.Fatalf("build the appstore declaration: %v", err)
	}
	return declared
}

// pageListed reports whether the app-center page is among the pages the shell
// serves.
func pageListed(t *testing.T, shell *harness.Shell, token string) bool {
	t.Helper()
	status, body := shell.Call(t, http.MethodGet, "/api/v1/pages", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list pages = %d %v", status, body)
	}
	for _, page := range harness.Items(t, body) {
		if harness.String(page, "id") == appcenter.PageID {
			return true
		}
	}
	return false
}

// pageUpgradeState reads the app-center upgrades item through the page's reader
// and returns the state of the first upgrade path.
func pageUpgradeState(t *testing.T, shell *harness.Shell) string {
	t.Helper()
	reader := appcenter.NewReadings(catalogue.New(shell.Registry, shell.Cluster.Dynamic), nil)
	reading, failure := reader.Read(context.Background(), identity.Session{Role: write.RoleMember},
		extension.PageItem{ID: appcenter.UpgradesItem, Reference: appcenter.UpgradesRef})
	if failure != nil {
		t.Fatalf("read the app-center upgrades item: %v", failure)
	}
	encoded, err := json.Marshal(reading.Value)
	if err != nil {
		t.Fatalf("encode the upgrades item: %v", err)
	}
	var paths []map[string]any
	if err := json.Unmarshal(encoded, &paths); err != nil {
		t.Fatalf("decode the upgrades item: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("page upgrade paths = %v, want the one installed application", paths)
	}
	return harness.String(paths[0], "state")
}

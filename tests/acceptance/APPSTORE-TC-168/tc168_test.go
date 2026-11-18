// Package appstoretc168 verifies the update notice of APPSTORE-AC-78: once the
// catalogue sync writes a higher version, the served page carries the target
// version with no user action and no check step, an application already on the
// offered package carries none, and the page declares no update-check entry
// (APPSTORE-C-80, KOSD-C-13, APPSTORE-TC-168).
package appstoretc168

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/markers"
	"github.com/tsic404/kosd/pkg/write"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/appcenter"
	"github.com/tsic404/kos-appstore/declaration"
	"github.com/tsic404/kos-appstore/tests/acceptance/support"
)

const (
	ownerName  = "owner"
	ownerPass  = "owner-password"
	component  = "appstore"
	sourceID   = "acceptance-catalogue"
	sourceName = "acceptance-catalogue"
	entryID    = "web"
	// behindApp is installed from the earlier package, currentApp from the one
	// the catalogue offers now.
	behindApp  = "behind"
	currentApp = "current"
	// targetNamespace is where the package renders.
	targetNamespace = "apps"
)

// template renders the ConfigMap the package writes.
func template(version string) string {
	return `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
data:
  version: "` + version + `"
`
}

// definition builds one definition pinned to its content digest.
func definition(t *testing.T, version, rendered string) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	declared := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: version},
		Template: template(rendered),
	}
	digest, err := app.DefinitionDigest(declared)
	if err != nil {
		t.Fatalf("digest the definition: %v", err)
	}
	declared.Package.Digest = digest
	if err := app.ValidateDefinition(declared); err != nil {
		t.Fatalf("the definition is invalid: %v", err)
	}
	return declared
}

// offering declares the catalogue entry under the fixture source.
func offering(declared v1beta1.ApplicationDefinitionSpec) extension.Declaration {
	return extension.Declaration{
		Identifier: sourceName,
		Version:    declared.Package.Version,
		Contributions: extension.Contributions{
			CatalogueSources: []extension.CatalogueSource{{
				ID:   sourceID,
				Sync: "pull",
				Entries: []extension.CatalogueEntry{{
					ID:         entryID,
					Display:    extension.CatalogueDisplay{Name: entryID},
					Definition: declared,
				}},
			}},
		},
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

// stampAttribution writes the attribution marker the framework's own update path
// requires on the definition the catalogue created: the pinned catalogue sync
// writes none at creation, so republishing would be refused before the product
// reads anything.
func stampAttribution(t *testing.T, cluster *harness.Cluster) {
	t.Helper()
	resource := cluster.Dynamic.Resource(schema.GroupVersionResource{
		Group: api.Group, Version: v1beta1.GroupVersion.Version, Resource: "applicationdefinitions",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stored, err := resource.Get(ctx, entryID, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read definition %s: %v", entryID, err)
	}
	labels := stored.GetLabels()
	if labels[markers.ManagedBy] != "" {
		return
	}
	labels[markers.ManagedBy] = write.ManagedByValue
	stored.SetLabels(labels)
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("stamp definition %s: %v", entryID, err)
	}
}

// publish republishes the entry, writes the definition the way the catalogue
// sync does and stamps the attribution the next republish needs.
func publish(t *testing.T, cluster *harness.Cluster, shell *harness.Shell, artifacts extension.MemoryArtifacts, declared v1beta1.ApplicationDefinitionSpec) {
	t.Helper()
	shell.Use(cluster.UpgradePackage(t, sourceName, offering(declared), artifacts))
	shell.SyncCatalogue(t)
	stampAttribution(t, cluster)
}

// installedOf returns one application inside the served installed item.
func installedOf(t *testing.T, shell *harness.Shell, token, name string) map[string]any {
	t.Helper()
	for _, application := range support.Installed(t, shell, token) {
		if harness.String(application, "name") == name {
			return application
		}
	}
	t.Fatalf("the served page carries no installed application %q", name)
	return nil
}

// updateOf returns the update view one served application carries.
func updateOf(t *testing.T, application map[string]any) map[string]any {
	t.Helper()
	update, ok := application["update"].(map[string]any)
	if !ok {
		t.Fatalf("application %v carries no update value", application)
	}
	return update
}

// install installs one application from the served install entry.
func install(t *testing.T, shell *harness.Shell, token, name string) {
	t.Helper()
	method, path := harness.PageAction(t, appcenter.Page(), appcenter.InstallAction, name, harness.Namespace)
	manifest := harness.ApplicationManifest(name, harness.Namespace, entryID, targetNamespace, nil, nil)
	if status, body := shell.Call(t, method, path+"?namespace="+harness.Namespace, manifest, token); status != http.StatusAccepted {
		t.Fatalf("install %s = %d %v", name, status, body)
	}
}

// settle drives one application to Ready.
func settle(t *testing.T, cluster *harness.Cluster, controller *app.Controller, name string) {
	t.Helper()
	if phase := cluster.SettleApplication(t, controller, name, 5); phase != app.PhaseReady {
		t.Fatalf("phase of %s = %q, want %q", name, phase, app.PhaseReady)
	}
}

// TestTC168UpdateNoticeFollowsTheCatalogue installs one application behind the
// catalogue and one on the offered package, republishes the entry, and reads the
// served page with no user action in between: only the application behind the
// catalogue carries an update with the target version, and no page entry checks
// for updates (APPSTORE-AC-78, APPSTORE-C-80).
func TestTC168UpdateNoticeFollowsTheCatalogue(t *testing.T) {
	cluster := harness.Start(t)
	cluster.EnsureNamespace(t, targetNamespace)
	cluster.EnsureNamespace(t, api.SystemNamespace)

	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}
	shell.Use(cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention()))
	shell.Use(cluster.InstallPackageWith(t, sourceName,
		offering(definition(t, "1.0.0", "one")), artifacts, shell.Retention()))
	shell.SyncCatalogue(t)
	stampAttribution(t, cluster)

	install(t, shell, token, behindApp)
	controller := cluster.ApplicationController(t, app.Options{})
	settle(t, cluster, controller, behindApp)

	// No higher version is offered yet: neither application shows an update.
	if update := updateOf(t, installedOf(t, shell, token, behindApp)); update["available"] != false {
		t.Fatalf("update = %v, want none while the offered package is the installed one", update)
	}

	// The catalogue publishes a higher version and syncs; the page read that
	// follows is the only thing that happens (APPSTORE-C-80).
	publish(t, cluster, shell, artifacts, definition(t, "2.0.0", "two"))

	behind := updateOf(t, installedOf(t, shell, token, behindApp))
	if available, _ := behind["available"].(bool); !available {
		t.Fatalf("update = %v, want the offered 2.0.0 with no user action", behind)
	}
	if version := harness.String(behind, "version"); version != "2.0.0" {
		t.Fatalf("target version = %q, want %q", version, "2.0.0")
	}
	target := harness.String(behind, "digest")
	if target == "" || target == harness.String(behind, "installedDigest") {
		t.Fatalf("update = %v, want the offered digest beside the applied one", behind)
	}

	// The control: an application installed from the package the catalogue
	// offers now carries no update.
	install(t, shell, token, currentApp)
	settle(t, cluster, controller, currentApp)
	if update := updateOf(t, installedOf(t, shell, token, currentApp)); update["available"] != false {
		t.Fatalf("update = %v, want none for an application on the offered package", update)
	}
	// The application behind the catalogue still carries it, so the value is a
	// fact about each application rather than a page-wide flag.
	if update := updateOf(t, installedOf(t, shell, token, behindApp)); update["available"] != true {
		t.Fatalf("update = %v, want the still outstanding 2.0.0", update)
	}

	// There is no separate update-check entry: the page declares the two write
	// entries and no action or item asks to check, refresh or sync.
	ids := make([]string, 0, len(appcenter.Page().Actions))
	for _, action := range appcenter.Page().Actions {
		ids = append(ids, action.ID)
		lowered := strings.ToLower(action.ID + " " + action.Path)
		if strings.Contains(lowered, "check") || strings.Contains(lowered, "refresh") || strings.Contains(lowered, "sync") {
			t.Fatalf("the page declares an update-check entry: %+v", action)
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{appcenter.InstallAction, appcenter.UpgradeAction}) {
		t.Fatalf("page actions = %v, want only the install and upgrade entries", ids)
	}
	for _, item := range appcenter.Page().Items {
		if reference := strings.ToLower(item.Reference); strings.Contains(reference, "check") || strings.Contains(reference, "refresh") {
			t.Fatalf("the page declares an update-check item: %+v", item)
		}
	}
	// After a restart-free read the value is still the same: the update is a
	// projection of the catalogue, not a cached check result.
	if update := updateOf(t, installedOf(t, shell, token, behindApp)); harness.String(update, "version") != "2.0.0" {
		t.Fatalf("update = %v, want the same target on a repeated read", update)
	}
}

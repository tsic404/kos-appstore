// Package appstoretc165 verifies the upgrade entry and the upgrade preview of
// APPSTORE-AC-77. When the catalogue entry publishes a higher version with a
// changed config default, the served app-center page carries the target version
// without a user action, and the preview it presents is the framework's own
// preflight report — including the changed default as a config difference.
// Before the upgrade is requested the phase and the downstream object stay put
// (APPSTORE-C-79, KOSD-C-32, KOSD-C-34).
package appstoretc165

import (
	"context"
	"net/http"
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
	ownerName = "owner"
	ownerPass = "owner-password"
	component = "appstore"
	// sourceComponent is the fixture package that offers the entry under test.
	sourceComponent = "acceptance-catalogue"
	sourceID        = "acceptance-catalogue"
	// entryID names both the catalogue entry and the definition it writes.
	entryID = "web"
	appName = "site"
	// targetNamespace is where the package renders.
	targetNamespace = "apps"
)

// template renders the ConfigMap the package writes; the version it names is
// what the case reads downstream.
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

// modeField declares the config field whose default the upgrade changes.
func modeField(value string) []v1beta1.ConfigField {
	return []v1beta1.ConfigField{{Name: "mode", Type: app.TypeString, Default: &value}}
}

// definition builds one definition pinned to its content digest.
func definition(t *testing.T, version, rendered string, fields []v1beta1.ConfigField) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	declared := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: version},
		Template: template(rendered),
		Config:   fields,
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

// offering declares one catalogue entry per definition under the fixture source.
func offering(declared v1beta1.ApplicationDefinitionSpec) extension.Declaration {
	return extension.Declaration{
		Identifier: sourceComponent,
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

// updateOf returns the update view one served installed application carries.
func updateOf(t *testing.T, application map[string]any) map[string]any {
	t.Helper()
	update, ok := application["update"].(map[string]any)
	if !ok {
		t.Fatalf("application %v carries no update value", application)
	}
	return update
}

// install installs one application from the entry through the page's install
// action, so the write is the one the page declares.
func install(t *testing.T, shell *harness.Shell, token, name string) {
	t.Helper()
	method, path := harness.PageAction(t, appcenter.Page(), appcenter.InstallAction, name, harness.Namespace)
	manifest := harness.ApplicationManifest(name, harness.Namespace, entryID, targetNamespace, nil, nil)
	if status, body := shell.Call(t, method, path+"?namespace="+harness.Namespace, manifest, token); status != http.StatusAccepted {
		t.Fatalf("install %s = %d %v", name, status, body)
	}
}

// requestUpgrade submits the upgrade through the write the page's upgrade entry
// declares; the body accepts the target package's config.
func requestUpgrade(t *testing.T, cluster *harness.Cluster, shell *harness.Shell, token, name string, config map[string]any) {
	t.Helper()
	method, path := harness.PageAction(t, appcenter.Page(), appcenter.UpgradeAction, name, harness.Namespace)
	if method != http.MethodPut {
		t.Fatalf("upgrade action = %s, want a PUT", method)
	}
	body := harness.ApplicationManifest(name, harness.Namespace, entryID, targetNamespace, config, nil)
	body["metadata"] = map[string]any{
		"name":            name,
		"namespace":       harness.Namespace,
		"labels":          map[string]any{"managed-by": "kosd"},
		"resourceVersion": cluster.Get(t, "v1beta1", "Application", name).GetResourceVersion(),
	}
	if status, answer := shell.Call(t, method, path+"/"+name+"?namespace="+harness.Namespace, body, token); status != http.StatusAccepted {
		t.Fatalf("request the upgrade of %s = %d %v", name, status, answer)
	}
}

// stampAttribution writes the attribution marker the framework's own update path
// requires on the definition the catalogue created: the pinned catalogue sync
// writes none at creation, so a later republish would be refused before this
// product reads anything.
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

// publish offers one definition and writes it the way the catalogue sync does,
// then stamps the attribution the republish needs.
func publish(t *testing.T, cluster *harness.Cluster, shell *harness.Shell, artifacts extension.MemoryArtifacts, declared v1beta1.ApplicationDefinitionSpec) {
	t.Helper()
	shell.Use(cluster.UpgradePackage(t, sourceComponent, offering(declared), artifacts))
	shell.SyncCatalogue(t)
	stampAttribution(t, cluster)
}

// downstreamVersion reads the version the rendered ConfigMap carries.
func downstreamVersion(t *testing.T, cluster *harness.Cluster) string {
	t.Helper()
	configMap, err := cluster.Kube.CoreV1().ConfigMaps(targetNamespace).Get(context.Background(), appName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the downstream ConfigMap: %v", err)
	}
	return configMap.Data["version"]
}

// TestTC165UpgradeEntryAndPreview walks one upgrade: the catalogue publishes a
// higher version with a changed default, the served page gives the target
// version without touching the application, and the preview it presents is the
// framework's preflight report with that changed default in it
// (APPSTORE-AC-77, APPSTORE-C-79).
func TestTC165UpgradeEntryAndPreview(t *testing.T) {
	cluster := harness.Start(t)
	cluster.EnsureNamespace(t, targetNamespace)
	cluster.EnsureNamespace(t, api.SystemNamespace)

	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}
	shell.Use(cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention()))
	shell.Use(cluster.InstallPackageWith(t, sourceComponent,
		offering(definition(t, "1.0.0", "one", modeField("one"))), artifacts, shell.Retention()))
	shell.SyncCatalogue(t)
	stampAttribution(t, cluster)

	install(t, shell, token, appName)
	controller := cluster.ApplicationController(t, app.Options{})
	if phase := cluster.SettleApplication(t, controller, appName, 5); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want %q after the install", phase, app.PhaseReady)
	}
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("downstream version = %q, want %q", version, "one")
	}
	if update := updateOf(t, installedOf(t, shell, token, appName)); update["available"] != false {
		t.Fatalf("update = %v, want none while the offered package is the installed one", update)
	}

	// The catalogue publishes a higher version and changes the field default.
	publish(t, cluster, shell, artifacts, definition(t, "2.0.0", "two", modeField("two")))

	// The target version appears with no user action, and the application is
	// untouched: the entry is not the application (APPSTORE-C-79, APPSTORE-C-80).
	offeringList := installedOf(t, shell, token, appName)
	update := updateOf(t, offeringList)
	if available, _ := update["available"].(bool); !available {
		t.Fatalf("update = %v, want the offered 2.0.0", update)
	}
	if version := harness.String(update, "version"); version != "2.0.0" {
		t.Fatalf("target version = %q, want %q", version, "2.0.0")
	}
	targetDigest := harness.String(update, "digest")
	installedDigest := harness.String(update, "installedDigest")
	if targetDigest == "" || installedDigest == "" || targetDigest == installedDigest {
		t.Fatalf("update = %v, want the offered digest beside the applied one", update)
	}
	if phase := harness.String(offeringList, "phase"); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want the unchanged %q", phase, app.PhaseReady)
	}
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("downstream version = %q, want the unchanged %q", version, "one")
	}

	// Confirming the upgrade submits the page's declared write. The framework
	// then runs its preflight, which the page presents unchanged
	// (APPSTORE-C-79, KOSD-C-34).
	requestUpgrade(t, cluster, shell, token, appName, map[string]any{"mode": "two"})
	cluster.ApplicationRound(t, controller, appName)

	preflighted := installedOf(t, shell, token, appName)
	preview, ok := updateOf(t, preflighted)["preview"].(map[string]any)
	if !ok {
		t.Fatalf("update = %v, want the framework's preflight report", updateOf(t, preflighted))
	}
	if target := harness.String(preview, "target"); target != targetDigest {
		t.Fatalf("preview target = %q, want the offered digest %q", target, targetDigest)
	}
	changes, _ := preview["configChanges"].([]any)
	if len(changes) != 1 {
		t.Fatalf("config changes = %v, want only the changed default", preview["configChanges"])
	}
	change, _ := changes[0].(map[string]any)
	if field := harness.String(change, "field"); field != "mode" {
		t.Fatalf("config change = %v, want the mode field", change)
	}
	if from, to := harness.String(change, "from"), harness.String(change, "to"); from != `"one"` || to != `"two"` {
		t.Fatalf("config change = %v, want the default %s replaced by %s", change, `"one"`, `"two"`)
	}
	// The preflight writes no object, and the phase the page gives is the
	// application object's own phase (KOSD-C-33, KOSD-C-34).
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("downstream version = %q, want %q while only the preflight ran", version, "one")
	}
	if phase, stored := harness.String(preflighted, "phase"), cluster.ApplicationPhase(t, appName); phase != stored {
		t.Fatalf("served phase = %q, want the application object's %q", phase, stored)
	}

	// The apply step finishes the upgrade; the page then reports the settled
	// state and no outstanding update.
	if phase := cluster.SettleApplication(t, controller, appName, 5); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want %q after the upgrade", phase, app.PhaseReady)
	}
	if version := downstreamVersion(t, cluster); version != "two" {
		t.Fatalf("downstream version = %q, want the upgraded %q", version, "two")
	}
	if settled := updateOf(t, installedOf(t, shell, token, appName)); settled["available"] != false {
		t.Fatalf("update = %v, want none once the target is applied", settled)
	}
}

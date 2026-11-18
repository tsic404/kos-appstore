// Package appstoretc167 verifies the progress reading of APPSTORE-AC-77: the
// phase and the conditions the served page carries are the application object's
// own, no separate task object or progress state exists, and a finished upgrade
// reports its final phase and result whether it was applied or rolled back
// (APPSTORE-C-79, KOSD-C-33, APPSTORE-TC-167).
package appstoretc167

import (
	"context"
	"net/http"
	"slices"
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
	appName    = "site"
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
func definition(t *testing.T, version, rendered string, hooks []v1beta1.Hook) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	mode := rendered
	declared := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: version},
		Template: template(rendered),
		Hooks:    hooks,
		Config:   []v1beta1.ConfigField{{Name: "mode", Type: app.TypeString, Default: &mode}},
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

// install installs one application from the served install entry.
func install(t *testing.T, shell *harness.Shell, token, name string) {
	t.Helper()
	method, path := harness.PageAction(t, appcenter.Page(), appcenter.InstallAction, name, harness.Namespace)
	manifest := harness.ApplicationManifest(name, harness.Namespace, entryID, targetNamespace, nil, nil)
	if status, body := shell.Call(t, method, path+"?namespace="+harness.Namespace, manifest, token); status != http.StatusAccepted {
		t.Fatalf("install %s = %d %v", name, status, body)
	}
}

// requestUpgrade submits the target package through the write the page's upgrade
// entry declares; the spec write is the framework's upgrade trigger (KOSD-C-32).
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

// downstreamVersion reads the version the rendered ConfigMap carries.
func downstreamVersion(t *testing.T, cluster *harness.Cluster) string {
	t.Helper()
	configMap, err := cluster.Kube.CoreV1().ConfigMaps(targetNamespace).Get(context.Background(), appName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the downstream ConfigMap: %v", err)
	}
	return configMap.Data["version"]
}

// objectConditions reads the conditions the application object carries.
func objectConditions(t *testing.T, cluster *harness.Cluster) map[string]string {
	t.Helper()
	conditions := harness.Conditions(t, cluster.Get(t, "v1beta1", "Application", appName))
	view := make(map[string]string, len(conditions))
	for _, condition := range conditions {
		view[condition.Type] = string(condition.Status)
	}
	return view
}

// servedConditions reads the conditions the served installed item carries.
func servedConditions(t *testing.T, application map[string]any) map[string]string {
	t.Helper()
	listed, _ := application["conditions"].([]any)
	view := make(map[string]string, len(listed))
	for _, raw := range listed {
		condition, _ := raw.(map[string]any)
		view[harness.String(condition, "type")] = harness.String(condition, "status")
	}
	return view
}

// assertMatchesObject asserts the served progress is the object's own reading.
func assertMatchesObject(t *testing.T, cluster *harness.Cluster, served map[string]any) {
	t.Helper()
	if phase, stored := harness.String(served, "phase"), cluster.ApplicationPhase(t, appName); phase != stored {
		t.Fatalf("served phase = %q, want the application object's %q", phase, stored)
	}
	if got, want := servedConditions(t, served), objectConditions(t, cluster); !equalConditions(got, want) {
		t.Fatalf("served conditions = %v, want the application object's %v", got, want)
	}
}

// equalConditions compares two condition type/status readings.
func equalConditions(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for conditionType, status := range right {
		if left[conditionType] != status {
			return false
		}
	}
	return true
}

// assertNoTaskState asserts the item carries no second progress state: only the
// application object's own facts and the catalogue update.
func assertNoTaskState(t *testing.T, served map[string]any) {
	t.Helper()
	allowed := []string{"namespace", "name", "definition", "phase", "health", "conditions", "update"}
	keys := make([]string, 0, len(served))
	for key := range served {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	expected := append([]string(nil), allowed...)
	slices.Sort(expected)
	if !slices.Equal(keys, expected) {
		t.Fatalf("installed item carries %v, want only %v", keys, expected)
	}
	if items := len(appcenter.Page().Items); items != 3 {
		t.Fatalf("page items = %d, want only the entries, installed and upgrades reads", items)
	}
	for _, item := range appcenter.Page().Items {
		if item.ID == appcenter.InstalledItem {
			continue
		}
		if string(item.Source) != string(extension.PageSourceComputed) {
			t.Fatalf("page item %q = %+v, want a catalogue read", item.ID, item)
		}
	}
}

// TestTC167ProgressIsTheApplicationObject drives one applied upgrade and one
// rolled-back upgrade, reading the served page at every step: the phase and the
// conditions are always the application object's, and the finished upgrade shows
// its final phase with the result the framework recorded
// (APPSTORE-C-79, KOSD-C-33).
func TestTC167ProgressIsTheApplicationObject(t *testing.T) {
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
		offering(definition(t, "1.0.0", "one", nil)), artifacts, shell.Retention()))
	shell.SyncCatalogue(t)
	stampAttribution(t, cluster)

	install(t, shell, token, appName)
	controller := cluster.ApplicationController(t, app.Options{})
	if phase := cluster.SettleApplication(t, controller, appName, 5); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want %q after the install", phase, app.PhaseReady)
	}
	assertNoTaskState(t, installedOf(t, shell, token, appName))

	// The preflight round: the object is Upgrading, and the page shows exactly
	// that phase with the conditions the object carries (KOSD-C-33).
	publish(t, cluster, shell, artifacts, definition(t, "2.0.0", "two", nil))
	requestUpgrade(t, cluster, shell, token, appName, map[string]any{"mode": "two"})
	cluster.ApplicationRound(t, controller, appName)
	upgrading := installedOf(t, shell, token, appName)
	if phase := harness.String(upgrading, "phase"); phase != app.PhaseUpgrading {
		t.Fatalf("served phase = %q, want %q while the preflight ran", phase, app.PhaseUpgrading)
	}
	assertMatchesObject(t, cluster, upgrading)
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("downstream version = %q, want the unchanged %q during the preflight", version, "one")
	}

	// The apply round finishes it: the page shows the final phase and the
	// conditions the object wrote, with the new object applied.
	if phase := cluster.SettleApplication(t, controller, appName, 5); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want %q after the upgrade", phase, app.PhaseReady)
	}
	settled := installedOf(t, shell, token, appName)
	assertMatchesObject(t, cluster, settled)
	if conditions := servedConditions(t, settled); conditions[api.ConditionReconciled] != "True" {
		t.Fatalf("served conditions = %v, want the reconciled result the object carries", conditions)
	}
	if version := downstreamVersion(t, cluster); version != "two" {
		t.Fatalf("downstream version = %q, want the upgraded %q", version, "two")
	}

	// A target whose upgrade hook fails is rolled back. The page shows the
	// result the object recorded: back at Ready with the failed attempt named,
	// and the previous object still rendered (KOSD-C-33, KOSD-C-34).
	failing := []v1beta1.Hook{{Name: app.HookUpgrade, Command: []string{"sh", "-c", "exit 1"}}}
	publish(t, cluster, shell, artifacts, definition(t, "3.0.0", "three", failing))
	requestUpgrade(t, cluster, shell, token, appName, map[string]any{"mode": "three"})
	if phase := cluster.SettleApplication(t, controller, appName, 6); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want %q after the rollback", phase, app.PhaseReady)
	}
	rolled := installedOf(t, shell, token, appName)
	assertMatchesObject(t, cluster, rolled)
	conditions := servedConditions(t, rolled)
	if conditions[app.ConditionUpgradeFailed] != "True" {
		t.Fatalf("served conditions = %v, want the failed upgrade the object carries", conditions)
	}
	update, ok := rolled["update"].(map[string]any)
	if !ok {
		t.Fatalf("application %v carries no update value", rolled)
	}
	preview, ok := update["preview"].(map[string]any)
	if !ok {
		t.Fatalf("update = %v, want the framework's report for the reverted target", update)
	}
	if reverted, _ := preview["rolledBack"].(bool); !reverted {
		t.Fatalf("preview = %v, want the reverted result", preview)
	}
	if version := downstreamVersion(t, cluster); version != "two" {
		t.Fatalf("downstream version = %q, want the previous %q after the rollback", version, "two")
	}
}

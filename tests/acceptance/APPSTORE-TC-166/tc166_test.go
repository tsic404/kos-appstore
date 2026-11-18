// Package appstoretc166 verifies the refused upgrade of APPSTORE-AC-77: a
// package that changes the data format is refused by the framework's preflight,
// the served page names the irreversible item and the condition it wrote, the
// phase and the downstream object stay where they were, and neither the page nor
// the control plane offers a path that bypasses the refusal
// (APPSTORE-C-79, KOSD-C-34, APPSTORE-TC-166).
package appstoretc166

import (
	"context"
	"net/http"
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
	"github.com/tsic404/kosd/pkg/state"
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
	// dataFormat is the format the higher version declares.
	dataFormat = "v2"
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
func definition(t *testing.T, version, format, rendered, mode string) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	declared := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: version, DataFormat: format},
		Template: template(rendered),
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

// publish republishes the entry and writes the definition the way the catalogue
// sync does, then stamps the attribution the next republish needs.
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

// conditionOf returns one condition of the application object.
func conditionOf(t *testing.T, cluster *harness.Cluster, conditionType string) *metav1.Condition {
	t.Helper()
	conditions := harness.Conditions(t, cluster.Get(t, "v1beta1", "Application", appName))
	condition, present := state.Find(conditions, conditionType)
	if !present {
		return nil
	}
	return &condition
}

// TestTC166IrreversibleUpgradeIsRefusedWithoutABypass republishes the entry with
// a changed data format, requests the upgrade and reads the served page: the
// framework's report names the irreversible item, the phase and the downstream
// object are unchanged, and no served or API path carries a confirmation that
// would bypass the refusal (APPSTORE-AC-77, APPSTORE-TC-166).
func TestTC166IrreversibleUpgradeIsRefusedWithoutABypass(t *testing.T) {
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
		offering(definition(t, "1.0.0", "", "one", "one")), artifacts, shell.Retention()))
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

	// The higher version declares a data format; the served page gives that
	// target before anything is requested (APPSTORE-C-79).
	publish(t, cluster, shell, artifacts, definition(t, "2.0.0", dataFormat, "two", "two"))
	before := installedOf(t, shell, token, appName)
	update, ok := before["update"].(map[string]any)
	if !ok {
		t.Fatalf("application %v carries no update value", before)
	}
	if available, _ := update["available"].(bool); !available {
		t.Fatalf("update = %v, want the offered 2.0.0", update)
	}
	if version := harness.String(update, "version"); version != "2.0.0" {
		t.Fatalf("target version = %q, want %q", version, "2.0.0")
	}

	requestUpgrade(t, cluster, shell, token, appName, map[string]any{"mode": "two"})
	cluster.ApplicationRound(t, controller, appName)

	// The framework refused it: the phase and the downstream object stay on the
	// installed version (APPSTORE-C-79, KOSD-C-34).
	if phase := cluster.ApplicationPhase(t, appName); phase != app.PhaseReady {
		t.Fatalf("phase = %q, want the pre-upgrade %q", phase, app.PhaseReady)
	}
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("downstream version = %q, want the unchanged %q", version, "one")
	}

	// The reason travels as the framework wrote it: the report names the
	// irreversible item, and the condition names the same kind
	// (APPSTORE-C-79, KOSD-C-34).
	refused := installedOf(t, shell, token, appName)
	if phase := harness.String(refused, "phase"); phase != app.PhaseReady {
		t.Fatalf("served phase = %q, want the application object's %q", phase, app.PhaseReady)
	}
	preview, ok := refused["update"].(map[string]any)["preview"].(map[string]any)
	if !ok {
		t.Fatalf("application %v carries no preflight report", refused)
	}
	items, _ := preview["irreversible"].([]any)
	if len(items) == 0 {
		t.Fatalf("the preview reported no irreversible item: %v", preview)
	}
	item, _ := items[0].(map[string]any)
	if kind := harness.String(item, "kind"); kind != v1beta1.IrreversibleDataFormat {
		t.Fatalf("irreversible item = %v, want the data format change", item)
	}
	if detail := harness.String(item, "detail"); detail == "" {
		t.Fatalf("irreversible item = %v, want the reason the upgrade cannot run", item)
	}
	condition := conditionOf(t, cluster, app.ConditionIrreversibleUpgrade)
	if condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("the irreversible condition is missing or not True: %+v", condition)
	}
	if !strings.Contains(condition.Message, v1beta1.IrreversibleDataFormat) {
		t.Fatalf("condition message = %q, want the irreversible kind named", condition.Message)
	}
	served := conditionsOf(t, refused)
	if !carries(served, app.ConditionIrreversibleUpgrade, metav1.ConditionTrue) {
		t.Fatalf("served conditions = %v, want the irreversible refusal the object carries", served)
	}

	// No path bypasses the refusal: the confirmation the external-change pause
	// would accept is refused, the page declares no confirmation entry, and the
	// application is not writable through a second upgrade request shape.
	if status, body := shell.Call(t, http.MethodPost, "/api/v1/action-requests", map[string]any{
		"kind":         "Application",
		"namespace":    harness.Namespace,
		"name":         appName,
		"action":       app.ActionConfirm,
		"targetDigest": preview["target"],
	}, token); status != http.StatusBadRequest {
		t.Fatalf("a confirmation bypassed the refusal = %d %v", status, body)
	}
	for _, action := range appcenter.Page().Actions {
		if action.ID == app.ActionConfirm || strings.Contains(strings.ToLower(action.Path), "confirm") {
			t.Fatalf("the page declares a confirmation path: %+v", action)
		}
	}
	if actions := len(appcenter.Page().Actions); actions != 2 {
		t.Fatalf("page actions = %d, want only the install and upgrade entries", actions)
	}

	// Repeated rounds keep the refusal: nothing applies the refused target.
	for range 3 {
		cluster.ApplicationRound(t, controller, appName)
	}
	if phase := cluster.ApplicationPhase(t, appName); phase != app.PhaseReady {
		t.Fatalf("phase = %q after repeated rounds, want %q", phase, app.PhaseReady)
	}
	if version := downstreamVersion(t, cluster); version != "one" {
		t.Fatalf("a repeated round applied the refused target: downstream version %q", version)
	}
	if name := harness.String(installedOf(t, shell, token, appName), "name"); name != appName {
		t.Fatalf("installed application = %q, want %q", name, appName)
	}
}

// conditionsOf renders the served conditions as type/status pairs.
func conditionsOf(t *testing.T, application map[string]any) map[string]string {
	t.Helper()
	listed, _ := application["conditions"].([]any)
	conditions := make(map[string]string, len(listed))
	for _, raw := range listed {
		condition, _ := raw.(map[string]any)
		conditions[harness.String(condition, "type")] = harness.String(condition, "status")
	}
	return conditions
}

// carries reports whether one served condition carries the expected status.
func carries(conditions map[string]string, conditionType string, status metav1.ConditionStatus) bool {
	return conditions[conditionType] == string(status)
}

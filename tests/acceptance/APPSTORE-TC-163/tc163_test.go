// Package appstoretc163 verifies the permission display of APPSTORE-AC-75 on
// the page value the window actually receives: after the appstore package and a
// window source package install, GET /api/v1/pages/app-center carries the
// capability set the package declares for the entry's window application, an
// entry whose application declares none carries an empty set, a capability
// nothing declared never appears, and no permission grant entry exists
// (APPSTORE-AC-75, APPSTORE-TC-163).
package appstoretc163

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/appcenter"
	"github.com/tsic404/kos-appstore/declaration"
	"github.com/tsic404/kos-appstore/tests/acceptance/support"
)

const (
	extensionID = "acceptance.windowstore"
	sourceID    = "acceptance-window-catalogue"
	// notifyEntry installs the window application that declares notifications.
	notifyEntry = "notify"
	// plainEntry installs the window application that declares no capability.
	plainEntry = "plain"
	ownerName  = "owner"
	ownerPass  = "owner-password"
	// planPath is the framework plan request the confirmation preview reads
	// (KOSD-C-62).
	planPath = "/api/v1/plan-requests"
)

// template renders one ConfigMap in the declared namespace.
const template = `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
data:
  mode: "acceptance"
`

// definition builds one entry's definition with its digest computed from the
// content, as C-32 requires.
func definition(t *testing.T) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	spec := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: "1.0.0"},
		Template: template,
	}
	digest, err := app.DefinitionDigest(spec)
	if err != nil {
		t.Fatalf("digest the definition: %v", err)
	}
	spec.Package.Digest = digest
	if err := app.ValidateDefinition(spec); err != nil {
		t.Fatalf("the definition is invalid: %v", err)
	}
	return spec
}

// fixtureDeclaration offers both window applications from one source.
func fixtureDeclaration(spec v1beta1.ApplicationDefinitionSpec) extension.Declaration {
	return extension.Declaration{
		Identifier: extensionID,
		Version:    "1.0.0",
		Contributions: extension.Contributions{
			CatalogueSources: []extension.CatalogueSource{{
				ID:   sourceID,
				Sync: "pull",
				Entries: []extension.CatalogueEntry{
					{ID: notifyEntry, Display: extension.CatalogueDisplay{Name: "Notify"}, Definition: spec},
					{ID: plainEntry, Display: extension.CatalogueDisplay{Name: "Plain"}, Definition: spec},
				},
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

// windowApp declares one window application of the offering package. The
// identity is the entry's, so the package declares the application that entry
// installs (KOSD-C-73).
func windowApp(id string, permissions []manifest.Permission) manifest.AppManifest {
	app := appcenter.App("1.0.0")
	app.ID = id
	app.Name = id
	app.Permissions = permissions
	return app
}

// permissionNames decodes one entry's permission list.
func permissionNames(t *testing.T, entry map[string]any) []string {
	t.Helper()
	listed, ok := entry["permissions"].([]any)
	if !ok {
		t.Fatalf("entry = %v, want a permission list", entry["permissions"])
	}
	names := make([]string, 0, len(listed))
	for _, raw := range listed {
		names = append(names, raw.(string))
	}
	return names
}

// TestTC163PermissionSetsAtDetailAndConfirmation checks the served page value
// both views render against the package declaration, then installs each entry
// and reads the same value again.
func TestTC163PermissionSetsAtDetailAndConfirmation(t *testing.T) {
	cluster := harness.Start(t)
	artifacts := extension.MemoryArtifacts{}

	// The package declares two window applications: one with notifications, one
	// with no capability. A third declaration nothing installs must never leak.
	apps := []manifest.AppManifest{
		windowApp(notifyEntry, []manifest.Permission{manifest.PermissionNotifications}),
		windowApp(plainEntry, []manifest.Permission{}),
		windowApp("unoffered", []manifest.Permission{manifest.PermissionExternalLink}),
	}
	configure, bind := support.Router(cluster, apps)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)

	appstoreInstall := cluster.InstallPackageWith(t, "appstore", shipped(t), artifacts, shell.Retention())
	shell.Use(appstoreInstall)
	installation := cluster.InstallPackageWith(t, "window-source", fixtureDeclaration(definition(t)), artifacts, shell.Retention())
	shell.Use(installation)
	shell.SyncCatalogue(t)

	// The detail shows the declared set of the application the entry installs,
	// and nothing for the entry whose application declares none (APPSTORE-C-77).
	notified := permissionNames(t, support.Offer(t, shell, token, notifyEntry))
	if len(notified) != 1 || notified[0] != "notifications" {
		t.Fatalf("entry %s permissions = %v, want only notifications", notifyEntry, notified)
	}
	if plain := permissionNames(t, support.Offer(t, shell, token, plainEntry)); len(plain) != 0 {
		t.Fatalf("entry %s permissions = %v, want none", plainEntry, plain)
	}

	// The install confirmation renders the same served value; the preview it
	// reads is the plan request and adds no capability.
	status, planned := shell.Call(t, http.MethodPost, planPath, map[string]any{
		"objects": []any{harness.ApplicationManifest("notified", harness.Namespace, notifyEntry, harness.Namespace, nil, nil)},
	}, token)
	if status != http.StatusOK {
		t.Fatalf("plan the install = %d %v", status, planned)
	}
	encoded, err := json.Marshal(planned)
	if err != nil {
		t.Fatalf("encode the plan answer: %v", err)
	}
	undeclared := []string{"clipboard-read", "clipboard-write", "file-selection", "external-link"}
	for _, name := range undeclared {
		if strings.Contains(string(encoded), name) {
			t.Fatalf("the confirmation preview carried an undeclared capability %q: %s", name, encoded)
		}
	}

	// Both applications install from their entries, and the set the page serves
	// is unchanged by the install.
	method, installPath := harness.PageAction(t, appcenter.Page(), appcenter.InstallAction, "notified", harness.Namespace)
	for name, entry := range map[string]string{"notified": notifyEntry, "plain-app": plainEntry} {
		manifest := harness.ApplicationManifest(name, harness.Namespace, entry, harness.Namespace, nil, nil)
		if status, accepted := shell.Call(t, method, installPath+"?namespace="+harness.Namespace, manifest, token); status != http.StatusAccepted {
			t.Fatalf("install %s = %d %v", name, status, accepted)
		}
	}
	if notified := permissionNames(t, support.Offer(t, shell, token, notifyEntry)); len(notified) != 1 || notified[0] != "notifications" {
		t.Fatalf("entry %s permissions after the install = %v, want only notifications", notifyEntry, notified)
	}
	if plain := permissionNames(t, support.Offer(t, shell, token, plainEntry)); len(plain) != 0 {
		t.Fatalf("entry %s permissions after the install = %v, want none", plainEntry, plain)
	}

	// No capability nothing declared appears anywhere in the served value, and
	// no grant entry exists: the page declares only the install and upgrade
	// writes, and neither names a permission (KOSD-C-75, APPSTORE-C-77).
	rendered, err := json.Marshal(support.Entries(t, shell, token))
	if err != nil {
		t.Fatalf("encode the served entries item: %v", err)
	}
	for _, name := range undeclared {
		if strings.Contains(string(rendered), name) {
			t.Fatalf("the served entries value carried an undeclared capability %q: %s", name, rendered)
		}
	}
	status, body := shell.Call(t, http.MethodGet, "/api/v1/pages/"+appcenter.PageID, nil, token)
	if status != http.StatusOK {
		t.Fatalf("read the app-center page = %d %v", status, body)
	}
	actions, _ := body["actions"].([]any)
	if len(actions) != 2 {
		t.Fatalf("page actions = %v, want only the install and upgrade writes", actions)
	}
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		label := strings.ToLower(harness.String(action, "id") + " " + harness.String(action, "entry", "path"))
		for _, forbidden := range []string{"permission", "grant", "revoke"} {
			if strings.Contains(label, forbidden) {
				t.Fatalf("the page offers a permission grant entry: %v", action)
			}
		}
	}

	// The route is what carries the set: a shell wired without this product's
	// reader cannot serve the page at all, so an unwired value never passes.
	unwired := cluster.StartShell(t)
	unwired.Use(installation)
	unwiredToken := unwired.SignInOwner(t, ownerName, ownerPass)
	status, body = unwired.Call(t, http.MethodGet, "/api/v1/pages/"+appcenter.PageID, nil, unwiredToken)
	if status == http.StatusOK {
		t.Fatalf("the page rendered without the product route: %v", body)
	}
	if code := harness.String(body, "error", "code"); code != "capability_missing" && code != "internal" {
		t.Fatalf("unwired page read = %d %v, want the source refused", status, body)
	}
}

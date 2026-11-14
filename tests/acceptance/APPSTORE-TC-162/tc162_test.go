// Package appstoretc162 verifies the install confirmation and execution of
// APPSTORE-AC-74: the confirmation preview is the framework plan request's
// answer and writes nothing, a confirmed install goes through the control-plane
// API and creates exactly the objects the preview listed, the application's
// owner is the installing session account, and no entry names another owner
// (APPSTORE-AC-74, APPSTORE-TC-162).
package appstoretc162

import (
	"net/http"
	"slices"
	"testing"

	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/write"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/appcenter"
	"github.com/tsic404/kos-appstore/declaration"
	"github.com/tsic404/kos-appstore/tests/acceptance/support"
)

const (
	ownerName  = "owner"
	ownerPass  = "owner-password"
	memberName = "alice"
	memberPass = "alice-password"
	// entryID is the shipped catalogue entry the member installs from.
	entryID = "redis"
	// appName is the application the install creates.
	appName = "cache"
	// component is the system component the appstore installs as.
	component = "appstore"
	// planPath is the framework plan request the confirmation preview reads
	// (KOSD-C-62).
	planPath = "/api/v1/plan-requests"
)

// shipped returns the declaration the appstore package ships.
func shipped(t *testing.T) extension.Declaration {
	t.Helper()
	declared, err := declaration.Declaration()
	if err != nil {
		t.Fatalf("build the appstore declaration: %v", err)
	}
	return declared
}

// refOf renders one plan entry's object reference for comparison.
func refOf(t *testing.T, planned any) string {
	t.Helper()
	entry, _ := planned.(map[string]any)
	ref, _ := entry["ref"].(map[string]any)
	if ref == nil {
		t.Fatalf("plan entry = %v, want an object reference", planned)
	}
	return harness.Reference(ref)
}

// listedRefs returns the object references one application listing carries.
func listedRefs(t *testing.T, body map[string]any) []string {
	t.Helper()
	refs := []string{}
	for _, item := range harness.Items(t, body) {
		ref, _ := item["ref"].(map[string]any)
		refs = append(refs, harness.Reference(ref))
	}
	return refs
}

// sortedUnique keeps the comparison a set comparison.
func sortedUnique(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

// TestTC162PreviewConfirmAndOwner walks the confirmation flow as a member: read
// the preview, submit the install, then compare the created set with the
// preview and check the owner.
func TestTC162PreviewConfirmAndOwner(t *testing.T) {
	cluster := harness.Start(t)
	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}
	installed := cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention())
	shell.Use(installed)
	shell.SyncCatalogue(t)

	// The entry the member installs from is offered and installable (C-24).
	status, body := shell.Call(t, http.MethodGet, "/api/v1/catalogue/entries", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list entries = %d %v", status, body)
	}
	offered := false
	for _, entry := range harness.Items(t, body) {
		if harness.String(entry, "id") != entryID {
			continue
		}
		offered = true
		if installable, _ := entry["installable"].(bool); !installable {
			t.Fatalf("entry = %v, want the installable offer", entry)
		}
	}
	if !offered {
		t.Fatalf("the catalogue offers no entry %q", entryID)
	}

	// The page declares the install write the confirmation submits: the
	// control-plane create of the application (APPSTORE-C-76).
	status, body = shell.Call(t, http.MethodGet, "/api/v1/pages/"+appcenter.PageID, nil, token)
	if status != http.StatusOK {
		t.Fatalf("read the app-center page = %d %v", status, body)
	}
	method, installPath := harness.PageAction(t, appcenter.Page(), appcenter.InstallAction, appName, harness.Namespace)
	if method != http.MethodPost || installPath != appcenter.ApplicationPath {
		t.Fatalf("install action = %s %s, want the create request to %s", method, installPath, appcenter.ApplicationPath)
	}

	// The member installs; the shell's own owner is not the installing account.
	system := cluster.StartAccess(t, nil, nil)
	member := system.Account(t, system.Owner(t, ownerName, ownerPass), memberName, memberPass, write.RoleMember)

	manifest := harness.ApplicationManifest(appName, harness.Namespace, entryID, harness.Namespace,
		map[string]any{"replicas": 2, "maxMemory": "64mb"}, nil)

	// The preview is the plan request's answer and writes nothing (C-62).
	status, planned := system.Call(t, member, http.MethodPost, planPath, map[string]any{"objects": []any{manifest}}, nil)
	if status != http.StatusOK {
		t.Fatalf("plan the install = %d %v", status, planned)
	}
	creates, _ := harness.Nested(planned, "plan", "create").([]any)
	if len(creates) != 1 {
		t.Fatalf("plan create = %v, want the one application the install creates", creates)
	}
	previewed := []string{refOf(t, creates[0])}
	status, body = system.Call(t, member, http.MethodGet, "/api/v1/resources/Application?namespace="+harness.Namespace, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list applications after the plan = %d %v", status, body)
	}
	if items := harness.Items(t, body); len(items) != 0 {
		t.Fatalf("applications after the plan = %v, want none: the preview writes nothing", items)
	}

	// Confirming submits the install through the control-plane API.
	status, accepted := system.Call(t, member, method, installPath+"?namespace="+harness.Namespace, manifest, nil)
	if status != http.StatusAccepted {
		t.Fatalf("install the entry = %d %v", status, accepted)
	}

	// The created object set is the previewed set, and the application belongs
	// to the installing account (C-60).
	status, body = system.Call(t, member, http.MethodGet, "/api/v1/resources/Application?namespace="+harness.Namespace, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list applications = %d %v", status, body)
	}
	created := sortedUnique(listedRefs(t, body))
	if len(created) != len(previewed) || created[0] != previewed[0] {
		t.Fatalf("created objects = %v, want the previewed %v", created, previewed)
	}
	stored := cluster.Get(t, "v1beta1", "Application", appName)
	if owner := harness.String(stored.Object, "spec", "owner"); owner != memberName {
		t.Fatalf("spec.owner = %q, want the installing account %q", owner, memberName)
	}

	// A submitted owner never stands: the API writes the session's account, so
	// there is no entry that names another owner (C-60).
	second := harness.ApplicationManifest("cache-two", harness.Namespace, entryID, harness.Namespace,
		map[string]any{"replicas": 1, "maxMemory": "32mb"}, nil)
	second["spec"].(map[string]any)["owner"] = ownerName
	status, accepted = system.Call(t, member, method, installPath+"?namespace="+harness.Namespace, second, nil)
	if status != http.StatusAccepted {
		t.Fatalf("install with a submitted owner = %d %v", status, accepted)
	}
	other := cluster.Get(t, "v1beta1", "Application", "cache-two")
	if owner := harness.String(other.Object, "spec", "owner"); owner != memberName {
		t.Fatalf("spec.owner = %q, want the submitted value to be discarded in favour of %q", owner, memberName)
	}

	// The install form is generated from the entry's config structure and
	// carries no owner field: the only owner entry is the session's account
	// (APPSTORE-C-31, KOSD-C-60).
	offer := support.Offer(t, shell, token, entryID)
	fields, _ := offer["config"].([]any)
	for _, raw := range fields {
		field, _ := raw.(map[string]any)
		if name := harness.String(field, "name"); name == "owner" {
			t.Fatalf("the install form offers an owner field: %v", field)
		}
	}
}

// Package appstoretc164 verifies the app-center visibility of APPSTORE-AC-76:
// the installed applications and the upgrade entry carry only the applications
// the reading session may see (KOSD-C-59), while the catalogue entry list is the
// same for every identity because an entry is not an application
// (APPSTORE-C-78, APPSTORE-TC-164).
package appstoretc164

import (
	"net/http"
	"slices"
	"testing"

	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/write"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/declaration"
	"github.com/tsic404/kos-appstore/tests/acceptance/support"
)

const (
	ownerName    = "owner"
	ownerPass    = "owner-password"
	listedName   = "bob"
	listedPass   = "bob-password"
	unlistedName = "alice"
	unlistedPass = "alice-password"
	guestName    = "gus"
	guestPass    = "gus-password"
	component    = "appstore"
	// mineApp is owned by the unlisted member; the other two are owned by the
	// global Owner and differ in visibility.
	mineApp   = "mine"
	sharedApp = "shared"
	openApp   = "open"
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

// created installs one application through the control-plane API, with the
// visibility the case declares. The API stamps the owner from the session
// (KOSD-C-60), so each application really belongs to the account that created
// it.
func created(t *testing.T, system *harness.Access, token, name, definition string, visibility map[string]any) {
	t.Helper()
	manifest := harness.ApplicationManifest(name, harness.Namespace, definition, harness.Namespace, nil, nil)
	if visibility != nil {
		spec, _ := manifest["spec"].(map[string]any)
		spec["visibility"] = visibility
	}
	status, body := system.Call(t, token, http.MethodPost,
		"/api/v1/resources/Application?namespace="+harness.Namespace, manifest, nil)
	if status != http.StatusAccepted {
		t.Fatalf("install application %s = %d %v", name, status, body)
	}
}

// namesOf returns the application names one served list carries, sorted.
func namesOf(values []map[string]any) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, harness.String(value, "name"))
	}
	slices.Sort(names)
	return names
}

// idsOf returns the entry identities one served list carries, sorted.
func idsOf(values []map[string]any) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, harness.String(value, "id"))
	}
	slices.Sort(ids)
	return ids
}

// TestTC164VisibilityAcrossFourIdentities reads the app-center page as the
// Owner, an unlisted member, a listed member and a guest. The installed
// applications and the upgrade entry follow KOSD-C-59; the entry list does not
// depend on who reads it (APPSTORE-C-78, APPSTORE-TC-164).
func TestTC164VisibilityAcrossFourIdentities(t *testing.T) {
	cluster := harness.Start(t)
	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	owner := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}
	installed := cluster.InstallPackageWith(t, component, shipped(t), artifacts, shell.Retention())
	shell.Use(installed)
	shell.SyncCatalogue(t)

	// The page's identities come from the shell's account surface.
	unlisted := shell.Account(t, owner, unlistedName, unlistedPass, write.RoleMember)
	listed := shell.Account(t, owner, listedName, listedPass, write.RoleMember)
	guest := shell.Account(t, owner, guestName, guestPass, write.RoleGuest)

	// The applications are created through the control plane so spec.owner is
	// the creating account (KOSD-C-60): the unlisted member owns one, the
	// global Owner owns two with different visibility.
	system := cluster.StartAccess(t, nil, nil)
	systemOwner := system.Owner(t, ownerName, ownerPass)
	systemUnlisted := system.Account(t, systemOwner, unlistedName, unlistedPass, write.RoleMember)
	created(t, system, systemUnlisted, mineApp, "redis", nil)
	created(t, system, systemOwner, sharedApp, "memcached", map[string]any{"members": []any{listedName}})
	created(t, system, systemOwner, openApp, "nginx", map[string]any{"allowGuests": true})

	want := map[string]struct {
		installed []string
		upgrades  []string
	}{
		ownerName:    {installed: []string{mineApp, openApp, sharedApp}, upgrades: []string{mineApp, openApp, sharedApp}},
		unlistedName: {installed: []string{mineApp}, upgrades: []string{mineApp}},
		listedName:   {installed: []string{sharedApp}, upgrades: []string{sharedApp}},
		guestName:    {installed: []string{openApp}, upgrades: []string{openApp}},
	}
	tokens := map[string]string{ownerName: owner, unlistedName: unlisted, listedName: listed, guestName: guest}

	entries := idsOf(support.Entries(t, shell, owner))
	if len(entries) == 0 {
		t.Fatal("the catalogue offers no entry; the case needs the shipped entries")
	}
	for _, name := range []string{ownerName, unlistedName, listedName, guestName} {
		token := tokens[name]
		expected := want[name]
		if got := namesOf(support.Installed(t, shell, token)); !slices.Equal(got, expected.installed) {
			t.Fatalf("installed applications seen by %s = %v, want %v", name, got, expected.installed)
		}
		if got := namesOf(support.Upgrades(t, shell, token)); !slices.Equal(got, expected.upgrades) {
			t.Fatalf("upgrade entry seen by %s = %v, want %v", name, got, expected.upgrades)
		}
		// An entry is not an application: the same list reaches every identity
		// (APPSTORE-C-78).
		if got := idsOf(support.Entries(t, shell, token)); !slices.Equal(got, entries) {
			t.Fatalf("catalogue entries seen by %s = %v, want %v", name, got, entries)
		}
	}
}

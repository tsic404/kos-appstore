// Package appstoretc9 verifies that the catalogue source the appstore
// contributes offers entries with display information, version, dependencies,
// install parameters and a digest, that installing one produces an application
// and its definition, and that a package whose content no longer matches the
// digest cannot be installed (APPSTORE-AC-4, APPSTORE-TC-9).
package appstoretc9

import (
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	appcatalogue "github.com/tsic404/kos-appstore/catalogue"
	"github.com/tsic404/kos-appstore/declaration"
	"github.com/tsic404/kos-appstore/tests/acceptance/support"
)

const (
	ownerName = "owner"
	ownerPass = "owner-password"
	// installedName is the application installed from the shipped redis entry.
	installedName = "cache"
	// installedDefinition is the entry the application is installed from.
	installedDefinition = "redis"
)

// TestTC9CatalogueEntrySyncAndDigest reads the shipped entries, installs one,
// then changes a definition's content so its digest no longer matches
// (APPSTORE-TC-9).
func TestTC9CatalogueEntrySyncAndDigest(t *testing.T) {
	cluster := harness.Start(t)
	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}
	installed := cluster.InstallPackageWith(t, "appstore", shipped(t), artifacts, shell.Retention())
	shell.Use(installed)
	shell.SyncCatalogue(t)

	entries := listed(t, shell, token)
	if len(entries) == 0 {
		t.Fatal("the catalogue lists no entry after the appstore installed")
	}
	// Every entry answers the read model of APPSTORE-C-24: display
	// information, version, dependencies, install parameters and a digest, and
	// the shipped entries install.
	for _, entry := range entries {
		if name := harness.String(entry, "name"); name == "" {
			t.Fatalf("entry = %v, want display information", entry)
		}
		if version := harness.String(entry, "version"); version == "" {
			t.Fatalf("entry = %v, want a version", entry)
		}
		if digest := harness.String(entry, "digest"); digest == "" {
			t.Fatalf("entry = %v, want a package digest", entry)
		}
		config, _ := entry["config"].([]any)
		if len(config) == 0 {
			t.Fatalf("entry = %v, want the install parameter structure", entry)
		}
		if installable, _ := entry["installable"].(bool); !installable {
			t.Fatalf("entry = %v, want installable: the shipped package matches its digest", entry)
		}
	}
	redis := named(t, entries, installedDefinition)
	if dependencies, _ := redis["dependencies"].([]any); len(dependencies) != 0 {
		t.Fatalf("dependencies = %v, want the entry's declared requirements", redis["dependencies"])
	}

	// The sync wrote the definition, so installing the entry creates an
	// application the definition renders.
	definitionRef := harness.String(redis, "definition")
	stored := cluster.Get(t, "v1beta1", "ApplicationDefinition", definitionRef)
	originalTemplate := templateOf(t, stored)
	manifest := harness.ApplicationManifest(installedName, harness.Namespace, definitionRef, harness.Namespace, nil, nil)
	status, body := shell.Call(t, http.MethodPost, "/api/v1/resources/Application", manifest, token)
	if status != http.StatusAccepted {
		t.Fatalf("install the entry = %d %v", status, body)
	}
	cluster.Get(t, "v1beta1", "Application", installedName)

	// The page read lays the install form out of the same structure, so the
	// window and the page show one entry (APPSTORE-C-31, KOSD-C-32).
	offer := support.Offer(t, shell, token, installedDefinition)
	fields, _ := offer["config"].([]any)
	if len(fields) == 0 {
		t.Fatalf("page offer = %v, want the generated install form", offer)
	}
	for _, raw := range fields {
		field, _ := raw.(map[string]any)
		if name := harness.String(field, "name"); name == "" {
			t.Fatalf("form field = %v, want the declared field", field)
		}
		if control := harness.String(field, "control"); control == "" {
			t.Fatalf("form field = %v, want the input the shell lays out", field)
		}
	}

	// A package whose content changed without its digest cannot be installed,
	// and the definition keeps the content it was installed from.
	changed := shipped(t)
	source := &changed.Contributions.CatalogueSources[0]
	for index := range source.Entries {
		if source.Entries[index].ID != installedDefinition {
			continue
		}
		source.Entries[index].Definition.Template = originalTemplate + "data:\n  changed: \"yes\"\n"
	}
	content, err := extension.Marshal(changed)
	if err != nil {
		t.Fatalf("marshal the changed declaration: %v", err)
	}
	artifacts["test://appstore-changed"] = content
	cluster.RepointComponent(t, "appstore", "test://appstore-changed", extension.DigestOf(content))
	shell.Use(cluster.ExtensionStateWith(t, artifacts, shell.Retention()))
	shell.SyncCatalogue(t)

	entries = listed(t, shell, token)
	if len(entries) == 0 {
		t.Fatalf("entries after the change = %v, want the entries still visible", entries)
	}
	refused := named(t, entries, installedDefinition)
	if installable, _ := refused["installable"].(bool); installable {
		t.Fatalf("entry = %v, want not installable when the content does not match the digest", refused)
	}
	if reason := harness.String(refused, "reason"); reason == "" {
		t.Fatalf("entry = %v, want the reason it cannot be installed", refused)
	}
	for _, entry := range entries {
		if id := harness.String(entry, "id"); id == installedDefinition {
			continue
		}
		if installable, _ := entry["installable"].(bool); !installable {
			t.Fatalf("entry = %v, want the untouched entries still installable", entry)
		}
	}
	stored = cluster.Get(t, "v1beta1", "ApplicationDefinition", definitionRef)
	if templateOf(t, stored) != originalTemplate {
		t.Fatalf("stored definition content changed; want the installed content kept")
	}

	// The page read carries the catalogue's refusal unchanged: an entry that
	// cannot be installed keeps its form and names why (APPSTORE-C-31).
	refusedOffer := support.Offer(t, shell, token, installedDefinition)
	if installable, _ := refusedOffer["installable"].(bool); installable {
		t.Fatalf("page offer = %v, want the catalogue's refusal", refusedOffer)
	}
	if reason := harness.String(refusedOffer, "reason"); reason == "" {
		t.Fatalf("page offer = %v, want the reason it cannot be installed", refusedOffer)
	}
	if fields, _ := refusedOffer["config"].([]any); len(fields) == 0 {
		t.Fatalf("page offer = %v, want the form kept on a refused entry", refusedOffer)
	}
}

// TestTC9CrossSourceEntryConflictKeepsOneDefinition installs a second source
// that offers an entry identity the appstore already claims: the appstore keeps
// the entry and the definition it names, the later component reports the
// conflict, and repeated syncs never hand the cluster-scoped definition back and
// forth (APPSTORE-C-24).
func TestTC9CrossSourceEntryConflictKeepsOneDefinition(t *testing.T) {
	cluster := harness.Start(t)
	configure, bind := support.Router(cluster, nil)
	shell := cluster.StartShell(t, configure)
	token := shell.SignInOwner(t, ownerName, ownerPass)
	bind(shell)
	artifacts := extension.MemoryArtifacts{}

	installed := cluster.InstallPackageWith(t, "appstore", shipped(t), artifacts, shell.Retention())
	shell.Use(installed)
	second := cluster.InstallPackageWith(t, "second-store",
		competing(t, "demo.store.second", "second-catalogue", installedDefinition), artifacts, shell.Retention())
	shell.Use(second)

	// Two syncs: the round after a write is where a flip between two writers
	// shows up.
	shell.SyncCatalogue(t)
	shell.SyncCatalogue(t)

	entries := listed(t, shell, token)
	claimed := named(t, entries, installedDefinition)
	if source := harness.String(claimed, "source"); source != appcatalogue.SourceID {
		t.Fatalf("entry source = %q, want the appstore's own source %q", source, appcatalogue.SourceID)
	}
	original := shipped(t)
	stored := cluster.Get(t, "v1beta1", "ApplicationDefinition", installedDefinition)
	for _, entry := range original.Contributions.CatalogueSources[0].Entries {
		if entry.ID == installedDefinition && templateOf(t, stored) != entry.Definition.Template {
			t.Fatalf("stored definition is not the appstore's content")
		}
	}
	result, found := second.Component("second-store")
	if !found || result.Phase != extension.PhaseDegraded {
		t.Fatalf("second component = %+v, want degraded on the entry conflict", result)
	}
	if !hasProblemClass(result.Problems, extension.ClassCatalogueSources) {
		t.Fatalf("problems = %v, want the catalogue conflict named", result.Problems)
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

// listed reads the catalogue entries the control plane serves.
func listed(t *testing.T, shell *harness.Shell, token string) []map[string]any {
	t.Helper()
	status, body := shell.Call(t, http.MethodGet, "/api/v1/catalogue/entries", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list entries = %d %v", status, body)
	}
	return harness.Items(t, body)
}

// named returns the one entry of a listing by identity.
func named(t *testing.T, entries []map[string]any, id string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if name := harness.String(entry, "id"); name == id {
			return entry
		}
	}
	t.Fatalf("the catalogue lists no entry %q: %v", id, entries)
	return nil
}

// competing declares one catalogue source offering the shared entry identity
// with a template of its own.
func competing(t *testing.T, identifier, source, entryID string) extension.Declaration {
	t.Helper()
	spec := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: "1.0.0"},
		Template: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Name }}\n  namespace: {{ .Namespace }}\ndata:\n  mode: \"second\"\n",
	}
	digest, err := app.DefinitionDigest(spec)
	if err != nil {
		t.Fatalf("digest the competing definition: %v", err)
	}
	spec.Package.Digest = digest
	return extension.Declaration{
		Identifier: identifier,
		Version:    "1.0.0",
		Contributions: extension.Contributions{
			CatalogueSources: []extension.CatalogueSource{{
				ID:   source,
				Sync: "pull",
				Entries: []extension.CatalogueEntry{{
					ID:         entryID,
					Display:    extension.CatalogueDisplay{Name: source},
					Definition: spec,
				}},
			}},
		},
	}
}

// templateOf reads the definition template out of a stored definition.
func templateOf(t *testing.T, definition *unstructured.Unstructured) string {
	t.Helper()
	template, found, err := unstructured.NestedString(definition.Object, "spec", "template")
	if err != nil || !found {
		t.Fatalf("read the stored definition template: %v", err)
	}
	return template
}

// hasProblemClass reports whether one component failed the named contribution
// class.
func hasProblemClass(problems []extension.ClassProblem, class string) bool {
	for _, problem := range problems {
		if problem.Class == class {
			return true
		}
	}
	return false
}

package declaration_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/pkg/extension"

	"github.com/tsic404/kos-appstore/declaration"
)

// TestDeclarationLoadsWithTheServedFramework checks the declaration the control
// plane loads: it declares only contributions the framework accepts, and its
// target range admits the framework version the host serves (KOSD-C-16,
// KOSD-C-18).
func TestDeclarationLoadsWithTheServedFramework(t *testing.T) {
	declared, err := declaration.Declaration()
	if err != nil {
		t.Fatalf("build the declaration: %v", err)
	}
	if declared.Identifier != declaration.Identifier || declared.Version != declaration.Version {
		t.Fatalf("declaration = %s@%s, want %s@%s",
			declared.Identifier, declared.Version, declaration.Identifier, declaration.Version)
	}
	loaded, err := declared.Load()
	if err != nil {
		t.Fatalf("load the declaration: %v", err)
	}
	if len(loaded.Problems) > 0 {
		t.Fatalf("the declaration has contribution problems: %+v", loaded.Problems)
	}
	if len(loaded.Contributions.CatalogueSources) != 1 || len(loaded.Contributions.Pages) != 1 {
		t.Fatalf("contributions = %+v, want one catalogue source and one page", loaded.Contributions)
	}
	constraint, err := extension.ParseConstraint(declaration.FrameworkVersion)
	if err != nil {
		t.Fatalf("parse the target framework range: %v", err)
	}
	for _, served := range extension.SupportedFrameworks() {
		if constraint.Allows(served) {
			return
		}
	}
	t.Fatalf("the target range %q admits no served framework version %v",
		declaration.FrameworkVersion, extension.SupportedFrameworks())
}

// TestManifestValidates checks the packed declaration through the SDK: the field
// set is the one KOSD-C-18 fixes, the window app satisfies KOSD-C-73, and the
// extension declares no write scope (the app center reads and delegates writes
// to the control plane).
func TestManifestValidates(t *testing.T) {
	document, err := declaration.ManifestDocument()
	if err != nil {
		t.Fatalf("build the manifest document: %v", err)
	}
	checked := manifest.ValidateExtensionManifest(document)
	if !checked.OK() {
		t.Fatalf("manifest is invalid: %v", checked.Issues)
	}
	declared := checked.Manifest
	if declared.ID != declaration.Identifier || declared.Version != declaration.Version {
		t.Fatalf("manifest = %s@%s, want %s@%s",
			declared.ID, declared.Version, declaration.Identifier, declaration.Version)
	}
	if declared.FrameworkVersion != declaration.FrameworkVersion {
		t.Fatalf("frameworkVersion = %q, want %q", declared.FrameworkVersion, declaration.FrameworkVersion)
	}
	if len(declared.Contributions.CatalogSources) != 1 || len(declared.Contributions.Pages) != 1 {
		t.Fatalf("contributions = %+v, want one catalogue source and one page", declared.Contributions)
	}
	if len(declared.Contributions.Apps) != 1 {
		t.Fatalf("apps = %+v, want the app-center window", declared.Contributions.Apps)
	}
	if window := declared.Contributions.Apps[0]; window.ID == "" || window.Entry == "" {
		t.Fatalf("app = %+v, want an identified window with an entry", window)
	}
	if len(declared.Permissions.Write) != 0 {
		t.Fatalf("write scopes = %+v, want none: the app center holds no cluster credential", declared.Permissions.Write)
	}
	if len(declared.Permissions.Read) != 1 {
		t.Fatalf("read scopes = %+v, want the control-plane read the page performs", declared.Permissions.Read)
	}
	for _, class := range []string{"catalogSources", "pages", "apps"} {
		if action, found := declared.UninstallRetention.Contributions[class]; !found || action != manifest.Delete {
			t.Errorf("uninstall retention of %s = %q, want %q", class, action, manifest.Delete)
		}
	}
}

// TestBothProjectionsCarryTheSameEntries checks that the manifest and the host
// declaration are generated from one product surface: the entries the package
// ships are the entries the control plane offers.
func TestBothProjectionsCarryTheSameEntries(t *testing.T) {
	declared, err := declaration.Declaration()
	if err != nil {
		t.Fatalf("build the declaration: %v", err)
	}
	document, err := declaration.ManifestDocument()
	if err != nil {
		t.Fatalf("build the manifest document: %v", err)
	}
	host, err := hosted(declared.Contributions.CatalogueSources[0].Entries)
	if err != nil {
		t.Fatalf("read the host entries: %v", err)
	}
	sources, _ := document["contributions"].(map[string]any)["catalogSources"].([]any)
	packed, err := hosted(sources[0].(map[string]any)["entries"])
	if err != nil {
		t.Fatalf("read the packed entries: %v", err)
	}
	if !reflect.DeepEqual(host, packed) {
		t.Fatalf("the package offers different entries than the declaration:\nhost: %v\npacked: %v", host, packed)
	}
}

// hosted decodes one projection's entries into the JSON values they render.
func hosted(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

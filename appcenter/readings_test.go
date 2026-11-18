package appcenter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/catalogue"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-sdk/manifest"
)

// template renders one ConfigMap; the catalogue only requires a template.
const template = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Name }}\n"

// declarations is a fixed contribution projection.
type declarations struct {
	sources []extension.CatalogueSource
}

// CatalogueSources implements catalogue.Declarations.
func (d declarations) CatalogueSources() []extension.CatalogueSource { return d.sources }

// Capabilities implements catalogue.Declarations.
func (d declarations) Capabilities() []extension.Capability { return nil }

// definitionWith builds one valid definition pinned to its content digest.
func definitionWith(t *testing.T, structure []v1beta1.ConfigField, requires []v1beta1.CapabilityRequirement) v1beta1.ApplicationDefinitionSpec {
	t.Helper()
	spec := v1beta1.ApplicationDefinitionSpec{
		Package:  v1beta1.PackageIdentity{Version: "1.0.0"},
		Config:   structure,
		Template: template,
		Requires: requires,
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

// entriesOf reads the app-center entries item the way the shell receives it.
func entriesOf(t *testing.T, reader Readings) []map[string]any {
	t.Helper()
	reading, failure := reader.Read(context.Background(), identity.Session{Role: write.RoleMember},
		extension.PageItem{ID: EntriesItem, Reference: EntriesRef})
	if failure != nil {
		t.Fatalf("read the entries item: %v", failure)
	}
	encoded, err := json.Marshal(reading.Value)
	if err != nil {
		t.Fatalf("encode the entries value: %v", err)
	}
	var offers []map[string]any
	if err := json.Unmarshal(encoded, &offers); err != nil {
		t.Fatalf("decode the entries value: %v", err)
	}
	return offers
}

// offerNamed returns one rendered entry by identity.
func offerNamed(t *testing.T, offers []map[string]any, id string) map[string]any {
	t.Helper()
	for _, offer := range offers {
		if name, _ := offer["id"].(string); name == id {
			return offer
		}
	}
	t.Fatalf("the entries item carries no entry %q: %v", id, offers)
	return nil
}

// fieldsOf returns the generated form fields of one rendered entry by name.
func fieldsOf(t *testing.T, offer map[string]any) map[string]map[string]any {
	t.Helper()
	listed, ok := offer["config"].([]any)
	if !ok {
		t.Fatalf("entry %v carries no field list", offer["id"])
	}
	fields := make(map[string]map[string]any, len(listed))
	for _, raw := range listed {
		field, _ := raw.(map[string]any)
		name, _ := field["name"].(string)
		fields[name] = field
	}
	return fields
}

// TestAppCenterLaysOutTheInstallFormFromTheStructure checks the entries item the
// interface installs from: every entry carries the fields its config structure
// generates, and an entry that cannot be installed carries the same refusal the
// write path answers with (APPSTORE-C-31, KOSD-C-32).
func TestAppCenterLaysOutTheInstallFormFromTheStructure(t *testing.T) {
	structure := []v1beta1.ConfigField{
		{Name: "tier", Type: app.TypeEnum, Required: true, Enum: []string{"small", "large"}, Description: "运行规格"},
		{Name: "replicas", Type: app.TypeInteger, Minimum: new("1"), Maximum: new("4")},
		{Name: "owner", Type: app.TypeString, Sensitive: true},
	}
	reader := NewReadings(catalogue.New(declarations{sources: []extension.CatalogueSource{{
		ID:   "fixture-source",
		Sync: "pull",
		Entries: []extension.CatalogueEntry{
			{ID: "wiki", Display: extension.CatalogueDisplay{Name: "Wiki"}, Definition: definitionWith(t, structure, nil)},
			{ID: "gpu", Display: extension.CatalogueDisplay{Name: "GPU 任务"},
				Definition: definitionWith(t, structure, []v1beta1.CapabilityRequirement{{Name: "compute.gpu", Version: "v1"}})},
		},
	}}}, nil), nil, nil, nil)

	offers := entriesOf(t, reader)
	if len(offers) != 2 {
		t.Fatalf("entries = %v, want the two offered entries", offers)
	}

	fields := fieldsOf(t, offerNamed(t, offers, "wiki"))
	if len(fields) != len(structure) {
		t.Fatalf("fields = %v, want one per declared field", fields)
	}
	if tier := fields["tier"]; tier["control"] != "select" || tier["required"] != true {
		t.Fatalf("tier = %v, want a required select", tier)
	}
	if accepted, _ := fields["tier"]["enum"].([]any); len(accepted) != 2 {
		t.Fatalf("tier = %v, want the accepted values", fields["tier"])
	}
	if replicas := fields["replicas"]; replicas["control"] != "number" || replicas["maximum"] != "4" {
		t.Fatalf("replicas = %v, want a bounded number", replicas)
	}
	if owner := fields["owner"]; owner["sensitive"] != true {
		t.Fatalf("owner = %v, want the sensitive mark so the shell renders it write-only", owner)
	}
	for name, field := range fields {
		if _, carried := field["value"]; carried {
			t.Fatalf("field %s carries a value; an offer shows the structure, never a parameter", name)
		}
	}

	refused := offerNamed(t, offers, "gpu")
	if installable, _ := refused["installable"].(bool); installable {
		t.Fatalf("entry = %v, want not installable while its capability is absent", refused)
	}
	missing, _ := refused["missing"].([]any)
	if len(missing) != 1 || missing[0] != "compute.gpu" {
		t.Fatalf("missing = %v, want the absent capability listed", refused["missing"])
	}
	if reason, _ := refused["reason"].(string); reason == "" {
		t.Fatalf("reason = %q, want the missing capability named", reason)
	}
	if len(fieldsOf(t, refused)) != len(structure) {
		t.Fatalf("a refused entry lost its form: %v", refused)
	}
}

// TestAppCenterCarriesOnlyDeclaredPermissions checks the entry value both the
// detail and the install confirmation render: an entry's permission set equals
// the set its declared window application carries, an entry without a
// declaration carries none, and a capability nothing declared never appears
// (APPSTORE-C-77, KOSD-C-73).
func TestAppCenterCarriesOnlyDeclaredPermissions(t *testing.T) {
	apps := []manifest.AppManifest{
		{ID: "notify", Permissions: []manifest.Permission{manifest.PermissionNotifications}},
		{ID: "wiki", Permissions: []manifest.Permission{manifest.PermissionClipboardRead, manifest.PermissionClipboardRead}},
		{ID: "unoffered", Permissions: []manifest.Permission{manifest.PermissionExternalLink}},
	}
	reader := NewReadings(catalogue.New(declarations{sources: []extension.CatalogueSource{{
		ID:   "fixture-source",
		Sync: "pull",
		Entries: []extension.CatalogueEntry{
			{ID: "notify", Display: extension.CatalogueDisplay{Name: "Notify"}, Definition: definitionWith(t, nil, nil)},
			{ID: "wiki", Display: extension.CatalogueDisplay{Name: "Wiki"}, Definition: definitionWith(t, nil, nil)},
			{ID: "plain", Display: extension.CatalogueDisplay{Name: "Plain"}, Definition: definitionWith(t, nil, nil)},
		},
	}}}, nil), nil, apps, nil)

	offers := entriesOf(t, reader)
	want := map[string][]string{
		"notify": {"notifications"},
		"wiki":   {"clipboard-read"},
		"plain":  {},
	}
	for id, expected := range want {
		offer := offerNamed(t, offers, id)
		listed, ok := offer["permissions"].([]any)
		if !ok {
			t.Fatalf("entry %s carries no permission list: %v", id, offer["permissions"])
		}
		got := make([]string, 0, len(listed))
		for _, raw := range listed {
			got = append(got, raw.(string))
		}
		if len(got) != len(expected) {
			t.Fatalf("entry %s permissions = %v, want %v", id, got, expected)
		}
		for index := range expected {
			if got[index] != expected[index] {
				t.Fatalf("entry %s permissions = %v, want %v", id, got, expected)
			}
		}
	}
}

// TestAppCenterShowsAnEmptyEntryListWithoutSources checks the empty state: with
// no active source the page read answers with an empty list, not a null value.
func TestAppCenterShowsAnEmptyEntryListWithoutSources(t *testing.T) {
	offers := entriesOf(t, NewReadings(catalogue.New(declarations{}, nil), nil, nil, nil))
	if offers == nil || len(offers) != 0 {
		t.Fatalf("entries = %v, want an empty list", offers)
	}
}

// TestAppCenterRejectsAnUndeclaredReference checks that a page reference the
// reader does not serve fails the read instead of answering a made-up value.
func TestAppCenterRejectsAnUndeclaredReference(t *testing.T) {
	reader := NewReadings(catalogue.New(declarations{}, nil), nil, nil, nil)
	_, failure := reader.Read(context.Background(), identity.Session{Role: write.RoleMember},
		extension.PageItem{ID: "unknown", Reference: "catalogue/unknown"})
	if failure == nil {
		t.Fatal("read an undeclared reference: want a failure")
	}
	if failure.Code != controlplane.CodeInternal {
		t.Fatalf("failure = %+v, want the internal code for an unresolved reference", failure)
	}
}

// TestUpdateOnlyAppearsWhileTheOfferedPackageDiffers checks the update-available
// projection of APPSTORE-C-79 and APPSTORE-C-80: the offered package digest
// decides whether an application is behind its entry, the target version and
// digest are the entry's, and an application without an applied baseline or
// without a catalogue entry carries no update.
func TestUpdateOnlyAppearsWhileTheOfferedPackageDiffers(t *testing.T) {
	entry := catalogue.Entry{ID: "web", Definition: "web", Version: "1.1.0", Digest: "sha256:new"}
	application := v1beta1.Application{}
	application.Status.Applied = &v1beta1.AppliedState{DefinitionDigest: "sha256:old"}

	update := updateOf(application, entry, true)
	if !update.Available || update.Version != "1.1.0" || update.Digest != "sha256:new" {
		t.Fatalf("update = %+v, want the offered version and digest", update)
	}
	if update.InstalledDigest != "sha256:old" {
		t.Fatalf("installed digest = %q, want the applied baseline", update.InstalledDigest)
	}

	same := updateOf(application, catalogue.Entry{Definition: "web", Version: "1.0.0", Digest: "sha256:old"}, true)
	if same.Available || same.Version != "" || same.Digest != "" {
		t.Fatalf("update = %+v, want none while the offered package is the applied one", same)
	}

	noBaseline := updateOf(v1beta1.Application{}, entry, true)
	if noBaseline.Available || noBaseline.InstalledDigest != "" {
		t.Fatalf("update = %+v, want none without an applied baseline", noBaseline)
	}

	notOffered := updateOf(application, catalogue.Entry{}, false)
	if notOffered.Available {
		t.Fatalf("update = %+v, want none without a catalogue entry", notOffered)
	}
}

// TestPreviewIsTheFrameworkReport checks that the page presents the upgrade
// preflight the framework wrote, including the changed config field and the
// irreversible item, and renders empty lists rather than nulls (KOSD-C-34).
func TestPreviewIsTheFrameworkReport(t *testing.T) {
	if preview := previewOf(v1beta1.Application{}); preview != nil {
		t.Fatalf("preview = %+v, want none without a preflight report", preview)
	}

	application := v1beta1.Application{}
	application.Status.Upgrade = &v1beta1.UpgradeStatus{
		Target: "sha256:new",
		ConfigChanges: []v1beta1.ConfigChange{
			{Field: "mode", From: `"one"`, To: `"two"`},
		},
		Irreversible: []v1beta1.IrreversibleItem{
			{Kind: v1beta1.IrreversibleDataFormat, Detail: "the package starts declaring data format"},
		},
	}
	preview := previewOf(application)
	if preview == nil || preview.Target != "sha256:new" {
		t.Fatalf("preview = %+v, want the framework's target", preview)
	}
	if len(preview.ConfigChanges) != 1 || preview.ConfigChanges[0].Field != "mode" ||
		preview.ConfigChanges[0].From != `"one"` || preview.ConfigChanges[0].To != `"two"` {
		t.Fatalf("config changes = %+v, want the changed default the framework reported", preview.ConfigChanges)
	}
	if len(preview.Irreversible) != 1 || preview.Irreversible[0].Kind != v1beta1.IrreversibleDataFormat {
		t.Fatalf("irreversible = %+v, want the item the framework refused", preview.Irreversible)
	}
	if preview.RenderChanges == nil || preview.Migrations == nil {
		t.Fatalf("preview = %+v, want empty lists rather than nulls", preview)
	}
}

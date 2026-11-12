package catalogue_test

import (
	"encoding/json"
	"testing"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"

	"github.com/tsic404/kos-appstore/catalogue"
)

// TestDefinitionsPinTheirDigest checks every shipped definition: its content
// recomputes to the digest the package pins, and the definition validates as an
// installable package (KOSD-C-32).
func TestDefinitionsPinTheirDigest(t *testing.T) {
	definitions, err := catalogue.Definitions()
	if err != nil {
		t.Fatalf("load the shipped definitions: %v", err)
	}
	if len(definitions) == 0 {
		t.Fatal("the package ships no definition")
	}
	names := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if names[definition.Name] {
			t.Fatalf("definition %s is shipped twice", definition.Name)
		}
		names[definition.Name] = true
		digest, err := app.DefinitionDigest(definition.Spec)
		if err != nil {
			t.Fatalf("digest %s: %v", definition.Name, err)
		}
		if digest != definition.Spec.Package.Digest {
			t.Errorf("%s pins %s, its content is %s", definition.Name, definition.Spec.Package.Digest, digest)
		}
		if err := app.ValidateDefinition(definition.Spec); err != nil {
			t.Errorf("%s is not installable: %v", definition.Name, err)
		}
	}
}

// TestShippedDefinitionsRenderStably checks the translation template of every
// shipped definition: the same configuration renders the same downstream
// objects, which is what the digest covers (KOSD-C-32).
func TestShippedDefinitionsRenderStably(t *testing.T) {
	definitions, err := catalogue.Definitions()
	if err != nil {
		t.Fatalf("load the shipped definitions: %v", err)
	}
	for _, definition := range definitions {
		t.Run(definition.Name, func(t *testing.T) {
			config, err := app.Resolve(definition.Spec.Config, nil)
			if err != nil {
				t.Fatalf("resolve the defaults: %v", err)
			}
			input := app.RenderInput{Name: "sample", Namespace: "apps", Config: config}
			first, err := app.Render(definition.Spec, input)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			second, err := app.Render(definition.Spec, input)
			if err != nil {
				t.Fatalf("second render: %v", err)
			}
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			if string(firstJSON) != string(secondJSON) {
				t.Fatal("the same config rendered differently")
			}
		})
	}
}

// TestSourceOffersEveryShippedDefinition checks the catalogue source
// contribution: one entry per shipped definition, each carrying the display
// information, version, dependencies, install parameter structure and package
// digest the app center reads (APPSTORE-C-24).
func TestSourceOffersEveryShippedDefinition(t *testing.T) {
	source, err := catalogue.Source()
	if err != nil {
		t.Fatalf("build the catalogue source: %v", err)
	}
	if source.ID != catalogue.SourceID || source.Sync != catalogue.SyncPull {
		t.Fatalf("source = %s/%s, want %s/%s", source.ID, source.Sync, catalogue.SourceID, catalogue.SyncPull)
	}
	definitions, err := catalogue.Definitions()
	if err != nil {
		t.Fatalf("load the shipped definitions: %v", err)
	}
	if len(source.Entries) != len(definitions) {
		t.Fatalf("entries = %d, want one per shipped definition (%d)", len(source.Entries), len(definitions))
	}
	offered := make(map[string]v1beta1.ApplicationDefinitionSpec, len(source.Entries))
	for _, entry := range source.Entries {
		if entry.Display.Name == "" {
			t.Errorf("entry %s carries no display name", entry.ID)
		}
		offered[entry.ID] = entry.Definition
	}
	for _, definition := range definitions {
		spec, found := offered[definition.Name]
		if !found {
			t.Errorf("the source does not offer the shipped definition %s", definition.Name)
			continue
		}
		if spec.Package.Digest != definition.Spec.Package.Digest {
			t.Errorf("entry %s pins %s, want the shipped digest %s",
				definition.Name, spec.Package.Digest, definition.Spec.Package.Digest)
		}
		if spec.Package.Version != definition.Spec.Package.Version {
			t.Errorf("entry %s offers %s, want the shipped version %s",
				definition.Name, spec.Package.Version, definition.Spec.Package.Version)
		}
		if len(spec.Config) != len(definition.Spec.Config) {
			t.Errorf("entry %s carries %d install parameters, want %d",
				definition.Name, len(spec.Config), len(definition.Spec.Config))
		}
		if len(spec.Requires) != len(definition.Spec.Requires) {
			t.Errorf("entry %s carries %d dependencies, want %d",
				definition.Name, len(spec.Requires), len(definition.Spec.Requires))
		}
	}
}

// TestShippedEntriesDeclareInstallParameters checks the structure every shipped
// entry offers: at least one field, each naming a type the shell can lay out
// (KOSD-C-32).
func TestShippedEntriesDeclareInstallParameters(t *testing.T) {
	source, err := catalogue.Source()
	if err != nil {
		t.Fatalf("build the catalogue source: %v", err)
	}
	for _, entry := range source.Entries {
		if len(entry.Definition.Config) == 0 {
			t.Errorf("entry %s declares no install parameter", entry.ID)
			continue
		}
		for _, field := range entry.Definition.Config {
			if field.Name == "" || field.Type == "" {
				t.Errorf("entry %s carries a field without name or type: %+v", entry.ID, field)
			}
		}
	}
}

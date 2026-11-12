// Package catalogue carries the application definitions the appstore ships and
// projects them as its catalogue source contribution (APPSTORE-C-24). The
// entries are product data: each one names a definition, the display
// information the app center shows and the package digest covered by it.
package catalogue

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"sigs.k8s.io/yaml"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/app"
	"github.com/tsic404/kosd/pkg/extension"
)

// The source the package contributes (APPSTORE-C-24): one identity across
// versions and the way it keeps its entries current.
const (
	// SourceID names the catalogue source.
	SourceID = "kos-appstore"
	// SyncPull tells how the source delivers its entries.
	SyncPull = "pull"
)

//go:embed definitions/*.yaml
var shipped embed.FS

// display is what the app center shows for one shipped definition (APPSTORE-C-24).
type display struct {
	// name is the display name; a shipped definition without one shows its own
	// name instead of disappearing.
	name string
	// description explains what the application does.
	description string
	// icon is the display icon key.
	icon string
}

// displays names the applications the package ships. It is product data, not a
// kernel table: a definition the table does not name is still offered under its
// own name.
var displays = map[string]display{
	"memcached":     {name: "Memcached", description: "分布式内存缓存", icon: "memcached"},
	"nginx":         {name: "Nginx", description: "静态站点与反向代理", icon: "nginx"},
	"node-exporter": {name: "Node Exporter", description: "节点指标采集", icon: "node-exporter"},
	"redis":         {name: "Redis", description: "内存键值存储", icon: "redis"},
	"whoami":        {name: "Whoami", description: "请求回显服务", icon: "whoami"},
}

// Definitions returns every shipped definition, ordered by name. Each one pins
// its package digest; loading recomputes it from the content and refuses a
// definition whose content no longer matches its pin (KOSD-C-32).
func Definitions() ([]*v1beta1.ApplicationDefinition, error) {
	paths, err := fs.Glob(shipped, "definitions/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("list the shipped definitions: %w", err)
	}
	definitions := make([]*v1beta1.ApplicationDefinition, 0, len(paths))
	for _, path := range paths {
		definition, err := load(path)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].Name < definitions[right].Name
	})
	return definitions, nil
}

// load decodes, pins and validates one shipped definition. The decode is
// strict: a misspelled field would otherwise be dropped before the digest is
// computed, and the pin would cover content the package never used.
func load(path string) (*v1beta1.ApplicationDefinition, error) {
	raw, err := fs.ReadFile(shipped, path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	definition := &v1beta1.ApplicationDefinition{}
	if err := yaml.UnmarshalStrict(raw, definition); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if definition.Kind != "ApplicationDefinition" {
		return nil, fmt.Errorf("%s is a %s, not an ApplicationDefinition", path, definition.Kind)
	}
	if definition.Spec.Package.Digest == "" {
		return nil, fmt.Errorf("%s pins no package digest", path)
	}
	digest, err := app.DefinitionDigest(definition.Spec)
	if err != nil {
		return nil, fmt.Errorf("digest %s: %w", path, err)
	}
	if definition.Spec.Package.Digest != digest {
		return nil, fmt.Errorf("%s: the package content does not match its pinned digest", path)
	}
	if err := app.ValidateDefinition(definition.Spec); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return definition, nil
}

// Source returns the catalogue source contribution: one entry per shipped
// definition. The entry identity is the definition name, so a sync writes one
// definition per shipped application, and the entry carries the definition spec
// as the package pins it (APPSTORE-C-24, KOSD-C-32).
func Source() (extension.CatalogueSource, error) {
	definitions, err := Definitions()
	if err != nil {
		return extension.CatalogueSource{}, err
	}
	entries := make([]extension.CatalogueEntry, 0, len(definitions))
	for _, definition := range definitions {
		entries = append(entries, extension.CatalogueEntry{
			ID:         definition.Name,
			Display:    displayOf(definition.Name),
			Definition: definition.Spec,
		})
	}
	return extension.CatalogueSource{ID: SourceID, Sync: SyncPull, Entries: entries}, nil
}

// displayOf renders one shipped definition's display information. A definition
// the table does not name is offered under its own name, so shipping a
// definition without display data cannot drop it from the app center.
func displayOf(name string) extension.CatalogueDisplay {
	shown, found := displays[name]
	if !found {
		return extension.CatalogueDisplay{Name: name}
	}
	return extension.CatalogueDisplay{Name: shown.name, Description: shown.description, Icon: shown.icon}
}

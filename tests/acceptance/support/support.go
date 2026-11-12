// Package support wires the appstore's page reader into the acceptance shell the
// way the host does. Page item references start with the package identity and
// pages.Router dispatches each item to the reader of the product that owns it
// (KOSD-C-22, KOSD-C-53), so every case reads the page over the served HTTP
// entry: an item no routed reader answers fails the read instead of showing a
// value the window could never receive.
package support

import (
	"net/http"
	"testing"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/pkg/catalogue"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-appstore/appcenter"
)

// Router returns the page-shell hook that routes app-center items to the product
// reader, and the call that connects the reader to the shell's catalogue once
// StartShell has built it. apps are the window applications the offering
// packages declare (KOSD-C-73); nil declares none.
func Router(cluster *harness.Cluster, apps []manifest.AppManifest) (func(*pages.Config), func(*harness.Shell)) {
	late := &declarations{}
	reader := appcenter.NewReadings(catalogue.New(late, cluster.Dynamic), apps, nil)
	configure := func(config *pages.Config) {
		config.Readings = pages.Router{appcenter.Identifier: reader}
	}
	bind := func(shell *harness.Shell) { late.source = shell.Registry }
	return configure, bind
}

// declarations forwards the catalogue projection to the shell registry. The
// reader is built before StartShell and reads only after bind, so the product
// reader and the page shell share one registry.
type declarations struct {
	source catalogue.Declarations
}

// CatalogueSources implements catalogue.Declarations.
func (d *declarations) CatalogueSources() []extension.CatalogueSource {
	if d.source == nil {
		return nil
	}
	return d.source.CatalogueSources()
}

// Capabilities implements catalogue.Declarations.
func (d *declarations) Capabilities() []extension.Capability {
	if d.source == nil {
		return nil
	}
	return d.source.Capabilities()
}

// Entries reads the served app-center page and returns the entries item's value
// as the window receives it.
func Entries(t *testing.T, shell *harness.Shell, token string) []map[string]any {
	t.Helper()
	return itemValue(t, shell, token, appcenter.EntriesItem)
}

// Upgrades reads the served app-center page and returns the upgrades item's
// value as the window receives it.
func Upgrades(t *testing.T, shell *harness.Shell, token string) []map[string]any {
	t.Helper()
	return itemValue(t, shell, token, appcenter.UpgradesItem)
}

// Offer returns one entry inside the served entries value.
func Offer(t *testing.T, shell *harness.Shell, token, id string) map[string]any {
	t.Helper()
	for _, offer := range Entries(t, shell, token) {
		if harness.String(offer, "id") == id {
			return offer
		}
	}
	t.Fatalf("the served page carries no entry %q", id)
	return nil
}

// itemValue reads one served item's value.
func itemValue(t *testing.T, shell *harness.Shell, token, id string) []map[string]any {
	t.Helper()
	status, body := shell.Call(t, http.MethodGet, "/api/v1/pages/"+appcenter.PageID, nil, token)
	if status != http.StatusOK {
		t.Fatalf("read the app-center page = %d %v", status, body)
	}
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if harness.String(item, "id") == id {
			return objects(t, item["value"], id)
		}
	}
	t.Fatalf("the served page has no item %q: %v", id, body)
	return nil
}

// objects decodes one served item value.
func objects(t *testing.T, value any, id string) []map[string]any {
	t.Helper()
	listed, ok := value.([]any)
	if !ok {
		t.Fatalf("item %q carries no list: %v", id, value)
	}
	values := make([]map[string]any, 0, len(listed))
	for _, raw := range listed {
		entry, _ := raw.(map[string]any)
		values = append(values, entry)
	}
	return values
}

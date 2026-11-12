// Readings resolves the app-center page's catalogue items (DASH-C-22, DASH-C-52):
// the entries item lays the install parameters out as the form fields the
// config structure generates (APPSTORE-C-31), the upgrades item carries the
// upgrade paths. The value is read at request time, so the observation time is
// the read time and the shell's stale marker compares against it (KOSD-C-13).
package appcenter

import (
	"context"
	"slices"
	"time"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/pkg/catalogue"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
)

// Readings serves the app-center page over the catalogue domain.
type Readings struct {
	catalogue *catalogue.Service
	// apps are the window applications the offering package declares (KOSD-C-73).
	// A permission set belongs to a declared application, so an entry with no
	// declaration shows none (APPSTORE-C-77).
	apps []manifest.AppManifest
	now  func() time.Time
}

// NewReadings builds the page reader. apps carries the window applications the
// offering package declares; nil declares none. now supplies the observation
// time; nil reads the clock at request time.
func NewReadings(service *catalogue.Service, apps []manifest.AppManifest, now func() time.Time) Readings {
	if now == nil {
		now = time.Now
	}
	return Readings{catalogue: service, apps: apps, now: now}
}

// offer is one catalogue entry as the app-center page renders it (APPSTORE-C-24,
// APPSTORE-C-31). The catalogue's judgement travels unchanged, so an entry that
// cannot be installed carries the reason and missing capabilities the write path
// refuses it with; the install parameters are the form fields generated from the
// entry's config structure, which the shell lays out as the submission form
// (KOSD-C-32).
type offer struct {
	catalogue.Entry
	// Config replaces the raw structure the catalogue read model carries with the
	// fields generated from it; the catalogue API still serves that structure.
	// encoding/json prefers this shallower field over the embedded one.
	Config []pages.FormField `json:"config"`
	// Permissions is the capability set the package declares for the window
	// application this entry installs (APPSTORE-C-77, KOSD-C-73). The entry
	// detail and the install confirmation render this one value: a permission the
	// declaration omits never appears, and there is no grant entry to add it.
	// Empty means the entry installs no window application or one that declares
	// no capability.
	Permissions []string `json:"permissions"`
}

// Read resolves one declared item of the app-center page. The session is not
// consulted: the catalogue lists what every signed-in role may read, and the
// page itself is already trimmed to the caller's role (KOSD-C-59).
func (r Readings) Read(ctx context.Context, _ identity.Session, item extension.PageItem) (pages.Reading, *controlplane.Error) {
	observedAt := r.now()
	switch item.Reference {
	case EntriesRef:
		return pages.Reading{Value: r.offers(), ObservedAt: observedAt}, nil
	case UpgradesRef:
		upgrades, failure := r.catalogue.Upgrades(ctx)
		if failure != nil {
			return pages.Reading{}, failure
		}
		return pages.Reading{Value: upgrades, ObservedAt: observedAt}, nil
	default:
		return pages.Reading{}, unresolved(item.Reference)
	}
}

// offers renders every catalogue entry with the install form its config
// structure generates and the permission set its declared application carries.
// An entry nothing offers stays out, and an empty catalogue renders an empty
// list rather than a missing value.
func (r Readings) offers() []offer {
	entries := r.catalogue.Entries()
	offers := make([]offer, 0, len(entries))
	for _, entry := range entries {
		offers = append(offers, offer{
			Entry:       entry,
			Config:      pages.FormOf(entry.Config),
			Permissions: declaredPermissions(r.apps, entry.ID),
		})
	}
	return offers
}

// declaredPermissions collects the capabilities the package declares for the
// window application an entry installs (APPSTORE-C-77). The application is the
// declared `App` of the entry's identity; duplicate names collapse and the
// declaration order is kept, so the same declaration always renders one list.
// The result is never nil: an entry without a declaration renders an empty set,
// not a missing value.
func declaredPermissions(apps []manifest.AppManifest, entryID string) []string {
	permissions := []string{}
	for _, app := range apps {
		if app.ID != entryID {
			continue
		}
		for _, permission := range app.Permissions {
			name := string(permission)
			if !slices.Contains(permissions, name) {
				permissions = append(permissions, name)
			}
		}
	}
	return permissions
}

// unresolved names a page reference this reader does not serve.
func unresolved(reference string) *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeInternal,
		Message:     "应用中心页的来源 " + reference + " 无法解析",
		Stage:       controlplane.StageRead,
		SideEffects: controlplane.SideEffectsNone,
	}
}

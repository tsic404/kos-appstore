// Readings resolves the app-center page's items (APPSTORE-C-31): the entries
// item lays the install parameters out as the form fields the config structure
// generates (APPSTORE-C-31), the installed item carries the applications the
// reading session may see with the phase it reads from the object
// (APPSTORE-C-78, APPSTORE-C-79), and the upgrades item carries those
// applications' catalogue upgrade paths (APPSTORE-C-24, APPSTORE-C-79). The
// value is read at request time, so the observation time is the read time and
// the shell's stale marker compares against it (KOSD-C-13).
package appcenter

import (
	"context"
	"slices"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/catalogue"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/pages/cluster"
)

// Readings serves the app-center page over the catalogue domain and the
// application objects it projects.
type Readings struct {
	catalogue *catalogue.Service
	// dynamic reads the applications the page projects; nil serves an empty
	// installed list.
	dynamic dynamic.Interface
	// apps are the window applications the offering package declares (KOSD-C-73).
	// A permission set belongs to a declared application, so an entry with no
	// declaration shows none (APPSTORE-C-77).
	apps []manifest.AppManifest
	now  func() time.Time
}

// NewReadings builds the page reader. client reads the cluster objects the page
// projects; nil serves no application. apps carries the window applications the
// offering package declares; nil declares none. now supplies the observation
// time; nil reads the clock at request time.
func NewReadings(service *catalogue.Service, client dynamic.Interface, apps []manifest.AppManifest, now func() time.Time) Readings {
	if now == nil {
		now = time.Now
	}
	return Readings{catalogue: service, dynamic: client, apps: apps, now: now}
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

// conditionView is one application condition as the page renders it (KOSD-C-07).
// The type, status, reason and message travel unchanged, so the progress and the
// result the page shows are the facts the application object carries.
type conditionView struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// previewView is the upgrade preflight report the framework wrote on the
// application (KOSD-C-34). It carries the config and render changes and the
// items the preflight refuses, so the page presents the framework's judgement
// instead of repeating it.
type previewView struct {
	// Target is the package digest the current spec names.
	Target string `json:"target,omitempty"`
	// SpecDigest is the digest of the spec that requests the target.
	SpecDigest string `json:"specDigest,omitempty"`
	// RenderDigest is the digest of the manifest set the target writes.
	RenderDigest string `json:"renderDigest,omitempty"`
	// ConfigChanges lists the config fields the upgrade rewrites; a changed
	// default appears here as the value the baseline held and the value the
	// upgrade writes.
	ConfigChanges []v1beta1.ConfigChange `json:"configChanges"`
	// RenderChanges lists the manifests the upgrade creates, rewrites or drops.
	RenderChanges []v1beta1.RenderChange `json:"renderChanges"`
	// Irreversible lists the items the preflight refuses (KOSD-C-34).
	Irreversible []v1beta1.IrreversibleItem `json:"irreversible"`
	// Migrations names the migration objects a storage-class change requires.
	Migrations []v1beta1.ObjectRef `json:"migrations"`
	// RolledBack reports that this target was applied and reverted.
	RolledBack bool `json:"rolledBack"`
}

// updateView is the update the catalogue offers one installed application
// (APPSTORE-C-79, APPSTORE-C-80).
type updateView struct {
	// Available reports that the offered package differs from the baseline the
	// application applied, so an update is available.
	Available bool `json:"available"`
	// Version is the target version the catalogue entry declares.
	Version string `json:"version,omitempty"`
	// Digest is the target package digest the catalogue entry declares.
	Digest string `json:"digest,omitempty"`
	// InstalledDigest is the package digest the applied baseline rendered from.
	InstalledDigest string `json:"installedDigest,omitempty"`
	// Preview is the framework's preflight report once the target is requested;
	// a page read never computes a second one (KOSD-C-34).
	Preview *previewView `json:"preview,omitempty"`
}

// installedApplication is one application the reading session may see as the
// installed list renders it (APPSTORE-C-78, APPSTORE-C-79).
type installedApplication struct {
	Namespace  string          `json:"namespace"`
	Name       string          `json:"name"`
	Definition string          `json:"definition,omitempty"`
	Phase      string          `json:"phase,omitempty"`
	Health     string          `json:"health,omitempty"`
	Conditions []conditionView `json:"conditions"`
	Update     updateView      `json:"update"`
}

// upgradePath is one installed application's catalogue upgrade path with the
// update the offered entry carries (APPSTORE-C-24, APPSTORE-C-79). The state is
// the catalogue's judgement: Available while an active source offers the
// definition, SourceMissing once its source is gone.
type upgradePath struct {
	Namespace  string     `json:"namespace"`
	Name       string     `json:"name"`
	Definition string     `json:"definition"`
	State      string     `json:"state"`
	Phase      string     `json:"phase,omitempty"`
	Update     updateView `json:"update"`
}

// Read resolves one declared item of the app-center page for the reading
// session. The entry list is not an application list, so visibility does not
// filter it (APPSTORE-C-78); the installed list and the upgrade entry carry only
// the applications the session may see (KOSD-C-59).
func (r Readings) Read(ctx context.Context, session identity.Session, item extension.PageItem) (pages.Reading, *controlplane.Error) {
	observedAt := r.now()
	switch item.Reference {
	case EntriesRef:
		return pages.Reading{Value: r.offers(), ObservedAt: observedAt}, nil
	case InstalledRef:
		installed, failure := r.installed(ctx, session)
		if failure != nil {
			return pages.Reading{}, failure
		}
		return pages.Reading{Value: installed, ObservedAt: observedAt}, nil
	case UpgradesRef:
		upgrades, failure := r.upgrades(ctx, session)
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

// installed lists the applications the session may see with the facts the page
// renders: the phase, health and conditions the application object carries
// (KOSD-C-33), and the update the catalogue offers it (APPSTORE-C-79,
// APPSTORE-C-80).
func (r Readings) installed(ctx context.Context, session identity.Session) ([]installedApplication, *controlplane.Error) {
	applications, failure := r.applications(ctx, session)
	if failure != nil {
		return nil, failure
	}
	offered := r.offered()
	installed := make([]installedApplication, 0, len(applications))
	for _, application := range applications {
		entry, isOffered := offered[application.Spec.DefinitionRef]
		installed = append(installed, installedApplication{
			Namespace:  application.Namespace,
			Name:       application.Name,
			Definition: application.Spec.DefinitionRef,
			Phase:      application.Status.Phase,
			Health:     string(application.Status.Health.State),
			Conditions: conditionsOf(application.Status.Conditions),
			Update:     updateOf(application, entry, isOffered),
		})
	}
	return installed, nil
}

// upgrades lists the catalogue upgrade paths of the applications the session may
// see. The path and its state stay the catalogue's judgement (APPSTORE-C-24);
// the page drops the applications the session may not see (APPSTORE-C-78) and
// adds the update the offered entry carries (APPSTORE-C-79).
func (r Readings) upgrades(ctx context.Context, session identity.Session) ([]upgradePath, *controlplane.Error) {
	applications, failure := r.applications(ctx, session)
	if failure != nil {
		return nil, failure
	}
	if r.catalogue == nil {
		return []upgradePath{}, nil
	}
	paths, failure := r.catalogue.Upgrades(ctx)
	if failure != nil {
		return nil, failure
	}
	visible := make(map[string]v1beta1.Application, len(applications))
	for _, application := range applications {
		visible[application.Namespace+"/"+application.Name] = application
	}
	offered := r.offered()
	upgrades := make([]upgradePath, 0, len(paths))
	for _, path := range paths {
		application, maySee := visible[path.Namespace+"/"+path.Name]
		if !maySee {
			continue
		}
		entry, isOffered := offered[path.Definition]
		upgrades = append(upgrades, upgradePath{
			Namespace:  path.Namespace,
			Name:       path.Name,
			Definition: path.Definition,
			State:      path.State,
			Phase:      application.Status.Phase,
			Update:     updateOf(application, entry, isOffered),
		})
	}
	return upgrades, nil
}

// applications reads the applications the session may see (KOSD-C-59). The
// visibility rule is the framework's: the cluster reader applies the same
// policy the resource read applies, so the page and a direct read never
// disagree.
func (r Readings) applications(ctx context.Context, session identity.Session) ([]v1beta1.Application, *controlplane.Error) {
	if r.dynamic == nil {
		return []v1beta1.Application{}, nil
	}
	reader := r.reader()
	objects, failure := reader.List(ctx, "Application", "")
	if failure != nil {
		return nil, failure
	}
	applications := make([]v1beta1.Application, 0, len(objects))
	for _, object := range objects {
		visible, failure := reader.Visible(ctx, session, "Application", object)
		if failure != nil {
			return nil, failure
		}
		if !visible {
			continue
		}
		var application v1beta1.Application
		if failure := cluster.Decode(object, &application); failure != nil {
			return nil, failure
		}
		applications = append(applications, application)
	}
	sort.Slice(applications, func(left, right int) bool {
		if applications[left].Namespace != applications[right].Namespace {
			return applications[left].Namespace < applications[right].Namespace
		}
		return applications[left].Name < applications[right].Name
	})
	return applications, nil
}

// reader is the cluster read seam for this call.
func (r Readings) reader() *cluster.Reader {
	return &cluster.Reader{Dynamic: r.dynamic, Now: r.now}
}

// offered indexes the catalogue entries by the definition they install.
func (r Readings) offered() map[string]catalogue.Entry {
	entries := map[string]catalogue.Entry{}
	if r.catalogue == nil {
		return entries
	}
	for _, entry := range r.catalogue.Entries() {
		entries[entry.Definition] = entry
	}
	return entries
}

// updateOf projects the update the catalogue offers one application: the entry
// declares a package digest other than the baseline the application applied
// (KOSD-C-32, KOSD-C-34). The version and digest are the entry's, and the
// preview is the preflight report the framework wrote on the application,
// presented unchanged (KOSD-C-34). An application with no baseline offers no
// update.
func updateOf(application v1beta1.Application, entry catalogue.Entry, offered bool) updateView {
	update := updateView{Preview: previewOf(application)}
	applied := application.Status.Applied
	if applied == nil {
		return update
	}
	update.InstalledDigest = applied.DefinitionDigest
	if !offered || applied.DefinitionDigest == "" || entry.Digest == "" {
		return update
	}
	if entry.Digest == applied.DefinitionDigest {
		return update
	}
	update.Available = true
	update.Version = entry.Version
	update.Digest = entry.Digest
	return update
}

// previewOf renders the preflight report the framework wrote, or nil when the
// application carries none (KOSD-C-34).
func previewOf(application v1beta1.Application) *previewView {
	report := application.Status.Upgrade
	if report == nil {
		return nil
	}
	return &previewView{
		Target:        report.Target,
		SpecDigest:    report.SpecDigest,
		RenderDigest:  report.RenderDigest,
		ConfigChanges: nonNil(report.ConfigChanges),
		RenderChanges: nonNil(report.RenderChanges),
		Irreversible:  nonNil(report.Irreversible),
		Migrations:    nonNil(report.Migrations),
		RolledBack:    report.RolledBack,
	}
}

// conditionsOf renders the conditions the page shows.
func conditionsOf(conditions []metav1.Condition) []conditionView {
	views := make([]conditionView, 0, len(conditions))
	for _, condition := range conditions {
		views = append(views, conditionView{
			Type:    condition.Type,
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		})
	}
	return views
}

// nonNil keeps an empty list in the served value instead of a null.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
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

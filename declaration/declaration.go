// Package declaration builds the package declaration of the appstore and its
// two projections: the manifest the packer validates (KOSD-C-18, SDK-C-3) and
// the declaration the control plane loads today. Both carry the same product
// data: the catalogue source that offers the shipped applications and the
// app-center page; the manifest additionally declares the window application
// (KOSD-C-73).
package declaration

import (
	"encoding/json"
	"fmt"

	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-appstore/appcenter"
	"github.com/tsic404/kos-appstore/catalogue"
)

// The package identity (KOSD-C-17).
const (
	// Identifier is the package identity across versions. The page items route
	// by the same identity, so the shell reaches this product's reader (KOSD-C-22).
	Identifier = appcenter.Identifier
	// Version is the package version.
	Version = "1.0.0"
	// FrameworkVersion is the framework range the package targets (KOSD-C-16).
	FrameworkVersion = ">=1.1 <2.0"
)

// Declaration returns the declaration the control plane loads (KOSD-C-18): the
// catalogue source and the app-center page. The contributions carry exactly the
// content the package ships, so an install writes what the digest covers
// (APPSTORE-C-24, APPSTORE-C-31).
func Declaration() (extension.Declaration, error) {
	source, err := catalogue.Source()
	if err != nil {
		return extension.Declaration{}, err
	}
	return extension.Declaration{
		Identifier:       Identifier,
		Version:          Version,
		FrameworkVersion: FrameworkVersion,
		Contributions: extension.Contributions{
			CatalogueSources: []extension.CatalogueSource{source},
			Pages:            []extension.Page{appcenter.Page()},
		},
		Permissions: []extension.Permission{
			{Contribution: appcenter.PageID, ReadScope: readScope(), RequiredRole: string(write.RoleMember)},
			{Contribution: catalogue.SourceID, ReadScope: readScope()},
		},
	}, nil
}

// Manifest returns the package declaration: the same contributions plus the
// app-center window application (KOSD-C-18, KOSD-C-73). It is UI-only, so it
// declares no write scope, no object class and no component configuration.
func Manifest() (manifest.ExtensionManifest, error) {
	source, err := catalogue.Source()
	if err != nil {
		return manifest.ExtensionManifest{}, err
	}
	sourceDocument, err := document(source)
	if err != nil {
		return manifest.ExtensionManifest{}, fmt.Errorf("render the catalogue source: %w", err)
	}
	pageDocument, err := document(appcenter.Page())
	if err != nil {
		return manifest.ExtensionManifest{}, fmt.Errorf("render the app-center page: %w", err)
	}
	return manifest.ExtensionManifest{
		ID:               Identifier,
		Version:          Version,
		FrameworkVersion: FrameworkVersion,
		Contributions: manifest.Contributions{
			CatalogSources: []map[string]any{sourceDocument},
			Pages:          []map[string]any{pageDocument},
			Apps:           []manifest.AppManifest{appcenter.App(Version)},
		},
		Dependencies: manifest.Dependencies{Framework: FrameworkVersion},
		Permissions:  manifest.ExtensionPermissions{Read: readScopes(), Roles: []string{string(write.RoleMember)}},
		Config:       map[string]any{},
		Status: manifest.StatusDeclaration{
			Component:     &manifest.ComponentStatus{Conditions: []string{}},
			ObjectClasses: map[string]manifest.ObjectStatus{},
		},
		UninstallRetention: manifest.UninstallRetention{
			Contributions: map[string]manifest.RetentionAction{
				"catalogSources": manifest.Delete,
				"pages":          manifest.Delete,
				"apps":           manifest.Delete,
			},
			ExternalResources: []string{},
		},
	}, nil
}

// ManifestDocument returns the manifest as the JSON document the packer
// validates (SDK-C-3). The packer reads a decoded document, and the SDK's
// contribution classes other than object classes, controllers and apps carry
// author payloads, so both projections are generated from the same typed
// product data.
func ManifestDocument() (map[string]any, error) {
	declared, err := Manifest()
	if err != nil {
		return nil, err
	}
	return document(declared)
}

// document renders one value as the decoded JSON document a manifest carries.
func document(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// readScope is what the app center reads: the definitions the catalogue lists
// and the applications the upgrade paths name (KOSD-C-12).
func readScope() extension.ReadScope {
	return extension.ReadScope{
		Groups: []string{api.Group},
		Kinds: []extension.GroupKind{
			{Kind: "Application"},
			{Kind: "ApplicationDefinition"},
		},
	}
}

// readScopes renders the same read scope for the packed manifest (KOSD-C-18).
func readScopes() []manifest.ReadScope {
	return []manifest.ReadScope{{
		APIGroup:  api.Group,
		Resources: []string{"applications", "applicationdefinitions"},
	}}
}

// Package artifact builds the installable package of the appstore (KOSD-C-17,
// SDK-C-4): the declaration plus the window's UI assets go through the one
// packing path every author uses, so a first-party product and a third-party
// extension ship the same structure and the digest covers both the manifest and
// every asset.
package artifact

import (
	"fmt"

	"github.com/tsic404/kos-sdk/packaging"

	"github.com/tsic404/kos-appstore/assets"
	"github.com/tsic404/kos-appstore/declaration"
)

// Package returns the logical package and its archive bytes.
func Package() (packaging.Package, []byte, error) {
	document, err := declaration.ManifestDocument()
	if err != nil {
		return packaging.Package{}, nil, fmt.Errorf("read the package declaration: %w", err)
	}
	carried, err := assets.Package()
	if err != nil {
		return packaging.Package{}, nil, err
	}
	built, archive, err := packaging.Pack(document, carried)
	if err != nil {
		return packaging.Package{}, nil, err
	}
	return built, archive, nil
}

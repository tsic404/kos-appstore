// Package assets carries the window document the app-center app ships
// (KOSD-C-73). The files are the package's UI assets: they are addressed by
// extension identity, version and digest, and the digest covers every one of
// them (KOSD-C-17, SDK-C-4).
package assets

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/tsic404/kos-sdk/packaging"
)

// Prefix is the package-relative directory every UI asset is stored under.
// It is the packer's own asset namespace, so the paths here are the paths the
// package carries.
const Prefix = packaging.AssetPrefix

//go:embed *.html *.css *.js *.svg
var files embed.FS

// Package returns every UI asset as the packer takes it, ordered by path so the
// digest does not depend on directory enumeration order.
func Package() ([]packaging.Asset, error) {
	names, err := fs.Glob(files, "*")
	if err != nil {
		return nil, fmt.Errorf("list the window assets: %w", err)
	}
	carried := make([]packaging.Asset, 0, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("read the window asset %s: %w", name, err)
		}
		carried = append(carried, packaging.Asset{Path: Prefix + name, Content: content})
	}
	return carried, nil
}

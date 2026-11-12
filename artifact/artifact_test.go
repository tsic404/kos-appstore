package artifact_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

	"github.com/tsic404/kos-sdk/packaging"

	"github.com/tsic404/kos-appstore/artifact"
	"github.com/tsic404/kos-appstore/assets"
	"github.com/tsic404/kos-appstore/declaration"
)

// entries reads the archive's member names and content.
func entries(t *testing.T, archive []byte) map[string][]byte {
	t.Helper()
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("open the archive: %v", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	carried := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return carried
		}
		if err != nil {
			t.Fatalf("read the archive: %v", err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read %s: %v", header.Name, err)
		}
		carried[header.Name] = content
	}
}

// TestPackageCarriesTheManifestAndEveryAsset checks the artifact the installer
// resolves: the manifest, every UI asset and the digest, nothing else, and the
// archive verifies against the digest it carries (KOSD-C-17, SDK-C-4).
func TestPackageCarriesTheManifestAndEveryAsset(t *testing.T) {
	built, archive, err := artifact.Package()
	if err != nil {
		t.Fatalf("build the package: %v", err)
	}
	carried := entries(t, archive)
	held, err := assets.Package()
	if err != nil {
		t.Fatalf("list the UI assets: %v", err)
	}
	if len(carried) != len(held)+2 {
		t.Fatalf("archive carries %d entries, want the manifest, %d assets and the digest", len(carried), len(held))
	}
	if _, found := carried[packaging.ManifestEntry]; !found {
		t.Fatalf("archive carries no %s", packaging.ManifestEntry)
	}
	for _, asset := range held {
		content, found := carried[asset.Path]
		if !found {
			t.Errorf("archive carries no asset %s", asset.Path)
			continue
		}
		if !bytes.Equal(content, asset.Content) {
			t.Errorf("asset %s differs from what the packer was given", asset.Path)
		}
	}
	named, found := carried[packaging.DigestEntry]
	if !found {
		t.Fatalf("archive carries no %s", packaging.DigestEntry)
	}
	if string(named) != built.Digest.String()+"\n" {
		t.Fatalf("digest entry = %q, want %q", string(named), built.Digest.String())
	}
	verified, err := packaging.Verify(archive, built.Digest)
	if err != nil {
		t.Fatalf("verify the package: %v", err)
	}
	if verified.Digest != built.Digest {
		t.Fatalf("verified digest = %s, want %s", verified.Digest, built.Digest)
	}

	// The window's icon must be an asset the package carries, or the launcher
	// would render nothing (KOSD-C-73).
	var document map[string]any
	if err := json.Unmarshal(carried[packaging.ManifestEntry], &document); err != nil {
		t.Fatalf("decode the manifest: %v", err)
	}
	if document["id"] != declaration.Identifier {
		t.Fatalf("manifest id = %v, want %q", document["id"], declaration.Identifier)
	}
}

// TestDigestCoversUIassets checks that a UI asset is part of the digest: the
// same manifest with different asset content yields a different package and a
// different digest, so an install or upgrade cannot accept swapped UI bytes
// (KOSD-C-17, SDK-C-4).
func TestDigestCoversUIassets(t *testing.T) {
	built, _, err := artifact.Package()
	if err != nil {
		t.Fatalf("build the package: %v", err)
	}
	document, err := declaration.ManifestDocument()
	if err != nil {
		t.Fatalf("build the manifest document: %v", err)
	}
	held, err := assets.Package()
	if err != nil {
		t.Fatalf("list the UI assets: %v", err)
	}
	changed := make([]packaging.Asset, len(held))
	copy(changed, held)
	changed[0].Content = append(append([]byte{}, changed[0].Content...), []byte("\n/* changed */\n")...)
	other, _, err := packaging.Pack(document, changed)
	if err != nil {
		t.Fatalf("pack the changed assets: %v", err)
	}
	if other.Digest == built.Digest {
		t.Fatalf("digest = %s, unchanged after the asset content changed", built.Digest)
	}
}

// TestPackageIsReproducible checks that the same manifest and assets produce the
// same archive and digest, so a release is verifiable against its recorded
// digest (SDK-C-4).
func TestPackageIsReproducible(t *testing.T) {
	first, firstArchive, err := artifact.Package()
	if err != nil {
		t.Fatalf("build the package: %v", err)
	}
	second, secondArchive, err := artifact.Package()
	if err != nil {
		t.Fatalf("build the package again: %v", err)
	}
	if first.Digest != second.Digest || !bytes.Equal(firstArchive, secondArchive) {
		t.Fatalf("two builds differ: %s vs %s", first.Digest, second.Digest)
	}
}

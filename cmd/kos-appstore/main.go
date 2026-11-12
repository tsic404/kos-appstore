// Command kos-appstore writes the installable package of the appstore to disk:
// the archive the installer fetches and the digest the system component names
// (KOSD-C-17).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsic404/kos-appstore/artifact"
	"github.com/tsic404/kos-appstore/declaration"
)

func main() {
	directory := flag.String("out", "dist", "directory the archive and its digest are written to")
	flag.Parse()
	if err := write(*directory); err != nil {
		fmt.Fprintln(os.Stderr, "build the appstore package:", err)
		os.Exit(1)
	}
}

// write packs the artifact and stores it under directory.
func write(directory string) error {
	built, archive, err := artifact.Package()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	base := "kos-appstore-" + declaration.Version
	if err := os.WriteFile(filepath.Join(directory, base+".tar.gz"), archive, 0o644); err != nil {
		return err
	}
	digest := []byte(built.Digest.String() + "\n")
	if err := os.WriteFile(filepath.Join(directory, base+".digest"), digest, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s %s\n", built.Digest, filepath.Join(directory, base+".tar.gz"))
	return nil
}

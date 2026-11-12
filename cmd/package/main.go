// Command package builds the kos-taskmanager extension package. It is the
// repository's build entry: the only way an installable artifact is produced.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsic404/kos-taskmanager/app"
	"github.com/tsic404/kos-taskmanager/internal/artifact"
)

func main() {
	root := flag.String("root", ".", "repository root holding the asset directory")
	out := flag.String("output", "dist", "directory the artifact is written to")
	flag.Parse()

	pkg, archive, err := artifact.Build(artifact.Options{Root: *root})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	name := fmt.Sprintf("%s-%s.tar.gz", app.Identifier, app.Version)
	archivePath := filepath.Join(*out, name)
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	digestPath := archivePath + ".digest"
	if err := os.WriteFile(digestPath, []byte(pkg.Digest+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("%s\n%s\n", archivePath, pkg.Digest)
}

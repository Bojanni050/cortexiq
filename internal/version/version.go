// Modifications copyright (c) 2026 Bojan van der Heide
// Package version carries the build identity of the binary.
//
// The values are injected at link time:
//
//	-ldflags "-X github.com/Bojanni050/cortexiq/internal/version.Version=v0.1.0 ..."
//
// Without those flags the binary reports "dev", which is the honest answer for
// a build from a working tree. Nothing in the application reads these values;
// they exist so an operator can tell two binaries apart.
package version

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	// Version is the release tag, for example "v0.1.0".
	Version = "dev"
	// Commit is the full git SHA the binary was built from.
	Commit = "none"
	// Date is the build timestamp in RFC3339.
	Date = "unknown"
)

// String returns the one-line identity: "cortexiq v0.1.0", or a clear
// "built from source" marker when no version was injected.
func String() string {
	if Version == "" || Version == "dev" {
		return "cortexiq dev (built from source)"
	}
	return "cortexiq " + Version
}

// Details returns the multi-line form printed by `cortexiq version`.
func Details() string {
	var b strings.Builder
	b.WriteString(String())
	b.WriteString("\n")
	fmt.Fprintf(&b, "commit: %s\n", shortCommit())
	fmt.Fprintf(&b, "built:  %s\n", Date)
	fmt.Fprintf(&b, "go:     %s\n", runtime.Version())
	return b.String()
}

func shortCommit() string {
	if Commit == "" || Commit == "none" {
		return "none"
	}
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}

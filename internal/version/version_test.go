// Modifications copyright (c) 2026 Bojan van der Heide
package version

import (
	"strings"
	"testing"
)

func TestStringDev(t *testing.T) {
	got := String()
	if !strings.Contains(got, "built from source") {
		t.Fatalf("String() = %q; want the built-from-source marker when no version is injected", got)
	}
	if !strings.Contains(got, "cortexiq dev") {
		t.Fatalf("String() = %q; want it to name the dev build", got)
	}
}

func TestStringRelease(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v0.1.0"

	if got, want := String(), "cortexiq v0.1.0"; got != want {
		t.Fatalf("String() = %q; want %q", got, want)
	}
}

func TestDetailsShortensCommit(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = oldVersion, oldCommit, oldDate })
	Version = "v0.1.0"
	Commit = "155a38a9d1f2c3b4"
	Date = "2026-10-24T10:00:00Z"

	got := Details()
	if !strings.Contains(got, "commit: 155a38a") {
		t.Fatalf("Details() = %q; want a 7-character commit", got)
	}
	if strings.Contains(got, "155a38a9d1f2c3b4") {
		t.Fatalf("Details() = %q; want the full SHA shortened", got)
	}
	if !strings.Contains(got, "built:  2026-10-24T10:00:00Z") {
		t.Fatalf("Details() = %q; want the build date", got)
	}
}

func TestDetailsUnknownCommit(t *testing.T) {
	old := Commit
	t.Cleanup(func() { Commit = old })
	Commit = ""

	if got := shortCommit(); got != "none" {
		t.Fatalf("shortCommit() = %q; want none for an empty commit", got)
	}
}

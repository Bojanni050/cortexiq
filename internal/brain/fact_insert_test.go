package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFactsAreInsertedInOnePlace is the gateway.no_bypass_path check from the
// Agent Memory Atlas, in its cheapest form: no file other than fact_insert.go
// may contain an INSERT INTO facts. It needs no database. A new path that
// writes facts directly would skip every check insertFact carries, and would do
// it silently, which is the failure the pattern exists to prevent.
func TestFactsAreInsertedInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "fact_insert.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(src)), "insert into facts") {
			t.Errorf("%s inserts into facts directly; use Brain.insertFact", f)
		}
	}

	// The scan above would pass if the guard file lost its statement, so the
	// one place that is allowed must actually hold it.
	src, err := os.ReadFile("fact_insert.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "INSERT INTO facts") {
		t.Error("fact_insert.go no longer holds the INSERT INTO facts statement")
	}
}

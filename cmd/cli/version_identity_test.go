// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/version"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The metric an operator alerts on must name the build that is running, not a
// constant someone typed once. This label read "0.3.0" while every release
// carried a v0.1.x tag, so Prometheus could not answer "which build is this?",
// and it disagreed with `cortexiq version` in the same container.
//
// The label is set in init() from version.Semver(), so this asserts the value
// package initialisation produced — no test-only rewriting of the gauge, which
// would only prove the test to itself. A test binary is built without the
// release's -ldflags, so the honest answer here is "dev".
func TestBuildInfoCarriesTheBuildsVersion(t *testing.T) {
	want := fmt.Sprintf(`
# HELP cortexiq_build_info Build information, value is always 1
# TYPE cortexiq_build_info gauge
cortexiq_build_info{version=%q} 1
`, version.Semver())

	if err := testutil.CollectAndCompare(buildInfo, strings.NewReader(want)); err != nil {
		t.Fatalf("cortexiq_build_info does not name the build that is running: %v", err)
	}
}

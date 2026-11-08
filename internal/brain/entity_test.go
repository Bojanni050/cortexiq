// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import "testing"

// The names in this table are the ones production actually stored, so the rule is
// tested against the mess it has to fold rather than against tidy examples.
func TestNormalizeEntityFoldsNameVariants(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		keeps bool
	}{
		{"Hindsight", "hindsight", true},
		{"hindsight mcp setup", "hindsight", true},
		{"hindsight mcp setup in claude desktop", "hindsight", true},
		{"Hindsight's MCP server", "hindsight", true},
		{"Hindsight Cloud", "hindsight cloud", true},
		{"Hindsight Cloud (vectorize.io)", "hindsight cloud", true},

		{"Claude Desktop", "claude desktop", true},
		{"The working Claude Desktop setup", "claude desktop", true},
		{"Claude Desktop on Windows", "claude desktop", true},
		{"Claude Desktop MSIX package", "claude desktop msix", true},

		{"claude_desktop_config.json", "claude_desktop_config.json", true},
		{"claude_desktop_config.json entry", "claude_desktop_config.json", true},

		{"npx", "npx", true},
		{"npx mcp-remote", "mcp-remote", true},
		{"npx with mcp-remote", "mcp-remote", true},
		{"--allow-http", "--allow-http", true},
		{"--allow-http flag", "--allow-http", true},

		{"mcpremotewatchdog scheduled task", "mcpremotewatchdog", true},
		{"strato.taile3931a.ts.net", "strato.taile3931a.ts.net", true},
		{"nginx basic auth", "nginx basic auth", true},

		{"the problem", "", false},
		{"the workload", "", false},
		{"the app", "", false},
		{"the setup", "", false},
		{"the", "", false},
		{"", "", false},
	}

	for _, c := range cases {
		got, keeps := NormalizeEntity(c.in)
		if keeps != c.keeps {
			t.Errorf("NormalizeEntity(%q) keeps = %v, want %v", c.in, keeps, c.keeps)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeEntity(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The qualifiers that carry identity must survive: folding "Hindsight Cloud" into
// "Hindsight" would merge two products, which is worse than leaving a duplicate.
func TestNormalizeEntityKeepsQualifiersThatIdentify(t *testing.T) {
	for _, name := range []string{"Hindsight Cloud", "Hindsight local", "Stash production environment"} {
		got, keeps := NormalizeEntity(name)
		if !keeps || got == "" {
			t.Errorf("NormalizeEntity(%q) dropped an entity with a real qualifier", name)
		}
	}
	if got, _ := NormalizeEntity("Hindsight Cloud"); got == "" {
		t.Fatal("empty")
	}
}

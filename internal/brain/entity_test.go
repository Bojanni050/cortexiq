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

		// Leftovers the dry run of 2026-11-08 exposed on /projects/infra: trimming
		// a trailing noun can leave an adjective, a clause or a gerund behind.
		{"separate hindsight instance", "hindsight", true},
		{"duplicate mcp-remote processes", "mcp-remote", true},
		{"seven slash-commands (mn-ask, mn-search, mn-stats)", "slash-commands", true},
		{"known-working setup", "", false},
		{"the self-hosted instance on a contabo vps", "", false},
		{"backup of the previous tailscale serve configuration", "", false},
		{"a separate store for each project located in <project>/.opencode", "", false},
		{"wrapping mcp-remote in conhost.exe --headless", "", false},
		{"the configuration known to work", "", false},
		{"no longer in use", "", false},
		{"relationships in batches of 5", "", false},

		// The second dry run: a port is a value, and a quote survived the trim.
		{"'--allow-http' flag", "--allow-http", true},
		{"'-y' flag", "-y", true},
		{":8443", "", false},
		{"8443 origin", "", false},
		{"8643 on localhost", "", false},
		{"bare 'url' entry", "", false},

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

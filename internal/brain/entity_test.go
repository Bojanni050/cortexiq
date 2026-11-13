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

// The third dry run left three kinds of noise that only look like names: a quote
// in the middle of the string, a table of entries, and an error message.
func TestNormalizeEntityDropsInnerQuotesAndMessages(t *testing.T) {
	fold := map[string]string{
		"bare 'url' entries":               "", // the quote goes, "url" is a role, "entries" a table
		"'--allow-http' flag":              "--allow-http",
		"'server disconnected' error":      "",
		"claude_desktop_config.json entry": "claude_desktop_config.json",
	}
	for in, want := range fold {
		got, ok := NormalizeEntity(in)
		if want == "" {
			if ok {
				t.Errorf("NormalizeEntity(%q) = %q, want dropped", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("NormalizeEntity(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestIsPlaceholderRelationType(t *testing.T) {
	placeholders := []string{"null", "NULL", " none ", "nil", "n/a", "N/A", "unknown", "undefined", "", "  ", "'null'", "null."}
	for _, s := range placeholders {
		if !IsPlaceholderRelationType(s) {
			t.Errorf("%q says nothing, it must count as a placeholder", s)
		}
	}
	// Real predicates must pass, including ones that merely contain a placeholder word.
	predicates := []string{"uses", "stored_in", "pushed to", "is_unknown_to", "no_longer_in_use", "nullifies", "none_of", "runs"}
	for _, s := range predicates {
		if IsPlaceholderRelationType(s) {
			t.Errorf("%q is a real predicate, it must not count as a placeholder", s)
		}
	}
}

// A word ending in -ing is a description when it is a plain verb form, and a name
// when it is built like an identifier. The dry run on production wanted to drop the
// project "PoYo-v6-uitbreiding" (68 relations) because uitbreiding ends in -ing.
func TestNormalizeEntityGerundRuleSparesIdentifiers(t *testing.T) {
	kept := map[string]string{
		"PoYo-v6-uitbreiding":     "poyo-v6-uitbreiding",
		"The PoYo-v6-uitbreiding": "poyo-v6-uitbreiding",
		"chrome.scripting":        "chrome.scripting",
		"macro-bevestiging":       "macro-bevestiging",
		"llm_bindings-binding":    "llm_bindings-binding",
	}
	for in, want := range kept {
		if got, ok := NormalizeEntity(in); !ok || got != want {
			t.Errorf("NormalizeEntity(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	dropped := []string{"dragging", "mapping", "reading", "packaging", "wrapping mcp-remote", "wrapping poyo-v6"}
	for _, in := range dropped {
		if got, ok := NormalizeEntity(in); ok {
			t.Errorf("NormalizeEntity(%q) = %q, true; a bare verb form or a gerund phrase names nothing", in, got)
		}
	}
}

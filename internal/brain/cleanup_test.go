package brain

import "testing"

func TestCleanupKey(t *testing.T) {
	same := [][]string{
		{"CortexIQ", "cortexiq", " cortexIQ "},
		{"Hermes agent", "hermes agent", "Hermes Agent", "hermes-agent", "The Hermes agent", "Hermes agent (hermes/desktop)"},
		{"White Vision", "white-vision"},
		{"Gaia-Cloud", "Gaia Cloud"},
		{"Chronicle-Gaia/clipper/", "Chronicle-Gaia clipper"},
	}
	for _, names := range same {
		want := cleanupKey(names[0])
		for _, n := range names[1:] {
			if got := cleanupKey(n); got != want {
				t.Errorf("cleanupKey(%q) = %q, want %q (same as %q)", n, got, want, names[0])
			}
		}
	}
	// Different things must not collapse together.
	if cleanupKey("Bo") == cleanupKey("Bojan") {
		t.Error("Bo and Bojan are aliases that spelling cannot tell, they must stay apart")
	}
	if cleanupKey("Hermes") == cleanupKey("Hermes agent") {
		t.Error("Hermes and Hermes agent are different names")
	}
	// Symbols that change the meaning of a name must keep names apart.
	if cleanupKey("C++") == cleanupKey("C#") || cleanupKey("C++") == cleanupKey("C") || cleanupKey("C#") == cleanupKey("C") {
		t.Errorf("C, C++ and C# are different things: %q %q %q", cleanupKey("C"), cleanupKey("C++"), cleanupKey("C#"))
	}
	// A name of only separators keeps its own form instead of an empty key.
	if cleanupKey("???") == "" || cleanupKey("???") == cleanupKey("!!!") {
		t.Errorf("symbol-only names must keep their own key: %q vs %q", cleanupKey("???"), cleanupKey("!!!"))
	}
}

func TestDuplicateKind(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"CortexIQ", "cortexiq", "cortexIQ"}, DupCase},
		{[]string{"White Vision", "white-vision"}, DupPunctuation},
		{[]string{"Gaia-Cloud", "Gaia Cloud"}, DupPunctuation},
		{[]string{"Foundation", "Foundation (Gaia)"}, DupQualifier},
		{[]string{"Hermes agent", "The Hermes agent"}, DupQualifier},
		// A qualifier anywhere in the group makes it the less safe kind.
		{[]string{"Hermes agent", "hermes-agent", "Hermes agent (hermes/desktop)"}, DupQualifier},
	}
	for _, tt := range tests {
		if got := duplicateKind(tt.names); got != tt.want {
			t.Errorf("duplicateKind(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func TestBuildCleanupDuplicatesCountAcrossFactsAndRelations(t *testing.T) {
	c := buildCleanup(
		map[string]int{"CortexIQ": 400, "cortexiq": 5, "Gaia": 10},
		map[string]int{"CortexIQ": 283, "cortexIQ": 2, "Gaia": 3},
		100)

	if c.DuplicateTotal != 1 || len(c.Duplicates) != 1 {
		t.Fatalf("only CortexIQ is spelled several ways: %+v", c.Duplicates)
	}
	g := c.Duplicates[0]
	if g.Kind != DupCase || g.Facts != 405 || g.Relations != 285 || len(g.Variants) != 3 {
		t.Errorf("group should add up both sources over three spellings: %+v", g)
	}
	if g.Variants[0].Name != "CortexIQ" || g.Variants[0].Facts != 400 || g.Variants[0].Relations != 283 {
		t.Errorf("busiest spelling first, with both counts: %+v", g.Variants[0])
	}
}

func TestBuildCleanupIslands(t *testing.T) {
	c := buildCleanup(
		map[string]int{"Chronicle-db": 6, "pc-monitor": 6, "Gaia": 10, "oc-mnemoria-v2": 2, "OC-Mnemoria-v2": 3},
		map[string]int{"gaia": 4}, // Gaia is related, only spelled with other capitals
		100)

	if c.IslandTotal != 3 {
		t.Fatalf("want Chronicle-db, pc-monitor and oc-mnemoria-v2, got %+v", c.Islands)
	}
	for _, is := range c.Islands {
		if is.Entity == "Gaia" {
			t.Errorf("Gaia is in a relation (capitals are ignored there too), not an island: %+v", is)
		}
	}
	// Two spellings of one island are one island, summed and named after the busier.
	var merged *Island
	for i := range c.Islands {
		if entityKey(c.Islands[i].Entity) == "oc-mnemoria-v2" {
			merged = &c.Islands[i]
		}
	}
	if merged == nil || merged.Facts != 5 || merged.Entity != "OC-Mnemoria-v2" {
		t.Errorf("spellings of one island should be summed: %+v", merged)
	}
	if c.Islands[0].Facts < c.Islands[len(c.Islands)-1].Facts {
		t.Errorf("busiest island first: %+v", c.Islands)
	}
}

func TestBuildCleanupLimitsListsButNotTotals(t *testing.T) {
	facts := map[string]int{}
	for i := 0; i < 30; i++ {
		facts[string(rune('a'+i%26))+string(rune('a'+i/26))+"x"] = 1 + i
	}
	c := buildCleanup(facts, map[string]int{}, 10)
	if c.IslandTotal != 30 || len(c.Islands) != 10 {
		t.Errorf("total must stay 30 while the list is cut to 10: total %d, shown %d", c.IslandTotal, len(c.Islands))
	}
	if c.Totals.FactEntities != 30 || c.Totals.RelationEntities != 0 {
		t.Errorf("totals count distinct entities per source: %+v", c.Totals)
	}
}

func TestBuildCleanupEmpty(t *testing.T) {
	c := buildCleanup(nil, nil, 100)
	if c.Duplicates == nil || c.Islands == nil || c.Weak == nil || c.Unmapped == nil {
		t.Error("lists must be empty arrays, not null, so the UI can map over them")
	}
	if c.DuplicateTotal != 0 || c.IslandTotal != 0 {
		t.Errorf("nothing to clean up: %+v", c)
	}
}

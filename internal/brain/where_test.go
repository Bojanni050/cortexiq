// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"reflect"
	"testing"
	"time"
)

// The WHERE builders are shared by the list query and its Count* sibling, so
// these tests pin the clause each filter produces; a change that silently alters
// one of them changes what the API reports as a total.

func TestFactWhere(t *testing.T) {
	ns := []int64{7, 8}
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		since     *time.Time
		until     *time.Time
		asOf      *time.Time
		wantWhere string
		wantArgs  int
	}{
		{"no range", nil, nil, nil, "namespace_id = ANY($1)", 1},
		{"since", &at, nil, nil, "namespace_id = ANY($1) AND created_at >= $2", 2},
		{"until", nil, &at, nil, "namespace_id = ANY($1) AND created_at <= $2", 2},
		{
			"since and until",
			&at, &at, nil,
			"namespace_id = ANY($1) AND created_at >= $2 AND created_at <= $3",
			3,
		},
		{
			// as-of reuses one placeholder for both comparisons, so the
			// argument appears once.
			"as of",
			nil, nil, &at,
			"namespace_id = ANY($1) AND COALESCE(valid_from, created_at) <= $2 AND (valid_until IS NULL OR valid_until > $2)",
			2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			where, args := factWhere(ns, tc.since, tc.until, tc.asOf)
			if where != tc.wantWhere {
				t.Errorf("where = %q, want %q", where, tc.wantWhere)
			}
			if len(args) != tc.wantArgs {
				t.Fatalf("args = %v, want %d entries", args, tc.wantArgs)
			}
			if !reflect.DeepEqual(args[0], ns) {
				t.Errorf("first arg = %v, want the namespace ids", args[0])
			}
		})
	}
}

func TestGoalWhere(t *testing.T) {
	ns := []int64{1}
	id := int64(42)

	tests := []struct {
		name      string
		status    string
		parentID  *int64
		wantWhere string
		wantArgs  int
	}{
		// Without a status the list means "top-level goals", which is also what
		// CountGoals must count.
		{
			"no filters means roots",
			"", nil,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND parent_id IS NULL",
			1,
		},
		{
			"status only",
			"active", nil,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND status = $2",
			2,
		},
		{
			"parent only",
			"", &id,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND parent_id = $2",
			2,
		},
		{
			"both",
			"active", &id,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND status = $2 AND parent_id = $3",
			3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			where, args := goalWhere(ns, tc.status, tc.parentID)
			if where != tc.wantWhere {
				t.Errorf("where = %q, want %q", where, tc.wantWhere)
			}
			if len(args) != tc.wantArgs {
				t.Fatalf("args = %v, want %d entries", args, tc.wantArgs)
			}
		})
	}
}

func TestFailureWhere(t *testing.T) {
	ns := []int64{3}
	id := int64(9)

	where, args := failureWhere(ns, nil)
	want := "namespace_id = ANY($1) AND deleted_at IS NULL"
	if where != want || len(args) != 1 {
		t.Errorf("without goal = (%q, %v), want (%q, [ids])", where, args, want)
	}

	where, args = failureWhere(ns, &id)
	want = "namespace_id = ANY($1) AND deleted_at IS NULL AND goal_id = $2"
	if where != want || len(args) != 2 {
		t.Errorf("with goal = (%q, %v), want (%q, [ids 9])", where, args, want)
	}
	if args[1] != id {
		t.Errorf("second arg = %v, want %d", args[1], id)
	}
}

func TestHypothesisWhere(t *testing.T) {
	ns := []int64{5}

	where, args := hypothesisWhere(ns, "")
	want := "namespace_id = ANY($1) AND deleted_at IS NULL"
	if where != want || len(args) != 1 {
		t.Errorf("without status = (%q, %v), want (%q, [ids])", where, args, want)
	}

	where, args = hypothesisWhere(ns, "proposed")
	want = "namespace_id = ANY($1) AND deleted_at IS NULL AND status = $2"
	if where != want || len(args) != 2 || args[1] != "proposed" {
		t.Errorf("with status = (%q, %v), want (%q, [ids proposed])", where, args, want)
	}
}

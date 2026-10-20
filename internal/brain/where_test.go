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
		{"no range", nil, nil, nil, "namespace_id = ANY($1) AND deleted_at IS NULL", 1},
		{"since", &at, nil, nil, "namespace_id = ANY($1) AND deleted_at IS NULL AND created_at >= $2", 2},
		{"until", nil, &at, nil, "namespace_id = ANY($1) AND deleted_at IS NULL AND created_at <= $2", 2},
		{
			"since and until",
			&at, &at, nil,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND created_at >= $2 AND created_at <= $3",
			3,
		},
		{
			// as-of is knowledge time and reuses one placeholder for both
			// comparisons, so the argument appears once.
			"as of",
			nil, nil, &at,
			"namespace_id = ANY($1) AND deleted_at IS NULL AND created_at <= $2 AND (known_until IS NULL OR known_until > $2)",
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

func TestFactWhereWindow(t *testing.T) {
	ns := []int64{7, 8}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	kFrom := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	kTo := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	const base = "namespace_id = ANY($1) AND deleted_at IS NULL"
	tests := []struct {
		name                               string
		from, to, asOf, knownFrom, knownTo *time.Time
		want                               string
		wantArgs                           []any
	}{
		{"from only", &from, nil, nil, nil, nil, base + " AND COALESCE(valid_from, created_at) < 'infinity'::timestamptz AND (valid_until IS NULL OR valid_until > $2)", []any{ns, from}},
		{"to only", nil, &to, nil, nil, nil, base + " AND COALESCE(valid_from, created_at) < $2 AND (valid_until IS NULL OR valid_until > '-infinity'::timestamptz)", []any{ns, to}},
		{"from and to", &from, &to, nil, nil, nil, base + " AND COALESCE(valid_from, created_at) < $3 AND (valid_until IS NULL OR valid_until > $2)", []any{ns, from, to}},
		{"as of and window", &from, &to, &asOf, nil, nil, base + " AND created_at <= $2 AND (known_until IS NULL OR known_until > $2) AND COALESCE(valid_from, created_at) < $4 AND (valid_until IS NULL OR valid_until > $3)", []any{ns, asOf, from, to}},
		// Knowledge time never looks at valid_from/valid_until.
		{"known window", nil, nil, nil, &kFrom, &kTo, base + " AND created_at < $2 AND (known_until IS NULL OR known_until > $3)", []any{ns, kTo, kFrom}},
		{"known to only", nil, nil, nil, nil, &kTo, base + " AND created_at < $2", []any{ns, kTo}},
		{"known from only", nil, nil, nil, &kFrom, nil, base + " AND (known_until IS NULL OR known_until > $2)", []any{ns, kFrom}},
		{"known window and valid window", &from, &to, nil, &kFrom, &kTo, base + " AND created_at < $2 AND (known_until IS NULL OR known_until > $3) AND COALESCE(valid_from, created_at) < $5 AND (valid_until IS NULL OR valid_until > $4)", []any{ns, kTo, kFrom, from, to}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			where, args := factWhereWindow(ns, nil, nil, tc.asOf, tc.from, tc.to, tc.knownFrom, tc.knownTo)
			if where != tc.want {
				t.Errorf("where = %q, want %q", where, tc.want)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tc.wantArgs)
			}
		})
	}
}

func TestCheckFactWindows(t *testing.T) {
	a := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	b := a.AddDate(0, 1, 0)
	tests := []struct {
		name                       string
		asOf, from, to, kFrom, kTo *time.Time
		wantErr                    bool
	}{
		{"none", nil, nil, nil, nil, nil, false},
		{"valid window", nil, &a, &b, nil, nil, false},
		{"reversed valid window", nil, &b, &a, nil, nil, true},
		{"known window", nil, nil, nil, &a, &b, false},
		{"reversed known window", nil, nil, nil, &b, &a, true},
		{"as of alone", &a, nil, nil, nil, nil, false},
		{"as of with known_from", &a, nil, nil, &a, nil, true},
		{"as of with known_to", &a, nil, nil, nil, &b, true},
		{"as of with valid window", &a, &a, &b, nil, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkFactWindows(tc.asOf, tc.from, tc.to, tc.kFrom, tc.kTo); (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
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

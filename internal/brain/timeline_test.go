package brain

import (
	"testing"
	"time"
)

func tr(id int64, day int, entity string) timelineRow {
	return timelineRow{ID: id, CreatedAt: time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC), Entity: entity}
}

func rowFor(tl *Timeline, entity string) *TimelineRow {
	for i := range tl.Rows {
		if tl.Rows[i].Entity == entity {
			return &tl.Rows[i]
		}
	}
	return nil
}

func TestBuildTimelineGroupsEntitiesIgnoringCase(t *testing.T) {
	tl := buildTimeline([]timelineRow{
		tr(1, 1, "CortexIQ"), tr(2, 2, "cortexiq"), tr(3, 3, " CortexIQ "), tr(4, 4, "Gaia"),
	}, TimelineOptions{})

	c := rowFor(tl, "CortexIQ")
	if c == nil || c.Total != 3 || len(c.Facts) != 3 {
		t.Fatalf("three spellings should fold into one CortexIQ row: %+v", tl.Rows)
	}
	if tl.Rows[0].Entity != "CortexIQ" || tl.Rows[1].Entity != "Gaia" {
		t.Errorf("busiest entity must come first: %+v", tl.Rows)
	}
}

func TestBuildTimelineFoldsTheRestAndEntitylessFactsIntoOther(t *testing.T) {
	tl := buildTimeline([]timelineRow{
		tr(1, 1, "A"), tr(2, 2, "A"), tr(3, 3, "A"),
		tr(4, 4, "B"), tr(5, 5, "B"),
		tr(6, 6, "C"),
		tr(7, 2, ""),
	}, TimelineOptions{Rows: 2})

	if len(tl.Rows) != 3 {
		t.Fatalf("want rows A, B and other, got %+v", tl.Rows)
	}
	other := tl.Rows[2]
	if other.Entity != "" || other.Total != 2 {
		t.Fatalf("other row should hold C and the entityless fact: %+v", other)
	}
	if other.Facts[0].ID != 7 || other.Facts[1].ID != 6 {
		t.Errorf("other row must stay oldest first: %+v", other.Facts)
	}
	if tl.Total != 7 {
		t.Errorf("Total = %d, want 7", tl.Total)
	}
}

func TestBuildTimelineRangeAndSuperseded(t *testing.T) {
	until := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	by := int64(2)
	old := tr(1, 3, "Postgres")
	old.ValidUntil, old.SupersededBy = &until, &by
	tl := buildTimeline([]timelineRow{tr(2, 9, "Postgres"), old, tr(3, 6, "Postgres")}, TimelineOptions{})

	if tl.Superseded != 1 {
		t.Errorf("Superseded = %d, want 1", tl.Superseded)
	}
	if tl.From == nil || tl.From.Day() != 3 || tl.To == nil || tl.To.Day() != 9 {
		t.Errorf("range should span day 3 to day 9: %v - %v", tl.From, tl.To)
	}
	first := tl.Rows[0].Facts[0]
	if first.ID != 1 || first.SupersededBy == nil || *first.SupersededBy != 2 || first.ValidUntil == nil {
		t.Errorf("oldest fact is the superseded one and must keep its link: %+v", first)
	}
	if tl.Rows[0].Facts[2].SupersededBy != nil {
		t.Errorf("the newest fact must not carry a superseded link: %+v", tl.Rows[0].Facts[2])
	}
}

func TestBuildTimelineEmptyAndRowLimits(t *testing.T) {
	tl := buildTimeline(nil, TimelineOptions{})
	if tl.Rows == nil || len(tl.Rows) != 0 || tl.From != nil || tl.To != nil || tl.Total != 0 {
		t.Errorf("empty input must give empty rows, no range: %+v", tl)
	}

	var rows []timelineRow
	for i := 0; i < 60; i++ {
		rows = append(rows, tr(int64(i+1), 1+i%28, string(rune('A'+i%26))+string(rune('a'+i/26))))
	}
	tl = buildTimeline(rows, TimelineOptions{Rows: 500})
	if got := len(tl.Rows); got != maxTimelineRows+1 {
		t.Errorf("row cap should be %d entities plus other, got %d rows", maxTimelineRows, got)
	}
}

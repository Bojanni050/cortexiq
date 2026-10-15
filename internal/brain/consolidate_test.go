package brain

import (
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func TestContiguousCheckpoint(t *testing.T) {
	tests := []struct {
		name    string
		ids     []int64
		done    []int64
		current int64
		want    int64
	}{
		{"all done", []int64{1, 2, 3}, []int64{1, 2, 3}, 0, 3},
		{"failure in the middle stops before it", []int64{1, 2, 3, 4}, []int64{1, 2, 4}, 0, 2},
		{"first failed keeps current", []int64{5, 6}, []int64{6}, 4, 4},
		{"nothing done keeps current", []int64{5, 6}, nil, 4, 4},
		{"empty batch keeps current", nil, nil, 7, 7},
	}
	for _, tt := range tests {
		done := map[int64]bool{}
		for _, id := range tt.done {
			done[id] = true
		}
		if got := contiguousCheckpoint(tt.ids, done, tt.current); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestCalculateConfidence(t *testing.T) {
	if got := calculateConfidence(0, true); got != 0 {
		t.Errorf("zero observations = %v, want 0", got)
	}
	plain, structured := calculateConfidence(2, false), calculateConfidence(2, true)
	if structured <= plain {
		t.Errorf("structured (%v) should exceed plain (%v)", structured, plain)
	}
	if got := calculateConfidence(1000, true); got > 1 {
		t.Errorf("confidence %v exceeds 1", got)
	}
}

func TestPartitionFragments(t *testing.T) {
	episodes := []models.Episode{
		{ID: 1, Content: "2"},
		{ID: 2, Content: "  gedaan  "},
		{ID: 3, Content: "CortexIQ draait op poort 9090"},
		{ID: 4, Content: "ok"},
	}

	kept, fragments := partitionFragments(episodes, 24)
	if len(kept) != 1 || kept[0].ID != 3 {
		t.Fatalf("kept = %+v, want only episode 3", kept)
	}
	if len(fragments) != 3 {
		t.Fatalf("fragments = %+v, want the three short episodes", fragments)
	}

	// Length is counted in runes after trimming, so an accented 24-rune memory
	// is not judged by its byte count.
	accented := strings.Repeat("é", 24)
	kept, fragments = partitionFragments([]models.Episode{{ID: 9, Content: accented}}, 24)
	if len(kept) != 1 || len(fragments) != 0 {
		t.Fatalf("24 accented runes should be kept: kept=%+v fragments=%+v", kept, fragments)
	}

	// 0 disables the filter; nothing is dropped.
	kept, fragments = partitionFragments(episodes, 0)
	if len(kept) != len(episodes) || fragments != nil {
		t.Fatalf("disabled filter changed the batch: kept=%+v fragments=%+v", kept, fragments)
	}
}

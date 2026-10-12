package brain

import "testing"

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

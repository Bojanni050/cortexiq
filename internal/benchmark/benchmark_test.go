// Modifications copyright (c) 2026 Bojan van der Heide
package benchmark

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadDataset(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
		check   func(t *testing.T, ds *Dataset)
	}{
		{
			name: "wrapped format",
			json: `{"name": "test", "items": [{"question_id": "i1", "haystack_sessions": [{"id": "s1", "date": "2025-01-01", "turns": [{"role": "user", "content": "hello"}]}], "qa_pairs": [{"id": "q1", "category": "single", "question": "what?", "answer": "hello"}]}]}`,
			check: func(t *testing.T, ds *Dataset) {
				if ds.Name != "test" {
					t.Errorf("Name = %q, want %q", ds.Name, "test")
				}
				if len(ds.Items) != 1 {
					t.Errorf("len(Items) = %d, want 1", len(ds.Items))
				}
				if ds.CountQuestions() != 1 {
					t.Errorf("CountQuestions() = %d, want 1", ds.CountQuestions())
				}
			},
		},
		{
			name: "bare array format (LongMemEval)",
			json: `[{"question_id": "i1", "haystack_sessions": [{"turns": [{"role": "user", "content": "hi"}]}], "qa_pairs": [{"id": "q1", "question": "what?", "answer": "hi"}]}]`,
			check: func(t *testing.T, ds *Dataset) {
				if len(ds.Items) != 1 {
					t.Errorf("len(Items) = %d, want 1", len(ds.Items))
				}
			},
		},
		{
			name:    "empty dataset",
			json:    `{"name": "empty", "items": []}`,
			wantErr: "no items",
		},
		{
			name:    "duplicate item id",
			json:    `{"name": "dup", "items": [{"question_id": "i1", "haystack_sessions": [{"turns": [{"role": "user", "content": "a"}]}], "qa_pairs": [{"id": "q1", "question": "?", "answer": "a"}]}, {"question_id": "i1", "haystack_sessions": [{"turns": [{"role": "user", "content": "b"}]}], "qa_pairs": [{"id": "q2", "question": "?", "answer": "b"}]}]}`,
			wantErr: "duplicate item id",
		},
		{
			name:    "empty question",
			json:    `{"name": "emptyq", "items": [{"question_id": "i1", "haystack_sessions": [{"turns": [{"role": "user", "content": "a"}]}], "qa_pairs": [{"id": "q1", "question": "", "answer": "a"}]}]}`,
			wantErr: "empty",
		},
		{
			name:    "empty answer",
			json:    `{"name": "emptya", "items": [{"question_id": "i1", "haystack_sessions": [{"turns": [{"role": "user", "content": "a"}]}], "qa_pairs": [{"id": "q1", "question": "?", "answer": ""}]}]}`,
			wantErr: "empty answer",
		},
		{
			name:    "no sessions",
			json:    `{"name": "nosess", "items": [{"question_id": "i1", "haystack_sessions": [], "qa_pairs": [{"id": "q1", "question": "?", "answer": "a"}]}]}`,
			wantErr: "no sessions",
		},
		{
			name:    "session without turns",
			json:    `{"name": "noturns", "items": [{"question_id": "i1", "haystack_sessions": [{"turns": []}], "qa_pairs": [{"id": "q1", "question": "?", "answer": "a"}]}]}`,
			wantErr: "no turns",
		},
		{
			name:    "invalid json",
			json:    `{not json}`,
			wantErr: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, err := LoadDatasetFromReader(strings.NewReader(tt.json), "test")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, ds)
			}
		})
	}
}

func TestExactMatchJudge(t *testing.T) {
	judge := ExactMatchJudge{}
	ctx := context.Background()

	tests := []struct {
		name      string
		expected  string
		generated string
		wantScore float64
	}{
		{"exact match", "Amsterdam", "Amsterdam", 1.0},
		{"case insensitive", "Amsterdam", "amsterdam", 1.0},
		{"whitespace trimmed", " Amsterdam ", "Amsterdam", 1.0},
		{"different", "Amsterdam", "Paris", 0.0},
		{"partial", "Amsterdam, Netherlands", "Amsterdam", 0.0},
		{"empty generated", "Amsterdam", "", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, err := judge.Judge(ctx, "q", tt.expected, tt.generated)
			if err != nil {
				t.Fatalf("Judge error: %v", err)
			}
			if score != tt.wantScore {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
		})
	}
}

func TestParseJudgeResponse(t *testing.T) {
	tests := []struct {
		name      string
		resp      string
		wantScore float64
		wantErr   bool
	}{
		{"plain json", `{"score": 0.85, "reasoning": "mostly correct"}`, 0.85, false},
		{"markdown code block", "```json\n{\"score\": 0.5, \"reasoning\": \"partial\"}\n```", 0.5, false},
		{"with surrounding text", `Here is my evaluation: {"score": 0.9, "reasoning": "good"} done.`, 0.9, false},
		{"integer score", `{"score": 1, "reasoning": "perfect"}`, 1.0, false},
		{"no json", "I think the answer is good.", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, err := parseJudgeResponse(tt.resp)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if score != tt.wantScore {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
		})
	}
}

func TestFallbackScore(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		generated string
		wantMin   float64
		wantMax   float64
	}{
		{"identical", "the cat sat on the mat", "the cat sat on the mat", 1.0, 1.0},
		{"no overlap", "the cat sat", "dogs run fast", 0.0, 0.1},
		{"partial overlap", "the cat sat on the mat", "the cat ran fast", 0.1, 0.6},
		{"empty expected", "", "something", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := fallbackScore(tt.expected, tt.generated)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("score = %f, want in [%f, %f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestFormatSession(t *testing.T) {
	s := Session{
		ID:   "s1",
		Date: "2025-01-15",
		Turns: []Turn{
			{Role: "user", Content: "Hello there"},
			{Role: "assistant", Content: "Hi! How can I help?"},
		},
	}
	got := formatSession(s)
	want := "[Conversation on 2025-01-15]\nUser: Hello there\nAssistant: Hi! How can I help?"
	if got != want {
		t.Errorf("formatSession() = %q, want %q", got, want)
	}
}

func TestFormatSessionNoDate(t *testing.T) {
	s := Session{
		Turns: []Turn{
			{Role: "user", Content: "Hi"},
		},
	}
	got := formatSession(s)
	if strings.Contains(got, "Conversation on") {
		t.Errorf("formatSession() should not include date header when date is empty: %q", got)
	}
	if !strings.Contains(got, "User: Hi") {
		t.Errorf("formatSession() should include the turn: %q", got)
	}
}

func TestParseSessionDate(t *testing.T) {
	tests := []struct {
		date    string
		wantNil bool
	}{
		{"2025-01-15", false},
		{"2025-01-15T10:30:00Z", false},
		{"2025-01-15T10:30:00", false},
		{"", true},
		{"invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			got := parseSessionDate(tt.date)
			if tt.wantNil && got != nil {
				t.Errorf("parseSessionDate(%q) = %v, want nil", tt.date, got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("parseSessionDate(%q) = nil, want a time", tt.date)
			}
		})
	}
}

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"item-001", "item-001"},
		{"Item_002", "item_002"},
		{"item 003", "item-003"},
		{"-leading", "i-leading"},
		{"", "item"},
		{"special!@#chars", "special---chars"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := sanitizeSlug(tt.id)
			if got != tt.want {
				t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestNormalizeNamespace(t *testing.T) {
	tests := []struct {
		ns   string
		want string
	}{
		{"", "/benchmark"},
		{"benchmark", "/benchmark"},
		{"/benchmark", "/benchmark"},
		{"/benchmark/", "/benchmark"},
		{"/my-bench", "/my-bench"},
		{"my-bench", "/my-bench"},
	}
	for _, tt := range tests {
		t.Run(tt.ns, func(t *testing.T) {
			got := normalizeNamespace(tt.ns)
			if got != tt.want {
				t.Errorf("normalizeNamespace(%q) = %q, want %q", tt.ns, got, tt.want)
			}
		})
	}
}

func TestSummarizeResults(t *testing.T) {
	results := []ItemResult{
		{
			ItemID:           "i1",
			IngestedEpisodes: 3,
			Questions: []BenchmarkResult{
				{QuestionID: "q1", Category: "single", JudgeScore: 1.0, ExactMatch: true, DurationMs: 100},
				{QuestionID: "q2", Category: "single", JudgeScore: 0.0, ExactMatch: false, DurationMs: 200},
				{QuestionID: "q3", Category: "multi", JudgeScore: 0.8, ExactMatch: false, DurationMs: 150},
			},
		},
		{
			ItemID:           "i2",
			IngestedEpisodes: 2,
			Questions: []BenchmarkResult{
				{QuestionID: "q4", Category: "multi", JudgeScore: 0.6, ExactMatch: false, DurationMs: 300, Error: ""},
				{QuestionID: "q5", Category: "temporal", Error: "reflect failed"},
			},
		},
	}

	s := summarizeResults(results)

	if s.Questions != 5 {
		t.Errorf("Questions = %d, want 5", s.Questions)
	}
	if s.TotalEpisodes != 5 {
		t.Errorf("TotalEpisodes = %d, want 5", s.TotalEpisodes)
	}
	if s.Errors != 1 {
		t.Errorf("Errors = %d, want 1", s.Errors)
	}
	// 3 out of 5 correct (score >= 0.5): q1, q3, q4
	if s.Accuracy != 0.6 {
		t.Errorf("Accuracy = %f, want 0.6", s.Accuracy)
	}
	// Exact matches: q1 only
	if s.ExactMatchRate != 0.2 {
		t.Errorf("ExactMatchRate = %f, want 0.2", s.ExactMatchRate)
	}
	// By category
	if s.ByCategory["single"].Questions != 2 {
		t.Errorf("single category questions = %d, want 2", s.ByCategory["single"].Questions)
	}
	if s.ByCategory["single"].Accuracy != 0.5 {
		t.Errorf("single category accuracy = %f, want 0.5", s.ByCategory["single"].Accuracy)
	}
	if s.ByCategory["multi"].Questions != 2 {
		t.Errorf("multi category questions = %d, want 2", s.ByCategory["multi"].Questions)
	}
	if s.ByCategory["temporal"].Questions != 1 {
		t.Errorf("temporal category questions = %d, want 1", s.ByCategory["temporal"].Questions)
	}
}

func TestBenchmarkReportJSON(t *testing.T) {
	report := &BenchmarkReport{
		Name:      "test",
		Items:     1,
		Questions: 2,
		Summary: Summary{
			Questions:         2,
			Accuracy:          0.5,
			AverageJudgeScore: 0.75,
		},
		Results: []ItemResult{
			{
				ItemID: "i1",
				Questions: []BenchmarkResult{
					{QuestionID: "q1", JudgeScore: 1.0, GeneratedAnswer: "yes"},
					{QuestionID: "q2", JudgeScore: 0.5, GeneratedAnswer: "maybe"},
				},
			},
		},
	}

	data, err := MarshalJSON(report)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify it's valid JSON and contains expected fields.
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["name"] != "test" {
		t.Errorf("name = %v, want test", decoded["name"])
	}
	if decoded["items"].(float64) != 1 {
		t.Errorf("items = %v, want 1", decoded["items"])
	}
}

func TestDatasetCounts(t *testing.T) {
	ds := &Dataset{
		Name: "test",
		Items: []Item{
			{
				ID: "i1",
				Sessions: []Session{
					{ID: "s1", Turns: []Turn{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}}},
					{ID: "s2", Turns: []Turn{{Role: "user", Content: "c"}}},
				},
				Questions: []Question{{ID: "q1"}, {ID: "q2"}},
			},
			{
				ID: "i2",
				Sessions: []Session{
					{ID: "s3", Turns: []Turn{{Role: "user", Content: "d"}}},
				},
				Questions: []Question{{ID: "q3"}},
			},
		},
	}

	if ds.CountQuestions() != 3 {
		t.Errorf("CountQuestions() = %d, want 3", ds.CountQuestions())
	}
	if ds.CountSessions() != 3 {
		t.Errorf("CountSessions() = %d, want 3", ds.CountSessions())
	}
	if ds.CountTurns() != 4 {
		t.Errorf("CountTurns() = %d, want 4", ds.CountTurns())
	}
}

func TestWordSet(t *testing.T) {
	ws := wordSet("The quick brown fox jumps over the lazy dog")
	// "the" is 3 chars, should be included (len > 2)
	if !ws["the"] {
		t.Error("wordSet should include 'the' (len 3 > 2)")
	}
	if !ws["quick"] {
		t.Error("wordSet should include 'quick'")
	}
	// "a" is 1 char, should be excluded
	ws2 := wordSet("a an is to")
	if len(ws2) != 0 {
		t.Errorf("wordSet should exclude short words, got %v", ws2)
	}
}

func TestParseScore(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"0.85", 0.85, false},
		{"85%", 85, false},
		{"score: 0.9", 0.9, false},
		{"  0.5  ", 0.5, false},
		{"invalid", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseScore(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseScore(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

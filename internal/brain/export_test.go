package brain

import (
	"strings"
	"testing"
)

func TestDecodeRecord(t *testing.T) {
	ok := `{"type":"fact","namespace":"/projects/stash","content":"Stash runs on port 8080","confidence":0.8}`
	rec, err := decodeRecord([]byte(ok))
	if err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	if rec.Type != RecordFact || rec.Namespace != "/projects/stash" || rec.Confidence == nil || *rec.Confidence != 0.8 {
		t.Errorf("unexpected record: %+v", rec)
	}

	bad := map[string]string{
		"not json":       `{nope`,
		"unknown type":   `{"type":"pattern","namespace":"/a","content":"x"}`,
		"bad namespace":  `{"type":"episode","namespace":"no-slash","content":"x"}`,
		"empty content":  `{"type":"episode","namespace":"/a","content":"  "}`,
		"too long":       `{"type":"episode","namespace":"/a","content":"` + strings.Repeat("x", maxContentLen+1) + `"}`,
		"confidence > 1": `{"type":"fact","namespace":"/a","content":"x","confidence":1.5}`,
	}
	for name, line := range bad {
		if _, err := decodeRecord([]byte(line)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCheckHeader(t *testing.T) {
	if err := checkHeader([]byte(`{"format":"stash-export","version":1}`)); err != nil {
		t.Errorf("valid header rejected: %v", err)
	}
	for name, line := range map[string]string{
		"wrong format":   `{"format":"other","version":1}`,
		"future version": `{"format":"stash-export","version":2}`,
		"not a header":   `{"type":"fact"}`,
		"garbage":        `hello`,
	} {
		if err := checkHeader([]byte(line)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestWantedTypes(t *testing.T) {
	both, err := wantedTypes(nil)
	if err != nil || !both[RecordEpisode] || !both[RecordFact] {
		t.Errorf("default should be both: %v %v", both, err)
	}
	only, err := wantedTypes([]string{" Fact "})
	if err != nil || only[RecordEpisode] || !only[RecordFact] {
		t.Errorf("fact only: %v %v", only, err)
	}
	if _, err := wantedTypes([]string{"pattern"}); err == nil {
		t.Error("unknown type should error")
	}
}

func TestImportResultFailCapsErrors(t *testing.T) {
	var r ImportResult
	for i := 0; i < maxImportErrors+5; i++ {
		r.fail(i+2, errBoom{})
	}
	if r.Failed != maxImportErrors+5 || len(r.Errors) != maxImportErrors {
		t.Errorf("failed=%d errors=%d", r.Failed, len(r.Errors))
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }

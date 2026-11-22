package brain

import (
	"reflect"
	"testing"
)

// The sweep must fold exactly the spellings the screen would fold, and nothing
// more: a group without a Keep is a question for a person and stays untouched.
func TestMergeTargets(t *testing.T) {
	tests := []struct {
		name string
		g    DuplicateGroup
		want []string
	}{
		{
			name: "capitals fold into the spelling the system writes",
			g: DuplicateGroup{Kind: DupCase, Keep: "monitor", Variants: []EntityVariant{
				{Name: "monitor", Facts: 2}, {Name: "Monitor", Facts: 3},
			}},
			want: []string{"Monitor"},
		},
		{
			name: "a leading the is dropped",
			g: DuplicateGroup{Kind: DupArticle, Keep: "aside element", Variants: []EntityVariant{
				{Name: "the aside element"}, {Name: "aside element"},
			}},
			want: []string{"the aside element"},
		},
		{
			name: "a variant that differs only in spacing or capitals is still folded in",
			g: DuplicateGroup{Kind: DupCase, Keep: "monitor", Variants: []EntityVariant{
				{Name: "  Monitor  "}, {Name: "MONITOR"},
			}},
			want: []string{"  Monitor  ", "MONITOR"},
		},
		{
			name: "punctuation keeps its choice",
			g: DuplicateGroup{Kind: DupPunctuation, Variants: []EntityVariant{
				{Name: "read endpoint"}, {Name: "read-endpoint"},
			}},
			want: nil,
		},
		{
			name: "a note keeps its choice",
			g: DuplicateGroup{Kind: DupQualifier, Variants: []EntityVariant{
				{Name: "Foundation"}, {Name: "Foundation (Gaia)"},
			}},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeTargets(tc.g)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mergeTargets = %#v, want %#v", got, tc.want)
			}
		})
	}
}

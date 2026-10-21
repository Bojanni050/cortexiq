// Modifications copyright (c) 2026 Bojan van der Heide
package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadDataset reads a benchmark dataset from a JSON file.
// The format is compatible with LongMemEval: an array of items, each with
// haystack_sessions and qa_pairs.
func LoadDataset(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dataset: %w", err)
	}
	defer f.Close()
	return LoadDatasetFromReader(f, filepathBase(path))
}

// LoadDatasetFromReader reads a benchmark dataset from an io.Reader.
// The name is used as the dataset name when the JSON has no "name" field.
func LoadDatasetFromReader(r io.Reader, name string) (*Dataset, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode dataset: %w", err)
	}

	// Try the wrapped format first: {"name": "...", "items": [...]}
	var wrapped struct {
		Name  string `json:"name"`
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Items != nil {
		ds := &Dataset{Name: wrapped.Name, Items: wrapped.Items}
		if ds.Name == "" {
			ds.Name = name
		}
		if err := ds.Validate(); err != nil {
			return nil, err
		}
		return ds, nil
	}

	// Fall back to a bare array of items (LongMemEval format).
	var items []Item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("dataset is neither {name, items} nor an array: %w", err)
	}
	ds := &Dataset{Name: name, Items: items}
	if err := ds.Validate(); err != nil {
		return nil, err
	}
	return ds, nil
}

// Validate checks the dataset for common errors: duplicate IDs, empty questions,
// sessions without turns, and questions without answers.
func (d *Dataset) Validate() error {
	if len(d.Items) == 0 {
		return fmt.Errorf("dataset has no items")
	}
	itemIDs := map[string]bool{}
	questionIDs := map[string]bool{}
	for i, item := range d.Items {
		if item.ID == "" {
			return fmt.Errorf("item %d has no id", i)
		}
		if itemIDs[item.ID] {
			return fmt.Errorf("duplicate item id %q", item.ID)
		}
		itemIDs[item.ID] = true

		if len(item.Sessions) == 0 {
			return fmt.Errorf("item %q has no sessions", item.ID)
		}
		sessionIDs := map[string]bool{}
		for j, s := range item.Sessions {
			if len(s.Turns) == 0 {
				return fmt.Errorf("item %q session %d has no turns", item.ID, j)
			}
			if s.ID != "" {
				if sessionIDs[s.ID] {
					return fmt.Errorf("item %q has duplicate session id %q", item.ID, s.ID)
				}
				sessionIDs[s.ID] = true
			}
		}

		if len(item.Questions) == 0 {
			return fmt.Errorf("item %q has no questions", item.ID)
		}
		for j, q := range item.Questions {
			if q.ID == "" {
				return fmt.Errorf("item %q question %d has no id", item.ID, j)
			}
			if questionIDs[q.ID] {
				return fmt.Errorf("duplicate question id %q", q.ID)
			}
			questionIDs[q.ID] = true
			if strings.TrimSpace(q.Question) == "" {
				return fmt.Errorf("question %q has empty text", q.ID)
			}
			if strings.TrimSpace(q.Answer) == "" {
				return fmt.Errorf("question %q has empty answer", q.ID)
			}
		}
	}
	return nil
}

// CountQuestions returns the total number of questions across all items.
func (d *Dataset) CountQuestions() int {
	n := 0
	for _, item := range d.Items {
		n += len(item.Questions)
	}
	return n
}

// CountSessions returns the total number of sessions across all items.
func (d *Dataset) CountSessions() int {
	n := 0
	for _, item := range d.Items {
		n += len(item.Sessions)
	}
	return n
}

// CountTurns returns the total number of conversation turns across all sessions.
func (d *Dataset) CountTurns() int {
	n := 0
	for _, item := range d.Items {
		for _, s := range item.Sessions {
			n += len(s.Turns)
		}
	}
	return n
}

// filepathBase returns the base name of a path without its extension.
func filepathBase(path string) string {
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[:i]
	}
	return base
}

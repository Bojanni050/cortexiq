package main

import (
	"errors"
	"strings"
	"testing"
)

// An omitted namespace used to file the memory at the root, where it is found by
// nobody. The refusal is the whole point: the client cannot notice a silent
// default, but it can act on an error that names the tool to call.
func TestRequiredNamespaceRefusesSilence(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n"} {
		_, err := requiredNamespace(raw)
		if err == nil {
			t.Fatalf("requiredNamespace(%q) accepted a missing namespace", raw)
		}
		if !errors.Is(err, ErrNamespaceRequired) {
			t.Fatalf("error = %v, want ErrNamespaceRequired", err)
		}
		if !strings.Contains(err.Error(), "list_namespaces") {
			t.Errorf("error text must name the tool that answers the question: %q", err)
		}
	}
}

// The root is a real namespace holding genuinely global memories. Refusing a
// forgotten one must not close it off, or the fix costs more than the bug.
func TestRequiredNamespaceKeepsTheRoot(t *testing.T) {
	got, err := requiredNamespace("/")
	if err != nil {
		t.Fatalf("explicit root was refused: %v", err)
	}
	if got != "/" {
		t.Fatalf("requiredNamespace(%q) = %q, want /", "/", got)
	}
}

func TestRequiredNamespaceTrimsSurroundingSpace(t *testing.T) {
	got, err := requiredNamespace("  /projects/cortexiq ")
	if err != nil {
		t.Fatalf("requiredNamespace: %v", err)
	}
	if got != "/projects/cortexiq" {
		t.Fatalf("got %q, want /projects/cortexiq", got)
	}
}

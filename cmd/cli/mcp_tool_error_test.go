package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// The whole point of the middleware is that a caller sees the reason instead of
// a bare JSON-RPC -32603 that hosted clients hide behind a generic message.
func TestToolErrorResultCarriesTheReason(t *testing.T) {
	handler := toolErrorResult(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, errors.New("brain: namespace not found — call create_namespace first")
	})

	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned a protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %d items, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, "create_namespace") {
		t.Errorf("text = %q, want it to carry the error", text.Text)
	}
}

func TestToolErrorResultPassesSuccessThrough(t *testing.T) {
	want := mcp.NewToolResultText(`{"id":7}`)
	handler := toolErrorResult(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return want, nil
	})

	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if result != want {
		t.Fatal("success result was replaced")
	}
}

package mcp

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestWriteFailureKeepsSyntaxDetails(t *testing.T) {
	result := &adt.WriteSourceResult{
		Message:      "Source has syntax errors - not saved",
		SyntaxErrors: []adt.SyntaxCheckResult{{Line: 42, Offset: 7, Severity: "E", Text: "Unknown test field"}},
	}
	verdict := validateWriteSourceResult(result)
	if verdict == nil {
		t.Fatal("logical write failure must return an error")
	}
	assertErrorWithDetails(t, newToolResultErrorWithPayload(verdict, result),
		"Source has syntax errors", "Unknown test field", `"line": 42`, `"offset": 7`)
}

func TestActivationFailureKeepsMessages(t *testing.T) {
	result := &adt.ActivationResult{
		Messages: []adt.ActivationResultMessage{{Type: "E", Line: 9, ShortText: "Synthetic activation failure"}},
	}
	verdict := adt.ActivationResultError(result)
	if verdict == nil {
		t.Fatal("logical activation failure must return an error")
	}
	assertErrorWithDetails(t, newToolResultErrorWithPayload(verdict, result),
		"activation failed", "Synthetic activation failure", `"line": 9`)
}

func assertErrorWithDetails(t *testing.T, result *mcp.CallToolResult, wants ...string) {
	t.Helper()
	if result == nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("expected one MCP error result, got %#v", result)
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	for _, want := range wants {
		if !strings.Contains(content.Text, want) {
			t.Errorf("diagnostic is missing %q: %s", want, content.Text)
		}
	}
}

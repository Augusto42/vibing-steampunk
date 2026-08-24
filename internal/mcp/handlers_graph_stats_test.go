package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// graph_stats took source and nothing else, "for now", for long enough that
// nobody found out — and the restriction was invisible because the type was
// undocumented. Its name promises statistics about a graph; a reader holding an
// object has no reason to expect they must paste its source first.

func statsAnswer(t *testing.T, s *Server, args map[string]any) map[string]any {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	result, err := s.handleGraphStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := toolResultText(t, result)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("the answer should be JSON, got %q", text)
	}
	return out
}

func TestGraphStatsCountsAnObjectWithoutBeingHandedItsSource(t *testing.T) {
	srv := boundaryServer(t, http.StatusOK)
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	out := statsAnswer(t, s, map[string]any{"object_type": "CLAS", "object_name": "ZCL_DEMO_ORDER"})

	if n, _ := out["node_count"].(float64); n < 2 {
		t.Errorf("the class and what it references are at least two nodes, got %v", out["node_count"])
	}
	if e, _ := out["edge_count"].(float64); e < 1 {
		t.Errorf("the source declares a dependency, so there is at least one edge, got %v", out["edge_count"])
	}
}

func TestGraphStatsOverAPackageSaysWhatItCouldNotRead(t *testing.T) {
	srv := boundaryServer(t, http.StatusForbidden)
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"package": "$ZDEMO"}
	result, err := s.handleGraphStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := toolResultText(t, result)
	if strings.Contains(text, "node_count") {
		t.Fatalf("nothing was read, so there is nothing to count:\n%s", text)
	}
	if !strings.Contains(text, "ZCL_DEMO_ORDER") {
		t.Errorf("the answer must name the object it could not open:\n%s", text)
	}
}

func TestGraphStatsStillAcceptsSource(t *testing.T) {
	s := &Server{}
	out := statsAnswer(t, s, map[string]any{"source": "REPORT zdemo.\nPERFORM x.\n"})
	if n, _ := out["node_count"].(float64); n < 1 {
		t.Errorf("offline analysis must keep working, got %v", out["node_count"])
	}
}

func TestGraphStatsSaysWhatItNeedsWhenGivenNothing(t *testing.T) {
	s := &Server{}
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{}
	result, _ := s.handleGraphStats(context.Background(), req)
	text := toolResultText(t, result)
	for _, want := range []string{"source", "object_type", "package"} {
		if !strings.Contains(text, want) {
			t.Errorf("the refusal should name every accepted input, %q missing from %q", want, text)
		}
	}
}

// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_sqltrace.go contains handlers for SQL trace (ST05).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// sqlTraceAnalysisTypes is the routing table as data; see analysisTypes.
func (s *Server) sqlTraceAnalysisTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"sql_trace_state": s.handleGetSQLTraceState,
		"list_sql_traces": s.handleListSQLTraces,
	}
}

// routeSQLTraceAction routes "analyze" with SQL trace types.
func (s *Server) routeSQLTraceAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	handler, known := s.sqlTraceAnalysisTypes()[getStringParam(params, "type")]
	if !known {
		return nil, false, nil
	}
	return s.callHandler(ctx, handler, params)
}

// --- SQL Trace (ST05) Handlers ---

func (s *Server) handleGetSQLTraceState(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	instances, err := s.adtClient.GetSQLTraceState(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get SQL trace state: %v", err)), nil
	}

	// The resource answers per application server instance and covers eight
	// trace types, of which SQL is one. A single boolean was the old model and
	// it could not have been right on any system with more than one instance.
	anyOn := false
	for i := range instances {
		if instances[i].Active() {
			anyOn = true
		}
	}
	answer := map[string]any{
		"instances":     instances,
		"anyTraceOn":    anyOn,
		"instanceCount": len(instances),
	}
	if len(instances) == 0 {
		answer["note"] = "the system reported no application server instances, which is not the same as tracing being off"
	}
	result, _ := json.MarshalIndent(answer, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleListSQLTraces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := ""
	maxResults := 100

	if u, ok := request.GetArguments()["user"].(string); ok {
		user = u
	}
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		maxResults = int(max)
	}

	dir, err := s.adtClient.ListSQLTraces(ctx, user, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list SQL traces: %v", err)), nil
	}

	answer := map[string]any{
		"entries": dir.Entries,
		"total":   len(dir.Entries),
	}
	if len(dir.Entries) == 0 && dir.AnalysisURL != "" {
		// Not "no traces". The resource on this release lists none and points
		// at the web application instead, and saying so is the difference
		// between a fact about the system and a fact about the API.
		answer["note"] = "this release does not list trace files through the ADT directory; " +
			"it returns a link to the trace analysis application instead. " +
			"An empty list here says nothing about whether traces exist."
		answer["analysisUrl"] = dir.AnalysisURL
	}
	result, _ := json.MarshalIndent(answer, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

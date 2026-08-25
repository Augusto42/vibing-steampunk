package mcp

// The last eleven, routed.
//
// Measured before written: of 147 tools registered in expert mode, the
// universal SAP() tool could reach 126. Of the twenty-one it could not, ten are
// gCTS — `registerGCTSTools` is defined and called from nowhere, so they are
// registered by no mode and have been dead since they landed. The live
// remainder is **eleven**: seven translation tools, three revision-history
// tools, and the ABAP lint.
//
// Eleven is therefore the entire cost of retiring focused and expert modes, and
// this file is that cost paid. Nothing new is built here — every handler
// already existed and was already advertised as a tool. What changes is that an
// agent in the mode that ships can reach them.
//
// The routing tables are maps rather than switches for the same reason as
// analysisTypes: the surface can then be enumerated without calling into SAP,
// which is what lets `vsp sweep` check that every advertised action is claimed
// by some route.

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeDeclared dispatches anything the registry claims.
//
// One function for every declared capability, because a per-action router is
// another place the set of actions is written down. The registry is the set.
func (s *Server) routeDeclared(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	// analyze type=lint is the one alias: static analysis is what a caller
	// reaches for under "analyze", and finding nothing there is how a
	// capability comes to look missing.
	if action == "analyze" && firstParam(params, "type") == "lint" {
		if c, ok := s.caps.Lookup("lint", ""); ok {
			return s.callHandler(ctx, c.Handler, params)
		}
	}

	ops := s.caps.Ops(action)
	if len(ops) == 0 {
		// No op-carrying capability; try the bare action.
		if c, ok := s.caps.Lookup(action, ""); ok {
			return s.callHandler(ctx, c.Handler, params)
		}
		return nil, false, nil
	}

	op := firstParam(params, "op", "type")
	if op == "" && action == "revisions" {
		op = "list" // the question somebody asking for history means first
	}
	if c, ok := s.caps.Lookup(action, op); ok {
		return s.callHandler(ctx, c.Handler, params)
	}

	// The action is recognised, so it owns the answer. The list of operations
	// comes from the registry, so it cannot name one that is not routed.
	return needParams(action, params, ops, s.exampleFor(action)), true, nil
}

// exampleFor returns a working call for the action, taken from a declaration.
func (s *Server) exampleFor(action string) string {
	for _, c := range s.caps.All() {
		if c.Action == action && len(c.Examples) > 0 {
			return c.Examples[0]
		}
	}
	return ""
}

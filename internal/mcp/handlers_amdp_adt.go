package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// AMDP debugging over ADT's own resources, with nothing installed on the
// server.
//
// The handlers next door take the other route — a WebSocket to ZADT_VSP, built
// when this project believed the system offered no way in. It does:
// /sap/bc/adt/amdp/debugger/* is a complete API and its breakpoints fire. See
// reports/2026-08-23-001-amdp-debugger-is-native-adt.md.
//
// These ride the server's one held debug session rather than opening anything
// of their own. That is not a convenience: the ADT resource keeps its handles
// in class-data, which is ABAP session memory, so a second connection finds an
// empty session and every call fails in a way that reads like a broken API.

// routeAMDPADTAction routes the ADT-native AMDP sub-actions of "debug".
func (s *Server) routeAMDPADTAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "debug" {
		return nil, false, nil
	}
	switch strings.ToUpper(objectType) {
	case "AMDP_ADT_START":
		return s.wrapAMDP(ctx, params, s.amdpADTStart)
	case "AMDP_ADT_BREAKPOINT":
		return s.wrapAMDP(ctx, params, s.amdpADTBreakpoint)
	case "AMDP_ADT_AWAIT":
		return s.wrapAMDP(ctx, params, s.amdpADTAwait)
	case "AMDP_ADT_STOP":
		return s.wrapAMDP(ctx, params, s.amdpADTStop)
	}
	return nil, false, nil
}

// wrapAMDP hands each handler the held debugger, so no handler has to know how
// the session is obtained or remember not to open its own.
func (s *Server) wrapAMDP(
	ctx context.Context,
	params map[string]any,
	fn func(context.Context, *saprfc.Debugger, map[string]any) (*mcp.CallToolResult, error),
) (*mcp.CallToolResult, bool, error) {
	session, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), true, nil
	}
	res, err := fn(ctx, session.dbg, params)
	return res, true, err
}

func (s *Server) amdpADTStart(ctx context.Context, dbg *saprfc.Debugger, params map[string]any) (*mcp.CallToolResult, error) {
	user, _ := params["user"].(string)
	if strings.TrimSpace(user) == "" {
		user = s.config.Username
	}
	session, err := dbg.AMDPStart(ctx, user, true)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"AMDP debug session %s started, bridged to HANA session %s.\n"+
			"Set a breakpoint with AMDP_ADT_BREAKPOINT, then run the AMDP method from elsewhere "+
			"and wait with AMDP_ADT_AWAIT.",
		session.MainID, session.HANASessionID)), nil
}

func (s *Server) amdpADTBreakpoint(ctx context.Context, dbg *saprfc.Debugger, params map[string]any) (*mcp.CallToolResult, error) {
	class, _ := params["class"].(string)
	if strings.TrimSpace(class) == "" {
		return newToolResultError("class is required: the AMDP method's class"), nil
	}
	line := intParam(params, "line", 0)
	if line <= 0 {
		return newToolResultError("line is required: a line inside the AMDP method body"), nil
	}

	class = strings.ToUpper(strings.TrimSpace(class))
	bp := saprfc.AMDPBreakpoint{
		ClientID: "vsp-mcp-1",
		URI: fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main#start=%d",
			strings.ToLower(class), line),
		Name: class,
		Type: "CLAS/OC",
	}
	if err := dbg.AMDPSyncBreakpoints(ctx, saprfc.AMDPSyncFull, []saprfc.AMDPBreakpoint{bp}); err != nil {
		return newToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"AMDP breakpoint set at %s line %d. SAP does not judge it until the next AMDP_ADT_AWAIT, "+
			"which reports whether it calls the position VALID.", class, line)), nil
}

func (s *Server) amdpADTAwait(ctx context.Context, dbg *saprfc.Debugger, params map[string]any) (*mcp.CallToolResult, error) {
	// Twelve is enough to walk past the acknowledgements that always come
	// first and still notice that nothing ever stopped.
	max := intParam(params, "max_events", 12)
	res, err := dbg.AMDPAwaitStop(ctx, max)

	var out strings.Builder
	// The verdict arrives on the way to the stop and is worth reporting even
	// when nothing stopped: a refused breakpoint and a method that never ran
	// look identical otherwise.
	if state, reason := dbg.AMDPBreakpointState(); state != "" {
		fmt.Fprintf(&out, "SAP calls the breakpoint %s", state)
		if reason != "" {
			fmt.Fprintf(&out, " — %s", reason)
		}
		out.WriteString("\n\n")
	}
	if err != nil {
		out.WriteString(err.Error())
		return newToolResultError(out.String()), nil
	}
	out.Write(res.Body)
	return mcp.NewToolResultText(out.String()), nil
}

func (s *Server) amdpADTStop(ctx context.Context, dbg *saprfc.Debugger, params map[string]any) (*mcp.CallToolResult, error) {
	if err := dbg.AMDPTerminate(ctx, true); err != nil {
		return newToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("AMDP debug session ended."), nil
}

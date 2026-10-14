// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_grep.go contains handlers for grep/search operations on ABAP objects.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeGrepAction routes "grep" action.
func (s *Server) routeGrepAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "grep" {
		return nil, false, nil
	}

	// GrepObjects (multiple objects)
	if hasAnyParam(params, "object_urls") {
		return s.callHandler(ctx, s.handleGrepObjects, params)
	}

	// GrepPackages (multiple packages)
	if hasAnyParam(params, "packages") {
		return s.callHandler(ctx, s.handleGrepPackages, params)
	}

	// GrepPackage (single package). `package` is the CLI's own flag name and
	// the one a caller reaches for; only `package_name` was accepted, so the
	// natural call fell off the end of the chain and was answered with "no
	// handler found for action=grep" — of an action that works.
	if pkgName := firstParam(params, "package_name", "package"); pkgName != "" {
		args := copyParams(params)
		args["package_name"] = pkgName
		return s.callHandler(ctx, s.handleGrepPackage, args)
	}

	// GrepObject (single object)
	if objectURL := firstParam(params, "object_url", "object"); objectURL != "" {
		args := copyParams(params)
		args["object_url"] = objectURL
		return s.callHandler(ctx, s.handleGrepObject, args)
	}

	return needParams("grep", params,
		[]string{"package_name (or package)", "object_url (or object)", "packages", "object_urls"},
		`SAP(action="grep", params={"pattern": "SELECT", "package": "$TMP"})
  SAP(action="grep", params={"pattern": "TODO", "object_url": "/sap/bc/adt/oo/classes/zcl_demo"})`), true, nil
}

// --- Grep/Search Handlers ---

// readIncludeEnhancementsFlag reads the include_enhancements param. Default
// is true — the whole point of the MCP grep surface is "Claude can see what
// SE80 sees", which on classic ECC means walking ENHO plug-in bodies that
// the raw source endpoint never returns.
func readIncludeEnhancementsFlag(args map[string]interface{}) bool {
	if v, ok := args["include_enhancements"].(bool); ok {
		return v
	}
	return true
}

// readMaxEnhancementsParam reads the max_enhancements cap. 0 ⇒ default cap
// (50, defined in workflows_grep.go).
func readMaxEnhancementsParam(args map[string]interface{}) int {
	if v, ok := args["max_enhancements"].(float64); ok && v > 0 {
		return int(v)
	}
	return 0
}

func (s *Server) handleGrepObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	objectURL, ok := args["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	contextLines := 0
	if cl, ok := args["context_lines"].(float64); ok {
		contextLines = int(cl)
	}

	includeEnhancements := readIncludeEnhancementsFlag(args)

	var (
		result interface{}
		err    error
	)
	if includeEnhancements {
		// Self-contained walk — no shared state needed for a single object.
		result, err = s.adtClient.GrepObjectWithEnhancements(ctx, objectURL, pattern, caseInsensitive, contextLines, nil)
	} else {
		result, err = s.adtClient.GrepObject(ctx, objectURL, pattern, caseInsensitive, contextLines)
	}
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepObject failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGrepPackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	packageName, ok := args["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	// Parse object_types (comma-separated string to slice)
	var objectTypes []string
	if ot, ok := args["object_types"].(string); ok && ot != "" {
		objectTypes = strings.Split(ot, ",")
		for i := range objectTypes {
			objectTypes[i] = strings.TrimSpace(objectTypes[i])
		}
	}

	maxResults := 100 // default
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	includeEnhancements := readIncludeEnhancementsFlag(args)
	maxEnhancements := readMaxEnhancementsParam(args)

	var (
		result interface{}
		err    error
	)
	if includeEnhancements {
		result, err = s.adtClient.GrepPackageWithEnhancements(ctx, packageName, pattern, caseInsensitive, objectTypes, maxResults, maxEnhancements)
	} else {
		result, err = s.adtClient.GrepPackage(ctx, packageName, pattern, caseInsensitive, objectTypes, maxResults)
	}
	if err != nil {
		return newToolResultError(fmt.Sprintf("GrepPackage failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func (s *Server) registerCreateEnhancement() {
	s.mcpServer.AddTool(mcp.NewTool("CreateEnhancement",
		mcp.WithDescription("Create and activate an ENHO with explicit host metadata. Supports XH source-code plug-ins, class enhancements, and BAdI implementations. BAdI implementation classes must already exist."),
		mcp.WithString("kind", mcp.Required(), mcp.Description("XH, CLASS, or BADI")),
		mcp.WithString("name", mcp.Required(), mcp.Description("New ENHO name")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Description")),
		mcp.WithString("package", mcp.Required(), mcp.Description("Target package")),
		mcp.WithString("transport", mcp.Description("Transport request, required for transportable packages")),
		mcp.WithString("host_object_type"), mcp.WithString("host_object_name"),
		mcp.WithString("host_program"), mcp.WithString("main_object_type"),
		mcp.WithString("main_object_name"), mcp.WithString("anchor"),
		mcp.WithString("parent_anchor"), mcp.WithString("source"),
		mcp.WithString("spot"), mcp.WithString("enhancement_mode"),
		mcp.WithBoolean("overwrite"), mcp.WithBoolean("hook_method"),
		mcp.WithString("class_name"), mcp.WithString("method_name"),
		mcp.WithString("method_description"), mcp.WithString("method_exposure"),
		mcp.WithString("badi_name"), mcp.WithString("implementation_name"),
		mcp.WithString("implementation_class"), mcp.WithString("implementation_description"),
		mcp.WithBoolean("inactive"), mcp.WithBoolean("default_implementation"),
	), s.handleCreateEnhancement)
}

func (s *Server) handleCreateEnhancement(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	str := func(name string) string { value, _ := args[name].(string); return value }
	boolean := func(name string) bool { value, _ := args[name].(bool); return value }
	for _, required := range []string{"kind", "name", "description", "package"} {
		if strings.TrimSpace(str(required)) == "" {
			return newToolResultError(required + " is required"), nil
		}
	}
	opts := adt.CreateEnhancementOptions{
		Kind: adt.EnhancementCreateKind(str("kind")), Name: str("name"),
		Description: str("description"), Package: str("package"), Transport: str("transport"),
		HostObjectType: str("host_object_type"), HostObjectName: str("host_object_name"),
		HostProgram: str("host_program"), MainObjectType: str("main_object_type"),
		MainObjectName: str("main_object_name"), Anchor: str("anchor"),
		ParentAnchor: str("parent_anchor"), Spot: str("spot"),
		EnhancementMode: str("enhancement_mode"), Overwrite: boolean("overwrite"),
		HookMethod: boolean("hook_method"), Source: str("source"),
		ClassName: str("class_name"), MethodName: str("method_name"),
		MethodDescription: str("method_description"), MethodExposure: str("method_exposure"),
		MethodSource: str("source"), SpotName: str("spot"), BAdIName: str("badi_name"),
		ImplementationName: str("implementation_name"), ImplementationClass: str("implementation_class"),
		ImplementationDescription: str("implementation_description"),
		Inactive:                  boolean("inactive"), DefaultImplementation: boolean("default_implementation"),
	}
	result, err := s.adtClient.CreateEnhancement(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CreateEnhancement blocked: %v", err)), nil
	}
	if result == nil {
		return newToolResultError("CreateEnhancement failed: no result returned"), nil
	}
	if !result.Success {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "operation returned success=false without a diagnostic"
		}
		return newToolResultError("CreateEnhancement failed: " + message), nil
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// newToolResultErrorWithPayload keeps structured diagnostics alongside a
// fail-closed verdict. Syntax and activation details would otherwise be lost
// when a logical failure is converted into an MCP error.
func newToolResultErrorWithPayload(verdict error, payload any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("%v (diagnostic serialization failed: %v)", verdict, err))
	}
	return newToolResultError(fmt.Sprintf("%v\n\n%s", verdict, data))
}

package adt

// The debugger's response parsers, exported for callers that obtain the same
// documents over a different transport. The bodies are SAP's own — a debug
// session reached through the classic-RFC tunnel answers with byte-identical
// XML to one reached over HTTP — so the model belongs in one place and only the
// envelope differs.

// ParseStackXML reads a /sap/bc/adt/debugger/stack document.
func ParseStackXML(data []byte) (*DebugStackInfo, error) { return parseStackResponse(data) }

// ParseVariablesXML reads a getVariables document.
func ParseVariablesXML(data []byte) ([]DebugVariable, error) { return parseVariablesResponse(data) }

// ParseChildVariablesXML reads a getChildVariables document: the variables plus
// the parent/child hierarchy that names them.
func ParseChildVariablesXML(data []byte) (*DebugChildVariablesInfo, error) {
	return parseChildVariablesResponse(data)
}

// ParseAttachXML reads the attach response.
func ParseAttachXML(data []byte) (*DebugAttachResult, error) { return parseAttachResponse(data) }

// ParseStepXML reads a step response.
func ParseStepXML(data []byte) (*DebugStepResult, error) { return parseStepResponse(data) }

// BuildBreakpointRequestXML renders the breakpoint request document.
func BuildBreakpointRequestXML(req *BreakpointRequest) (string, error) {
	return buildBreakpointRequestXML(req)
}

// ParseBreakpointResponseXML reads what SAP answers a breakpoint request with.
func ParseBreakpointResponseXML(data []byte) (*BreakpointResponse, error) {
	return parseBreakpointResponse(data)
}

package adt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// --- RFC Domain Operations ---

// RFCResult contains the result of an RFC call.
type RFCResult struct {
	Subrc   int            `json:"subrc"`
	Exports map[string]any `json:"exports"`
	Tables  map[string]any `json:"tables"`
}

// CallRFC calls a function module via WebSocket.
func (c *DebugWebSocketClient) CallRFC(ctx context.Context, function string, params map[string]string) (*RFCResult, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc")

	// Build params as proper object (not JSON string)
	paramsObj := map[string]any{
		"function": function,
	}
	for k, v := range params {
		paramsObj[k] = v
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "call",
		"params":  paramsObj,
		"timeout": 120000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 125*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("RFC call failed")
	}

	var result RFCResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RunReport executes a report via background job (RFC domain, runReport action).
// This schedules the report as a background job, which runs in a separate work process
// and CAN hit external breakpoints.
func (c *DebugWebSocketClient) RunReport(ctx context.Context, report string, variant string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_run")

	paramsObj := map[string]any{
		"report": report,
	}
	if variant != "" {
		paramsObj["variant"] = variant
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "runReport",
		"params":  paramsObj,
		"timeout": 30000,
	}

	respCh := make(chan *WSResponse, 1)
	c.RegisterPending(id, respCh)

	data, err := json.Marshal(rawMsg)
	if err != nil {
		c.UnregisterPending(id)
		return err
	}

	if err := c.WriteMessage(data); err != nil {
		c.UnregisterPending(id)
		return err
	}

	// Don't wait for response - the report might be blocked on breakpoint
	// The listener will catch the debuggee
	go func() {
		select {
		case <-respCh:
			// Report finished (no breakpoint hit or continued past)
		case <-time.After(60 * time.Second):
			c.UnregisterPending(id)
		}
	}()

	return nil
}

// RunReportSync executes a report via background job and waits for the response.
func (c *DebugWebSocketClient) RunReportSync(ctx context.Context, report string, variant string) (*WSResponse, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_run")

	paramsObj := map[string]any{
		"report": report,
	}
	if variant != "" {
		paramsObj["variant"] = variant
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "runReport",
		"params":  paramsObj,
		"timeout": 30000,
	}

	return c.SendRawRequest(ctx, id, rawMsg, 60*time.Second)
}

// ReadSourceResult is the response shape from rfc/readSource.
type ReadSourceResult struct {
	Program string   `json:"program"`
	Source  []string `json:"source"`
}

// ReadSource reads ABAP source for a program/include via the rfc domain's
// readSource action. The ABAP-side handler uses native `READ REPORT`, so it
// works for SUBC=I includes (enhancement plug-in source) where
// RPY_PROGRAM_READ raises CANCELLED in RFC contexts on classic ECC.
func (c *DebugWebSocketClient) ReadSource(ctx context.Context, program string) ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_read")

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "readSource",
		"params":  map[string]any{"program": program},
		"timeout": 30000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 30*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("readSource failed")
	}

	var result ReadSourceResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Source, nil
}

// WriteEnhancementSource updates an existing classic HOOK_IMPL enhancement
// through the optional ZADT_VSP bridge. The ABAP side uses the Enhancement
// Framework API (not a raw INSERT REPORT), so locking, transport assignment,
// saving, and activation remain owned by SAP.
func (c *DebugWebSocketClient) WriteEnhancementSource(ctx context.Context, enhancement, source, transport string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_enho_write")
	rawMsg := map[string]any{
		"id":     id,
		"domain": "rfc",
		"action": "writeEnhancementSource",
		"params": map[string]any{
			"enhancement":   strings.ToUpper(strings.TrimSpace(enhancement)),
			"source_base64": base64.StdEncoding.EncodeToString([]byte(source)),
			"transport":     strings.ToUpper(strings.TrimSpace(transport)),
		},
		"timeout": 120000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 125*time.Second)
	if err != nil {
		return err
	}
	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("writeEnhancementSource failed")
	}
	return nil
}

// CreateEnhancement schedules creation through SAP's Enhancement Framework.
// The bridge deliberately receives every relationship explicitly; it never
// tries to infer an anchor, host class, enhancement spot, or BAdI definition
// from source text.
func (c *DebugWebSocketClient) CreateEnhancement(ctx context.Context, opts CreateEnhancementOptions) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	active := "X"
	if opts.Inactive {
		active = ""
	}
	defaultImpl := ""
	if opts.DefaultImplementation {
		defaultImpl = "X"
	}
	overwrite := ""
	if opts.Overwrite {
		overwrite = "X"
	}
	hookMethod := ""
	if opts.HookMethod {
		hookMethod = "X"
	}

	id := c.GenerateID("rfc_enho_create")
	rawMsg := map[string]any{
		"id":     id,
		"domain": "rfc",
		"action": "createEnhancement",
		"params": map[string]any{
			"kind":                       string(opts.Kind),
			"enhancement":                opts.Name,
			"description_base64":         base64.StdEncoding.EncodeToString([]byte(opts.Description)),
			"package":                    opts.Package,
			"transport":                  opts.Transport,
			"host_object_type":           opts.HostObjectType,
			"host_object_name":           opts.HostObjectName,
			"host_program":               opts.HostProgram,
			"main_object_type":           opts.MainObjectType,
			"main_object_name":           opts.MainObjectName,
			"anchor_base64":              base64.StdEncoding.EncodeToString([]byte(opts.Anchor)),
			"parent_anchor_base64":       base64.StdEncoding.EncodeToString([]byte(opts.ParentAnchor)),
			"spot":                       opts.Spot,
			"enhancement_mode":           opts.EnhancementMode,
			"overwrite":                  overwrite,
			"hook_method":                hookMethod,
			"source_base64":              base64.StdEncoding.EncodeToString([]byte(opts.Source)),
			"class_name":                 opts.ClassName,
			"method_name":                opts.MethodName,
			"method_description_base64":  base64.StdEncoding.EncodeToString([]byte(opts.MethodDescription)),
			"method_exposure":            opts.MethodExposure,
			"method_source_base64":       base64.StdEncoding.EncodeToString([]byte(opts.MethodSource)),
			"spot_name":                  opts.SpotName,
			"badi_name":                  opts.BAdIName,
			"implementation_name":        opts.ImplementationName,
			"implementation_class":       opts.ImplementationClass,
			"implementation_desc_base64": base64.StdEncoding.EncodeToString([]byte(opts.ImplementationDescription)),
			"active":                     active,
			"default_implementation":     defaultImpl,
		},
		"timeout": 120000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 125*time.Second)
	if err != nil {
		return err
	}
	if !resp.Success {
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("createEnhancement failed")
	}
	return nil
}

// DescribeEnhancement returns tool-specific metadata from the Enhancement
// Framework. This is especially useful for BAdI implementations, whose source
// lives in a separate implementation class rather than in an ENHO include.
func (c *DebugWebSocketClient) DescribeEnhancement(ctx context.Context, enhancement string) (string, error) {
	if !c.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	id := c.GenerateID("rfc_enho_describe")
	rawMsg := map[string]any{
		"id":     id,
		"domain": "rfc",
		"action": "describeEnhancement",
		"params": map[string]any{
			"enhancement": strings.ToUpper(strings.TrimSpace(enhancement)),
		},
		"timeout": 30000,
	}
	resp, err := c.SendRawRequest(ctx, id, rawMsg, 35*time.Second)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		if resp.Error != nil {
			return "", fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return "", fmt.Errorf("describeEnhancement failed")
	}
	return string(resp.Data), nil
}

// --- Package Operations ---

// MoveObjectResult contains the result of a package reassignment.
type MoveObjectResult struct {
	Success    bool   `json:"success"`
	Pgmid      string `json:"pgmid"`
	Object     string `json:"object"`
	ObjName    string `json:"obj_name"`
	NewPackage string `json:"new_package"`
	Message    string `json:"message"`
}

// MoveObject moves an ABAP object to a different package via WebSocket.
// Uses the rfc domain's moveToPackage action which calls ZADT_CL_TADIR_MOVE.
// objectType: CLAS, PROG, INTF, FUGR, etc.
// objectName: Name of the object (e.g., ZCL_TEST)
// newPackage: Target package (e.g., $ZRAY)
func (c *DebugWebSocketClient) MoveObject(ctx context.Context, objectType, objectName, newPackage string) (*MoveObjectResult, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := c.GenerateID("move")

	paramsObj := map[string]any{
		"object":      objectType,
		"obj_name":    objectName,
		"new_package": newPackage,
	}

	rawMsg := map[string]any{
		"id":      id,
		"domain":  "rfc",
		"action":  "moveToPackage",
		"params":  paramsObj,
		"timeout": 30000,
	}

	resp, err := c.SendRawRequest(ctx, id, rawMsg, 30*time.Second)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("move failed")
	}

	var result MoveObjectResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

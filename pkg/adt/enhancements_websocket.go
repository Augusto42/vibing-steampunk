package adt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ReadSource uses the optional SAP bridge's native READ REPORT path.
func (c *DebugWebSocketClient) ReadSource(ctx context.Context, program string) ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}
	id := c.GenerateID("rfc_read")
	resp, err := c.SendRawRequest(ctx, id, map[string]any{
		"id": id, "domain": "rfc", "action": "readSource",
		"params": map[string]any{"program": strings.ToUpper(strings.TrimSpace(program))}, "timeout": 30000,
	}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("readSource failed")
	}
	var result struct {
		Source []string `json:"source"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Source, nil
}

// WriteEnhancementSource asks SAP's Enhancement Framework to own locking,
// transport assignment, save and activation; it never writes the include raw.
func (c *DebugWebSocketClient) WriteEnhancementSource(ctx context.Context, enhancement, source, transport string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}
	id := c.GenerateID("rfc_enho_write")
	resp, err := c.SendRawRequest(ctx, id, map[string]any{
		"id": id, "domain": "rfc", "action": "writeEnhancementSource",
		"params": map[string]any{
			"enhancement":   strings.ToUpper(strings.TrimSpace(enhancement)),
			"source_base64": base64.StdEncoding.EncodeToString([]byte(source)),
			"transport":     strings.ToUpper(strings.TrimSpace(transport)),
		}, "timeout": 120000,
	}, 125*time.Second)
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

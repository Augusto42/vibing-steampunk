package adt

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"
)

// CreateEnhancement sends all host and subtype relationships explicitly to
// the optional ZADT_VSP Enhancement Framework bridge. It does not report
// success from the scheduling response; Client.CreateEnhancement verifies the
// active repository object before returning success.
func (c *DebugWebSocketClient) CreateEnhancement(ctx context.Context, opts CreateEnhancementOptions) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}
	flag := func(enabled bool) string {
		if enabled {
			return "X"
		}
		return ""
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
			"overwrite":                  flag(opts.Overwrite),
			"hook_method":                flag(opts.HookMethod),
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
			"active":                     flag(!opts.Inactive),
			"default_implementation":     flag(opts.DefaultImplementation),
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

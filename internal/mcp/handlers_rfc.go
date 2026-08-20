// handlers_rfc.go adds classic RFC to the universal SAP tool: action="rfc" with
// an op in params. RFC is a second protocol to the same system (gateway instead
// of HTTP/ADT), served by the SDK-free open-rfc-go client. It is one action, not
// a family of tools, so the MCP tool space stays a single SAP tool.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// routeRFCAction handles SAP(action="rfc", …).
//
//	SAP(action="rfc", params={"op":"info"})                       — RFC_SYSTEM_INFO
//	SAP(action="rfc", params={"op":"ping"})                       — RFC_PING
//	SAP(action="rfc", target="BAPI_USER_*", params={"op":"search"})
//	SAP(action="rfc", target="STFC_CONNECTION")                   — describe (default)
//	SAP(action="rfc", target="Z_DOUBLE", params={"op":"call","args":{"N":21}})
//	SAP(action="rfc", target="T000", params={"op":"read_table","fields":["MANDT"],"top":5})
//
// Destination overrides: params host / sysnr / port / user.
func (s *Server) routeRFCAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "rfc" {
		return nil, false, nil
	}
	// The target is a plain name (FM or table), so parseTarget puts it in objectType.
	name := strings.TrimSpace(objectName)
	if name == "" {
		name = strings.TrimSpace(objectType)
	}
	op := strings.ToLower(getStringParam(params, "op"))
	if op == "" {
		switch {
		case name == "":
			op = "info"
		case params["args"] != nil:
			op = "call"
		default:
			op = "describe"
		}
	}

	c, err := s.rfcClient(ctx, params)
	if err != nil {
		return nil, true, err
	}
	defer c.Close(ctx)

	switch op {
	case "info":
		r, err := c.Call(ctx, "RFC_SYSTEM_INFO", nil)
		if err != nil {
			return nil, true, err
		}
		return rfcResult(r.Get("RFCSI_EXPORT"))
	case "ping":
		if _, err := c.Call(ctx, "RFC_PING", nil); err != nil {
			return nil, true, err
		}
		return mcp.NewToolResultText("ok"), true, nil
	case "describe":
		if name == "" {
			return nil, true, fmt.Errorf("describe needs a function module in target")
		}
		tool, err := c.DescribeTool(ctx, strings.ToUpper(name))
		if err != nil {
			return nil, true, err
		}
		return rfcResult(tool)
	case "call":
		if name == "" {
			return nil, true, fmt.Errorf("call needs a function module in target")
		}
		args, _ := params["args"].(map[string]any)
		r, err := c.Call(ctx, strings.ToUpper(name), openrfc.Params(args))
		if err != nil {
			return nil, true, err
		}
		return rfcResult(r)
	case "search":
		like := strings.ReplaceAll(strings.ToUpper(name), "*", "%")
		if like == "" {
			return nil, true, fmt.Errorf("search needs a name mask in target")
		}
		if !strings.Contains(like, "%") {
			like = "%" + like + "%"
		}
		where := "FUNCNAME LIKE '" + like + "'"
		if all, ok := getBoolParam(params, "all"); !ok || !all {
			where += " AND FMODE = 'R'"
		}
		rows, err := s.rfcReadTable(ctx, c, "TFDIR", where, []string{"FUNCNAME", "PNAME"}, intParam(params, "top", 100))
		if err != nil {
			return nil, true, err
		}
		return rfcResult(rows)
	case "read_table", "read-table", "table":
		if name == "" {
			return nil, true, fmt.Errorf("read_table needs a table name in target")
		}
		var fields []string
		if raw, ok := params["fields"].([]any); ok {
			for _, f := range raw {
				fields = append(fields, strings.ToUpper(fmt.Sprint(f)))
			}
		}
		rows, err := s.rfcReadTable(ctx, c, strings.ToUpper(name), getStringParam(params, "where"), fields, intParam(params, "top", 0))
		if err != nil {
			return nil, true, err
		}
		return rfcResult(rows)
	}
	return nil, true, fmt.Errorf("unknown rfc op %q (info, ping, describe, call, search, read_table)", op)
}

// rfcClient dials RFC for this server's system, honouring per-call overrides and
// the RFC settings of the default .vsp.json system.
func (s *Server) rfcClient(ctx context.Context, params map[string]any) (*openrfc.Client, error) {
	in := saprfc.Input{
		URL:      s.config.BaseURL,
		User:     s.config.Username,
		Password: s.config.Password,
		Client:   s.config.Client,
		Language: s.config.Language,
		RFCUser:  os.Getenv("SAP_USER"),
	}
	if pwd := os.Getenv("SAP_PASSWORD"); pwd != "" {
		in.RFCPassword = pwd
	}
	// Per-system RFC settings from the default .vsp.json system, when present.
	if cfg, _, err := config.LoadSystems(); err == nil && cfg != nil && cfg.Default != "" {
		if sys, err := cfg.GetSystem(cfg.Default); err == nil {
			in.RFCHost, in.RFCSysnr, in.RFCPort = sys.RFCHost, sys.RFCSysnr, sys.RFCPort
			if sys.RFCUser != "" {
				in.RFCUser = sys.RFCUser
			}
			if sys.RFCPassword != "" {
				in.RFCPassword = sys.RFCPassword
			}
		}
	}
	in.HostFlag = getStringParam(params, "host")
	in.SysnrFlag = getStringParam(params, "sysnr")
	in.UserFlag = getStringParam(params, "user")
	in.PortFlag = intParam(params, "port", 0)

	dest, err := saprfc.Resolve(in)
	if err != nil {
		return nil, err
	}
	c, err := saprfc.Open(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	return c, nil
}

// rfcReadTable runs RFC_READ_TABLE and splits each row into a column->value map.
func (s *Server) rfcReadTable(ctx context.Context, c *openrfc.Client, table, where string, fields []string, top int) ([]map[string]string, error) {
	in := openrfc.Params{"QUERY_TABLE": table, "DELIMITER": "|"}
	if top > 0 {
		in["ROWCOUNT"] = int64(top)
	}
	if where != "" {
		in["OPTIONS"] = []map[string]any{{"TEXT": where}}
	}
	if len(fields) > 0 {
		fs := make([]map[string]any, 0, len(fields))
		for _, f := range fields {
			fs = append(fs, map[string]any{"FIELDNAME": f})
		}
		in["FIELDS"] = fs
	}
	r, err := c.Call(ctx, "RFC_READ_TABLE", in)
	if err != nil {
		return nil, err
	}
	var cols []string
	for _, fr := range r.Table("FIELDS") {
		cols = append(cols, strings.TrimSpace(fmt.Sprint(fr["FIELDNAME"])))
	}
	var out []map[string]string
	for _, dr := range r.Table("DATA") {
		parts := strings.Split(fmt.Sprint(dr["WA"]), "|")
		row := map[string]string{}
		for i, col := range cols {
			if i < len(parts) {
				row[col] = strings.TrimRight(parts[i], " ")
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func rfcResult(v any) (*mcp.CallToolResult, bool, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, true, err
	}
	return mcp.NewToolResultText(string(b)), true, nil
}

func intParam(params map[string]any, key string, def int) int {
	if v, ok := getFloatParam(params, key); ok {
		return int(v)
	}
	return def
}

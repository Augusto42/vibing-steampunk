package adt

import (
	"context"
	"fmt"
	"strings"
)

// Dynpro is a read-only, structured representation of an ABAP screen.
// Mutating screens is intentionally not exposed until the classic RFC write
// path can be validated against a dedicated SAP development sandbox.
type Dynpro struct {
	Program            string              `json:"program"`
	Screen             string              `json:"screen"`
	Header             map[string]string   `json:"header"`
	Containers         []map[string]string `json:"containers"`
	FieldsToContainers []map[string]string `json:"fieldsToContainers"`
	FlowLogic          []string            `json:"flowLogic"`
}

// GetDynpro reads an ABAP screen through the ZADT_VSP WebSocket bridge and
// RPY_DYNPRO_READ. Classic Dynpro has no portable native ADT REST source
// contract, so the method fails explicitly when the bridge is unavailable.
func (c *Client) GetDynpro(ctx context.Context, program, screen string) (*Dynpro, error) {
	if err := c.checkSafety(OpRead, "GetDynpro"); err != nil {
		return nil, err
	}

	program = strings.ToUpper(strings.TrimSpace(program))
	if program == "" {
		return nil, fmt.Errorf("dynpro parent program is required")
	}
	normalizedScreen, err := normalizeDynproNumber(screen)
	if err != nil {
		return nil, err
	}

	factory := c.rfcFetcherFactory
	if factory == nil {
		factory = c.defaultRFCSourceFetcher
	}
	fetcher, err := factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s requires the ZADT_VSP bridge: %w", program, normalizedScreen, err)
	}
	defer fetcher.Close()

	result, err := fetcher.CallRFC(ctx, "RPY_DYNPRO_READ", map[string]string{
		"PROGNAME": program,
		"DYNNR":    normalizedScreen,
	})
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s: %w", program, normalizedScreen, err)
	}
	if result == nil {
		return nil, fmt.Errorf("reading dynpro %s/%s returned no result", program, normalizedScreen)
	}
	if result.Subrc != 0 {
		return nil, fmt.Errorf("reading dynpro %s/%s failed in RPY_DYNPRO_READ (subrc=%d)", program, normalizedScreen, result.Subrc)
	}

	headerValue, ok := lookupRFCValue(result.Exports, "HEADER")
	if !ok {
		return nil, fmt.Errorf("reading dynpro %s/%s returned no HEADER", program, normalizedScreen)
	}
	header, err := normalizeRFCStringMap(headerValue, "HEADER")
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s: %w", program, normalizedScreen, err)
	}
	containers, err := normalizeRFCStringRows(result.Tables, "CONTAINERS")
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s: %w", program, normalizedScreen, err)
	}
	fields, err := normalizeRFCStringRows(result.Tables, "FIELDS_TO_CONTAINERS")
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s: %w", program, normalizedScreen, err)
	}
	flowLogic, err := normalizeRFCFlowLogic(result.Tables)
	if err != nil {
		return nil, fmt.Errorf("reading dynpro %s/%s: %w", program, normalizedScreen, err)
	}

	return &Dynpro{
		Program:            program,
		Screen:             normalizedScreen,
		Header:             header,
		Containers:         containers,
		FieldsToContainers: fields,
		FlowLogic:          flowLogic,
	}, nil
}

func normalizeDynproNumber(screen string) (string, error) {
	screen = strings.TrimSpace(screen)
	if screen == "" || len(screen) > 4 {
		return "", fmt.Errorf("dynpro screen number must contain 1 to 4 digits")
	}
	for _, char := range screen {
		if char < '0' || char > '9' {
			return "", fmt.Errorf("invalid dynpro screen number %q: expected 1 to 4 digits", screen)
		}
	}
	return strings.Repeat("0", 4-len(screen)) + screen, nil
}

func parseDynproReference(name, parent string) (program, screen string, err error) {
	parent = strings.TrimSpace(parent)
	name = strings.TrimSpace(name)
	if parent != "" {
		return parent, name, nil
	}

	separator := strings.LastIndex(name, "/")
	if separator <= 0 || separator == len(name)-1 {
		return "", "", fmt.Errorf("DYNP requires parent program plus screen number: set parent, or use name PROGRAM/0100")
	}
	return name[:separator], name[separator+1:], nil
}

func lookupRFCValue(values map[string]any, name string) (any, bool) {
	for key, value := range values {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}

func normalizeRFCStringMap(value any, label string) (map[string]string, error) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s has unexpected RFC shape %T", label, value)
	}
	result := make(map[string]string, len(raw))
	for key, cell := range raw {
		text, ok := cell.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s has unexpected RFC value %T", label, key, cell)
		}
		result[key] = text
	}
	return result, nil
}

func normalizeRFCStringRows(tables map[string]any, name string) ([]map[string]string, error) {
	value, ok := lookupRFCValue(tables, name)
	if !ok || value == nil {
		return []map[string]string{}, nil
	}
	rawRows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s has unexpected RFC shape %T", name, value)
	}
	rows := make([]map[string]string, 0, len(rawRows))
	for index, rawRow := range rawRows {
		row, err := normalizeRFCStringMap(rawRow, fmt.Sprintf("%s[%d]", name, index))
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func normalizeRFCFlowLogic(tables map[string]any) ([]string, error) {
	rows, err := normalizeRFCStringRows(tables, "FLOW_LOGIC")
	if err != nil {
		return nil, err
	}
	flow := make([]string, 0, len(rows))
	for index, row := range rows {
		var line string
		found := false
		for key, value := range row {
			if strings.EqualFold(key, "LINE") {
				line = value
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("FLOW_LOGIC[%d] returned no LINE field", index)
		}
		flow = append(flow, line)
	}
	return flow, nil
}

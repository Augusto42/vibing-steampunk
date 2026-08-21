package saprfc

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The typed half of the ADT debugger surface. The methods next door return the
// raw HTTP envelope, which is what a REPL and a tunnel test want; everything
// that has to *reason* about a stop — an MCP tool, a boundary capture — wants
// the model instead. The documents are SAP's own, so the parsers are the ones
// in pkg/adt and not a second copy.

// StackInfo returns the attached debuggee's call stack, parsed.
func (d *Debugger) StackInfo(ctx context.Context) (*adt.DebugStackInfo, error) {
	res, err := d.ADTStack(ctx)
	if err != nil {
		return nil, err
	}
	return adt.ParseStackXML(res.Body)
}

// Vars reads named variables and parses them. Empty names read the two roots.
func (d *Debugger) Vars(ctx context.Context, names []string) ([]adt.DebugVariable, error) {
	res, err := d.ADTVariables(ctx, names)
	if err != nil {
		return nil, err
	}
	return adt.ParseVariablesXML(res.Body)
}

// Expand returns the children of one composite variable — a structure's
// components, a table's rows, or one of the debugger's synthetic roots.
func (d *Debugger) Expand(ctx context.Context, parentID string) (*adt.DebugChildVariablesInfo, error) {
	res, err := d.ADTChildVariables(ctx, []string{parentID})
	if err != nil {
		return nil, err
	}
	return adt.ParseChildVariablesXML(res.Body)
}

// localsRoot is the synthetic child of @ROOT that holds the current program's
// own data. The debugger also offers @GLOBALS, @ME and @DATAAGING there; the
// locals are what a caller asking "what are the variables" means.
const localsRoot = "@LOCALS"

// Locals reads the variables of the stack frame the debugger is sitting in.
//
// It takes two calls because the debugger's variable tree is addressed by id
// and the ids are only handed out by the level above: @ROOT names @LOCALS,
// @LOCALS names the program's own variables. Asking for "@LOCALS" directly
// works only because the id happens to be stable — the walk does not assume it,
// and falls back to every child of @ROOT when a release spells it differently.
func (d *Debugger) Locals(ctx context.Context) ([]adt.DebugVariable, error) {
	roots, err := d.Expand(ctx, "@ROOT")
	if err != nil {
		return nil, err
	}
	if roots == nil {
		return nil, nil
	}

	var parents []string
	for _, h := range roots.Hierarchies {
		if strings.EqualFold(h.ChildID, localsRoot) || strings.EqualFold(h.ChildName, localsRoot) {
			parents = []string{h.ChildID}
			break
		}
	}
	if parents == nil {
		// No @LOCALS on this release: expand whatever @ROOT does offer, rather
		// than reporting "no variables" when there plainly are some.
		for _, h := range roots.Hierarchies {
			parents = append(parents, h.ChildID)
		}
	}
	if len(parents) == 0 {
		return roots.Variables, nil
	}

	res, err := d.ADTChildVariables(ctx, parents)
	if err != nil {
		return nil, err
	}
	info, err := adt.ParseChildVariablesXML(res.Body)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	return info.Variables, nil
}

// FormatVariables renders a variable list the way a terminal wants it: one line
// per variable, the composite ones marked with the id needed to expand them.
func FormatVariables(vars []adt.DebugVariable) string {
	if len(vars) == 0 {
		return "no variables at this stop"
	}
	var sb strings.Builder
	for _, v := range vars {
		name := v.Name
		if name == "" {
			name = v.ID
		}
		fmt.Fprintf(&sb, "%-30s %-24s", name, v.DeclaredTypeName)
		switch {
		case v.MetaType == adt.DebugMetaTypeTable:
			fmt.Fprintf(&sb, "[%d lines] → %s", v.TableLines, v.ID)
		case v.IsComplexType():
			fmt.Fprintf(&sb, "{…} → %s", v.ID)
		default:
			// ABAP pads a fixed-length field to its full width, and justifies
			// some of them right, so the padding lands on either side. 200
			// blanks are not information; the exact bytes stay one 'eraw' away.
			sb.WriteString(strings.TrimSpace(v.Value))
		}
		if v.IsValueIncomplete {
			sb.WriteString(" …(truncated)")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatStack renders a call stack, marking the frame the debugger is in.
func FormatStack(info *adt.DebugStackInfo) string {
	if info == nil || len(info.Stack) == 0 {
		return "no stack — nothing is attached"
	}
	var sb strings.Builder
	for _, e := range info.Stack {
		marker := "  "
		if e.StackPosition == info.DebugCursorStackIndex {
			marker = "→ "
		}
		fmt.Fprintf(&sb, "%s[%d] %s/%s:%d %s %s\n",
			marker, e.StackPosition, e.ProgramName, e.IncludeName, e.Line, e.EventType, e.EventName)
	}
	return sb.String()
}

// SetVariable overwrites a variable in the stopped frame.
//
// The debugger is not only an observer: the value goes in and the next
// statement computes with it. Proven on A4H over both transports — LV_LOW came
// out of the database as 46, was overwritten with 900, and the following
// statement produced 901 rather than 47.
//
// That is what makes a scenario harness possible: reach a point by whatever
// route, then set the inputs to the case you actually want to exercise instead
// of arranging for the system to produce it. Save what was there first if the
// session is somebody else's — this changes real execution, including what it
// writes to the database.
func (d *Debugger) SetVariable(ctx context.Context, name, value string) error {
	q := url.Values{}
	q.Set("method", "setVariableValue")
	q.Set("variableName", name)

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "text/plain"}}, []byte(value))
	if err != nil {
		return err
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("setVariableValue", res)
	}
	return nil
}

// GoToFrame moves the debugger's cursor to another stack frame, so the
// variables read next are that frame's own.
//
// It is how the caller's half of a call boundary is reached: stopped inside a
// unit, step up one frame and the arguments as the caller sees them are
// readable. The uri is a frame's StackURI from the stack document.
func (d *Debugger) GoToFrame(ctx context.Context, stackURI string) error {
	res, err := d.ADT(ctx, "PUT", stackURI, nil, nil)
	if err != nil {
		return err
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("goToStack", res)
	}
	return nil
}

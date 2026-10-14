package saprfc

import (
	"context"
	"fmt"
	"net/url"
	"sort"
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
// maxExpandedTableRows bounds what one expansion asks for. A debugger stops in
// code that may hold a table of a million rows, and reading all of them would
// hang the session it is trying to describe.
const maxExpandedTableRows = 99

// TableSample says what an expansion actually read. A table small enough is
// read whole; a larger one is sampled, and then the difference matters — a
// caller shown rows 1..99 of a million-row table has been told almost nothing
// and, worse, has no way to tell that from a table with 99 rows in it.
type TableSample struct {
	// Lines is how many rows the table holds.
	Lines int
	// Rows are the row numbers actually read, in order.
	Rows []int
}

// Partial reports whether rows were left unread.
func (s *TableSample) Partial() bool {
	return s != nil && len(s.Rows) < s.Lines
}

// tableRowSample chooses which rows to read from a table of `lines` rows within
// a budget of requests.
//
// Reading the first N is the obvious thing and the least useful one: the head
// of a large table is where the least surprising data lives, and a bug in the
// last rows — a total that never accumulated, a key that never advanced — is
// invisible from there. So a table too large to read whole is sampled in three
// windows: the head, the middle, and the end. The end is worth as much as the
// head, and the middle says whether the two belong to the same run of data.
func tableRowSample(lines, budget int) []int {
	if lines <= 0 || budget <= 0 {
		return nil
	}
	if lines <= budget {
		rows := make([]int, 0, lines)
		for i := 1; i <= lines; i++ {
			rows = append(rows, i)
		}
		return rows
	}

	window := budget / 3
	if window < 1 {
		window = 1
	}
	middleStart := (lines-window)/2 + 1
	tailStart := lines - window + 1

	seen := make(map[int]bool, budget)
	rows := make([]int, 0, budget)
	take := func(from int) {
		for i := from; i < from+window && i <= lines; i++ {
			if i < 1 || seen[i] || len(rows) >= budget {
				continue
			}
			seen[i] = true
			rows = append(rows, i)
		}
	}
	take(1)
	take(middleStart)
	take(tailStart)

	sort.Ints(rows)
	return rows
}

// LastTableSample describes the rows the most recent expansion read, or nil if
// the last expansion was not of a table.
func (d *Debugger) LastTableSample() *TableSample {
	return d.lastTableSample
}

func (d *Debugger) Expand(ctx context.Context, parentID string) (*adt.DebugChildVariablesInfo, error) {
	d.lastTableSample = nil
	res, err := d.ADTChildVariables(ctx, []string{parentID})
	if err != nil {
		return nil, err
	}
	info, err := adt.ParseChildVariablesXML(res.Body)
	if err != nil {
		return nil, err
	}
	// A synthetic root such as @ROOT answers with hierarchies and no variables,
	// and that is a complete answer — Locals() is built on exactly that. Only an
	// answer empty of both is worth a second look.
	if info != nil && (len(info.Variables) > 0 || len(info.Hierarchies) > 0) {
		return info, nil
	}

	// An internal table is the one thing that does not expand by its own name:
	// SAP answers with an empty body rather than an error, so "expand this
	// table" looked like "this table has nothing in it" — even for a table that
	// had just been reported as holding two rows. Its rows are addressable, but
	// only one subscript at a time: LT_ROWS[1], LT_ROWS[2]. Ask for them.
	return d.expandTableRows(ctx, parentID, info)
}

// expandTableRows reads a table's rows as children, or reports that the
// variable was not a table after all — in which case the empty answer stands.
func (d *Debugger) expandTableRows(ctx context.Context, parentID string, empty *adt.DebugChildVariablesInfo) (*adt.DebugChildVariablesInfo, error) {
	described, err := d.Vars(ctx, []string{parentID})
	if err != nil || len(described) == 0 {
		// Nothing more to learn; the empty expansion was the answer, and it is
		// returned as it came rather than replaced by one we made up.
		return empty, nil
	}
	table := described[0]
	if table.MetaType != adt.DebugMetaTypeTable || table.TableLines <= 0 {
		return empty, nil
	}

	sample := tableRowSample(table.TableLines, maxExpandedTableRows)
	subscripts := make([]string, 0, len(sample))
	for _, row := range sample {
		subscripts = append(subscripts, fmt.Sprintf("%s[%d]", parentID, row))
	}
	// Kept so the caller can say which rows these are. A sample that presents
	// itself as the whole table is worse than no sample.
	d.lastTableSample = &TableSample{Lines: table.TableLines, Rows: sample}

	res, err := d.ADTChildVariables(ctx, subscripts)
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
	// Remember them. The batched capture cannot afford to ask @ROOT and then
	// its children as two round trips per statement, and it used to guess
	// @LOCALS — which this release does not have, so every recorded trace came
	// out with no values in it at all.
	d.localsRoots = parents

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
// GoToFrameAt moves the cursor to a frame by its position in the stack, which
// is how a person refers to one — "the caller", "frame 2" — rather than by the
// URI SAP names it with. The URI is a long opaque string that only exists in
// the output of a previous stack read, so requiring it means no script can move
// frames, and neither can anything driving the debugger programmatically.
//
// Positions are 1-based and count from the innermost frame, the same order the
// stack is printed in.
func (d *Debugger) GoToFrameAt(ctx context.Context, position int) error {
	info, err := d.StackInfo(ctx)
	if err != nil {
		return err
	}
	if position < 1 || position > len(info.Stack) {
		return fmt.Errorf("the stack has %d frames; there is no frame %d", len(info.Stack), position)
	}
	uri := info.Stack[position-1].StackURI
	if strings.TrimSpace(uri) == "" {
		return fmt.Errorf("frame %d carries no URI to move to; this release may not allow moving to it", position)
	}
	return d.GoToFrame(ctx, uri)
}

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

// localsRootsFor returns the hierarchy roots that hold the stopped frame's
// variables on this system, discovering them once per session.
//
// SAP does not agree with itself about the name: some releases offer @LOCALS,
// this one offers @GLOBALS and, inside a subroutine, @PARAMETERS. Asking @ROOT
// is how you find out, and the answer holds for the session.
func (d *Debugger) localsRootsFor(ctx context.Context) []string {
	if len(d.localsRoots) > 0 {
		return d.localsRoots
	}
	roots, err := d.Expand(ctx, "@ROOT")
	if err != nil || roots == nil {
		return []string{localsRoot}
	}
	var parents []string
	for _, h := range roots.Hierarchies {
		if strings.EqualFold(h.ChildID, localsRoot) || strings.EqualFold(h.ChildName, localsRoot) {
			parents = []string{h.ChildID}
			break
		}
	}
	if parents == nil {
		for _, h := range roots.Hierarchies {
			parents = append(parents, h.ChildID)
		}
	}
	if len(parents) == 0 {
		return []string{localsRoot}
	}
	d.localsRoots = parents
	return parents
}

// FormatRowRanges renders row numbers as compact ranges — "1-33, 500-532,
// 999967-999999" rather than a wall of numbers.
func FormatRowRanges(rows []int) string {
	if len(rows) == 0 {
		return "none"
	}
	var parts []string
	start, prev := rows[0], rows[0]
	flush := func() {
		if start == prev {
			parts = append(parts, fmt.Sprintf("%d", start))
			return
		}
		parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
	}
	for _, row := range rows[1:] {
		if row == prev+1 {
			prev = row
			continue
		}
		flush()
		start, prev = row, row
	}
	flush()
	return strings.Join(parts, ", ")
}

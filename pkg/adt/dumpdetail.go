package adt

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// The feed says what failed and where it failed at program granularity. Two of
// the four similarity rungs want more than that — the failing line, and the
// application component — and neither is in the feed.
//
// Both are in the formatted dump, and only there. The structured document at
// /sap/bc/adt/runtime/dump/<id> carries the error, the author, the exception
// class and the terminated program as XML attributes, and it does hide the
// termination line inside a link fragment ("...#start=36"), but it has no
// application component at all. So one fetch of the formatted text answers
// both questions and the structured one answers neither completely, which is
// why this file parses text instead of XML.
//
// The text is the English rendering. A client logged on in another language
// gets the same layout with translated labels, and the labels are what this
// matches on, so on such a system the detail comes back empty rather than
// wrong. Empty is the right failure here: an empty component simply drops the
// rung that needs it.

// DumpDetail is what one dump says beyond its feed entry.
type DumpDetail struct {
	ID        string `json:"id"`
	ErrorType string `json:"errorType,omitempty"`
	// Program is the header table's "ABAP: Program", and it is not always the
	// feed's terminated program. On a RAISE_EXCEPTION the feed names the
	// standard class that raised — CL_GUI_SPLITTER_CONTAINER — while the
	// header names the custom class that called it. Both are true statements
	// about the dump; they answer different questions, and the header one is
	// what the application component below belongs to.
	Program string `json:"program,omitempty"`
	// Exception is the class of an uncaught exception, empty for the many
	// runtime errors that are not one.
	Exception string `json:"exception,omitempty"`
	// Component is the application component of Program — see above, which
	// means it can describe the caller rather than the code that died. Empty
	// when the dump names none; see normalizeComponent for why "Not assigned"
	// counts as naming none.
	Component string `json:"component,omitempty"`
	// Include and Line are the termination point. Line is 0 when the dump has
	// no source position, which is a real state and not a parse failure.
	Include string `json:"include,omitempty"`
	Line    int    `json:"line,omitempty"`
	// Procedure is the method, form or function module that died.
	Procedure   string `json:"procedure,omitempty"`
	MainProgram string `json:"mainProgram,omitempty"`
	// Stack costs nothing extra: it is parsed out of the same document.
	Stack []DumpFrame `json:"stack,omitempty"`
}

// DumpDetail reads one dump beyond what the feed carries.
//
// Returns ErrDumpDetailUnavailable on a release that has the feed and not the
// detail resource. That is a statement about the release, not a fault, and
// callers are expected to carry on with the rungs that need no detail.
func (c *Client) DumpDetail(ctx context.Context, dumpID string) (*DumpDetail, error) {
	formatted, err := c.formattedDump(ctx, dumpID)
	if err != nil {
		return nil, err
	}
	detail := parseDumpDetail(formatted)
	detail.ID = dumpID
	return detail, nil
}

func parseDumpDetail(formatted string) *DumpDetail {
	header := parseDumpHeader(formatted)
	detail := &DumpDetail{
		ErrorType: header["Runtime Errors"],
		Program:   header["ABAP: Program"],
		Exception: header["Except."],
		Component: normalizeComponent(header["Application Component"]),
		Stack:     parseDumpStack(formatted),
	}
	where := parseDumpTermination(formatted)
	detail.Include, detail.Line = where.include, where.line
	detail.Procedure, detail.MainProgram = where.procedure, where.mainProgram
	return detail
}

// parseDumpHeader reads the table above the first chapter.
//
// Which rows are present depends on the dump's category: an "ABAP programming
// error" names the program and the component, a "Resource bottleneck" — the
// SYSTEM_NO_ROLL and SYSTEM_NO_MEMORY family — names neither. So this reads by
// label into a map and lets the caller find nothing, rather than assuming a
// fixed set of rows in a fixed order.
//
// Label and value are separated by a run of spaces wide enough to be a column
// gap. Two is enough and is what distinguishes the gap from the single spaces
// inside "Application Component".
func parseDumpHeader(formatted string) map[string]string {
	header := map[string]string{}
	for _, line := range strings.Split(formatted, "\n") {
		trimmed := strings.TrimRight(line, " \r")
		// The header table sits above every chapter, and chapter rows are
		// fenced with pipes. The first fenced row ends the header.
		if strings.HasPrefix(strings.TrimSpace(trimmed), "|") {
			break
		}
		if strings.TrimSpace(strings.Trim(trimmed, "-")) == "" {
			continue
		}
		label, value, ok := splitHeaderRow(trimmed)
		if !ok {
			continue
		}
		header[label] = value
	}
	return header
}

var headerGap = regexp.MustCompile(`\s{2,}`)

func splitHeaderRow(row string) (label, value string, ok bool) {
	// A leading space means this is a continuation or an indented body line,
	// not a label.
	if row == "" || row[0] == ' ' {
		return "", "", false
	}
	parts := headerGap.Split(strings.TrimRight(row, " "), 2)
	if len(parts) != 2 {
		return "", "", false
	}
	label = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if label == "" || value == "" {
		return "", "", false
	}
	return label, value, true
}

// normalizeComponent turns "Not assigned" into no component at all.
//
// This is the difference between a neighbourhood and an empty word. Custom
// code is almost never assigned to an application component, so a Z class that
// dumps reports "Not assigned" — and if that were treated as a value, every
// unassigned object in the system would look like one neighbourhood, which is
// the least useful grouping available. The rung that needs a component is
// better skipped than answered with that.
func normalizeComponent(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "Not assigned") {
		return ""
	}
	return value
}

const dumpTerminationChapter = "Information on where terminated"

type dumpTermination struct {
	include     string
	line        int
	procedure   string
	mainProgram string
}

var (
	// The chapter wraps at the fence width, and SAP wraps mid-sentence: the
	// line number and the include it belongs to routinely land on different
	// rows. So the chapter is joined into one string before any of this is
	// matched — a line-by-line regex silently finds nothing on exactly the
	// dumps where the include name is long, which is to say on class pools.
	terminationLine = regexp.MustCompile(`termination point is in line (\d+) of include "([^"]*)"`)
	// "occurred in ABAP program" is written "occurred inABAP program" on some
	// dumps — SAP's wrapper drops the space when the break falls there, and it
	// survives into the text. Requiring the space silently loses the procedure
	// name on exactly those dumps, so the space is optional.
	terminationProc    = regexp.MustCompile(`occurred in\s*ABAP program or include "([^"]*)", in "([^"]*)"`)
	terminationProgram = regexp.MustCompile(`main program was "([^"]*)"`)
)

func parseDumpTermination(formatted string) dumpTermination {
	text := joinDumpChapter(formatted, dumpTerminationChapter)
	if text == "" {
		return dumpTermination{}
	}
	var where dumpTermination
	if m := terminationLine.FindStringSubmatch(text); m != nil {
		line, _ := strconv.Atoi(m[1])
		include := strings.TrimSpace(m[2])
		// A dump that terminated outside ABAP source says so as line 0 of
		// include " " — every DYNPRO_SEND_IN_BACKGROUND on a live system looks
		// like this, because it died in SYSTEM-EXIT. Recording that as include
		// " " at line 0 would make unrelated dumps match on a position that
		// does not exist.
		if line > 0 && include != "" {
			where.line, where.include = line, include
		}
	}
	if m := terminationProc.FindStringSubmatch(text); m != nil {
		where.procedure = strings.TrimSpace(m[2])
	}
	if m := terminationProgram.FindStringSubmatch(text); m != nil {
		where.mainProgram = strings.TrimSpace(m[1])
	}
	return where
}

// joinDumpChapter returns one chapter's body as a single line.
func joinDumpChapter(formatted string, title string) string {
	lines := strings.Split(formatted, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, title) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	var body []string
	for i := start + 1; i < len(lines); i++ {
		row := strings.TrimSpace(lines[i])
		// A rule of dashes closes the chapter; anything unfenced is outside it.
		if !strings.HasPrefix(row, "|") {
			break
		}
		body = append(body, strings.TrimSpace(strings.Trim(row, "|")))
	}
	return strings.Join(strings.Fields(strings.Join(body, " ")), " ")
}

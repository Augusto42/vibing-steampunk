package saprfc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
)

// The debugger's server-side registry is three ordinary transparent tables, so
// the read half of debugging needs no ABAP at all: ABDBG_ACTIVATION lists the
// debuggees parked and waiting for someone to attach, and ABDBG_EXTDBPS lists
// the external breakpoints per user. That is enough to answer "did anyone hit my
// breakpoint, and where" — the question people usually open a debugger to ask.
//
// Attaching and stepping is the other half, and it needs both a pinned RFC
// session and the ZADT_DEBUG facade (abap/src/zadt_debug).

// Debuggee is one ABAP session stopped and waiting to be attached to.
type Debuggee struct {
	ID       string `json:"debuggee_id"`
	User     string `json:"user"`
	Program  string `json:"program,omitempty"`
	Include  string `json:"include,omitempty"`
	Line     int    `json:"line,omitempty"`
	Kind     string `json:"kind,omitempty"`   // DEBUGGEE, PMORTEM, PMORTDIA, …
	Server   string `json:"server,omitempty"` // application server it is parked on
	SystemID string `json:"system_id,omitempty"`
	IDEID    string `json:"ide_id,omitempty"`
	Terminal string `json:"terminal_id,omitempty"`
	DumpID   string `json:"dump_id,omitempty"` // set for a post-mortem (short dump) debuggee
	DumpDate string `json:"dump_date,omitempty"`
	DumpTime string `json:"dump_time,omitempty"`
}

// PostMortem reports whether this debuggee is a captured short dump rather than
// a running session.
func (d Debuggee) PostMortem() bool { return strings.TrimSpace(d.DumpID) != "" }

// Text renders one debuggee as a single line.
func (d Debuggee) Text() string {
	where := d.Program
	if d.Include != "" && d.Include != d.Program {
		where += "/" + d.Include
	}
	if d.Line > 0 {
		where += fmt.Sprintf(":%d", d.Line)
	}
	if where == "" {
		where = "(unknown position)"
	}
	line := fmt.Sprintf("%s  %-12s %s on %s", d.ID, d.User, where, d.Server)
	if d.PostMortem() {
		line += fmt.Sprintf("  [dump %s %s %s]", d.DumpID, d.DumpDate, d.DumpTime)
	}
	return line
}

// debuggeeFields excludes DBGKEY: it is an XSTRING, and RFC_READ_TABLE cannot
// return LOB columns.
var debuggeeFields = []string{
	"DEBUGGEE_ID", "DEBUGGEE_USER", "PRG_CURR", "INCL_CURR", "LINE_CURR",
	"DBGEE_KIND", "APPLSERVER", "SYSID", "IDE_ID", "TERMINAL_ID",
	"DUMPID", "DUMPDATE", "DUMPTIME",
}

// WaitingDebuggees lists the sessions currently parked in the debugger. Pass a
// user to see only that user's; an empty user lists every one.
func WaitingDebuggees(ctx context.Context, c *rfc.Client, user string) ([]Debuggee, error) {
	where := ""
	if user = strings.ToUpper(strings.TrimSpace(user)); user != "" {
		where = fmt.Sprintf("DEBUGGEE_USER = '%s'", user)
	}
	rows, err := ReadTable(ctx, c, "ABDBG_ACTIVATION", where, debuggeeFields, 0)
	if err != nil {
		return nil, fmt.Errorf("reading waiting debuggees: %w", err)
	}
	out := make([]Debuggee, 0, len(rows))
	for _, r := range rows {
		line, _ := strconv.Atoi(strings.TrimSpace(r["LINE_CURR"]))
		out = append(out, Debuggee{
			ID:       strings.TrimSpace(r["DEBUGGEE_ID"]),
			User:     strings.TrimSpace(r["DEBUGGEE_USER"]),
			Program:  strings.TrimSpace(r["PRG_CURR"]),
			Include:  strings.TrimSpace(r["INCL_CURR"]),
			Line:     line,
			Kind:     strings.TrimSpace(r["DBGEE_KIND"]),
			Server:   strings.TrimSpace(r["APPLSERVER"]),
			SystemID: strings.TrimSpace(r["SYSID"]),
			IDEID:    strings.TrimSpace(r["IDE_ID"]),
			Terminal: strings.TrimSpace(r["TERMINAL_ID"]),
			DumpID:   strings.TrimSpace(r["DUMPID"]),
			DumpDate: strings.TrimSpace(r["DUMPDATE"]),
			DumpTime: strings.TrimSpace(r["DUMPTIME"]),
		})
	}
	return out, nil
}

// ExternalBreakpoint is one row of the external-breakpoint registry. The payload
// columns (BREAKPOINT, ATTRIBUTES) are STRG and therefore unreadable through
// RFC_READ_TABLE — program and line come from ZADT_DEBUG_BP_LIST instead.
type ExternalBreakpoint struct {
	User      string `json:"user"`
	Index     int    `json:"index"`
	SetBy     string `json:"set_by,omitempty"`
	IDEID     string `json:"ide_id,omitempty"`
	Terminal  string `json:"terminal_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// ExternalBreakpoints lists the external breakpoints registered for a user.
func ExternalBreakpoints(ctx context.Context, c *rfc.Client, user string) ([]ExternalBreakpoint, error) {
	where := ""
	if user = strings.ToUpper(strings.TrimSpace(user)); user != "" {
		where = fmt.Sprintf("USERNAME = '%s'", user)
	}
	rows, err := ReadTable(ctx, c, "ABDBG_EXTDBPS", where,
		[]string{"USERNAME", "BP_INDEX", "RQ_USER", "RQ_IDEID", "RQ_TERMID", "TIMESTAMP"}, 0)
	if err != nil {
		return nil, fmt.Errorf("reading external breakpoints: %w", err)
	}
	out := make([]ExternalBreakpoint, 0, len(rows))
	for _, r := range rows {
		idx, _ := strconv.Atoi(strings.TrimSpace(r["BP_INDEX"]))
		out = append(out, ExternalBreakpoint{
			User:      strings.TrimSpace(r["USERNAME"]),
			Index:     idx,
			SetBy:     strings.TrimSpace(r["RQ_USER"]),
			IDEID:     strings.TrimSpace(r["RQ_IDEID"]),
			Terminal:  strings.TrimSpace(r["RQ_TERMID"]),
			Timestamp: strings.TrimSpace(r["TIMESTAMP"]),
		})
	}
	return out, nil
}

// WatchDebuggees polls the registry and reports each debuggee the first time it
// appears, until the context is cancelled or the deadline passes. It is the
// zero-ABAP half of "tell me when my breakpoint is hit": the debuggee is already
// parked and waiting, so nothing has to be pushed.
func WatchDebuggees(ctx context.Context, c *rfc.Client, user string, interval, until time.Duration, onNew func(Debuggee)) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	seen := map[string]bool{}
	var deadline time.Time
	if until > 0 {
		deadline = time.Now().Add(until)
	}
	for {
		found, err := WaitingDebuggees(ctx, c, user)
		if err != nil {
			return err
		}
		for _, d := range found {
			if !seen[d.ID] {
				seen[d.ID] = true
				onNew(d)
			}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

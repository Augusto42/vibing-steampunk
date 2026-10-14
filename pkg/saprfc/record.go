package saprfc

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Recording what a unit did, statement by statement, with the values.
//
// This is the expensive half of "what really ran". A SAT trace gives the call
// tree for almost nothing but carries no data; this walks a unit with the
// debugger and reads its variables at every stop, which costs one round trip
// each — 14ms batched, so a few thousand stops a minute. It is a deliberate
// mode, not something to leave running.
//
// Three constraints shape it, all measured rather than assumed:
//
//   - Thirty external breakpoints per user, system-wide. Instrumenting every
//     call site is therefore impossible; one anchor per unit and stepping from
//     it is the affordable shape.
//   - stepOver walks a unit without descending into what it calls, which is
//     what keeps the stop count proportional to the unit rather than to the
//     whole program beneath it.
//   - stepReturn out of a unit whose caller is SAP's own code does not stop —
//     with system debugging off there is nowhere to stop — and the debuggee
//     runs to completion. So a recording ends when the frame is gone, and the
//     exit is recognised by watching the stack rather than by stepping out.

// StopRecord is one recorded stop. The stream of them is the history: one JSON
// object per line, the format the offline tools read.
type StopRecord struct {
	Seq       int               `json:"seq"`
	Kind      string            `json:"kind"` // enter | step | exit
	Program   string            `json:"program"`
	Include   string            `json:"include"`
	Line      int               `json:"line"`
	Event     string            `json:"event,omitempty"`
	EventType string            `json:"eventType,omitempty"`
	Depth     int               `json:"depth"`
	Vars      map[string]string `json:"vars,omitempty"`
	Composite map[string]string `json:"composite,omitempty"` // name → id to expand
}

// RecordOptions bounds a recording. A recorder without a bound is a way to hang
// somebody's work process, so MaxStops is applied whether or not it is set.
type RecordOptions struct {
	MaxStops int
	StepKind string // stepOver by default: walk the unit, do not descend
	// StayIn decides whether a stop still belongs to the recording. The default
	// follows the unit the recording started in.
	StayIn func(program, include string) bool
	// Redact hides values. On by default: a boundary capture is business data
	// by construction.
	Redact bool
}

const defaultMaxStops = 2000

// Record walks from wherever the debuggee is stopped and emits one record per
// stop until the unit is left, the bound is reached, or the debuggee ends.
//
// It returns the number of stops recorded. Ending because the debuggee ran to
// completion is a normal outcome, not an error: a unit that returns into SAP's
// own code takes the session with it.
func (d *Debugger) Record(ctx context.Context, opts RecordOptions, emit func(StopRecord) error) (int, error) {
	if opts.MaxStops <= 0 {
		opts.MaxStops = defaultMaxStops
	}
	if opts.StepKind == "" {
		opts.StepKind = "stepOver"
	}

	seq := 0
	var stayIn func(string, string) bool = opts.StayIn
	var last StopRecord

	for kind := ""; seq < opts.MaxStops; kind = opts.StepKind {
		stop, err := d.CaptureStep(ctx, kind)
		if err != nil {
			// The debuggee running to completion ends the recording rather than
			// failing it — see the note above about stepping out of a unit
			// whose caller is standard code. It is still marked, so a reader can
			// tell a finished recording from a truncated one.
			if strings.Contains(err.Error(), "debuggeeEnded") {
				if seq > 0 {
					last.Seq, last.Kind, last.Vars, last.Composite = seq, "exit", nil, nil
					if eerr := emit(last); eerr != nil {
						return seq, eerr
					}
					seq++
				}
				return seq, nil
			}
			return seq, err
		}

		info, err := adt.ParseStackXML(stop.Stack)
		if err != nil || info == nil || len(info.Stack) == 0 {
			return seq, err
		}
		top := info.Stack[0]

		// The first stop defines the unit unless the caller said otherwise.
		if stayIn == nil {
			program, include := top.ProgramName, top.IncludeName
			stayIn = func(p, i string) bool { return p == program && i == include }
		}

		rec := StopRecord{
			Seq: seq, Kind: "step",
			Program: top.ProgramName, Include: top.IncludeName, Line: top.Line,
			Event: top.EventName, EventType: top.EventType, Depth: len(info.Stack),
		}
		if seq == 0 {
			rec.Kind = "enter"
		}
		if !stayIn(top.ProgramName, top.IncludeName) {
			rec.Kind = "exit"
		}

		if vars, verr := adt.ParseChildVariablesXML(stop.Locals); verr == nil && vars != nil {
			rec.Vars, rec.Composite = splitVariables(vars.Variables, opts.Redact)
		}
		if err := emit(rec); err != nil {
			return seq, err
		}
		last = rec
		seq++
		if rec.Kind == "exit" {
			return seq, nil
		}
	}
	return seq, nil
}

// splitVariables separates what has a value from what has to be expanded. A
// table or a structure is named with the id that fetches it, so a reader can
// ask for the one it cares about instead of paying for all of them.
func splitVariables(vars []adt.DebugVariable, redact bool) (map[string]string, map[string]string) {
	var scalars, composite map[string]string
	for _, v := range vars {
		name := v.Name
		if name == "" {
			name = v.ID
		}
		if v.IsComplexType() {
			if composite == nil {
				composite = map[string]string{}
			}
			composite[name] = v.ID
			continue
		}
		if scalars == nil {
			scalars = map[string]string{}
		}
		if redact {
			scalars[name] = redactValue(v)
			continue
		}
		scalars[name] = strings.TrimSpace(v.Value)
	}
	return scalars, composite
}

// redactValue keeps the shape of a value and hides the value itself. Length and
// emptiness are usually what a reader needs to follow control flow; the content
// is business data and belongs behind an explicit flag.
func redactValue(v adt.DebugVariable) string {
	trimmed := strings.TrimSpace(v.Value)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("«%s:%d»", strings.ToLower(v.TechnicalType), len(trimmed))
}

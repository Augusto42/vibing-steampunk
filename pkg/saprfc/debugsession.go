package saprfc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/oisee/open-rfc-go/rfc"
)

// The debugger's session half — listen, attach, step, stack — only works when
// consecutive calls reach the same ABAP roll area, because ATTACH_DEBUGGEE
// returns an object reference and everything else hangs off it. That is what a
// pinned RFC conversation is for, and it is the whole reason this type exists:
// one Debugger owns one pinned connection for its lifetime, and every operation
// goes through it. Calling the facade through Client.Call instead would land in
// a different work process and lose the session between two calls.
//
// The server side is ZADT_DEBUG_RFC (abap/src/zadt_debug), which dispatches on
// I_OP and answers with one JSON payload.

// FacadeFunction is the single RFC entry point of the ABAP-side facade.
const FacadeFunction = "ZADT_DEBUG_RFC"

// Debugger drives the ABAP debugger over one pinned RFC conversation.
type Debugger struct {
	session *rfc.Session
	user    string
}

// NewDebugger pins a connection out of the pool and keeps it until Close. The
// user is whose debuggees to listen for; empty means the logon user.
func NewDebugger(ctx context.Context, c *rfc.Client, user string) (*Debugger, error) {
	session, err := c.Pin(ctx)
	if err != nil {
		return nil, fmt.Errorf("pinning a connection for the debug session: %w", err)
	}
	return &Debugger{session: session, user: strings.ToUpper(strings.TrimSpace(user))}, nil
}

// Close releases the pinned connection. It first tries to leave the system
// tidy: an attached debuggee and a registered listener both outlive the
// conversation otherwise, and a stale ABDBG_LISTENER row blocks the next attach.
func (d *Debugger) Close(ctx context.Context) error {
	if d.session == nil {
		return nil
	}
	_, _ = d.Detach(ctx)
	err := d.session.Close()
	d.session = nil
	return err
}

// Op runs one facade operation and returns its JSON payload.
func (d *Debugger) Op(ctx context.Context, op string, args rfc.Params) (json.RawMessage, error) {
	if d.session == nil {
		return nil, fmt.Errorf("the debug session is closed")
	}
	params := rfc.Params{"I_OP": op}
	if d.user != "" {
		params["I_USER"] = d.user
	}
	for k, v := range args {
		params[k] = v
	}

	res, err := d.session.Call(ctx, FacadeFunction, params)
	if err != nil {
		return nil, fmt.Errorf("%s(%s): %w", FacadeFunction, op, err)
	}
	// The facade never raises an RFC exception, because that would discard the
	// exporting parameters and with them the message.
	if rc := asInt32(res.Get("E_RC")); rc != 0 {
		msg := strings.TrimSpace(fmt.Sprint(res.Get("E_MESSAGE")))
		if msg == "" {
			msg = fmt.Sprintf("rc=%d", rc)
		}
		return nil, fmt.Errorf("%s: %s", op, msg)
	}
	payload := strings.TrimSpace(fmt.Sprint(res.Get("E_JSON")))
	if payload == "" {
		return nil, nil
	}
	return json.RawMessage(payload), nil
}

// ADT tunnels one ADT REST request through this pinned session. Eclipse drives
// the debugger over exactly these resources, and the only reason a stateless
// client cannot is that ADT keeps the debug session in an ABAP roll area. Here
// the roll area is the pinned conversation, so the standard surface should work
// with no Z code at all — the open question this makes testable.
func (d *Debugger) ADT(ctx context.Context, method, uri string, headers []ADTHeader, body []byte) (*ADTResponse, error) {
	if d.session == nil {
		return nil, fmt.Errorf("the debug session is closed")
	}
	return CallADTOn(ctx, d.session, ADTRequest{Method: method, URI: uri, Headers: headers, Body: body})
}

// State returns the facade's view of this session: which roll area it landed
// in, how many calls it has served, and whether a debuggee is attached. Two
// calls on one Debugger must report the same "roll" and a rising "calls" — if
// they do not, the connection is not pinned and nothing else here will work.
func (d *Debugger) State(ctx context.Context) (json.RawMessage, error) {
	return d.Op(ctx, "state", nil)
}

// SetBreakpoint registers an external line breakpoint for the session's user.
// A line number alone is read against the main program, so a breakpoint inside
// a function module or a class method needs its include named too — for
// ZADT_DEBUG_LOOP that is program SAPLZADT_DEBUG, include LZADT_DEBUGU01.
func (d *Debugger) SetBreakpoint(ctx context.Context, program, include string, line int, condition string) (json.RawMessage, error) {
	args := rfc.Params{"I_PROGRAM": strings.ToUpper(program), "I_LINE": line}
	if include != "" {
		args["I_INCLUDE"] = strings.ToUpper(include)
	}
	if condition != "" {
		args["I_CONDITION"] = condition
	}
	return d.Op(ctx, "bp_set", args)
}

// Breakpoints lists the external breakpoints with their payload — program and
// line, which the ABDBG_EXTDBPS read cannot give (those columns are LOBs).
func (d *Debugger) Breakpoints(ctx context.Context) (json.RawMessage, error) {
	return d.Op(ctx, "bp_list", nil)
}

// DeleteBreakpoints removes breakpoints matching a program (and optionally a
// line), or all of them.
func (d *Debugger) DeleteBreakpoints(ctx context.Context, program string, line int, all bool) (json.RawMessage, error) {
	args := rfc.Params{}
	if all {
		args["I_ALL"] = "X"
	} else {
		args["I_PROGRAM"] = strings.ToUpper(program)
		if line > 0 {
			args["I_LINE"] = line
		}
	}
	return d.Op(ctx, "bp_delete", args)
}

// Listen blocks server-side until a debuggee of this user stops or the timeout
// expires, then returns the waiting debuggees. The call occupies the pinned
// conversation for its whole duration, so the client call timeout has to be
// longer than the ABAP timeout — see rfctool.OpenWithTimeout.
func (d *Debugger) Listen(ctx context.Context, timeoutSeconds int) (json.RawMessage, error) {
	return d.Op(ctx, "listen", rfc.Params{"I_TIMEOUT": timeoutSeconds})
}

// WaitingDebuggee is one entry of the listen payload, as the facade serialises
// IF_TPDAPI_SERVICE~TYP_TAB_DEBUGGEES.
type WaitingDebuggee struct {
	ID   string `json:"debuggee_id"`
	User string `json:"debuggee_user"`
	// TPDAPI names the stop location program/include/line in the debuggee
	// structure it returns — not PRG_CURR/INCL_CURR/LINE_CURR, which is how the
	// same information is spelled in the ABDBG_ACTIVATION table.
	Program string `json:"program"`
	Include string `json:"include"`
	Line    int    `json:"line"`
	Kind    string `json:"dbgee_kind"`
}

// ListenAndAttach blocks for a debuggee and attaches to the first one that
// appears. It exists because listen and attach have to happen on the same
// pinned session: a script cannot name a debuggee id it has not seen yet, and
// starting a second command to attach would land in another roll area.
// It returns the debuggee it took and the attach payload; a timeout with
// nobody waiting yields a nil debuggee and no error.
func (d *Debugger) ListenAndAttach(ctx context.Context, timeoutSeconds int) (*WaitingDebuggee, json.RawMessage, error) {
	raw, err := d.Listen(ctx, timeoutSeconds)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		Status    string            `json:"status"`
		Debuggees []WaitingDebuggee `json:"debuggees"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("reading the listen result: %w", err)
	}
	if len(payload.Debuggees) == 0 {
		return nil, nil, nil
	}
	first := payload.Debuggees[0]
	attached, err := d.Attach(ctx, first.ID)
	if err != nil {
		return &first, nil, err
	}
	return &first, attached, nil
}

// Attach binds this session to a waiting debuggee and reports where it stopped.
func (d *Debugger) Attach(ctx context.Context, debuggeeID string) (json.RawMessage, error) {
	return d.Op(ctx, "attach", rfc.Params{"I_DEBUGGEE_ID": debuggeeID})
}

// Step executes one debugger step: into, over, out, or continue.
func (d *Debugger) Step(ctx context.Context, kind string) (json.RawMessage, error) {
	if kind == "" {
		kind = "into"
	}
	return d.Op(ctx, "step", rfc.Params{"I_KIND": strings.ToLower(kind)})
}

// Stack returns the call stack of the attached debuggee.
func (d *Debugger) Stack(ctx context.Context) (json.RawMessage, error) {
	return d.Op(ctx, "stack", nil)
}

// Detach ends the debugger session and stops the listener, leaving no
// ABDBG_LISTENER row behind.
//
// END_DEBUGGER tears down the debugger's own ABAP session along with the
// debuggee's, so the conversation usually dies mid-answer: the transport
// reports CM_NO_DATA_RECEIVED and there is no reply to read. That is the
// successful outcome, not a failure — the session is gone either way, so the
// pinned connection is dropped rather than returned to the pool.
func (d *Debugger) Detach(ctx context.Context) (json.RawMessage, error) {
	out, err := d.Op(ctx, "detach", nil)
	if err != nil && errors.Is(err, rfc.ErrTransport) {
		return nil, nil
	}
	return out, err
}

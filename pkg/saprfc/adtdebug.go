package saprfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// Driving SAP's own ADT debugger resources over the RFC tunnel — the same
// endpoints Eclipse uses, with no Z code on the server at all.
//
// The one thing that made this look impossible from a tool like vsp is that ADT
// keeps the debug session in an ABAP roll area and selects it with a
// sap-contextid cookie, which a short-lived stateless client cannot hold. Over a
// pinned RFC conversation there is nothing to hold: the roll area IS the
// conversation. Proven live on A4H — POST /debugger/listeners returned a
// DebuggeesList through the tunnel.
//
// The window between the listener returning and the attach is small: the
// debuggee is only valid while it waits, and a human copying an id from one
// command into the next loses that race (CX_ABDBG_ACTEXT_CANNOT_ATTACH,
// subType invalidDebuggee). So listen, attach and stack run as one step here.

// ADTDebuggee is the subset of STPDA_DEBUGGEE worth carrying.
type ADTDebuggee struct {
	ID      string `xml:"DEBUGGEE_ID"`
	User    string `xml:"DEBUGGEE_USER"`
	Program string `xml:"PRG_CURR"`
	Include string `xml:"INCL_CURR"`
	Line    int    `xml:"LINE_CURR"`
	Kind    string `xml:"DBGEE_KIND"`
	Name    string `xml:"NAME"`
	Type    string `xml:"TYPE"`
	URI     string `xml:"URI"`
	// IS_ATTACH_IMPOSSIBLE is "true"/"false" text, not a boolean type.
	AttachImpossible string `xml:"IS_ATTACH_IMPOSSIBLE"`
}

// adtDebuggeeList is the DebuggeesList envelope the listener answers with.
type adtDebuggeeList struct {
	Debuggees []ADTDebuggee `xml:"values>DATA>STPDA_DEBUGGEE"`
}

// ADTListen posts the blocking listener and returns the debuggee that stopped,
// or nil when the wait timed out with nobody there.
func (d *Debugger) ADTListen(ctx context.Context, user, ideID, terminalID string, timeoutSeconds int) (*ADTDebuggee, error) {
	d.engaged = true
	// Remembered for the teardown: a listener is removed by naming the exact
	// triple it was registered with, and a row left behind blocks the next one.
	d.listenUser, d.ideID, d.terminalID = strings.ToUpper(user), ideID, terminalID
	q := url.Values{}
	q.Set("debuggingMode", "user")
	q.Set("requestUser", strings.ToUpper(user))
	q.Set("ideId", ideID)
	q.Set("terminalId", terminalID)
	q.Set("timeout", fmt.Sprint(timeoutSeconds))

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger/listeners?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, adtError("listen", res)
	}
	if len(res.Body) == 0 {
		return nil, nil // the listener timed out: nobody stopped
	}
	var list adtDebuggeeList
	if err := xml.Unmarshal(res.Body, &list); err != nil {
		return nil, fmt.Errorf("reading the debuggee list: %w", err)
	}
	if len(list.Debuggees) == 0 {
		return nil, nil
	}
	return &list.Debuggees[0], nil
}

// ADTAttach attaches this session to a waiting debuggee.
func (d *Debugger) ADTAttach(ctx context.Context, debuggeeID, user string) (*ADTResponse, error) {
	d.engaged = true
	q := url.Values{}
	q.Set("method", "attach")
	q.Set("debuggeeId", debuggeeID)
	q.Set("debuggingMode", "user")
	q.Set("requestUser", strings.ToUpper(user))
	q.Set("dynproDebugging", "true")

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "application/xml"}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("attach", res)
	}
	return res, nil
}

// ADTStack reads the attached debuggee's call stack.
func (d *Debugger) ADTStack(ctx context.Context) (*ADTResponse, error) {
	q := url.Values{}
	q.Set("method", "getStack")
	q.Set("emode", "_")
	q.Set("semanticURIs", "true")

	res, err := d.ADT(ctx, "GET", "/sap/bc/adt/debugger/stack?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "application/xml"}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("stack", res)
	}
	return res, nil
}

// ADTVariables reads named variables from the attached debuggee. The names it
// wants are the ones the stack and child-variable calls hand back — @ROOT and
// @DATAAGING are the two roots that always exist, a local is just its name.
func (d *Debugger) ADTVariables(ctx context.Context, names []string) (*ADTResponse, error) {
	if len(names) == 0 {
		names = []string{"@ROOT", "@DATAAGING"}
	}
	var items []string
	for _, n := range names {
		items = append(items, "<STPDA_ADT_VARIABLE><ID>"+xmlEsc(n)+"</ID></STPDA_ADT_VARIABLE>")
	}
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>` +
		strings.Join(items, "") + `</DATA></asx:values></asx:abap>`)
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=getVariables",
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"},
			{Name: "Content-Type", Value: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables"}}, body)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("variables", res)
	}
	return res, nil
}

// ADTChildVariables expands a structure or table variable by parent id.
func (d *Debugger) ADTChildVariables(ctx context.Context, parents []string) (*ADTResponse, error) {
	if len(parents) == 0 {
		parents = []string{"@ROOT", "@DATAAGING"}
	}
	var items []string
	for _, p := range parents {
		items = append(items, "<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>"+xmlEsc(p)+"</PARENT_ID></STPDA_ADT_VARIABLE_HIERARCHY>")
	}
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><HIERARCHIES>` +
		strings.Join(items, "") + `</HIERARCHIES></DATA></asx:values></asx:abap>`)
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=getChildVariables",
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"},
			{Name: "Content-Type", Value: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables"}}, body)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("childVariables", res)
	}
	return res, nil
}

// xmlEsc escapes the few characters that matter inside an element body.
func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// ADTDetach ends an ADT debug session: it releases the debuggee and removes the
// listener registration.
//
// It exists because closing the client is not enough on the HTTP route. Over RFC
// the facade's detach ends external debugging for the user and the debuggee runs
// on; over HTTPS there is no facade, so a session that simply exits leaves its
// debuggee suspended in a work process until the caller's own timeout fires —
// which is what it looked like from the other side: an RFC call that never
// answered.
func (d *Debugger) ADTDetach(ctx context.Context) error {
	if !d.engaged {
		return nil
	}
	// SAP's own word for "let it go" is detach; a release that does not know it
	// still has to release the debuggee, so continue is the fallback rather than
	// terminateDebuggee, which would kill the user's session outright.
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=detach",
		[]ADTHeader{{Name: "Accept", Value: "application/xml"}}, nil)
	if err != nil || res.Status < 200 || res.Status >= 300 {
		_, _ = d.ADTStep(ctx, "stepContinue")
	}
	if d.terminalID != "" {
		q := url.Values{}
		q.Set("debuggingMode", "user")
		q.Set("requestUser", d.listenUser)
		q.Set("ideId", d.ideID)
		q.Set("terminalId", d.terminalID)
		if _, lerr := d.ADT(ctx, "DELETE", "/sap/bc/adt/debugger/listeners?"+q.Encode(), nil, nil); lerr != nil {
			return lerr
		}
	}
	d.engaged = false
	return nil
}

// ADTStep executes one step: stepInto, stepOver, stepReturn, stepContinue.
func (d *Debugger) ADTStep(ctx context.Context, method string) (*ADTResponse, error) {
	q := url.Values{}
	q.Set("method", method)

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "application/xml"}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError(method, res)
	}
	return res, nil
}

// ADTCatch is listen → attach → stack with nothing in between, because the
// debuggee stays attachable only while it waits.
func (d *Debugger) ADTCatch(ctx context.Context, user, ideID, terminalID string, timeoutSeconds int) (*ADTDebuggee, *ADTResponse, error) {
	who, err := d.ADTListen(ctx, user, ideID, terminalID, timeoutSeconds)
	if err != nil || who == nil {
		return nil, nil, err
	}
	if _, err := d.ADTAttach(ctx, who.ID, user); err != nil {
		return who, nil, err
	}
	stack, err := d.ADTStack(ctx)
	return who, stack, err
}

// adtError turns an ADT exception document into a Go error, keeping the
// subType — "invalidDebuggee" and "noSessionAttached" are the two that actually
// tell you what went wrong.
func adtError(what string, res *ADTResponse) error {
	body := string(res.Body)
	detail := ""
	if i := strings.Index(body, `subType">`); i >= 0 {
		if j := strings.Index(body[i+9:], "<"); j > 0 {
			detail = body[i+9 : i+9+j]
		}
	}
	if detail == "" {
		if i := strings.Index(body, "<message lang=\"EN\">"); i >= 0 {
			if j := strings.Index(body[i+19:], "<"); j > 0 {
				detail = body[i+19 : i+19+j]
			}
		}
	}
	return fmt.Errorf("%s: ADT %d %s (%s)", what, res.Status, res.ReasonPhrase, detail)
}

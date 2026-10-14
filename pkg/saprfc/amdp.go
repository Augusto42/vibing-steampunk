package saprfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// AMDP — ABAP Managed Database Procedures — run inside HANA rather than inside
// ABAP, so debugging one means bridging two debuggers. ADT does that itself:
// /sap/bc/adt/amdp/debugger/* is a complete API, described in the discovery
// document, and none of it needs code installed on the server.
//
// That is worth stating because this project spent a long time believing
// otherwise, and built a Z service and a WebSocket protocol to reach what the
// system was already offering. See
// reports/2026-08-23-001-amdp-debugger-is-native-adt.md.
//
// Everything here has to happen on one held session. The ADT resource keeps its
// state in class-data — the debugger handle, the control handle, the main id —
// which is ABAP session memory, so a second connection finds an empty one. The
// same constraint as the ABAP debugger, for the same reason.

// AMDPSession is a started AMDP debug session.
type AMDPSession struct {
	// MainID identifies the session to every other AMDP resource. It arrives in
	// the Location header of the start response, not in its body — the body
	// carries the HANA session id, which is a different thing and was mistaken
	// for this one until the handler was read.
	MainID string
	// HANASessionID is the database side of the bridge, as host:port:session.
	// Nothing here needs it; it is reported because its presence is the
	// evidence that the ABAP-to-HANA binding was actually established.
	HANASessionID string
}

// AMDPStart begins an AMDP debug session for a user.
//
// stopExisting kills a session that user already has. Without it a second start
// fails, and a session left behind by a crashed client would block every
// attempt until it timed out.
func (d *Debugger) AMDPStart(ctx context.Context, user string, stopExisting bool) (*AMDPSession, error) {
	q := url.Values{}
	q.Set("requestUser", strings.ToUpper(strings.TrimSpace(user)))
	if stopExisting {
		q.Set("stopExisting", "true")
	}

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/amdp/debugger/main?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return nil, adtError("amdp start", res)
	}

	session := &AMDPSession{
		MainID:        mainIDFromLocation(res.Header("location")),
		HANASessionID: amdpStartParameter(res.Body, "HANA_SESSION_ID"),
	}
	if session.MainID == "" {
		return nil, fmt.Errorf("the AMDP debugger started without naming its session; expected it in the Location header")
	}
	d.amdpMain = session.MainID
	return session, nil
}

// mainIDFromLocation reads the id out of the Location header the start response
// carries: /sap/bc/adt/amdp/debugger/main/{mainId}.
func mainIDFromLocation(location string) string {
	const marker = "/main/"
	i := strings.LastIndex(location, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(location[i+len(marker):])
}

// amdpStartParameter pulls one named value out of the start response.
func amdpStartParameter(body []byte, key string) string {
	var doc struct {
		Parameters []struct {
			Key   string `xml:"key,attr"`
			Value string `xml:"value,attr"`
		} `xml:"parameter"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return ""
	}
	for _, p := range doc.Parameters {
		if strings.EqualFold(p.Key, key) {
			return p.Value
		}
	}
	return ""
}

// AMDPBreakpoint is one breakpoint in an AMDP method.
type AMDPBreakpoint struct {
	// ClientID is ours to choose; SAP echoes it back so a client can recognise
	// its own breakpoints.
	ClientID string
	// URI is an ordinary adtcore object reference with a line fragment, the
	// same shape used everywhere else in ADT:
	// /sap/bc/adt/oo/classes/zcl_x/source/main#start=41
	URI string
	// Name and Type describe the object the URI points at.
	Name, Type string
}

// AMDP sync modes. The resource class defines exactly two and rejects anything
// else with INVALID SYNCMODE — in the exception's subType, while its message
// stays a useless "An exception was raised".
const (
	AMDPSyncFull    = "FULL"
	AMDPSyncProgram = "PROGRAM"
)

const amdpBreakpointMediaType = "application/vnd.sap.adt.amdp.dbg.bpsync.v1+xml"

// AMDPSyncBreakpoints replaces the session's breakpoints with this set.
func (d *Debugger) AMDPSyncBreakpoints(ctx context.Context, mode string, bps []AMDPBreakpoint) error {
	if d.amdpMain == "" {
		return fmt.Errorf("no AMDP debug session on this connection; start one first")
	}
	if mode == "" {
		mode = AMDPSyncFull
	}

	body := amdpBreakpointDocument(mode, bps)
	res, err := d.ADT(ctx, "POST",
		"/sap/bc/adt/amdp/debugger/main/"+url.PathEscape(d.amdpMain)+"/breakpoints",
		[]ADTHeader{
			{Name: "Accept", Value: acceptAnything},
			{Name: "Content-Type", Value: amdpBreakpointMediaType},
		}, body)
	if err != nil {
		return err
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("amdp breakpoints", res)
	}
	return nil
}

// amdpBreakpointDocument builds the sync request.
//
// The shape is not guessed. CL_AMDP_DBG_ADT_RES_BPS names its transformation,
// amdp_dbg_adt_sync_bp_req, and transformations are readable over ADT at
// /sap/bc/adt/xslt/transformations/{name}/source/main — the template states
// every element and attribute. Reading it took one request; guessing at it took
// several rounds and got the namespace wrong, because the position is a plain
// adtcore reference rather than anything AMDP-specific.
func amdpBreakpointDocument(mode string, bps []AMDPBreakpoint) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<amdpdbg:breakpointsSyncRequest xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger"` +
		` xmlns:adtcore="http://www.sap.com/adt/core" amdpdbg:syncMode="` + mode + `">` +
		`<amdpdbg:breakpoints>`)
	for _, bp := range bps {
		b.WriteString(`<amdpdbg:breakpoint amdpdbg:clientId="` + xmlAttr(bp.ClientID) + `"`)
		if bp.URI != "" {
			b.WriteString(` adtcore:uri="` + xmlAttr(bp.URI) + `"`)
		}
		if bp.Name != "" {
			b.WriteString(` adtcore:name="` + xmlAttr(bp.Name) + `"`)
		}
		if bp.Type != "" {
			b.WriteString(` adtcore:type="` + xmlAttr(bp.Type) + `"`)
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</amdpdbg:breakpoints></amdpdbg:breakpointsSyncRequest>`)
	return []byte(b.String())
}

// xmlAttr escapes a value for an XML attribute. Object names come from a dump,
// a stack or an argument rather than from us.
func xmlAttr(s string) string {
	r := strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// AMDPEventKind names what one resume answer is about.
const (
	// AMDPEventSyncBreakpoints acknowledges a breakpoint sync. It is queued
	// like everything else, so the first resume after setting breakpoints
	// answers with this and not with a stop — which reads exactly like "the
	// breakpoint did not fire", and is the trap this API sets. Keep asking.
	AMDPEventSyncBreakpoints = "SYNC_BREAKPOINTS"
	// AMDPEventToggleBreakpoints is SAP reporting what it made of the
	// breakpoints — each with a state and, when it refused one, a reason. It is
	// the most useful answer in the whole API and still not a stop: a
	// breakpoint reported VALID has been accepted, not reached.
	AMDPEventToggleBreakpoints = "ON_TOGGLE_BREAKPOINTS"
)

// amdpAcknowledgements are answers about the session rather than about a
// stopped program. Treating either as a stop is how a client concludes that a
// breakpoint never fired while the debuggee is blocked on it.
var amdpAcknowledgements = map[string]bool{
	AMDPEventSyncBreakpoints:   true,
	AMDPEventToggleBreakpoints: true,
}

// AMDPEventKindOf reports what an answer was about, and which debuggee it
// concerns. An empty debuggee id means the event is about the session rather
// than about a stopped program.
func AMDPEventKindOf(body []byte) (kind, debuggee string) {
	var doc struct {
		Responses []struct {
			Kind     string `xml:"kind,attr"`
			Debuggee string `xml:"debuggeeId,attr"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil || len(doc.Responses) == 0 {
		return "", ""
	}
	last := doc.Responses[len(doc.Responses)-1]
	return last.Kind, last.Debuggee
}

// AMDPAwaitStop keeps resuming until something actually stops, or until the
// budget of answers runs out.
//
// The single resume below is not enough on its own: the responses arrive as a
// queue, and the acknowledgement of the breakpoint sync sits at its head. A
// client that resumes once, sees SYNC_BREAKPOINTS and stops looking concludes
// the breakpoint never fired — while the debuggee is, at that moment, blocked
// on it. That is worth spelling out, because it is the shape of the conclusion
// this project drew for months.
func (d *Debugger) AMDPAwaitStop(ctx context.Context, maxEvents int) (*ADTResponse, error) {
	if maxEvents <= 0 {
		maxEvents = 10
	}
	for i := 0; i < maxEvents; i++ {
		res, err := d.AMDPResume(ctx)
		if err != nil {
			return res, err
		}
		kind, debuggee := AMDPEventKindOf(res.Body)
		if debuggee != "" || (kind != "" && !amdpAcknowledgements[kind]) {
			return res, nil
		}
		if kind == AMDPEventToggleBreakpoints {
			// Worth showing even though the wait continues: it is where SAP
			// says whether the position was understood.
			if state, reason := amdpBreakpointState(res.Body); state != "" {
				d.amdpLastBreakpointState = state
				d.amdpLastBreakpointError = reason
			}
		}
		if d.AMDPOnAck != nil {
			d.AMDPOnAck(kind, d.amdpLastBreakpointState, d.amdpLastBreakpointError)
		}
	}
	return nil, fmt.Errorf("nothing stopped within %d answers; the debuggee may not have run", maxEvents)
}

// AMDPResume asks for the next answer from the session's queue.
//
// It is not a listener: the handler raises when its response is initial, so
// this is only meaningful once something is running under the debugger. And it
// checks the id — unlike the breakpoint resource, which uses its own and will
// happily answer 200 for a main id that does not exist.
func (d *Debugger) AMDPResume(ctx context.Context) (*ADTResponse, error) {
	if d.amdpMain == "" {
		return nil, fmt.Errorf("no AMDP debug session on this connection; start one first")
	}
	res, err := d.ADT(ctx, "GET", "/sap/bc/adt/amdp/debugger/main/"+url.PathEscape(d.amdpMain),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("amdp resume", res)
	}
	return res, nil
}

// AMDPTerminate ends the session. hardStop does not wait for the debuggee.
func (d *Debugger) AMDPTerminate(ctx context.Context, hardStop bool) error {
	if d.amdpMain == "" {
		return nil
	}
	uri := "/sap/bc/adt/amdp/debugger/main/" + url.PathEscape(d.amdpMain)
	if hardStop {
		uri += "?hardStop=true"
	}
	res, err := d.ADT(ctx, "DELETE", uri, []ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	d.amdpMain = ""
	if err != nil {
		return err
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("amdp terminate", res)
	}
	return nil
}

// amdpBreakpointState reads what SAP made of the first breakpoint it reported.
func amdpBreakpointState(body []byte) (state, reason string) {
	var doc struct {
		Responses []struct {
			Value struct {
				Toggle struct {
					// The list is wrapped: onToggleBreakpoints > breakpoints >
					// breakpoint. Skipping the plural level silently finds
					// nothing, which reads as "SAP said nothing about the
					// breakpoint" rather than as a parser bug.
					Breakpoints []struct {
						State  string `xml:"state,attr"`
						ErrMsg string `xml:"errorMessage,attr"`
					} `xml:"breakpoints>breakpoint"`
				} `xml:"onToggleBreakpoints"`
			} `xml:"value"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return "", ""
	}
	for _, r := range doc.Responses {
		for _, bp := range r.Value.Toggle.Breakpoints {
			if bp.State != "" {
				return bp.State, bp.ErrMsg
			}
		}
	}
	return "", ""
}

// AMDPBreakpointState reports what SAP last said about the breakpoints it was
// given: VALID, or a state with a reason attached.
func (d *Debugger) AMDPBreakpointState() (state, reason string) {
	return d.amdpLastBreakpointState, d.amdpLastBreakpointError
}

// AMDPStep moves the stopped debuggee one step.
//
// Only two kinds exist on this resource — "over" and "continue" — which is
// less than the ABAP debugger offers and is worth saying rather than
// discovering: SQLScript has no "into" because there is nothing below the
// statement to step into.
func (d *Debugger) AMDPStep(ctx context.Context, debuggeeID, kind string) (*ADTResponse, error) {
	if d.amdpMain == "" {
		return nil, fmt.Errorf("no AMDP debug session on this connection; start one first")
	}
	if strings.TrimSpace(debuggeeID) == "" {
		return nil, fmt.Errorf("no debuggee: nothing has stopped yet")
	}
	switch kind {
	case "over", "continue":
	default:
		return nil, fmt.Errorf("AMDP steps are over or continue; %q is neither", kind)
	}

	uri := fmt.Sprintf("/sap/bc/adt/amdp/debugger/main/%s/debuggees/%s?step=%s",
		url.PathEscape(d.amdpMain), url.PathEscape(debuggeeID), kind)
	res, err := d.ADT(ctx, "POST", uri, []ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return res, adtError("amdp step "+kind, res)
	}
	return res, nil
}

// amdpVariableWindow is how much of a value one read asks for.
//
// A SQLScript variable can be an NVARCHAR of considerable size, and the
// resource pages rather than truncating, so some window has to be named. This
// one is large enough that a scalar never needs a second read and small enough
// that a mistake costs a request rather than a session.
const amdpVariableWindow = 8192

// AMDPVariable reads one variable of the stopped SQLScript.
//
// offset and length are shown as optional in the discovery template —
// variables/{varname}{?offset,length} — and are not: the server answers 400
// "Parameter offset could not be found." without them. That cost a run to find,
// and it is the third time today that a template's optional-looking parameter
// turned out to be required. Read the braces as documentation of the parameter
// names, not of what may be left out.
func (d *Debugger) AMDPVariable(ctx context.Context, debuggeeID, name string) (*ADTResponse, error) {
	if d.amdpMain == "" {
		return nil, fmt.Errorf("no AMDP debug session on this connection; start one first")
	}
	if strings.TrimSpace(debuggeeID) == "" {
		return nil, fmt.Errorf("no debuggee: nothing has stopped yet")
	}
	uri := fmt.Sprintf("/sap/bc/adt/amdp/debugger/main/%s/debuggees/%s/variables/%s?offset=0&length=%d",
		url.PathEscape(d.amdpMain), url.PathEscape(debuggeeID), url.PathEscape(name), amdpVariableWindow)
	res, err := d.ADT(ctx, "GET", uri, []ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return res, adtError("amdp variable "+name, res)
	}
	return res, nil
}

// AMDPReadVariable asks for a variable's value and waits for it.
//
// The read is asynchronous, which the empty answer does not advertise.
// CL_AMDP_DBG_ADT_RES_VARS calls get_scalar_values and puts the resulting
// request id in the Location header, leaving the body empty — so a caller that
// reads the body sees nothing and concludes the variable has no value, or does
// not exist. It is the queue again, one level further down: every operation
// here is a command, and its result arrives through the main response list
// keyed by that request id.
func (d *Debugger) AMDPReadVariable(ctx context.Context, debuggeeID, name string, maxEvents int) (*ADTResponse, error) {
	asked, err := d.AMDPVariable(ctx, debuggeeID, name)
	if err != nil {
		return nil, err
	}
	requestID := strings.TrimSpace(asked.Header("location"))
	if requestID == "" {
		return nil, fmt.Errorf("reading %s was accepted without a request id, so there is nothing to wait for", name)
	}
	return d.AMDPAwaitRequest(ctx, requestID, maxEvents)
}

// AMDPAwaitRequest resumes until the answer to one request comes back.
//
// Answers to other requests, and the acknowledgements that always lead, are
// walked past rather than returned: a caller waiting for a variable must not be
// handed the queue's next item and told it is the value.
func (d *Debugger) AMDPAwaitRequest(ctx context.Context, requestID string, maxEvents int) (*ADTResponse, error) {
	if maxEvents <= 0 {
		maxEvents = 12
	}
	for i := 0; i < maxEvents; i++ {
		res, err := d.AMDPResume(ctx)
		if err != nil {
			return res, err
		}
		if amdpAnswers(res.Body, requestID) {
			return res, nil
		}
	}
	return nil, fmt.Errorf("no answer to request %s within %d events", requestID, maxEvents)
}

// amdpAnswers reports whether one of the responses carries this request id.
func amdpAnswers(body []byte, requestID string) bool {
	var doc struct {
		Responses []struct {
			RequestID string `xml:"requestId,attr"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return false
	}
	for _, r := range doc.Responses {
		if strings.TrimSpace(r.RequestID) == requestID {
			return true
		}
	}
	return false
}

// AMDPPosition is where a stopped debuggee is.
type AMDPPosition struct {
	DebuggeeID string
	Procedure  string
	URI        string
	Line       int
}

// AMDPStopPosition reads the position out of an ON_BREAK answer.
func AMDPStopPosition(body []byte) *AMDPPosition {
	var doc struct {
		Responses []struct {
			Kind     string `xml:"kind,attr"`
			Debuggee string `xml:"debuggeeId,attr"`
			Value    struct {
				Position struct {
					Procedure string `xml:"procedureName,attr"`
					URI       string `xml:"uri,attr"`
				} `xml:"abapPosition"`
			} `xml:"value"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	for _, r := range doc.Responses {
		if r.Value.Position.Procedure == "" && r.Debuggee == "" {
			continue
		}
		pos := &AMDPPosition{
			DebuggeeID: r.Debuggee,
			Procedure:  r.Value.Position.Procedure,
			URI:        r.Value.Position.URI,
		}
		// The line rides in the URI fragment, the way every ADT position does.
		if i := strings.LastIndex(pos.URI, "#start="); i >= 0 {
			fmt.Sscanf(pos.URI[i+len("#start="):], "%d", &pos.Line)
		}
		return pos
	}
	return nil
}

// AMDPStepAndWait steps and returns where the debuggee stopped next.
//
// The step itself answers with nothing: it is a command, and the new position
// arrives through the same response queue as everything else. A caller that
// steps and reads the step's own answer sees an empty body and concludes the
// step did nothing — the same trap the breakpoint set, one level down. So the
// wait belongs here rather than in every caller.
func (d *Debugger) AMDPStepAndWait(ctx context.Context, debuggeeID, kind string, maxEvents int) (*AMDPPosition, error) {
	if _, err := d.AMDPStep(ctx, debuggeeID, kind); err != nil {
		return nil, err
	}
	res, err := d.AMDPAwaitStop(ctx, maxEvents)
	if err != nil {
		return nil, err
	}
	pos := AMDPStopPosition(res.Body)
	if pos == nil {
		return nil, fmt.Errorf("the debuggee answered the step without saying where it stopped")
	}
	return pos, nil
}

// AMDPTrace walks the stopped debuggee and reports each line it stops on.
//
// This is the AMDP counterpart of the ABAP recorder: a statement-level trace of
// SQLScript running inside HANA, taken over plain ADT with nothing installed.
// It ends when the program runs out, when the budget is spent, or when a step
// stops saying where it went.
func (d *Debugger) AMDPTrace(ctx context.Context, debuggeeID string, maxSteps int, emit func(AMDPPosition) error) (int, error) {
	if maxSteps <= 0 {
		maxSteps = 100
	}
	stops := 0
	for i := 0; i < maxSteps; i++ {
		pos, err := d.AMDPStepAndWait(ctx, debuggeeID, "over", 6)
		if err != nil {
			// A trace that ends early is still a trace; the caller is told how
			// far it got rather than losing the stops already collected.
			return stops, err
		}
		if pos.DebuggeeID != "" {
			debuggeeID = pos.DebuggeeID
		}
		stops++
		if emit != nil {
			if err := emit(*pos); err != nil {
				return stops, err
			}
		}
	}
	return stops, nil
}

// AMDPScalar is one variable of a stopped SQLScript, as SAP reports it.
type AMDPScalar struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Value is what the window asked for. OriginalLength says how long the
	// whole thing is, so a caller can tell a complete value from a first page —
	// without it, a truncated string reads as the value itself.
	Value          string `json:"value"`
	Length         int    `json:"length"`
	OriginalLength int    `json:"originalLength"`
	IsNull         bool   `json:"isNull"`
}

// Truncated reports whether more of the value exists than was returned.
func (s AMDPScalar) Truncated() bool {
	return s.OriginalLength > s.Length
}

// AMDPScalarValues reads the variables out of a GET_SCALAR_VALUES answer.
func AMDPScalarValues(body []byte) []AMDPScalar {
	var doc struct {
		Responses []struct {
			Value struct {
				Scalars []struct {
					Name           string `xml:"name,attr"`
					Type           string `xml:"type,attr"`
					IsNull         string `xml:"isNullValue,attr"`
					Length         int    `xml:"length,attr"`
					OriginalLength int    `xml:"originalLength,attr"`
					Text           string `xml:",chardata"`
				} `xml:"scalarValues>scalarValue"`
			} `xml:"value"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	var out []AMDPScalar
	for _, r := range doc.Responses {
		for _, sc := range r.Value.Scalars {
			out = append(out, AMDPScalar{
				Name:           sc.Name,
				Type:           sc.Type,
				Value:          sc.Text,
				Length:         sc.Length,
				OriginalLength: sc.OriginalLength,
				IsNull:         strings.EqualFold(strings.TrimSpace(sc.IsNull), "true"),
			})
		}
	}
	return out
}

// FormatAMDPScalar renders one variable for a person.
func FormatAMDPScalar(s AMDPScalar) string {
	if s.IsNull {
		return fmt.Sprintf("%-24s %-12s NULL", s.Name, s.Type)
	}
	line := fmt.Sprintf("%-24s %-12s %s", s.Name, s.Type, s.Value)
	if s.Truncated() {
		// Said outright, because a value cut at the window boundary is
		// indistinguishable from a short one otherwise.
		line += fmt.Sprintf("  … %d of %d characters", s.Length, s.OriginalLength)
	}
	return line
}

// AMDPTableRows reads a table-valued variable of the stopped SQLScript.
//
// **This does not work yet.** It is committed because what is established is
// worth more than the guessing it replaces, and because the remaining question
// is narrow and named.
//
// Tables do not come back through the debugger at all: they go through data
// preview, at /sap/bc/adt/datapreview/amdpdebugger, a separate resource that
// knows nothing about our session and therefore needs the whole address.
//
// Established by reading rather than probing:
//
//   - The handler is CL_ADT_DP_DBG_AMDP_RES, registered for that path in
//     CL_ADT_DATAPREVIEW_RES_APP. The class next door,
//     CL_ADT_AMDP_DATAPREVIEW_RES, serves /datapreview/amdp — a different
//     relation that takes uri and maxRows, and reading it first sent me the
//     wrong way for a while.
//   - The parameter names below are the constants of
//     if_adt_dp_dbg_amdp_res_co, not a reading of the discovery template, so
//     they are right.
//   - It is a GET. A POST answers "Content type missing", because the
//     transport sets no content type without a body.
//
// What fails is the values. The server answers 400 with
//
//	Debugger operation "INIT" failed with error code "3" ("internal error")
//
// and initialize_data_provider builds the provider from debuggerId and
// sessionId alone — so one of those two is not what it wants. Note the shapes:
// the debuggee id is the HANA session id plus two more segments
// (host:port:session:context:n), so "session" may mean something narrower or
// wider than the id the start call returns.
//
// The factory has since been read: lif_provider_factory hands debuggerId and
// sessionId to cl_amdp_dbg_data_preview, which for HDB ends in
// db_req_init_access — an `init` command sent to the SQLScript debugger on the
// HANA side with connection_type "general". So the refusal is HANA's, not
// ADT's, and one of those two ids is not what HANA knows.
//
// Also tried and not the answer: passing `schema`, which the stop does hand
// over (nativePosition/schemaName, see AMDPCallStack). Same INIT failure with
// it as without.
//
// What is left untried is the tableHandle the stop reports for the variable —
// it appears in no parameter of this resource, so it may belong to a different
// route entirely rather than to this one.
func (d *Debugger) AMDPTableRows(ctx context.Context, session *AMDPSession, debuggeeID, name, schema string, rows int) (*ADTResponse, error) {
	if session == nil || session.MainID == "" {
		return nil, fmt.Errorf("no AMDP debug session on this connection; start one first")
	}
	if strings.TrimSpace(debuggeeID) == "" {
		return nil, fmt.Errorf("no debuggee: nothing has stopped yet")
	}
	if rows <= 0 {
		rows = 50
	}

	q := url.Values{}
	q.Set("sessionId", session.HANASessionID)
	q.Set("debuggerId", session.MainID)
	q.Set("debuggeeId", debuggeeID)
	q.Set("variableName", strings.ToUpper(strings.TrimSpace(name)))
	q.Set("rowNumber", strconv.Itoa(rows))
	q.Set("colNumber", "100")
	if schema != "" {
		q.Set("schema", schema)
	}

	res, err := d.ADT(ctx, "GET", "/sap/bc/adt/datapreview/amdpdebugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return res, adtError("amdp table "+name, res)
	}
	return res, nil
}

// AMDPVariableInfo is one variable as the stop event describes it.
//
// The stop already carries every variable in scope — name, type, scope,
// nullness and, for a table, its handle. Nothing had to be asked for. That is
// worth knowing before reaching for the variable resource: reading one at a
// time is for values that changed since the stop, not for finding out what is
// there.
type AMDPVariableInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Scope is system, input, output or local. The system ones are HANA's own
	// (::ROWCOUNT and friends) and are usually noise.
	Scope  string `json:"scope"`
	IsNull bool   `json:"isNull"`
	// TableHandle is non-zero for a table-valued variable and zero for a
	// scalar, which is how the two are told apart.
	TableHandle string `json:"tableHandle,omitempty"`
	TableLength int    `json:"tableLength,omitempty"`
	IsTrimmed   bool   `json:"isTrimmed,omitempty"`
}

// IsTable reports whether this variable holds a table rather than a scalar.
func (v AMDPVariableInfo) IsTable() bool {
	return v.TableHandle != "" && v.TableHandle != "0"
}

// AMDPVariablesAtStop reads the variables a stop event carries.
func AMDPVariablesAtStop(body []byte) []AMDPVariableInfo {
	var doc struct {
		Responses []struct {
			Value struct {
				Variables []struct {
					Name        string `xml:"name,attr"`
					Type        string `xml:"type,attr"`
					Scope       string `xml:"scope,attr"`
					IsNull      string `xml:"isNullValue,attr"`
					TableHandle string `xml:"tableHandle,attr"`
					TableLength int    `xml:"tableLength,attr"`
					IsTrimmed   string `xml:"isTrimmed,attr"`
				} `xml:"variables>variable"`
			} `xml:"value"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	var out []AMDPVariableInfo
	for _, r := range doc.Responses {
		for _, v := range r.Value.Variables {
			out = append(out, AMDPVariableInfo{
				Name:        v.Name,
				Type:        v.Type,
				Scope:       v.Scope,
				IsNull:      strings.EqualFold(strings.TrimSpace(v.IsNull), "true"),
				TableHandle: strings.TrimSpace(v.TableHandle),
				TableLength: v.TableLength,
				IsTrimmed:   strings.EqualFold(strings.TrimSpace(v.IsTrimmed), "true"),
			})
		}
	}
	return out
}

// FormatAMDPVariableInfo renders one variable of a stopped procedure.
func FormatAMDPVariableInfo(v AMDPVariableInfo) string {
	what := v.Type
	if v.IsTable() {
		// The row count is the useful part of a table here, and the handle is
		// what any deeper read will need.
		what = fmt.Sprintf("table[%d] handle %s", v.TableLength, v.TableHandle)
	} else if v.IsNull {
		what += " = NULL"
	}
	return fmt.Sprintf("%-8s %-26s %s", v.Scope, v.Name, what)
}

// AMDPFrame is one entry of a stopped procedure's call stack.
//
// It carries two positions for the same statement, and both are worth having:
// the ABAP one names the line in the class as a person wrote it, and the native
// one names the line in the SQLScript procedure HANA actually generated. They
// differ — line 41 of a class was line 19 of its procedure — so a caller
// looking at HANA's own tooling and a caller looking at the source need
// different numbers for the same stop.
type AMDPFrame struct {
	Index int    `json:"index"`
	Kind  string `json:"kind,omitempty"`
	// Procedure is the ABAP-facing name, CLASS=>METHOD.
	Procedure string `json:"procedure"`
	URI       string `json:"uri,omitempty"`
	Line      int    `json:"line"`
	// NativeLine is the line in the generated procedure, and Schema is where
	// that procedure lives. The schema is not decoration: the data preview
	// resource asks for it by name.
	NativeLine int    `json:"nativeLine,omitempty"`
	Schema     string `json:"schema,omitempty"`
	// DebugCompiled is false when the procedure was built without debug
	// information, which is why a breakpoint in it would never be reached.
	DebugCompiled bool `json:"debugCompiled"`
}

// AMDPCallStack reads the call stack a stop event carries.
func AMDPCallStack(body []byte) []AMDPFrame {
	var doc struct {
		Responses []struct {
			Value struct {
				Entries []struct {
					Index         int    `xml:"index,attr"`
					Language      string `xml:"language,attr"`
					Type          string `xml:"type,attr"`
					DebugCompiled string `xml:"isDebugCompiled,attr"`
					Abap          struct {
						Procedure string `xml:"procedureName,attr"`
						URI       string `xml:"uri,attr"`
					} `xml:"abapPosition"`
					Native struct {
						Procedure string `xml:"procedureName,attr"`
						Schema    string `xml:"schemaName,attr"`
						Line      int    `xml:"line,attr"`
					} `xml:"nativePosition"`
				} `xml:"callstack>callstackEntry"`
			} `xml:"value"`
		} `xml:"mainResponse"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	var out []AMDPFrame
	for _, r := range doc.Responses {
		for _, e := range r.Value.Entries {
			f := AMDPFrame{
				Index:         e.Index,
				Kind:          e.Type,
				Procedure:     e.Abap.Procedure,
				URI:           e.Abap.URI,
				NativeLine:    e.Native.Line,
				Schema:        e.Native.Schema,
				DebugCompiled: strings.EqualFold(strings.TrimSpace(e.DebugCompiled), "true"),
			}
			if i := strings.LastIndex(f.URI, "#start="); i >= 0 {
				fmt.Sscanf(f.URI[i+len("#start="):], "%d", &f.Line)
			}
			out = append(out, f)
		}
	}
	return out
}

// FormatAMDPFrame renders one stack entry.
func FormatAMDPFrame(f AMDPFrame) string {
	line := fmt.Sprintf("%3d %-44s :%d", f.Index, f.Procedure, f.Line)
	if f.NativeLine > 0 {
		line += fmt.Sprintf("   native %s:%d", orDash(f.Schema), f.NativeLine)
	}
	if !f.DebugCompiled {
		// Worth saying outright: a frame built without debug information is one
		// where a breakpoint would never be reached, and that looks exactly
		// like a breakpoint that does not work.
		line += "   (not debug-compiled)"
	}
	return line
}

// AMDPSchemaAtStop reports the schema of the stopped procedure, which the data
// preview resource asks for by name.
func AMDPSchemaAtStop(body []byte) string {
	for _, f := range AMDPCallStack(body) {
		if f.Schema != "" {
			return f.Schema
		}
	}
	return ""
}

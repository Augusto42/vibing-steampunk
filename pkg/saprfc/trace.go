package saprfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SAT runtime traces through ADT: the measured call tree.
//
// A static call graph is a hypothesis. ABAP resolves CALL FUNCTION lv_name,
// PERFORM (f), CALL METHOD (m) and every RFC destination at runtime, so the only
// way to know what a program actually called is to watch it. SAT does the
// watching and has done for decades; ADT exposes it as REST, and REST goes
// through either transport, so this needs nothing installed.
//
// The flow has three steps and one trap:
//
//  1. POST /parameters — what to record. Skipping this is not an option if you
//     want a tree: with no parameters id the request handler hardcodes full
//     aggregation, and an aggregated trace has a hit list but no call hierarchy.
//  2. POST /requests — who to record: user, client, process type, and the object
//     whose execution arms it. The request is consumed by the first session that
//     matches, up to maximalExecutions.
//  3. Run the workload from somewhere else, then read the trace file.
//
// The trap is step 2 aimed too widely. A request for "any object, any process
// type, this user" is armed for *this tool's own session* as well, and since vsp
// talks to the same system it will usually match first — the first trace taken
// here recorded vsp reading the trace request it had just created. Naming the
// object is what makes the aim reliable.

// traceBase is the collection every trace resource hangs off.
const traceBase = "/sap/bc/adt/runtime/traces/abaptraces"

// Tracer reads and arms SAT traces over an ADT transport.
type Tracer struct {
	adt    ADTTransport
	user   string
	client string
}

// NewTracer binds a tracer to a transport — a pinned RFC conversation, a pooled
// one, or a stateful HTTPS session. Nothing here needs a session: every call is
// self-contained, which is why the pooled client is a legitimate choice.
func NewTracer(transport ADTTransport, user, client string) *Tracer {
	return &Tracer{adt: transport, user: strings.ToUpper(strings.TrimSpace(user)), client: client}
}

// TraceObjectType and TraceProcessType are the enumerations SAP serves under
// /objecttypes and /processtypes. They travel as full resource paths.
type TraceObjectType string

const (
	TraceObjectAny      TraceObjectType = "any"
	TraceObjectReport   TraceObjectType = "report"
	TraceObjectFunction TraceObjectType = "functionmodule"
	TraceObjectTransact TraceObjectType = "transaction"
	TraceObjectURL      TraceObjectType = "url"
)

type TraceProcessType string

const (
	TraceProcessAny    TraceProcessType = "any"
	TraceProcessDialog TraceProcessType = "dialog"
	TraceProcessBatch  TraceProcessType = "batch"
	TraceProcessRFC    TraceProcessType = "rfc"
	TraceProcessHTTP   TraceProcessType = "http"
)

// TraceFile is one recorded trace.
type TraceFile struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	User       string    `json:"user"`
	Client     string    `json:"client"`
	ObjectName string    `json:"objectName,omitempty"`
	Host       string    `json:"host,omitempty"`
	State      string    `json:"state,omitempty"`
	Size       int       `json:"size"`
	Runtime    int       `json:"runtimeMicros"`
	RuntimeABA int       `json:"runtimeAbapMicros"`
	RuntimeSys int       `json:"runtimeSystemMicros"`
	RuntimeDB  int       `json:"runtimeDatabaseMicros"`
	Aggregated bool      `json:"aggregated"`
	Published  time.Time `json:"published,omitempty"`
}

// TraceStatement is one node of the measured tree. CallLevel is the depth, so a
// stream of these in order *is* the tree; nothing needs to be reconstructed.
type TraceStatement struct {
	ID        int    `json:"id"`
	Index     int    `json:"index"`
	CallLevel int    `json:"callLevel"`
	Kind      string `json:"kind"`             // "Call Function", "Select Single", "PERFORM (ext)" …
	Target    string `json:"target,omitempty"` // what it called — the name a static graph cannot know
	Caller    string `json:"caller,omitempty"`
	Callee    string `json:"callee,omitempty"`
	GrossTime int    `json:"grossMicros"`
	NetTime   int    `json:"netMicros"`
	Package   string `json:"package,omitempty"`
	Component string `json:"component,omitempty"`
	Procedure bool   `json:"procedureLike"`
}

// IsCall reports whether this statement handed control to another code unit —
// the statements that make up the call graph, as opposed to the database and
// housekeeping events recorded alongside them.
func (s TraceStatement) IsCall() bool {
	k := strings.ToUpper(s.Kind)
	return strings.HasPrefix(k, "CALL ") || strings.HasPrefix(k, "PERFORM") ||
		strings.HasPrefix(k, "SUBMIT") || strings.HasPrefix(k, "CREATE ABAP OBJECT")
}

// TraceParams is what to record. The zero value is not what anyone wants: use
// CallTreeParams, which switches aggregation off — that is the whole point.
type TraceParams struct {
	Description        string
	Aggregate          bool
	ProceduralUnits    bool
	DBEvents           bool
	InternalTables     bool
	DynproEvents       bool
	SystemKernelEvents bool
	MiscStatements     bool
	MemoryConsumption  bool
	RFCTracing         bool
	MaxSizeMB          int
	MaxMinutes         int
}

// CallTreeParams records the call hierarchy and the database work under it, and
// nothing else. Internal-table and kernel events multiply the volume without
// telling you who called whom.
func CallTreeParams(description string) TraceParams {
	return TraceParams{
		Description:     description,
		Aggregate:       false,
		ProceduralUnits: true,
		DBEvents:        true,
		RFCTracing:      true,
		MaxSizeMB:       30,
		MaxMinutes:      30,
	}
}

// TraceRequest arms the recording.
type TraceRequest struct {
	Description  string
	ObjectType   TraceObjectType
	ObjectName   string
	ProcessType  TraceProcessType
	User         string
	Client       string
	Server       string // "" or "null" for this server, "*" for all
	MaxRuns      int
	Expires      time.Time
	ParametersID string // from NewParameters; without it SAP forces aggregation
}

// Traces lists the trace files this system holds.
func (t *Tracer) Traces(ctx context.Context) ([]TraceFile, error) {
	res, err := t.get(ctx, traceBase, "application/atom+xml;type=feed")
	if err != nil {
		return nil, err
	}
	return parseTraceFeed(res.Body)
}

// Tree reads a trace's statements: the measured call tree, in order.
func (t *Tracer) Tree(ctx context.Context, traceID string) ([]TraceStatement, error) {
	// id=1 is the root; the drill-down threshold is SAP's own default and only
	// affects which subtrees the UI pre-expands, not what is returned.
	uri := fmt.Sprintf("%s/%s/statements?id=1&withDetails=false&autoDrillDownThreshold=80",
		traceBase, url.PathEscape(traceID))
	res, err := t.get(ctx, uri, "application/xml")
	if err != nil {
		return nil, err
	}
	return parseTraceStatements(res.Body)
}

// NewParameters registers a parameter set and returns its id. The set lives in
// shared memory with an expiry of its own, so arm the request soon after.
func (t *Tracer) NewParameters(ctx context.Context, p TraceParams) (string, error) {
	body := renderTraceParams(p)
	res, err := t.adt.Do(ctx, ADTRequest{
		Method: "POST", URI: traceBase + "/parameters",
		Headers: []ADTHeader{{Name: "Content-Type", Value: "application/xml"},
			{Name: "Accept", Value: acceptAnything}},
		Body: []byte(body),
	})
	if err != nil {
		return "", err
	}
	if res.Status < 200 || res.Status >= 300 {
		return "", adtError("trace parameters", res)
	}
	id := res.Header("location")
	if id == "" {
		return "", fmt.Errorf("SAP accepted the trace parameters but returned no Location")
	}
	return id, nil
}

// Arm creates a trace request and returns its id.
func (t *Tracer) Arm(ctx context.Context, r TraceRequest) (string, error) {
	if r.ObjectType == "" {
		r.ObjectType = TraceObjectAny
	}
	if r.ProcessType == "" {
		r.ProcessType = TraceProcessAny
	}
	if r.MaxRuns <= 0 {
		r.MaxRuns = 1
	}
	if r.Expires.IsZero() {
		r.Expires = time.Now().UTC().Add(24 * time.Hour)
	}
	user := r.User
	if user == "" {
		user = t.user
	}
	client := r.Client
	if client == "" {
		client = t.client
	}

	q := url.Values{}
	q.Set("description", orDash(r.Description))
	q.Set("objectType", traceBase+"/objecttypes/"+string(r.ObjectType))
	q.Set("objectName", strings.ToUpper(r.ObjectName))
	q.Set("processType", traceBase+"/processtypes/"+string(r.ProcessType))
	q.Set("traceUser", strings.ToUpper(user))
	q.Set("traceClient", client)
	q.Set("server", orNull(r.Server))
	q.Set("maximalExecutions", strconv.Itoa(r.MaxRuns))
	q.Set("expires", r.Expires.UTC().Format("2006-01-02T15:04:05Z"))
	if r.ParametersID != "" {
		q.Set("parametersId", r.ParametersID)
	}

	res, err := t.adt.Do(ctx, ADTRequest{
		Method: "POST", URI: traceBase + "/requests?" + q.Encode(),
		Headers: []ADTHeader{{Name: "Accept", Value: "application/atom+xml;type=feed"}},
	})
	if err != nil {
		return "", err
	}
	if res.Status < 200 || res.Status >= 300 {
		return "", adtError("trace request", res)
	}
	ids := feedIDs(res.Body)
	if len(ids) == 0 {
		return "", fmt.Errorf("SAP accepted the trace request but named no id")
	}
	// The feed carries every request this user has; the new one is last.
	return ids[len(ids)-1], nil
}

// Requests lists the armed, not yet consumed trace requests.
func (t *Tracer) Requests(ctx context.Context) ([]string, error) {
	res, err := t.get(ctx, traceBase+"/requests", "application/atom+xml;type=feed")
	if err != nil {
		return nil, err
	}
	return feedIDs(res.Body), nil
}

// DeleteRequest disarms a trace request. An armed request that nobody consumes
// keeps waiting, and the next matching session pays for it.
func (t *Tracer) DeleteRequest(ctx context.Context, requestID string) error {
	return t.delete(ctx, absolute(requestID, traceBase+"/requests/"))
}

// DeleteTrace removes a trace file.
func (t *Tracer) DeleteTrace(ctx context.Context, traceID string) error {
	return t.delete(ctx, absolute(traceID, traceBase+"/"))
}

func (t *Tracer) get(ctx context.Context, uri, accept string) (*ADTResponse, error) {
	res, err := t.adt.Do(ctx, ADTRequest{Method: "GET", URI: uri,
		Headers: []ADTHeader{{Name: "Accept", Value: accept}}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, adtError("trace", res)
	}
	return res, nil
}

func (t *Tracer) delete(ctx context.Context, uri string) error {
	res, err := t.adt.Do(ctx, ADTRequest{Method: "DELETE", URI: uri})
	if err != nil {
		return err
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("trace delete", res)
	}
	return nil
}

// absolute accepts either a bare id or a full resource path.
func absolute(id, prefix string) string {
	if strings.HasPrefix(id, "/sap/bc/adt/") {
		return id
	}
	return prefix + url.PathEscape(id)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// orNull spells "this application server". SAP reads an empty server as
// unspecified and 'null' as local; '*' means every server in the system.
func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

// renderTraceParams writes the document ATRADT_PARAMS reads. Every element is
// optional and carries its value in a `value` attribute; SAP's own comment in
// the transformation is the only documentation of the names.
func renderTraceParams(p TraceParams) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<trc:parameters xmlns:trc="http://www.sap.com/adt/runtime/traces/abaptraces">` + "\n")
	el := func(name string, value string) {
		fmt.Fprintf(&sb, "  <trc:%s value=\"%s\"/>\n", name, value)
	}
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	el("aggregate", b(p.Aggregate))
	el("explicitOnOff", "false")
	el("withRfcTracing", b(p.RFCTracing))
	el("allProceduralUnits", b(p.ProceduralUnits))
	el("allDbEvents", b(p.DBEvents))
	el("allInternalTableEvents", b(p.InternalTables))
	el("allDynproEvents", b(p.DynproEvents))
	el("allSystemKernelEvents", b(p.SystemKernelEvents))
	el("allMiscAbapStatements", b(p.MiscStatements))
	el("withMemoryConsumption", b(p.MemoryConsumption))
	if p.MaxSizeMB > 0 {
		el("maxSizeForTraceFile", strconv.Itoa(p.MaxSizeMB))
	}
	if p.MaxMinutes > 0 {
		el("maxTimeForTracing", strconv.Itoa(p.MaxMinutes))
	}
	if p.Description != "" {
		el("description", xmlEsc(p.Description))
	}
	sb.WriteString(`</trc:parameters>`)
	return sb.String()
}

// --- parsing ---

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Extended struct {
		Host       string `xml:"host"`
		Size       int    `xml:"size"`
		Runtime    int    `xml:"runtime"`
		RuntimeABA int    `xml:"runtimeABAP"`
		RuntimeSys int    `xml:"runtimeSystem"`
		RuntimeDB  int    `xml:"runtimeDatabase"`
		System     string `xml:"system"`
		Client     string `xml:"client"`
		User       string `xml:"user"`
		Aggregated string `xml:"isAggregated"`
		ObjectName string `xml:"objectName"`
		State      struct {
			Text string `xml:"text,attr"`
		} `xml:"state"`
	} `xml:"extendedData"`
}

func parseTraceFeed(body []byte) ([]TraceFile, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("reading the trace feed: %w", err)
	}
	var out []TraceFile
	for _, e := range feed.Entries {
		f := TraceFile{
			ID:         lastSegment(e.ID),
			Title:      e.Title,
			User:       e.Extended.User,
			Client:     e.Extended.Client,
			ObjectName: strings.TrimSpace(e.Extended.ObjectName),
			Host:       e.Extended.Host,
			State:      e.Extended.State.Text,
			Size:       e.Extended.Size,
			Runtime:    e.Extended.Runtime,
			RuntimeABA: e.Extended.RuntimeABA,
			RuntimeSys: e.Extended.RuntimeSys,
			RuntimeDB:  e.Extended.RuntimeDB,
			Aggregated: e.Extended.Aggregated == "true",
		}
		if e.Author.Name != "" && f.User == "" {
			f.User = e.Author.Name
		}
		if ts, err := time.Parse(time.RFC3339, e.Published); err == nil {
			f.Published = ts
		}
		out = append(out, f)
	}
	return out, nil
}

type trcStatements struct {
	Statements []trcStatement `xml:"statement"`
}

type trcStatement struct {
	ID        int    `xml:"id,attr"`
	Index     int    `xml:"index,attr"`
	CallLevel int    `xml:"callLevel,attr"`
	Text      string `xml:"text,attr"`
	Variable  string `xml:"variable,attr"`
	Package   string `xml:"package,attr"`
	Component string `xml:"component,attr"`
	Procedure bool   `xml:"isProcedureLike,attr"`
	Calling   struct {
		Context string `xml:"context,attr"`
	} `xml:"callingProgram"`
	Called struct {
		Context string `xml:"context,attr"`
	} `xml:"calledProgram"`
	Gross struct {
		Time int `xml:"time,attr"`
	} `xml:"grossTime"`
	Net struct {
		Time int `xml:"time,attr"`
	} `xml:"proceduralNetTime"`
}

func parseTraceStatements(body []byte) ([]TraceStatement, error) {
	var doc trcStatements
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("reading the trace statements: %w", err)
	}
	out := make([]TraceStatement, 0, len(doc.Statements))
	for _, s := range doc.Statements {
		out = append(out, TraceStatement{
			ID: s.ID, Index: s.Index, CallLevel: s.CallLevel,
			Kind:      s.Text,
			Target:    s.Variable,
			Caller:    trimPool(s.Calling.Context),
			Callee:    trimPool(s.Called.Context),
			GrossTime: s.Gross.Time,
			NetTime:   s.Net.Time,
			Package:   s.Package,
			Component: s.Component,
			Procedure: s.Procedure,
		})
	}
	return out, nil
}

// trimPool strips the '=' padding SAP uses to fill a class pool name to 30
// characters. ZCL_X==========CP is ZCL_X to everyone but the kernel.
func trimPool(context string) string {
	if i := strings.Index(context, "="); i > 0 {
		return context[:i]
	}
	return context
}

func lastSegment(uri string) string {
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		return uri[i+1:]
	}
	return uri
}

// feedIDs pulls the atom:id values out of a feed without modelling the rest.
func feedIDs(body []byte) []string {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}
	var ids []string
	for _, e := range feed.Entries {
		ids = append(ids, e.ID)
	}
	return ids
}

// FormatTree renders the statements as the tree they already are.
func FormatTree(stmts []TraceStatement, callsOnly bool) string {
	var sb strings.Builder
	for _, s := range stmts {
		if callsOnly && !s.IsCall() && s.CallLevel > 1 {
			continue
		}
		fmt.Fprintf(&sb, "%s%s", strings.Repeat("  ", s.CallLevel), s.Kind)
		if s.Target != "" {
			fmt.Fprintf(&sb, " %s", s.Target)
		}
		if s.Callee != "" {
			fmt.Fprintf(&sb, " → %s", s.Callee)
		}
		fmt.Fprintf(&sb, "  %s\n", micros(s.GrossTime))
	}
	return sb.String()
}

func micros(us int) string {
	if us >= 1000 {
		return fmt.Sprintf("%.1fms", float64(us)/1000)
	}
	return strconv.Itoa(us) + "µs"
}

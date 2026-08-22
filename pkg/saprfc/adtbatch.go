package saprfc

import (
	"context"
	"fmt"
	"net/textproto"
	"strconv"
	"strings"
)

// Several ADT requests in one round trip.
//
// The debugger's cost is not computation, it is latency: a stop worth recording
// needs the step, the stack and the variables, and each one measured 13ms on
// either transport — so three quarters of a recording run is spent waiting.
// /sap/bc/adt/debugger/batch takes them as one multipart/mixed document and
// answers with one, which is what Eclipse does and what makes a capture mode
// affordable.
//
// Verified through the RFC tunnel on A4H: 202 Accepted, three parts back, the
// variables part carrying real data. Note the method of each part matters —
// the stack resource is a GET, and sending it as POST inside a batch earns
// "Resource controller does not support method POST" for that part alone while
// the others succeed.

// ADTBatch sends the requests as one batch and returns one response per
// request, in order. A part that failed carries its own status; the call itself
// only fails if the batch could not be delivered or parsed.
func (d *Debugger) ADTBatch(ctx context.Context, reqs []ADTRequest) ([]ADTResponse, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	d.batchSeq++
	boundary := fmt.Sprintf("batch_vsp_%d", d.batchSeq)

	var body strings.Builder
	for _, r := range reqs {
		method := strings.ToUpper(strings.TrimSpace(r.Method))
		if method == "" {
			method = "GET"
		}
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Type: application/http\r\n")
		body.WriteString("content-transfer-encoding: binary\r\n\r\n")
		body.WriteString(method + " " + r.URI + " HTTP/1.1\r\n")
		hasAccept := false
		for _, h := range r.Headers {
			if strings.EqualFold(h.Name, "accept") {
				hasAccept = true
			}
			body.WriteString(h.Name + ":" + h.Value + "\r\n")
		}
		if !hasAccept {
			body.WriteString("Accept:application/xml\r\n")
		}
		body.WriteString("\r\n")
		body.Write(r.Body)
		body.WriteString("\r\n")
	}
	body.WriteString("--" + boundary + "--")

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger/batch",
		[]ADTHeader{
			{Name: "Content-Type", Value: "multipart/mixed;boundary=" + boundary},
			{Name: "Accept", Value: "multipart/mixed"},
		}, []byte(body.String()))
	if err != nil {
		return nil, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return nil, adtError("batch", res)
	}
	return parseBatchParts(res.Body, res.Header("content-type"))
}

// parseBatchParts splits a multipart/mixed answer into the responses it carries.
func parseBatchParts(body []byte, contentType string) ([]ADTResponse, error) {
	boundary := ""
	for _, p := range strings.Split(contentType, ";") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "boundary=") {
			boundary = strings.Trim(strings.TrimPrefix(p, "boundary="), `"`)
			break
		}
	}
	if boundary == "" {
		return nil, fmt.Errorf("the batch answer named no boundary in %q", contentType)
	}

	var out []ADTResponse
	for _, part := range strings.Split(string(body), "--"+boundary) {
		part = strings.TrimPrefix(part, "\r\n")
		if part == "" || strings.HasPrefix(part, "--") {
			continue // the preamble and the closing delimiter
		}
		// Each part is an HTTP message wrapped in MIME headers: skip past the
		// MIME headers to the status line.
		i := strings.Index(part, "\r\n\r\n")
		if i < 0 {
			continue
		}
		res, err := parseHTTPPart(part[i+4:])
		if err != nil {
			return nil, err
		}
		out = append(out, *res)
	}
	return out, nil
}

// parseHTTPPart reads one "HTTP/1.1 200 OK" message out of a batch part.
func parseHTTPPart(msg string) (*ADTResponse, error) {
	head, rest, found := strings.Cut(msg, "\r\n\r\n")
	if !found {
		head, rest = msg, ""
	}
	lines := strings.Split(head, "\r\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("a batch part carried no status line")
	}
	res := &ADTResponse{Body: []byte(strings.TrimSuffix(rest, "\r\n"))}
	fields := strings.Fields(lines[0])
	if len(fields) >= 2 {
		res.Version = fields[0]
		res.Status, _ = strconv.Atoi(fields[1])
		res.ReasonPhrase = strings.Join(fields[2:], " ")
	}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		res.Headers = append(res.Headers, ADTHeader{
			Name:  textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name)),
			Value: strings.TrimSpace(value),
		})
	}
	return res, nil
}

// Stop is everything worth knowing about one debugger stop, fetched together.
type Stop struct {
	Stack  []byte // the stack document
	Locals []byte // the child variables of @ROOT
	Step   []byte // the step response, when the stop was reached by stepping
}

// CaptureStep performs one step and reads the stack and the locals with it, in
// a single round trip. kind is empty to capture where the debuggee already is
// without moving it.
func (d *Debugger) CaptureStep(ctx context.Context, kind string) (*Stop, error) {
	var reqs []ADTRequest
	if kind != "" {
		reqs = append(reqs, ADTRequest{Method: "POST", URI: "/sap/bc/adt/debugger?method=" + kind})
	}
	reqs = append(reqs,
		ADTRequest{Method: "GET", URI: "/sap/bc/adt/debugger/stack?method=getStack&emode=_&semanticURIs=true"},
		ADTRequest{
			Method: "POST", URI: "/sap/bc/adt/debugger?method=getChildVariables",
			Headers: []ADTHeader{
				{Name: "Accept", Value: "application/vnd.sap.as+xml"},
				{Name: "Content-Type", Value: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables"},
			},
			Body: childVariablesBody(d.localsRootsFor(ctx)),
		})

	parts, err := d.ADTBatch(ctx, reqs)
	if err != nil {
		return nil, err
	}
	stop := &Stop{}
	if kind != "" {
		if len(parts) > 0 {
			stop.Step = parts[0].Body
			if parts[0].Status != 200 {
				return nil, adtError(kind, &parts[0])
			}
		}
		parts = parts[1:]
	}
	if len(parts) > 0 {
		stop.Stack = parts[0].Body
	}
	if len(parts) > 1 {
		stop.Locals = parts[1].Body
	}
	return stop, nil
}

// childVariablesBody builds the request that asks for the children of one or
// more hierarchy roots. It is here rather than inline because the roots are not
// a constant: which ones hold a stopped frame's variables depends on the
// release, and asking for a root this system does not have returns an empty
// answer rather than an error — which is how every recorded trace came out
// with no values in it.
func childVariablesBody(parents []string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><HIERARCHIES>`)
	for _, parent := range parents {
		b.WriteString("<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>")
		b.WriteString(parent)
		b.WriteString("</PARENT_ID></STPDA_ADT_VARIABLE_HIERARCHY>")
	}
	b.WriteString(`</HIERARCHIES></DATA></asx:values></asx:abap>`)
	return []byte(b.String())
}

package saprfc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/oisee/open-rfc-go/rfc"
)

// ADT REST without HTTP.
//
// SADT_REST_RFC_ENDPOINT is SAP's own RFC entry point into the ADT REST
// dispatcher: it takes an HTTP request envelope (method, URI, headers, body)
// and returns an HTTP response envelope, running the same handlers the ICF
// nodes under /sap/bc/adt/ run. Eclipse ADT reaches it this way when it is
// configured against a system whose HTTP port is closed, and so can vsp — the
// only thing needed on the wire is classic RFC.
//
// Its TFDIR-FMODE is 'X', not 'R'. Both are remote-enabled; 'X' additionally
// marks the interface basXML-capable, which SAP sets on every FM carrying
// deep/nested parameters. A search that filters TFDIR on FMODE = 'R' will not
// find this module even though it is perfectly callable — see docs/design/
// adt-over-rfc.md.
const ADTEndpointFM = "SADT_REST_RFC_ENDPOINT"

// ADTHeader is one HTTP header field (IHTTPNVP).
type ADTHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ADTRequest is one HTTP request to tunnel.
type ADTRequest struct {
	Method  string
	URI     string
	Headers []ADTHeader
	Body    []byte
}

// ADTResponse is the HTTP response the ADT dispatcher produced.
type ADTResponse struct {
	Status       int         `json:"status"`
	ReasonPhrase string      `json:"reason_phrase"`
	Version      string      `json:"version"`
	Headers      []ADTHeader `json:"headers"`
	Body         []byte      `json:"-"`
}

// Header returns the first header with the given name, case-insensitively.
func (r *ADTResponse) Header(name string) string {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// rfcCaller is what CallADT needs from a connection: either the pooled client
// or one pinned session. The distinction matters more here than anywhere else —
// see CallADTOn.
type rfcCaller interface {
	Call(ctx context.Context, function string, in rfc.Params) (rfc.Result, error)
}

// CallADT tunnels one HTTP request to the ADT REST dispatcher over classic RFC
// and returns the response. It is transport only: no CSRF token is fetched and
// no session is kept, so it serves read-only requests (GET) directly and leaves
// stateful, token-protected flows to the HTTP client — or to CallADTOn.
func CallADT(ctx context.Context, c *rfc.Client, req ADTRequest) (*ADTResponse, error) {
	return CallADTOn(ctx, c, req)
}

// CallADTOn is CallADT over a caller you choose. Give it a pinned rfc.Session
// and consecutive ADT requests run in one ABAP session — which is what ADT's
// stateful resources (locks, and the debugger) need, and what a stateless HTTP
// client has to simulate with sap-contextid cookies. Over RFC the session is
// the conversation, so there is nothing to simulate.
func CallADTOn(ctx context.Context, c rfcCaller, req ADTRequest) (*ADTResponse, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "GET"
	}
	if !strings.HasPrefix(req.URI, "/") {
		return nil, fmt.Errorf("ADT URI must be absolute, got %q", req.URI)
	}
	headers := make([]any, 0, len(req.Headers)+1)
	hasAccept := false
	for _, h := range req.Headers {
		if strings.EqualFold(h.Name, "accept") {
			hasAccept = true
		}
		headers = append(headers, map[string]any{"NAME": h.Name, "VALUE": h.Value})
	}
	// ADT refuses a request without an Accept header outright — "Accept header
	// missing", before the resource is even reached. Over HTTP that never shows
	// up, because every client sends one; a hand-built envelope has to say it
	// itself. It has to be */* rather than a concrete type: ADT resources each
	// answer in their own media type (discovery insists on atomsvc+xml, the
	// debugger on its own vnd.sap.adt.* variants), so naming one type here
	// turns "missing" into 406 Not Acceptable for every resource but that one.
	if !hasAccept {
		headers = append(headers, map[string]any{"NAME": "Accept", "VALUE": "*/*"})
	}
	body := req.Body
	if body == nil {
		body = []byte{}
	}
	res, err := c.Call(ctx, ADTEndpointFM, rfc.Params{
		"REQUEST": map[string]any{
			"REQUEST_LINE":  map[string]any{"METHOD": method, "URI": req.URI, "VERSION": "HTTP/1.1"},
			"HEADER_FIELDS": headers,
			"MESSAGE_BODY":  body,
		},
	})
	if err != nil {
		return nil, err
	}
	envelope, ok := res.Get("RESPONSE").(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s returned no RESPONSE structure", ADTEndpointFM)
	}
	return decodeADTResponse(envelope)
}

func decodeADTResponse(envelope map[string]any) (*ADTResponse, error) {
	out := &ADTResponse{}
	// STATUS_LINE is a substructure; the flat VERSION/STATUS_CODE/REASON_PHRASE
	// fields of SADT_REST_RESPONSE are the same include seen from the other side,
	// so read whichever the graph happened to produce.
	line, _ := envelope["STATUS_LINE"].(map[string]any)
	pick := func(field string) string {
		if line != nil {
			if s, ok := line[field].(string); ok && s != "" {
				return s
			}
		}
		s, _ := envelope[field].(string)
		return s
	}
	out.Version = strings.TrimSpace(pick("VERSION"))
	out.ReasonPhrase = strings.TrimSpace(pick("REASON_PHRASE"))
	code := strings.TrimSpace(pick("STATUS_CODE"))
	if code != "" {
		n, err := strconv.Atoi(code)
		if err != nil {
			return nil, fmt.Errorf("%s returned status code %q", ADTEndpointFM, code)
		}
		out.Status = n
	}
	switch rows := envelope["HEADER_FIELDS"].(type) {
	case []map[string]any:
		for _, r := range rows {
			out.Headers = append(out.Headers, adtHeaderOf(r))
		}
	case []any:
		for _, r := range rows {
			if m, ok := r.(map[string]any); ok {
				out.Headers = append(out.Headers, adtHeaderOf(m))
			}
		}
	}
	switch b := envelope["MESSAGE_BODY"].(type) {
	case []byte:
		out.Body = b
	case string:
		out.Body = []byte(b)
	}
	return out, nil
}

func adtHeaderOf(m map[string]any) ADTHeader {
	name, _ := m["NAME"].(string)
	value, _ := m["VALUE"].(string)
	return ADTHeader{Name: name, Value: value}
}

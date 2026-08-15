package adt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type stubDynproRFCFetcher struct {
	result   *RFCResult
	err      error
	function string
	params   map[string]string
	closed   bool
}

func (s *stubDynproRFCFetcher) CallRFC(_ context.Context, function string, params map[string]string) (*RFCResult, error) {
	s.function = function
	s.params = params
	return s.result, s.err
}

func (s *stubDynproRFCFetcher) ReadSource(_ context.Context, _ string) ([]string, error) {
	return nil, errors.New("ReadSource is not used by Dynpro tests")
}

func (s *stubDynproRFCFetcher) Close() error {
	s.closed = true
	return nil
}

func newDynproTestClient(t *testing.T, stub *stubDynproRFCFetcher) *Client {
	t.Helper()
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, &mockWorkflowTransport{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)
	client.rfcFetcherFactory = func(context.Context) (rfcSourceFetcher, error) {
		return stub, nil
	}
	return client
}

func TestGetSourceReadsDynproThroughBridge(t *testing.T) {
	stub := &stubDynproRFCFetcher{result: &RFCResult{
		Subrc: 0,
		Exports: map[string]any{
			"HEADER": map[string]any{
				"PROGRAM": "ZSYNTHETIC_APP",
				"SCREEN":  "0100",
				"TYPE":    "N",
			},
		},
		Tables: map[string]any{
			"CONTAINERS": []any{
				map[string]any{"NAME": "SYNTHETIC_CONTAINER", "TYPE": "NORMAL"},
			},
			"FIELDS_TO_CONTAINERS": []any{
				map[string]any{"NAME": "SYNTHETIC_FIELD", "CONTAINER": "SYNTHETIC_CONTAINER"},
			},
			"FLOW_LOGIC": []any{
				map[string]any{"LINE": "PROCESS BEFORE OUTPUT."},
				map[string]any{"LINE": "PROCESS AFTER INPUT."},
			},
		},
	}}
	client := newDynproTestClient(t, stub)

	raw, err := client.GetSource(context.Background(), "DYNP", "ZSYNTHETIC_APP/100", nil)
	if err != nil {
		t.Fatalf("GetSource(DYNP) failed: %v", err)
	}
	var got Dynpro
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("GetSource(DYNP) returned invalid JSON: %v\n%s", err, raw)
	}
	if got.Program != "ZSYNTHETIC_APP" || got.Screen != "0100" {
		t.Fatalf("wrong dynpro identity: %#v", got)
	}
	if len(got.FlowLogic) != 2 || got.FlowLogic[0] != "PROCESS BEFORE OUTPUT." {
		t.Fatalf("wrong flow logic: %#v", got.FlowLogic)
	}
	if stub.function != "RPY_DYNPRO_READ" {
		t.Fatalf("RFC function = %q", stub.function)
	}
	if stub.params["PROGNAME"] != "ZSYNTHETIC_APP" || stub.params["DYNNR"] != "0100" {
		t.Fatalf("RFC params = %#v", stub.params)
	}
	if !stub.closed {
		t.Fatal("RFC bridge was not closed")
	}
}

func TestGetDynproFailsClosedOnRFCSubrc(t *testing.T) {
	stub := &stubDynproRFCFetcher{result: &RFCResult{Subrc: 2}}
	client := newDynproTestClient(t, stub)

	_, err := client.GetDynpro(context.Background(), "ZSYNTHETIC_APP", "0100")
	if err == nil || !strings.Contains(err.Error(), "subrc=2") {
		t.Fatalf("expected structured RFC failure, got %v", err)
	}
	if !stub.closed {
		t.Fatal("RFC bridge was not closed after failure")
	}
}

func TestGetSourceDynproRequiresParentProgram(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockWorkflowTransport{responses: map[string]*http.Response{}}))

	_, err := client.GetSource(context.Background(), "DYNP", "0100", nil)
	if err == nil || !strings.Contains(err.Error(), "parent program") {
		t.Fatalf("expected missing-parent error, got %v", err)
	}
}

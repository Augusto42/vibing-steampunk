package saprfc

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// These run against a conversation recorded from a live system and committed
// as a fixture, so the debugger is finally exercised by `go test ./...` with no
// SAP, no RFC channel and no Z code. What they check is not the recording — it
// is the code that builds the requests and reads the answers, which until now
// only ever ran on somebody's desk.

func loadFixture(t *testing.T, name string) *ReplayTransport {
	t.Helper()
	rt, err := LoadCassette("testdata/cassettes/" + name)
	if err != nil {
		t.Fatalf("loading cassette %s: %v", name, err)
	}
	return rt
}

// The recorded session was driven as TESTUSER, and the request carries the user
// in its query string, so a debugger asking for anyone else asks a different
// question and the cassette rightly has no answer for it.
const fixtureUser = "TESTUSER"

func TestReplayListsBreakpointsOverPlainADT(t *testing.T) {
	rt := loadFixture(t, "a4h-step.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)

	bps, err := dbg.ADTBreakpoints(context.Background())
	if err != nil {
		t.Fatalf("listing breakpoints: %v", err)
	}
	if len(bps) != 0 {
		t.Fatalf("the recorded system had no breakpoints registered, got %d", len(bps))
	}

	// The point of the fixture: this went to SAP's own debugger resource. If
	// somebody reroutes it through a function module, the recorded answer stops
	// matching and this fails.
	if got := rt.System(); got != "a4h" {
		t.Fatalf("cassette should name where it came from, got %q", got)
	}
}

// A refusal is an answer, and reporting it is code like any other. SAP says no
// with a status and an exception document; the caller must get the server's own
// sentence, not a bare status.
func TestReplayReportsARefusalWithTheServersReason(t *testing.T) {
	rt := loadFixture(t, "a4h-step.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)

	// The recorded session ended with a step the system refused. What matters
	// is that the server's own sentence reaches the caller: before this, the
	// machine-readable category was reported instead, so every distinct refusal
	// arrived as the same unhelpful word.
	_, err := dbg.ADTStep(context.Background(), "stepContinue")
	if err == nil {
		t.Fatal("the recorded system refused this step; the replay must refuse it too")
	}
	if !strings.Contains(err.Error(), "exception was raised") {
		t.Fatalf("the server's own reason should survive to the caller, got: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("the status should survive too, got: %v", err)
	}
}

// A cassette that answered a request it never recorded would be a mock wearing
// a recording's clothes. Strict replay is what keeps the fixture honest.
func TestReplayRefusesToInventAnswers(t *testing.T) {
	rt := loadFixture(t, "a4h-step.jsonl")

	_, err := rt.Do(context.Background(), ADTRequest{
		Method: "GET",
		URI:    "/sap/bc/adt/debugger/nothing-like-this-was-recorded",
	})
	if err == nil {
		t.Fatal("an unrecorded request must fail, not return a made-up answer")
	}
	if !strings.Contains(err.Error(), "no recorded answer") {
		t.Fatalf("the failure should say why, got: %v", err)
	}
}

// Recording and replaying must round-trip: what the recorder writes, the
// replayer answers, keyed on method, URI and body together — the debugger sends
// the same URI with different payloads and must not get the wrong one back.
func TestRecordThenReplayRoundTrips(t *testing.T) {
	fake := &scriptedTransport{answers: map[string]string{
		"POST /x|one": "first",
		"POST /x|two": "second",
	}}
	rec := Recorder(fake)
	ctx := context.Background()

	for _, body := range []string{"one", "two"} {
		if _, err := rec.Do(ctx, ADTRequest{Method: "POST", URI: "/x", Body: []byte(body)}); err != nil {
			t.Fatalf("recording %s: %v", body, err)
		}
	}

	path := t.TempDir() + "/round.jsonl"
	if err := rec.Save(path, Cassette{System: "fixture"}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	rt, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	// Deliberately out of recorded order: the key is the request, not the turn.
	for _, want := range []struct{ body, reply string }{{"two", "second"}, {"one", "first"}} {
		resp, err := rt.Do(ctx, ADTRequest{Method: "POST", URI: "/x", Body: []byte(want.body)})
		if err != nil {
			t.Fatalf("replaying %s: %v", want.body, err)
		}
		if string(resp.Body) != want.reply {
			t.Fatalf("body %q should replay %q, got %q", want.body, want.reply, resp.Body)
		}
	}
}

// Whatever else a cassette carries, it must not carry the session it was
// recorded on. Set-Cookie holds a live token and names the application server
// inside sap-contextid, so it has to be gone before the file is written.
func TestRecordingDropsTheSession(t *testing.T) {
	fake := &scriptedTransport{
		answers: map[string]string{"GET /y|": "body"},
		headers: []ADTHeader{
			{Name: "Set-Cookie", Value: "sap-contextid=SID%3aANON%3aliveserver_ABC_00%3atoken; path=/"},
			{Name: "Content-Type", Value: "application/xml"},
		},
	}
	rec := Recorder(fake)
	if _, err := rec.Do(context.Background(), ADTRequest{Method: "GET", URI: "/y"}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	rec.Redact = map[string]string{"REALNAME": "TESTUSER"}

	path := t.TempDir() + "/scrubbed.jsonl"
	if err := rec.Save(path, Cassette{System: "fixture"}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	raw := readFile(t, path)
	for _, forbidden := range []string{"Set-Cookie", "sap-contextid", "liveserver"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("cassette still carries %q — it must never reach a tracked file", forbidden)
		}
	}
	if !strings.Contains(raw, "application/xml") {
		t.Fatal("scrubbing dropped a harmless header along with the sensitive ones")
	}
}

// The committed fixtures are checked on every run, so a careless re-record
// cannot quietly put a live identifier back into the repository. This is the
// backstop for the scrub, not a substitute: the scrub works from a list, and a
// list is only ever as complete as the last surprise.
func TestCommittedCassettesCarryNoLiveIdentifiers(t *testing.T) {
	// host_SID_NN — the shape of an SAP instance name, which is how a server
	// gives itself away even when the field carrying it looks opaque.
	instanceName := regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9-]{2,}_[A-Z][A-Z0-9]{2}_[0-9]{2}`)
	ipv4 := regexp.MustCompile(`\b[0-9]{1,3}(\.[0-9]{1,3}){3}\b`)

	entries, err := os.ReadDir("testdata/cassettes")
	if err != nil {
		t.Fatalf("reading the cassette directory: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		checked++
		// Scanning the file as text would prove nothing: bodies are base64, so
		// a hostname sitting inside one is invisible to a plain search. Decode
		// first, then look at what a reader of the repository would see.
		plain := decodedCassette(t, "testdata/cassettes/"+e.Name())
		for _, forbidden := range []string{"Set-Cookie", "sap-contextid", "SID%3a", "X-CSRF-Token"} {
			if strings.Contains(plain, forbidden) {
				t.Errorf("%s carries %q", e.Name(), forbidden)
			}
		}
		if m := instanceName.FindString(plain); m != "" {
			t.Errorf("%s names an application server: %q", e.Name(), m)
		}
		if m := ipv4.FindString(plain); m != "" {
			t.Errorf("%s carries an address: %q", e.Name(), m)
		}
	}
	if checked == 0 {
		t.Fatal("no cassettes found — the fixtures these tests rest on are missing")
	}
}

// decodedCassette returns everything a cassette holds as readable text: URIs,
// header values, and the request and answer bodies with their base64 undone.
func decodedCassette(t *testing.T, path string) string {
	t.Helper()
	var out strings.Builder
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ex Exchange
		if err := json.Unmarshal([]byte(line), &ex); err != nil {
			// The header line is a Cassette, not an Exchange; it is plain text
			// either way, so keep it in the scan.
			out.WriteString(line)
			continue
		}
		out.WriteString(ex.URI)
		out.WriteString("\n")
		for _, h := range ex.Headers {
			out.WriteString(h.Name + ": " + h.Value + "\n")
		}
		out.Write(decode(ex.Body))
		out.WriteString("\n")
		out.Write(decode(ex.Reply))
		out.WriteString("\n")
	}
	return out.String()
}

type scriptedTransport struct {
	answers map[string]string
	headers []ADTHeader
}

func (s *scriptedTransport) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	key := req.Method + " " + req.URI + "|" + string(req.Body)
	body, ok := s.answers[key]
	if !ok {
		return &ADTResponse{Status: 404}, nil
	}
	return &ADTResponse{Status: 200, Body: []byte(body), Headers: s.headers}, nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// This walks the whole recorded session in the order it happened: catch a
// stopped program, read its stack, read its locals with real values, step into
// a subroutine, step back out. Every answer came from a live 7.58 system over
// plain ADT — no RFC channel, no ZADT_DEBUG, no Z code of any kind — and the
// code under test is the same debugger the CLI drives.
//
// The recording is pkg/saprfc/testdata/cassettes/a4h-step.jsonl, taken with
// `vsp adt debug --record` while ZVSP_DEBUG_DEMO stopped on its PERFORM.
func TestReplayWalksARecordedDebugSession(t *testing.T) {
	rt := loadFixture(t, "a4h-step.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	// Catch the debuggee the recording caught.
	who, err := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	if who == nil {
		t.Fatal("the recorded listener caught a debuggee; the replay caught nothing")
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	// The stack, as SAP described it at the stop.
	stack, err := dbg.StackInfo(ctx)
	if err != nil {
		t.Fatalf("reading the stack: %v", err)
	}
	if len(stack.Stack) == 0 {
		t.Fatal("a stopped program has a stack")
	}
	top := stack.Stack[0]
	if top.Line != 26 {
		t.Fatalf("the breakpoint was on line 26, the stack says %d", top.Line)
	}
	if !strings.Contains(strings.ToUpper(top.ProgramName), "ZVSP_DEBUG_DEMO") {
		t.Fatalf("top frame should be the demo program, got %q", top.ProgramName)
	}

	// The recorded run read the stack twice; follow it, so the answers stay
	// lined up with the requests that produced them.
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack again: %v", err)
	}

	// Locals, with values — the part that makes a trace worth having.
	locals, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("reading locals: %v", err)
	}
	byName := map[string]adt.DebugVariable{}
	for _, v := range locals {
		byName[strings.ToUpper(v.Name)] = v
	}
	counter, ok := byName["LV_COUNTER"]
	if !ok {
		t.Fatalf("LV_COUNTER should be in scope, got %v", namesOf(locals))
	}
	if strings.TrimSpace(counter.Value) != "7" {
		t.Fatalf("LV_COUNTER was 7 at the stop, replay says %q", counter.Value)
	}
	if _, ok := byName["LT_ROWS"]; !ok {
		t.Fatalf("the internal table should be in scope, got %v", namesOf(locals))
	}

	// Into the subroutine.
	if _, err := dbg.ADTStep(ctx, "stepInto"); err != nil {
		t.Fatalf("stepping into: %v", err)
	}
	inner, err := dbg.StackInfo(ctx)
	if err != nil {
		t.Fatalf("reading the stack inside the FORM: %v", err)
	}
	if len(inner.Stack) < 2 {
		t.Fatalf("stepping into a PERFORM adds a frame; got %d", len(inner.Stack))
	}
	if inner.Stack[0].Line != 9 {
		t.Fatalf("the FORM body is on line 9, the stack says %d", inner.Stack[0].Line)
	}

	// The subroutine's own parameters are now in scope, and the caller's
	// variables still are.
	innerLocals, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("reading locals inside the FORM: %v", err)
	}
	innerNames := namesOf(innerLocals)
	for _, want := range []string{"IV_IN", "CV_OUT", "LV_COUNTER"} {
		if !contains(innerNames, want) {
			t.Fatalf("%s should be in scope inside the FORM, got %v", want, innerNames)
		}
	}

	// And back out, to the line after the PERFORM.
	if _, err := dbg.ADTStep(ctx, "stepReturn"); err != nil {
		t.Fatalf("stepping out: %v", err)
	}
	after, err := dbg.StackInfo(ctx)
	if err != nil {
		t.Fatalf("reading the stack after stepping out: %v", err)
	}
	if after.Stack[0].Line != 28 {
		t.Fatalf("stepping out of the PERFORM lands on line 28, the stack says %d", after.Stack[0].Line)
	}
}

// Reading a variable's value is the whole point of a debugger, and the values
// SAP sends need decoding before they mean anything — a padded integer, a
// structure announced as deep, a table announced by its shape.
func TestReplayReadsValuesAsSAPSendsThem(t *testing.T) {
	rt := loadFixture(t, "a4h-step.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	who, err := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if err != nil || who == nil {
		t.Fatalf("listening: %v", err)
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("stack: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("stack: %v", err)
	}

	locals, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("locals: %v", err)
	}
	for _, v := range locals {
		switch strings.ToUpper(v.Name) {
		case "LV_DOUBLED":
			// Not yet assigned at this stop, and SAP pads it.
			if strings.TrimSpace(v.Value) != "0" {
				t.Errorf("LV_DOUBLED should still be 0, got %q", v.Value)
			}
		case "LT_ROWS":
			// The table holds the two rows the program appended before the stop.
			if !strings.Contains(v.Value, "2x2") && !strings.Contains(v.Value, "Table") {
				t.Errorf("LT_ROWS should be described as a table, got %q", v.Value)
			}
		}
	}
}

func namesOf(vars []adt.DebugVariable) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, strings.ToUpper(v.Name))
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

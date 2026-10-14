package saprfc

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The debugger has no tests that run without a live system. Its one
// cross-transport check needs a running SAP, an RFC channel and a Z facade — so
// it cannot exercise the claim that no Z code is needed, and nothing at all
// exercises stepping, variable writes, frame navigation, batch capture or the
// paths that report a refusal.
//
// The transport is a single method, which makes this cheap: record what a real
// system answered once, then replay it forever. The debugger under test is the
// real one — same session logic, same parsers, same batch document — and only
// the wire is substituted. A recording is an oracle, not a mock: nobody writes
// the answers by hand, so nobody can quietly write them to match the code.

// Exchange is one request and the answer a system gave to it.
type Exchange struct {
	Method  string      `json:"method"`
	URI     string      `json:"uri"`
	Body    string      `json:"body,omitempty"` // base64, absent when empty
	Status  int         `json:"status"`
	Reason  string      `json:"reason,omitempty"`
	Headers []ADTHeader `json:"headers,omitempty"`
	Reply   string      `json:"reply,omitempty"` // base64, absent when empty
}

// Cassette is a recorded conversation with a system.
type Cassette struct {
	System    string     `json:"system,omitempty"`
	Release   string     `json:"release,omitempty"`
	Recorded  string     `json:"recorded,omitempty"`
	Exchanges []Exchange `json:"-"`
}

// key identifies a request. The body is part of it because the debugger sends
// the same URI with different payloads — getChildVariables for one parent and
// another differ only there.
func exchangeKey(req ADTRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n", strings.ToUpper(req.Method), req.URI)
	h.Write(req.Body)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// RecordingTransport writes every exchange to a cassette as it passes.
type RecordingTransport struct {
	inner ADTTransport
	mu    sync.Mutex
	out   []Exchange
	// Redact replaces literals on the way to disk. A recording is taken from a
	// real system, so it carries a real logon name and a real terminal id, and
	// a cassette is a tracked test fixture. Substituting at save time keeps the
	// exchange keys consistent — request and answer are rewritten together —
	// so a scrubbed cassette replays exactly like the run it came from, as
	// long as the test drives it with the same substituted names.
	Redact map[string]string
}

// Recorder wraps a live transport so a session can be captured.
func Recorder(inner ADTTransport) *RecordingTransport {
	return &RecordingTransport{inner: inner}
}

func (r *RecordingTransport) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	resp, err := r.inner.Do(ctx, req)
	if resp == nil {
		// Nothing came back at all — a transport failure, not an answer. There
		// is nothing to replay, so there is nothing to record.
		return resp, err
	}
	// A refusal is recorded like any other answer. It is the answer that most
	// needs a test: a 403 carrying an ADT exception document is how the server
	// says no, and the code that turns it into a message for the caller has
	// never been exercised without a live system.
	ex := Exchange{
		Method: req.Method, URI: req.URI,
		Body:  encode(req.Body),
		Reply: encode(resp.Body),
	}
	if resp != nil {
		ex.Status, ex.Reason, ex.Headers = resp.Status, resp.ReasonPhrase, resp.Headers
	}
	r.mu.Lock()
	r.out = append(r.out, ex)
	r.mu.Unlock()
	return resp, err
}

// Save writes the cassette as JSONL, one exchange per line, with a header line
// naming the system it came from. A recording without that provenance is a
// number nobody can check later.
func (r *RecordingTransport) Save(path string, c Cassette) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	header, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", header); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ex := range r.out {
		line, err := json.Marshal(r.scrub(ex))
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// sensitiveHeader reports headers that must never reach a cassette. A cassette
// is a tracked test fixture, and these carry a live session token or name the
// box that answered — Set-Cookie alone hands over both, because sap-contextid
// embeds the application server's own hostname. Replay never needs them: it
// answers from a file, so there is no session to keep.
func sensitiveHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "set-cookie", "cookie", "authorization", "x-csrf-token",
		"sap-contextid", "sap-server", "sap-perf-fesrec", "sap-isc-uagent":
		return true
	}
	return false
}

// hostBearingAttributes are XML attributes whose value is the application
// server's own name. The debugger's step document announces it twice — once
// plainly and once inside a human-readable session title — so a cassette
// recorded from a working debug session names the box unless these are
// blanked. The debugger reads neither, so blanking costs replay nothing.
// debuggeeSessionId is here because SAP concatenates the instance name onto the
// end of the id, so the host arrives inside a field that looks like an opaque
// handle. Nothing reads it for control flow, so blanking costs replay nothing.
var hostBearingAttributes = []string{"serverName", "sessionTitle", "debuggeeSessionId"}

// hostBearingElements are elements whose text is the application server's name.
// The listener's debuggee listing gives it four different ways — the RFC
// destination, the application server, the instance and the host — so blanking
// one is not enough.
var hostBearingElements = []string{
	"RFCDEST", "APPSERVER", "APPLSERVER", "INSTANCE_NAME", "HOST", "SERVER_NAME",
}

// blankAttributes replaces the value of each named attribute with a fixed
// placeholder, leaving the document otherwise byte-for-byte as SAP sent it.
func blankAttributes(body string) string {
	var b strings.Builder
	for _, attr := range hostBearingAttributes {
		needle := attr + `="`
		b.Reset()
		rest := body
		for {
			i := strings.Index(rest, needle)
			if i < 0 {
				b.WriteString(rest)
				break
			}
			valueStart := i + len(needle)
			j := strings.Index(rest[valueStart:], `"`)
			if j < 0 {
				b.WriteString(rest)
				break
			}
			b.WriteString(rest[:valueStart])
			b.WriteString("REDACTED")
			rest = rest[valueStart+j:]
		}
		body = b.String()
	}
	return body
}

// blankElements replaces the text of each named element with a placeholder.
func blankElements(body string) string {
	var b strings.Builder
	for _, name := range hostBearingElements {
		open, close := "<"+name+">", "</"+name+">"
		b.Reset()
		rest := body
		for {
			i := strings.Index(rest, open)
			if i < 0 {
				b.WriteString(rest)
				break
			}
			textStart := i + len(open)
			j := strings.Index(rest[textStart:], close)
			if j < 0 {
				b.WriteString(rest)
				break
			}
			b.WriteString(rest[:textStart])
			if j > 0 {
				// An empty element names nothing; leave it empty so the shape
				// of what SAP sent is preserved.
				b.WriteString("REDACTED")
			}
			rest = rest[textStart+j:]
		}
		body = b.String()
	}
	return body
}

// scrub is what stands between a live recording and a public repository. It
// drops the headers that carry a session or a hostname, blanks the attributes
// that carry one inside a document, then applies the caller's substitutions.
func (r *RecordingTransport) scrub(ex Exchange) Exchange {
	kept := ex.Headers[:0:0]
	for _, h := range ex.Headers {
		if !sensitiveHeader(h.Name) {
			kept = append(kept, h)
		}
	}
	ex.Headers = kept
	ex.Reply = encode([]byte(blankElements(blankAttributes(string(decode(ex.Reply))))))

	if len(r.Redact) == 0 {
		return ex
	}
	replace := func(s string) string {
		for from, to := range r.Redact {
			if from == "" {
				continue
			}
			s = strings.ReplaceAll(s, from, to)
		}
		return s
	}
	ex.URI = replace(ex.URI)
	ex.Body = encode([]byte(replace(string(decode(ex.Body)))))
	ex.Reply = encode([]byte(replace(string(decode(ex.Reply)))))
	for i, h := range ex.Headers {
		ex.Headers[i].Value = replace(h.Value)
	}
	return ex
}

// Count reports how many exchanges were captured.
func (r *RecordingTransport) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.out)
}

// ReplayTransport answers from a cassette and never touches a network.
type ReplayTransport struct {
	meta Cassette
	// byKey holds every answer recorded for a request, in order, because the
	// same request legitimately gets different answers as a session advances:
	// the second getStack of a run is a different stack.
	mu    sync.Mutex
	byKey map[string][]Exchange
	used  map[string]int
	// Strict makes an unrecorded request an error rather than a gap. A replay
	// that quietly invents answers proves nothing.
	Strict bool
}

// LoadCassette reads a recorded conversation.
func LoadCassette(path string) (*ReplayTransport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadCassette(f)
}

// ReadCassette parses a cassette from any source.
func ReadCassette(r io.Reader) (*ReplayTransport, error) {
	scanner := bufio.NewScanner(r)
	// A recorded stack or variable document runs to tens of kilobytes; the
	// default token size would truncate one and the failure would look like a
	// parser bug.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	rt := &ReplayTransport{byKey: map[string][]Exchange{}, used: map[string]int{}, Strict: true}
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if err := json.Unmarshal([]byte(line), &rt.meta); err == nil && rt.meta.System != "" {
				continue
			}
			// No header: treat the line as an exchange after all.
		}
		var ex Exchange
		if err := json.Unmarshal([]byte(line), &ex); err != nil {
			return nil, fmt.Errorf("cassette line is not an exchange: %w", err)
		}
		key := exchangeKey(ADTRequest{Method: ex.Method, URI: ex.URI, Body: decode(ex.Body)})
		rt.byKey[key] = append(rt.byKey[key], ex)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(rt.byKey) == 0 {
		return nil, fmt.Errorf("cassette holds no exchanges")
	}
	return rt, nil
}

// System reports where the recording came from.
func (rt *ReplayTransport) System() string { return rt.meta.System }

// Release reports the release the recording came from.
func (rt *ReplayTransport) Release() string { return rt.meta.Release }

func (rt *ReplayTransport) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	key := exchangeKey(req)

	rt.mu.Lock()
	defer rt.mu.Unlock()

	recorded, ok := rt.byKey[key]
	if !ok || len(recorded) == 0 {
		if rt.Strict {
			return nil, fmt.Errorf("no recorded answer for %s %s — the cassette was taken from a run that did not make this request",
				strings.ToUpper(req.Method), req.URI)
		}
		return &ADTResponse{Status: 404, ReasonPhrase: "not in cassette"}, nil
	}

	// Advance through repeats, and hold on the last one: a loop that steps more
	// times than were recorded should see the final state rather than fall off
	// the end.
	i := rt.used[key]
	if i >= len(recorded) {
		i = len(recorded) - 1
	}
	rt.used[key] = i + 1
	ex := recorded[i]

	return &ADTResponse{
		Status:       ex.Status,
		ReasonPhrase: ex.Reason,
		Headers:      ex.Headers,
		Body:         decode(ex.Reply),
	}, nil
}

func encode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func decode(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

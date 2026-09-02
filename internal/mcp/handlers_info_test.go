package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The empty call is the one this exists for, and it is the one the probe table
// cannot check: that table dispatches on an action, so a row with none would be
// indistinguishable from a broken row. Checked here instead.
func TestAnEmptyCallIsAnsweredRatherThanRefused(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("empty call: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "action is required") {
		t.Fatalf("an empty call is still refused:\n%s", text)
	}
	// Whatever the connection turns out to be, these three have to be there:
	// which build answered, what state the session is in, and what to do next.
	for _, want := range []string{"vsp ", "connection", "SAP(action=\"help\")"} {
		if !strings.Contains(text, want) {
			t.Errorf("the card does not mention %q:\n%s", want, text)
		}
	}
}

// A card that cannot say which build produced it is a bug report with no
// version in it.
func TestTheCardNamesTheBuild(t *testing.T) {
	srv := NewServer(&Config{BaseURL: "https://example.invalid", Mode: "hyperfocused",
		Build: "v9.9.9 (commit deadbee, built then)"})
	text := resultText(srv.handleInfo(t.Context()))
	if !strings.Contains(text, "v9.9.9 (commit deadbee, built then)") {
		t.Errorf("the build stamp is missing:\n%s", text)
	}
}

// An unset stamp must say so rather than print an empty line that reads like a
// version.
func TestAnUnstampedBuildSaysSo(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	if text := resultText(srv.handleInfo(t.Context())); !strings.Contains(text, "unknown build") {
		t.Errorf("an unstamped build is not reported:\n%s", text)
	}
}

// The point of splitting the tail: a caller whose session is dead must not be
// handed five object calls that will all fail. It gets the diagnosis instead.
func TestADeadSessionGetsADiagnosisNotAListOfCalls(t *testing.T) {
	srv := serverForMode(t, "hyperfocused") // example.invalid resolves to nothing
	text := resultText(srv.handleInfo(t.Context()))

	if !strings.Contains(text, "NOT usable") {
		t.Fatalf("a connection that cannot work is not reported as such:\n%s", text)
	}
	if strings.Contains(text, "Next call") {
		t.Errorf("calls that cannot succeed are still being suggested:\n%s", text)
	}
	if !strings.Contains(text, "Nothing else will work") {
		t.Errorf("no diagnosis was offered:\n%s", text)
	}
	// The lockout warning is the one line here with a cost attached to
	// ignoring it, and it belongs in front of an agent that is about to retry.
	if !strings.Contains(text, "counts toward the lock") {
		t.Errorf("the card invites a retry on 401 without naming the cost:\n%s", text)
	}
}

// A live session gets the opposite tail, and the system's own identity.
func TestALiveSessionGetsTheNextCalls(t *testing.T) {
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Any ADT session that works hands out a CSRF token; that is what the
		// check is, and it is what an expired SSO session cannot produce.
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		w.WriteHeader(http.StatusOK)
	}))
	defer sap.Close()

	srv := NewServer(&Config{BaseURL: sap.URL, Username: "TESTUSER", Client: "001",
		Mode: "hyperfocused", Build: "v9.9.9"})
	text := resultText(srv.handleInfo(t.Context()))

	if !strings.Contains(text, "authenticated as TESTUSER") {
		t.Errorf("a working session is not reported as one:\n%s", text)
	}
	if !strings.Contains(text, "Next call") {
		t.Errorf("a working session was given no next call:\n%s", text)
	}
}

// The instance number is a convention about ICM ports, not something the system
// was asked. A landscape that does not follow the convention gets "unknown",
// and one that does gets the number with the derivation named — because a
// number presented as fact is a number somebody will act on.
func TestTheInstanceNumberIsMarkedAsDerived(t *testing.T) {
	for _, c := range []struct{ url, want string }{
		{"https://host.invalid:44301", "01 (derived"},
		{"https://host.invalid:8002", "02 (derived"},
		{"https://host.invalid:50300", "03 (derived"},
		{"https://host.invalid:12345", "unknown"},
	} {
		srv := NewServer(&Config{BaseURL: c.url, Mode: "hyperfocused"})
		if text := resultText(srv.handleInfo(t.Context())); !strings.Contains(text, c.want) {
			t.Errorf("%s: expected %q in the card", c.url, c.want)
		}
	}
}

package adt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSSOSessionCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devsys.json")
	cookies := map[string]string{"SAP_SESSIONID_DEV_100": "abc", "sap-usercontext": "def"}

	if err := SaveSSOSession(path, "sap.example", cookies); err != nil {
		t.Fatalf("SaveSSOSession: %v", err)
	}

	// The cache holds a credential that authenticates with no password, so the
	// permissions matter as much as the contents.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("cache mode = %o, want 600", perm)
	}

	sess, err := LoadSSOSession(path)
	if err != nil {
		t.Fatalf("LoadSSOSession: %v", err)
	}
	if sess == nil {
		t.Fatal("LoadSSOSession returned nothing for a cache just written")
	}
	if sess.Host != "sap.example" {
		t.Errorf("host = %q, want sap.example", sess.Host)
	}
	if len(sess.Cookies) != 2 || sess.Cookies["SAP_SESSIONID_DEV_100"] != "abc" {
		t.Errorf("cookies = %v, want the two saved", sess.Cookies)
	}
	if age := sess.Age(); age < 0 || age > time.Minute {
		t.Errorf("age = %v, want a fresh session", age)
	}
}

func TestLoadSSOSessionTreatsUnusableCacheAsAbsent(t *testing.T) {
	dir := t.TempDir()
	// A cache is a convenience, never a dependency: anything unreadable in it
	// must send the caller off to capture a new session rather than fail the run.
	cases := map[string]string{
		"corrupt":    "{not json",
		"no-cookies": `{"host":"sap.example","cookies":{}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			sess, err := LoadSSOSession(path)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if sess != nil {
				t.Errorf("session = %+v, want nil", sess)
			}
		})
	}

	t.Run("missing", func(t *testing.T) {
		sess, err := LoadSSOSession(filepath.Join(dir, "absent.json"))
		if err != nil || sess != nil {
			t.Errorf("got (%+v, %v), want (nil, nil)", sess, err)
		}
	})
}

func TestSSOProviderTriggerURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  SSOConfig
		want string
	}{
		{
			name: "derived from the system URL",
			cfg:  SSOConfig{BaseURL: "https://sap.example:44300", Client: "100"},
			want: "https://sap.example:44300/sap/bc/adt/?sap-client=100",
		},
		{
			name: "no client, no client parameter",
			cfg:  SSOConfig{BaseURL: "https://sap.example"},
			want: "https://sap.example/sap/bc/adt/",
		},
		{
			name: "a path on the system URL is replaced, not appended to",
			cfg:  SSOConfig{BaseURL: "https://sap.example/sap/bc/adt", Client: "001"},
			want: "https://sap.example/sap/bc/adt/?sap-client=001",
		},
		{
			name: "explicit trigger wins",
			cfg:  SSOConfig{BaseURL: "https://sap.example", Client: "100", TriggerURL: "https://sap.example/sap/bc/ui2/flp?sap-client=200"},
			want: "https://sap.example/sap/bc/ui2/flp?sap-client=200",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewSSOProvider(tc.cfg)
			if err != nil {
				t.Fatalf("NewSSOProvider: %v", err)
			}
			got, err := p.triggerURL()
			if err != nil {
				t.Fatalf("triggerURL: %v", err)
			}
			if got != tc.want {
				t.Errorf("triggerURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSSOProviderReauthBudgetCoversTheSignIn(t *testing.T) {
	// A budget sized for the silent path would cut off a sign-in mid-way
	// through a second factor, so an interactive provider must ask for more.
	silent, err := NewSSOProvider(SSOConfig{BaseURL: "https://sap.example"})
	if err != nil {
		t.Fatal(err)
	}
	interactive, err := NewSSOProvider(SSOConfig{BaseURL: "https://sap.example", Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if interactive.ReauthBudget() <= silent.ReauthBudget() {
		t.Errorf("interactive budget %v is not greater than silent %v",
			interactive.ReauthBudget(), silent.ReauthBudget())
	}
	if interactive.ReauthBudget() < DefaultSSOTimeoutInteractive {
		t.Errorf("interactive budget %v is shorter than one sign-in window (%v)",
			interactive.ReauthBudget(), DefaultSSOTimeoutInteractive)
	}
}

func TestSSOProviderCachesAndClears(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p, err := NewSSOProvider(SSOConfig{System: "devsys", BaseURL: "https://sap.example"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".vsp", "sso", "devsys.json"); p.CachePath() != want {
		t.Fatalf("cache path = %q, want %q", p.CachePath(), want)
	}

	cookies := map[string]string{"SAP_SESSIONID_X": "live"}
	if err := SaveSSOSession(p.CachePath(), "sap.example", cookies); err != nil {
		t.Fatal(err)
	}

	// Cookies must come from the cache without any browser being started; a
	// capture here would try to launch one and fail the test on a build machine.
	got, err := p.Cookies(context.Background())
	if err != nil {
		t.Fatalf("Cookies: %v", err)
	}
	if got["SAP_SESSIONID_X"] != "live" {
		t.Errorf("cookies = %v, want the cached session", got)
	}

	if err := p.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(p.CachePath()); !os.IsNotExist(err) {
		t.Errorf("cache still present after Clear (err = %v)", err)
	}
	// Clearing twice is how a caller tidies up without first checking, so it
	// must not turn a missing file into a failure.
	if err := p.Clear(); err != nil {
		t.Errorf("second Clear: %v", err)
	}
}

func TestSanitizeHostForPath(t *testing.T) {
	tests := map[string]string{
		"sap.example.com":  "sap.example.com",
		"sap-dev01":        "sap-dev01",
		"../../etc/passwd": ".._.._etc_passwd",
		"a/b\\c:d":         "a_b_c_d",
	}
	for in, want := range tests {
		if got := sanitizeHostForPath(in); got != want {
			t.Errorf("sanitizeHostForPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultSSOProfileDirIsPerHost(t *testing.T) {
	a, err := DefaultSSOProfileDir("sap-a.example")
	if err != nil {
		t.Fatal(err)
	}
	b, err := DefaultSSOProfileDir("sap-b.example")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Errorf("two hosts share a browser profile (%q) — a sign-in to one would disturb the other", a)
	}
}

func TestSSOHelperOutputContract(t *testing.T) {
	// The helper's stdout is the only thing that crosses the WSL boundary, so
	// the provider must read exactly what the helper writes.
	blob, err := json.Marshal(SSOSession{
		Host:       "sap.example",
		CapturedAt: time.Now().UTC(),
		Cookies:    map[string]string{"SAP_SESSIONID_X": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var back SSOSession
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Cookies["SAP_SESSIONID_X"] != "value" || back.Host != "sap.example" {
		t.Errorf("round trip lost data: %+v", back)
	}
}

// reissuingClient answers the way SAP does when a logon ticket outlives the
// session it opened: it authenticates on the ticket, quietly opens a new
// session, and returns that session's id in Set-Cookie along with a token
// minted for it.
type reissuingClient struct{ newSession string }

func (c *reissuingClient) Do(req *http.Request) (*http.Response, error) {
	h := http.Header{}
	h.Set("X-CSRF-Token", "token-for-the-new-session")
	h.Add("Set-Cookie", "SAP_SESSIONID_DEV_100="+c.newSession+"; path=/")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func TestReissuedSessionCookieReplacesTheHeldOne(t *testing.T) {
	// Holding the lapsed id while the jar holds the new one sends both under the
	// same name; the server honours one and the token belongs to the other, and
	// the client is told its CSRF token is invalid while being fully
	// authenticated.
	cfg := NewConfig("https://sap.example", "", "",
		WithCookies(map[string]string{
			"SAP_SESSIONID_DEV_100": "lapsed",
			"MYSAPSSO2":             "ticket-still-good",
		}),
	)
	transport := NewTransportWithClient(cfg, &reissuingClient{newSession: "fresh"})

	if err := transport.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}

	if got := cfg.Cookies["SAP_SESSIONID_DEV_100"]; got != "fresh" {
		t.Errorf("session cookie = %q, want the reissued one", got)
	}
	// The ticket was not reissued and must be left exactly as it was.
	if got := cfg.Cookies["MYSAPSSO2"]; got != "ticket-still-good" {
		t.Errorf("ticket = %q, want it untouched", got)
	}
}

func TestServerCookiesDoNotIntroduceNewOnes(t *testing.T) {
	// Adopting a value for a cookie already held is one thing; collecting every
	// cookie a server offers is another, and would have this client sending
	// whatever any response happened to set.
	cfg := NewConfig("https://sap.example", "", "",
		WithCookies(map[string]string{"SAP_SESSIONID_DEV_100": "held"}))
	transport := NewTransportWithClient(cfg, &mockHTTPClient{})

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Add("Set-Cookie", "SOMETHING_ELSE=value; path=/")
	transport.adoptServerCookies(resp)

	if _, present := cfg.Cookies["SOMETHING_ELSE"]; present {
		t.Error("a cookie the client never held was adopted")
	}
	if len(cfg.Cookies) != 1 {
		t.Errorf("cookies = %v, want only the one held", cfg.Cookies)
	}
}

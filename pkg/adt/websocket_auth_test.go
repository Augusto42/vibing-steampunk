package adt

import (
	"net/http"
	"testing"
)

func TestWebSocketAuthPrefersTheSession(t *testing.T) {
	// A password is what the client was built around; a browser session is the
	// only credential some systems can offer at all. When both are present the
	// session wins, because sending both invites the server to pick.
	c := NewBaseWebSocketClient("https://sap.example", "100", "USER", "secret", false)
	c.SetCookies(map[string]string{"SAP_SESSIONID_DEV_100": "abc"})

	header := http.Header{}
	c.applyAuth(header)

	if got := header.Get("Cookie"); got != "SAP_SESSIONID_DEV_100=abc" {
		t.Errorf("Cookie = %q, want the session", got)
	}
	if got := header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want it left off when a session is used", got)
	}
}

func TestWebSocketAuthFallsBackToBasic(t *testing.T) {
	c := NewBaseWebSocketClient("https://sap.example", "100", "USER", "secret", false)

	header := http.Header{}
	c.applyAuth(header)

	if header.Get("Authorization") == "" {
		t.Error("Authorization is empty with no session — basic auth must still work")
	}
	if got := header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want none", got)
	}
}

func TestWebSocketCookieHeaderIsStable(t *testing.T) {
	// Two captures of the same session should be comparable byte for byte;
	// map iteration order would otherwise reshuffle the header on every run.
	c := NewBaseWebSocketClient("https://sap.example", "100", "", "", false)
	c.SetCookies(map[string]string{
		"sap-usercontext":       "sap-client=100",
		"SAP_SESSIONID_DEV_100": "abc",
		"MYSAPSSO2":             "ticket",
	})
	want := "MYSAPSSO2=ticket; SAP_SESSIONID_DEV_100=abc; sap-usercontext=sap-client=100"
	for i := 0; i < 8; i++ {
		if got := c.cookieHeader(); got != want {
			t.Fatalf("cookieHeader() = %q, want %q", got, want)
		}
	}
}

func TestWebSocketHasCookieAuth(t *testing.T) {
	c := NewBaseWebSocketClient("https://sap.example", "100", "USER", "secret", false)
	if c.hasCookieAuth() {
		t.Error("a fresh client reports a session it was never given")
	}
	c.SetCookies(map[string]string{})
	if c.hasCookieAuth() {
		t.Error("an empty session counts as one")
	}
	c.SetCookies(map[string]string{"SAP_SESSIONID_DEV_100": "abc"})
	if !c.hasCookieAuth() {
		t.Error("a session was set and is not reported")
	}
}

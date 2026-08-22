package mcp

import "testing"

// A long-running server re-authenticates when its session lapses. The config it
// was constructed with still holds the map from startup, so anything reading
// that instead of the client is holding a dead session — ordinary calls keep
// working while every WebSocket fails, which points at the WebSocket.
func TestWSAuthFollowsTheRefreshedSession(t *testing.T) {
	startup := map[string]string{"SAP_SESSIONID_DEV_100": "lapsed"}
	s := NewServer(&Config{
		BaseURL: "https://example.invalid", Client: "000", Language: "EN",
		Mode: "hyperfocused", Cookies: startup,
	})

	// What a re-authentication does: replace the map wholesale.
	s.adtClient.SetCookies(map[string]string{"SAP_SESSIONID_DEV_100": "fresh"})

	var got map[string]string
	s.applyWSAuth(func(c map[string]string) { got = c })

	if got["SAP_SESSIONID_DEV_100"] != "fresh" {
		t.Errorf("WebSocket would authenticate with %q, want the refreshed session",
			got["SAP_SESSIONID_DEV_100"])
	}
}

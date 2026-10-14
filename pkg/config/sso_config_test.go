package config

import "testing"

func TestUsesSSO(t *testing.T) {
	tests := map[string]bool{"sso": true, "SSO": true, "Sso": true, "": false, "basic": false, "cookie": false}
	for auth, want := range tests {
		sys := SystemConfig{Auth: auth}
		if got := sys.UsesSSO(); got != want {
			t.Errorf("UsesSSO(auth=%q) = %v, want %v", auth, got, want)
		}
	}
}

func TestInteractiveOnExpiryDefaultsToOpeningAWindow(t *testing.T) {
	// A system that says nothing should recover by itself rather than wait for
	// someone to notice and run a command.
	var unset *SSOSettings
	if !unset.InteractiveOnExpiry() {
		t.Error("nil settings: want a window by default")
	}
	if !(&SSOSettings{}).InteractiveOnExpiry() {
		t.Error("empty settings: want a window by default")
	}
	if !(&SSOSettings{OnExpiry: "window"}).InteractiveOnExpiry() {
		t.Error(`OnExpiry "window": want a window`)
	}
	if (&SSOSettings{OnExpiry: "error"}).InteractiveOnExpiry() {
		t.Error(`OnExpiry "error": want no window`)
	}
	if (&SSOSettings{OnExpiry: "ERROR"}).InteractiveOnExpiry() {
		t.Error(`OnExpiry "ERROR": want no window`)
	}
}

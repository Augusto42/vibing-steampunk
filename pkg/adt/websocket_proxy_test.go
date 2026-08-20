package adt

import (
	"crypto/tls"
	"testing"
)

// TestWebSocketDialerHonoursProxyEnv guards the regression where the ZADT_VSP
// WebSocket dialer carried no proxy resolver: behind a corporate proxy the upgrade
// failed with no useful diagnosis, while every plain HTTP call went through, because
// the ADT client sets Proxy and this one did not.
//
// It asserts the wiring rather than a resolution, because http.ProxyFromEnvironment
// reads the environment once per process — a test cannot meaningfully vary it.
func TestWebSocketDialerHonoursProxyEnv(t *testing.T) {
	d := newWebSocketDialer(&tls.Config{}) //nolint:gosec // test config, no handshake
	if d.Proxy == nil {
		t.Fatal("the WebSocket dialer must resolve a proxy from the environment")
	}
	if d.HandshakeTimeout == 0 {
		t.Fatal("the dialer must keep its handshake timeout")
	}
}

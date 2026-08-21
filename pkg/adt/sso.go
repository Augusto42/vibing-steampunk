package adt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// SSOOptions configures a browser-driven SSO cookie capture.
//
// The capture is deliberately generic: nothing about a particular SAP system,
// identity provider or tenant is baked in. Everything that identifies a system
// arrives through these fields, so the same helper serves any SAP host behind
// any browser-completable SSO (Entra/SAML, Kerberos/SPNEGO, IAS, Keycloak).
type SSOOptions struct {
	// TriggerURL is any authentication-gated page on the SAP host. Navigating
	// there starts the SSO redirect chain. The Fiori launchpad works well; so
	// does /sap/bc/adt/. Its host also scopes the cookies we harvest.
	TriggerURL string

	// UserDataDir is the browser profile directory. A persistent directory lets
	// a later run reuse the identity provider's session cookies and finish
	// silently. Empty means a fresh throwaway profile — still fine where the
	// device itself carries the credential (e.g. an Entra PRT via WAM), which
	// is device-level rather than profile-level.
	UserDataDir string

	// BrowserPath is the browser executable. Empty auto-detects, preferring
	// Edge. This choice is load-bearing on Entra tenants with device-based
	// Conditional Access: only a real vendor browser bridges to the OS account
	// broker, so a bundled Chromium loops on the IdP forever.
	BrowserPath string

	// Headless runs without a visible window — the silent refresh path. It
	// succeeds only when no interactive step (MFA, consent, password) is due.
	Headless bool

	// Timeout caps the whole capture.
	Timeout time.Duration

	// Insecure skips TLS verification (self-signed SAP certificates).
	Insecure bool

	// Verbose logs navigation hops to stderr, with query strings stripped.
	Verbose bool
}

// ErrSSOInteractiveRequired reports that a silent capture cannot finish because
// the identity provider wants a human — a password, an expired device token, a
// second factor, a consent screen. Callers distinguish it from a real failure
// to decide whether opening a visible browser window would help.
var ErrSSOInteractiveRequired = errors.New("interactive SSO login required")

// DefaultSSOTimeoutSilent bounds a headless attempt. It is short on purpose: a
// silent refresh either sails through the redirect chain in seconds or has
// parked on a login form that no amount of waiting will clear.
const DefaultSSOTimeoutSilent = 45 * time.Second

// DefaultSSOTimeoutInteractive bounds a headed attempt, leaving room for a
// human to work through a password prompt and a second factor.
const DefaultSSOTimeoutInteractive = 5 * time.Minute

// CaptureSSOCookies drives a browser through an SSO handshake and returns the
// resulting SAP session cookies.
//
// What comes back is the *result* of the handshake, not the handshake itself:
// plain host cookies (SAP_SESSIONID_<SID>_<client>, MYSAPSSO2) that are not
// bound to the device or the browser profile. They can be replayed by any HTTP
// client until the server expires them. The SAML assertion that produced them
// is single-use and short-lived, and is never stored.
func CaptureSSOCookies(ctx context.Context, opts SSOOptions) (map[string]string, error) {
	u, err := url.Parse(opts.TriggerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid trigger URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid trigger URL (missing scheme or host): %s", opts.TriggerURL)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		if opts.Headless {
			timeout = DefaultSSOTimeoutSilent
		} else {
			timeout = DefaultSSOTimeoutInteractive
		}
	}

	execPath := opts.BrowserPath
	browserName := "browser"
	if execPath == "" {
		found, name := FindBrowser()
		if found == "" {
			return nil, fmt.Errorf("no Chromium-based browser found — install Microsoft Edge (needed for device-based Conditional Access) or pass an explicit path")
		}
		execPath, browserName = found, name
	} else {
		if _, err := os.Stat(execPath); err != nil {
			return nil, fmt.Errorf("browser executable not found: %s", execPath)
		}
		browserName = friendlyBrowserName(execPath)
	}

	// chromedp's defaults mirror the flag set Puppeteer and Playwright use, which
	// is the configuration the Entra broker path was verified against. Keep them
	// and add only what this flow needs.
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.Flag("headless", opts.Headless),
		// SAP ICF hands out Negotiate only to recognized ADT clients on some
		// systems; a temporary profile also lacks the system's Integrated
		// Windows Auth allowlist, so name the host explicitly.
		chromedp.Flag("auth-server-whitelist", u.Host),
		chromedp.Flag("auth-negotiate-delegate-whitelist", u.Host),
	)
	if opts.UserDataDir != "" {
		if err := os.MkdirAll(opts.UserDataDir, 0700); err != nil {
			return nil, fmt.Errorf("creating browser profile dir: %w", err)
		}
		allocOpts = append(allocOpts, chromedp.UserDataDir(opts.UserDataDir))
	}
	if opts.Insecure {
		allocOpts = append(allocOpts, chromedp.Flag("ignore-certificate-errors", true))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()
	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, timeout)
	defer timeoutCancel()

	mode := "headed"
	if opts.Headless {
		mode = "silent"
	}
	fmt.Fprintf(os.Stderr, "[SSO] %s, %s mode, %s budget → %s\n",
		browserName, mode, timeout, sanitizeURLForLog(opts.TriggerURL))

	if opts.Verbose {
		logNavigation(timeoutCtx)
	}

	// SSO handshakes routinely abort the initial navigation (401 challenge,
	// redirect to the IdP, a download-typed response). The browser carries on
	// regardless, so a navigation error here is not fatal — the cookie poll
	// below is the real verdict.
	if err := chromedp.Run(timeoutCtx, chromedp.Navigate(opts.TriggerURL)); err != nil {
		if timeoutCtx.Err() != nil {
			return nil, fmt.Errorf("SSO capture timed out after %s", timeout)
		}
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "[SSO] navigation reported %v (normal during SSO)\n", err)
		}
	}

	cookies, err := pollForSAPCookies(timeoutCtx, opts.TriggerURL, opts.Verbose)
	if err != nil {
		if opts.Headless {
			return nil, fmt.Errorf("%w: %v", ErrSSOInteractiveRequired, err)
		}
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "[SSO] captured %d cookies\n", len(cookies))
	return cookies, nil
}

// DefaultSSOProfileDir returns a per-host browser profile directory under the
// user's cache location. Keeping one profile per host means a silent refresh
// for one system is never disturbed by a login to another.
func DefaultSSOProfileDir(host string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	if host == "" {
		host = "default"
	}
	return filepath.Join(base, "vsp-sso", "profile-"+sanitizeHostForPath(host)), nil
}

// sanitizeHostForPath makes a hostname safe to use as a single path element.
func sanitizeHostForPath(host string) string {
	out := make([]rune, 0, len(host))
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// logNavigation streams the redirect chain to stderr. Query strings are
// stripped: in redirect-binding SAML flows they carry the assertion itself.
func logNavigation(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *page.EventFrameNavigated:
			if e.Frame != nil && e.Frame.URL != "" {
				fmt.Fprintf(os.Stderr, "[SSO]   → %s\n", sanitizeURLForLog(e.Frame.URL))
			}
		case *network.EventResponseReceived:
			if e.Response != nil && e.Response.Status >= 300 && e.Response.Status < 400 {
				fmt.Fprintf(os.Stderr, "[SSO]   %d → %s\n", e.Response.Status, sanitizeURLForLog(e.Response.URL))
			}
		}
	})
}

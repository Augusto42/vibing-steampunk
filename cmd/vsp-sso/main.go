// Command vsp-sso captures SAP session cookies through a browser SSO handshake
// and prints them as JSON.
//
// It exists as a separate, small binary because the browser step is not always
// runnable where vsp itself runs. Under WSL the credential that satisfies
// device-based Conditional Access — an Entra Primary Refresh Token, held by the
// Windows account broker — is reachable only from a Windows process. So vsp
// stages this helper on the Windows side, runs it through interop, and reads
// the cookies back over stdout. Nothing but cookies crosses the boundary.
//
// The helper is system-agnostic: every identifier arrives by flag.
//
//	vsp-sso --url https://sap.example/sap/bc/ui2/flp?sap-client=100          # silent
//	vsp-sso --url https://sap.example/sap/bc/ui2/flp?sap-client=100 --headed # interactive
//
// Exit codes: 0 captured, 2 interactive login required, 1 anything else.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// exitInteractiveRequired signals that a silent capture is impossible right now
// because the identity provider needs a human. Callers use it to decide whether
// to escalate to a visible browser window rather than reporting a hard failure.
const exitInteractiveRequired = 2

func main() {
	var (
		triggerURL = flag.String("url", "", "authentication-gated URL that starts the SSO redirect (required)")
		profile    = flag.String("profile", "", "browser profile directory (default: per-host dir under the user cache)")
		headed     = flag.Bool("headed", false, "show the browser window so a human can complete the login")
		timeout    = flag.Duration("timeout", 0, "capture budget (default: 45s silent, 5m headed)")
		browser    = flag.String("browser", "", "browser executable (default: auto-detect, preferring Edge)")
		insecure   = flag.Bool("insecure", false, "skip TLS certificate verification")
		verbose    = flag.Bool("verbose", false, "log the redirect chain to stderr")
		out        = flag.String("out", "", "write the JSON to this file instead of stdout")
	)
	flag.Parse()

	if *triggerURL == "" {
		fmt.Fprintln(os.Stderr, "vsp-sso: --url is required")
		flag.Usage()
		os.Exit(1)
	}
	u, err := url.Parse(*triggerURL)
	if err != nil || u.Host == "" {
		fmt.Fprintf(os.Stderr, "vsp-sso: invalid --url: %s\n", *triggerURL)
		os.Exit(1)
	}

	profileDir := *profile
	if profileDir == "" {
		if profileDir, err = adt.DefaultSSOProfileDir(u.Hostname()); err != nil {
			fmt.Fprintf(os.Stderr, "vsp-sso: %v\n", err)
			os.Exit(1)
		}
	}

	// Ctrl-C must reach the browser: an orphaned headed window would hold the
	// profile lock and make the next run fail for a reason nobody would guess.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cookies, err := adt.CaptureSSOCookies(ctx, adt.SSOOptions{
		TriggerURL:  *triggerURL,
		UserDataDir: profileDir,
		BrowserPath: *browser,
		Headless:    !*headed,
		Timeout:     *timeout,
		Insecure:    *insecure,
		Verbose:     *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "vsp-sso: %v\n", err)
		if errors.Is(err, adt.ErrSSOInteractiveRequired) {
			fmt.Fprintf(os.Stderr, "vsp-sso: retry with --headed to complete the login\n")
			os.Exit(exitInteractiveRequired)
		}
		os.Exit(1)
	}

	payload := struct {
		Host       string            `json:"host"`
		CapturedAt string            `json:"captured_at"`
		Cookies    map[string]string `json:"cookies"`
	}{
		Host:       u.Hostname(),
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Cookies:    cookies,
	}

	blob, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vsp-sso: encoding result: %v\n", err)
		os.Exit(1)
	}

	if *out != "" {
		// Session cookies authenticate as the user with no password. Create the
		// file owner-only, and before writing anything into it.
		if err := os.WriteFile(*out, append(blob, '\n'), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "vsp-sso: writing %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "vsp-sso: wrote %s\n", *out)
		return
	}
	// stdout carries the payload alone — every diagnostic goes to stderr, so a
	// caller can pipe this straight into a JSON parser.
	fmt.Println(string(blob))
}

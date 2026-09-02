package adt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SSOConfig describes how to obtain a browser session for one SAP system.
type SSOConfig struct {
	// System names the cache entry. Sessions for different systems never share
	// a file, so refreshing one cannot invalidate another.
	System string

	// BaseURL is the SAP system URL. Used to derive the trigger URL and to
	// scope the captured cookies.
	BaseURL string

	// Client is the SAP client, appended to a derived trigger URL so the
	// handshake lands in the right client.
	Client string

	// TriggerURL overrides the derived authentication-gated URL. Set it when
	// the ADT node is not the right entry point — a Fiori launchpad URL, for
	// instance, on systems that gate SSO there.
	TriggerURL string

	// Profile is the browser profile directory. On WSL this is a Windows path,
	// since the browser is a Windows process. Empty lets the capture pick a
	// per-host directory of its own.
	Profile string

	// HelperPath points at the Windows capture helper. Empty auto-discovers it.
	HelperPath string

	// Interactive permits escalating to a visible browser window when a silent
	// capture cannot finish. Off means such a case is reported as an error
	// naming the command to run, which is the right behaviour where no one is
	// watching — a scheduled run, a pipeline.
	Interactive bool

	// SilentTimeout and InteractiveTimeout override the capture budgets.
	SilentTimeout      time.Duration
	InteractiveTimeout time.Duration

	Insecure bool
	Verbose  bool
}

// SSOProvider supplies SAP session cookies obtained through a browser SSO
// handshake, caching them between runs and re-capturing when they expire.
//
// One provider serves a whole vsp process: the cached session is shared, and a
// refresh triggered by one request is seen by the rest.
type SSOProvider struct {
	cfg       SSOConfig
	cachePath string

	mu      sync.Mutex
	current map[string]string
}

// NewSSOProvider builds a provider for one system.
func NewSSOProvider(cfg SSOConfig) (*SSOProvider, error) {
	if cfg.BaseURL == "" && cfg.TriggerURL == "" {
		return nil, errors.New("SSO needs either a system URL or an explicit trigger URL")
	}
	cacheKey := cfg.System
	if cacheKey == "" {
		if u, err := url.Parse(cfg.BaseURL); err == nil {
			cacheKey = u.Hostname()
		}
	}
	cachePath, err := SSOCachePath(cacheKey)
	if err != nil {
		return nil, err
	}
	return &SSOProvider{cfg: cfg, cachePath: cachePath}, nil
}

// CachePath returns the file backing this provider's session cache.
func (p *SSOProvider) CachePath() string { return p.cachePath }

// ReauthBudget is how long a caller should allow for one recovery.
//
// It covers the silent attempt and, where the system permits one, the window a
// person then signs in through — which is the term that dominates, since it is
// paced by a human reaching for a second factor rather than by any network.
func (p *SSOProvider) ReauthBudget() time.Duration {
	budget := p.cfg.SilentTimeout
	if budget <= 0 {
		budget = DefaultSSOTimeoutSilent
	}
	if p.cfg.Interactive {
		interactive := p.cfg.InteractiveTimeout
		if interactive <= 0 {
			interactive = DefaultSSOTimeoutInteractive
		}
		budget += interactive
	}
	// Staging the helper and starting a browser are not free; leave room.
	return budget + 30*time.Second
}

// triggerURL returns the URL whose loading starts the SSO redirect chain.
//
// The ADT root is the default rather than a Fiori launchpad: any system vsp can
// talk to has it, it is authentication-gated, and it renders as a page instead
// of a download — a distinction that matters, because a response the browser
// treats as a file leaves the tab on about:blank with no cookies to read.
func (p *SSOProvider) triggerURL() (string, error) {
	if p.cfg.TriggerURL != "" {
		return p.cfg.TriggerURL, nil
	}
	u, err := url.Parse(p.cfg.BaseURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("cannot derive an SSO trigger URL from %q", p.cfg.BaseURL)
	}
	u.Path = "/sap/bc/adt/"
	u.Fragment = ""
	q := url.Values{}
	if p.cfg.Client != "" {
		q.Set("sap-client", p.cfg.Client)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Cookies returns session cookies, preferring the cache. A cached session is
// used whatever its age: only the server knows when it expired, and finding out
// costs a request that the caller is about to make anyway. Staleness is handled
// on the way back, when that request comes home unauthenticated.
func (p *SSOProvider) Cookies(ctx context.Context) (map[string]string, error) {
	p.mu.Lock()
	if len(p.current) > 0 {
		cookies := p.current
		p.mu.Unlock()
		return cookies, nil
	}
	p.mu.Unlock()

	sess, err := LoadSSOSession(p.cachePath)
	if err != nil {
		return nil, err
	}
	if sess != nil {
		p.mu.Lock()
		p.current = sess.Cookies
		p.mu.Unlock()
		if p.cfg.Verbose {
			fmt.Fprintf(os.Stderr, "[SSO] reusing cached session (%s old) from %s\n",
				sess.Age().Truncate(time.Second), p.cachePath)
		}
		return sess.Cookies, nil
	}
	return p.Refresh(ctx)
}

// Refresh captures a new session, ignoring whatever is cached, and stores it.
//
// This is what a client hands to WithReauthFunc: the HTTP layer calls it after
// a request comes back unauthenticated, then retries with what it returns.
func (p *SSOProvider) Refresh(ctx context.Context) (map[string]string, error) {
	return p.acquire(ctx, false)
}

// Login captures a session with a visible browser window, skipping the silent
// attempt. Use it when a person is present and expects to sign in — the first
// setup of a system, or recovery after the silent path has already failed.
func (p *SSOProvider) Login(ctx context.Context) (map[string]string, error) {
	return p.acquire(ctx, true)
}

// Clear removes the cached session, so the next call captures a fresh one.
func (p *SSOProvider) Clear() error {
	p.mu.Lock()
	p.current = nil
	p.mu.Unlock()
	if err := os.Remove(p.cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing SSO cache: %w", err)
	}
	return nil
}

func (p *SSOProvider) acquire(ctx context.Context, headed bool) (map[string]string, error) {
	trigger, err := p.triggerURL()
	if err != nil {
		return nil, err
	}

	cookies, err := p.capture(ctx, trigger, headed)
	if !headed && errors.Is(err, ErrSSOInteractiveRequired) && p.cfg.Interactive {
		fmt.Fprintf(os.Stderr, "[SSO] silent refresh needs a human — opening a browser window\n")
		cookies, err = p.capture(ctx, trigger, true)
	}
	if err != nil {
		if errors.Is(err, ErrSSOInteractiveRequired) {
			return nil, fmt.Errorf("%w — run `vsp sso login%s` to sign in once", err, p.systemFlag())
		}
		return nil, err
	}

	host := ""
	if u, perr := url.Parse(trigger); perr == nil {
		host = u.Hostname()
	}
	if err := SaveSSOSession(p.cachePath, host, cookies); err != nil {
		// A session we hold but failed to cache is still usable for this run.
		fmt.Fprintf(os.Stderr, "[SSO] warning: %v\n", err)
	}

	p.mu.Lock()
	p.current = cookies
	p.mu.Unlock()
	return cookies, nil
}

func (p *SSOProvider) systemFlag() string {
	if p.cfg.System == "" {
		return ""
	}
	return " -s " + p.cfg.System
}

// capture runs one handshake, either in this process or — under WSL — in a
// Windows helper process.
func (p *SSOProvider) capture(ctx context.Context, trigger string, headed bool) (map[string]string, error) {
	if IsWSL() {
		return p.captureViaHelper(ctx, trigger, headed)
	}
	return CaptureSSOCookies(ctx, SSOOptions{
		TriggerURL:  trigger,
		UserDataDir: p.cfg.Profile,
		Headless:    !headed,
		Timeout:     p.timeout(headed),
		Insecure:    p.cfg.Insecure,
		Verbose:     p.cfg.Verbose,
	})
}

func (p *SSOProvider) timeout(headed bool) time.Duration {
	if headed {
		return p.cfg.InteractiveTimeout
	}
	return p.cfg.SilentTimeout
}

// captureViaHelper runs the capture as a Windows process and reads the cookies
// back over its stdout.
//
// Only cookies cross the boundary. Nothing is written to the Windows
// filesystem, where a secrets file would sit outside this user's Linux
// permissions and outside the hygiene the rest of vsp keeps.
func (p *SSOProvider) captureViaHelper(ctx context.Context, trigger string, headed bool) (map[string]string, error) {
	helper, err := FindSSOHelper(p.cfg.HelperPath)
	if err != nil {
		return nil, err
	}
	staged, err := StageSSOHelper(ctx, helper)
	if err != nil {
		return nil, err
	}

	args := []string{"--url", trigger}
	if headed {
		args = append(args, "--headed")
	}
	if p.cfg.Profile != "" {
		args = append(args, "--profile", p.cfg.Profile)
	}
	if timeout := p.timeout(headed); timeout > 0 {
		args = append(args, "--timeout", timeout.String())
	}
	if p.cfg.Insecure {
		args = append(args, "--insecure")
	}
	if p.cfg.Verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.CommandContext(ctx, staged, args...)
	// Run from the staging directory: started from a Linux working directory,
	// a Windows process gets a UNC-path warning and an unrelated fallback cwd.
	cmd.Dir = filepath.Dir(staged)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if p.cfg.Verbose && stderr.Len() > 0 {
		os.Stderr.Write(stderr.Bytes())
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 2 {
			return nil, ErrSSOInteractiveRequired
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return nil, fmt.Errorf("SSO helper failed: %s", detail)
	}

	var payload SSOSession
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &payload); err != nil {
		return nil, fmt.Errorf("SSO helper returned unreadable output: %w", err)
	}
	if len(payload.Cookies) == 0 {
		return nil, errors.New("SSO helper returned no cookies")
	}
	return payload.Cookies, nil
}

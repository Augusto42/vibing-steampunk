package adt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SSOSession is a captured browser session, as cached between vsp runs.
//
// The cookies inside authenticate as the user with no password, so the cache is
// a credential file: it is created owner-only and lives outside any repository.
type SSOSession struct {
	Host       string            `json:"host"`
	CapturedAt time.Time         `json:"captured_at"`
	Cookies    map[string]string `json:"cookies"`
}

// Age reports how long ago the session was captured.
func (s *SSOSession) Age() time.Duration {
	if s == nil || s.CapturedAt.IsZero() {
		return 0
	}
	return time.Since(s.CapturedAt)
}

// SSOCachePath returns the cache file for a named system. Callers that have no
// system name (a bare --url run) can pass the hostname instead.
func SSOCachePath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory for the SSO cache: %w", err)
	}
	if name == "" {
		name = "default"
	}
	return filepath.Join(home, ".vsp", "sso", sanitizeHostForPath(name)+".json"), nil
}

// LoadSSOSession reads a cached session. A missing file is not an error — it
// reports (nil, nil), which callers read as "nothing cached, go capture one".
func LoadSSOSession(path string) (*SSOSession, error) {
	blob, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading SSO cache %s: %w", path, err)
	}
	var sess SSOSession
	if err := json.Unmarshal(blob, &sess); err != nil {
		// A corrupt cache should never be fatal: it is a cache. Report it as
		// absent so the caller re-captures instead of failing the whole run.
		return nil, nil
	}
	if len(sess.Cookies) == 0 {
		return nil, nil
	}
	return &sess, nil
}

// SaveSSOSession writes the session to the cache, owner-only.
func SaveSSOSession(path, host string, cookies map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating SSO cache directory: %w", err)
	}
	blob, err := json.MarshalIndent(SSOSession{
		Host:       host,
		CapturedAt: time.Now().UTC(),
		Cookies:    cookies,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding SSO cache: %w", err)
	}
	// Write through a temporary file so a concurrent reader never sees a half
	// written cache, and so the 0600 mode is in place before any cookie is.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(blob, '\n'), 0600); err != nil {
		return fmt.Errorf("writing SSO cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("installing SSO cache: %w", err)
	}
	return nil
}

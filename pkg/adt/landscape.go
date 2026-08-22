package adt

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// SAP GUI keeps the list of systems a developer can reach in SAPUILandscape.xml —
// which is exactly the information vsp otherwise asks the user to retype as a
// URL, a client and an instance number. Reading it turns "configure a system"
// into "name a system".
//
// The file describes SAP GUI connectivity: message servers, application servers,
// routers, SNC partner names. It says nothing about HTTP. But an instance number
// is derivable from the ports it does give, and SAP's own port convention then
// yields the HTTP and HTTPS ports — a candidate to try, never a certainty, which
// is why the derived addresses are offered as candidates and probed rather than
// written down as fact.

// LandscapeService is one entry a user picks in SAP Logon.
type LandscapeService struct {
	UUID     string `xml:"uuid,attr"`
	Name     string `xml:"name,attr"`
	SystemID string `xml:"systemid,attr"`
	Type     string `xml:"type,attr"`
	// Server is "host:port" for a direct application-server connection.
	Server string `xml:"server,attr"`
	// MessageServerID points at a Messageserver entry for a load-balanced one.
	MessageServerID string `xml:"msid,attr"`
	RouterID        string `xml:"routerid,attr"`
	Group           string `xml:"group,attr"`
	SNCName         string `xml:"sncname,attr"`
	SNCOp           string `xml:"sncop,attr"`
}

// LandscapeMessageServer is a system's message server.
type LandscapeMessageServer struct {
	UUID string `xml:"uuid,attr"`
	Name string `xml:"name,attr"`
	Host string `xml:"host,attr"`
	Port string `xml:"port,attr"`
}

// LandscapeRouter is a SAProuter entry.
type LandscapeRouter struct {
	UUID   string `xml:"uuid,attr"`
	Name   string `xml:"name,attr"`
	Router string `xml:"router,attr"`
}

// LandscapeInclude points at another landscape file, typically a shared one on a
// company file server that holds the systems everyone uses.
type LandscapeInclude struct {
	URL string `xml:"url,attr"`
}

// LandscapeFile is a parsed SAPUILandscape.xml.
type LandscapeFile struct {
	XMLName        xml.Name                 `xml:"Landscape"`
	Includes       []LandscapeInclude       `xml:"Includes>Include"`
	Services       []LandscapeService       `xml:"Services>Service"`
	MessageServers []LandscapeMessageServer `xml:"Messageservers>Messageserver"`
	Routers        []LandscapeRouter        `xml:"Routers>Router"`
}

// LandscapeSystem is one system, with everything resolved: the service, the
// message server or application server behind it, and the router in front.
type LandscapeSystem struct {
	SystemID    string
	Name        string
	Host        string
	InstanceNr  string // two digits, empty when it cannot be derived
	Router      string
	SNCName     string
	LoadBalance bool // reached through a message server rather than an app server
	Source      string
}

// ParseLandscape reads a landscape file.
func ParseLandscape(path string) (*LandscapeFile, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading landscape file %s: %w", path, err)
	}
	return ParseLandscapeBytes(blob, path)
}

// ParseLandscapeBytes parses landscape XML that was read from somewhere other
// than the local filesystem.
func ParseLandscapeBytes(blob []byte, source string) (*LandscapeFile, error) {
	var lf LandscapeFile
	if err := xml.Unmarshal(blob, &lf); err != nil {
		return nil, fmt.Errorf("parsing landscape file %s: %w", source, err)
	}
	return &lf, nil
}

// Systems resolves the file's services into systems.
func (lf *LandscapeFile) Systems(source string) []LandscapeSystem {
	byMS := make(map[string]LandscapeMessageServer, len(lf.MessageServers))
	for _, m := range lf.MessageServers {
		byMS[m.UUID] = m
	}
	byRouter := make(map[string]LandscapeRouter, len(lf.Routers))
	for _, r := range lf.Routers {
		byRouter[r.UUID] = r
	}

	out := make([]LandscapeSystem, 0, len(lf.Services))
	for _, s := range lf.Services {
		// A shared landscape is edited by many hands over years, and some
		// entries carry a system id of nothing but spaces. Left untrimmed it
		// passes an emptiness check and turns into a nameless row.
		systemID := strings.ToUpper(strings.TrimSpace(s.SystemID))
		if systemID == "" {
			// SAP GUI for Java does not write systemid at all — its entries
			// carry the system in name ("A4H"). Only accept a name that is
			// shaped like a system id: on Windows files name is a free-text
			// description, and an entry whose systemid is blank there is meant
			// to be dropped, not renamed after its description.
			if candidate := strings.ToUpper(strings.TrimSpace(s.Name)); looksLikeSystemID(candidate) {
				systemID = candidate
			}
		}
		if systemID == "" {
			continue
		}
		sys := LandscapeSystem{
			SystemID: systemID,
			Name:     strings.TrimSpace(s.Name),
			SNCName:  strings.TrimSpace(s.SNCName),
			Source:   source,
		}
		if r, ok := byRouter[s.RouterID]; ok {
			sys.Router = r.Router
		}
		switch {
		case s.MessageServerID != "":
			// Load balanced: the message server's port carries the instance,
			// 3600 + nn.
			if m, ok := byMS[s.MessageServerID]; ok {
				sys.Host = strings.TrimSpace(m.Host)
				sys.InstanceNr = instanceFromPort(m.Port, 3600)
				sys.LoadBalance = true
			}
		case s.Server != "":
			// Direct: "host:port", where the dispatcher port is 3200 + nn.
			host, port := splitHostPort(strings.TrimSpace(s.Server))
			sys.Host = strings.TrimSpace(host)
			sys.InstanceNr = instanceFromPort(port, 3200)
		}
		if sys.Host == "" {
			continue
		}
		out = append(out, sys)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SystemID != out[j].SystemID {
			return out[i].SystemID < out[j].SystemID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// looksLikeSystemID reports whether a string has the shape of a SAP system id:
// three alphanumerics. It is the only thing that makes a name safe to use when
// systemid is missing.
func looksLikeSystemID(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// splitHostPort splits "host:port", tolerating a bare host.
func splitHostPort(server string) (host, port string) {
	if i := strings.LastIndex(server, ":"); i > 0 {
		return server[:i], server[i+1:]
	}
	return server, ""
}

// instanceFromPort recovers the instance number from a SAP port built as
// base + instance. Anything that does not fit the shape yields "", because a
// wrong instance number produces a wrong URL that fails in a puzzling way.
func instanceFromPort(port string, base int) string {
	n, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return ""
	}
	nr := n - base
	if nr < 0 || nr > 99 {
		return ""
	}
	return fmt.Sprintf("%02d", nr)
}

// CandidateURLs returns the addresses worth trying for ADT, most likely first.
//
// SAP's ICM numbers its ports by instance — HTTPS at 443nn, HTTP at 80nn — so an
// instance number is enough to guess. It is only a guess: a system fronted by a
// web dispatcher answers on 443 instead, and some landscapes move the ports
// outright, which is why these are candidates to probe rather than an answer.
func (s LandscapeSystem) CandidateURLs(domains ...string) []string {
	var urls []string
	for _, host := range s.hostCandidates(domains) {
		if s.InstanceNr != "" {
			urls = append(urls,
				fmt.Sprintf("https://%s:443%s", host, s.InstanceNr),
				fmt.Sprintf("http://%s:80%s", host, s.InstanceNr),
			)
		}
		// A web dispatcher, or an ICM left on the default ports, answers here.
		urls = append(urls, fmt.Sprintf("https://%s", host))
	}
	return urls
}

// hostCandidates returns the names worth trying for this system's host.
//
// A landscape file records the short host name, because SAP GUI resolves it
// through the workstation's DNS suffix. A tool that inherits no suffix — which
// is the ordinary case under WSL — cannot resolve that name at all, so the
// qualified forms are tried first and the bare name kept as a fallback for
// networks where it does resolve.
func (s LandscapeSystem) hostCandidates(domains []string) []string {
	if s.Host == "" {
		return nil
	}
	if strings.Contains(s.Host, ".") || len(domains) == 0 {
		return []string{s.Host}
	}
	hosts := make([]string, 0, len(domains)+1)
	for _, d := range domains {
		if d = strings.Trim(strings.TrimSpace(d), "."); d != "" {
			hosts = append(hosts, s.Host+"."+d)
		}
	}
	return append(hosts, s.Host)
}

// DNSSearchDomains returns the domains a short host name might live under.
//
// Under WSL the resolver usually carries no search list, while the Windows host
// it sits on is domain-joined and knows exactly which domain that is — so the
// answer is one interop call away, and without it every short name in a
// corporate landscape is unreachable.
func DNSSearchDomains(ctx context.Context) []string {
	var domains []string
	seen := map[string]bool{}
	add := func(d string) {
		d = strings.Trim(strings.TrimSpace(d), ".")
		if d == "" || seen[strings.ToLower(d)] {
			return
		}
		seen[strings.ToLower(d)] = true
		domains = append(domains, d)
	}

	if blob, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(blob), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch strings.ToLower(fields[0]) {
			case "search":
				for _, d := range fields[1:] {
					add(d)
				}
			case "domain":
				add(fields[1])
			}
		}
	}

	if IsWSL() {
		// Ask Windows for its DNS suffixes, not for its domain: a device joined
		// to Entra rather than to Active Directory answers "WORKGROUP" for the
		// latter, and appending that to a host name produces a name that cannot
		// resolve anywhere. The suffix search list and the per-connection
		// suffix are what the workstation itself uses to reach a short name.
		script := "@((Get-DnsClientGlobalSetting).SuffixSearchList) + " +
			"@(Get-DnsClient | Select-Object -ExpandProperty ConnectionSpecificSuffix) -join \"`n\""
		out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive",
			"-Command", script).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				add(line)
			}
		}
	}

	// A suffix with no dot in it is a workgroup or NetBIOS name, never a DNS
	// domain, and qualifying a host with one only wastes a lookup.
	usable := domains[:0]
	for _, d := range domains {
		if strings.Contains(d, ".") {
			usable = append(usable, d)
		}
	}
	return usable
}

// landscapeFileName is the file SAP GUI writes.
const landscapeFileName = "SAPUILandscape.xml"

// javaLandscapeFileName is what SAP GUI for Java writes instead — same schema,
// different name.
const javaLandscapeFileName = "SAPGUILandscape.xml"

// FindLandscapeFiles returns the landscape files worth reading, most specific
// first: an explicit path, then SAPLOGON_LSXML_FILE, then the per-platform
// default location.
//
// Under WSL the default location is on the Windows side. A developer running vsp
// in Linux still logs on with SAP GUI for Windows, so the list of systems lives
// across the boundary — and looking only at the Linux home would find nothing
// while the file sits a directory traversal away.
func FindLandscapeFiles(ctx context.Context, explicit string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	if fromEnv := os.Getenv("SAPLOGON_LSXML_FILE"); fromEnv != "" {
		return []string{fromEnv}
	}

	var candidates []string
	switch {
	case IsWSL():
		if appData, err := windowsEnvVar(ctx, "APPDATA"); err == nil {
			if linux, err := windowsPathToLinux(ctx, appData); err == nil {
				candidates = append(candidates, filepath.Join(linux, "SAP", "Common", landscapeFileName))
			}
		}
	case runtime.GOOS == "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "SAP", "Common", landscapeFileName))
		}
	default:
		// SAP GUI for Java keeps its own copy, and names it differently:
		// SAPGUILandscape.xml, not the SAPUILandscape.xml Windows writes.
		// On macOS it lives under Library/Preferences, not a dot-directory.
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, "Library", "Preferences", "SAP", javaLandscapeFileName),
				filepath.Join(home, "Library", "Preferences", "SAP", landscapeFileName),
				filepath.Join(home, ".SAPGUI", "Configuration", javaLandscapeFileName),
				filepath.Join(home, ".SAPGUI", "Configuration", landscapeFileName),
				filepath.Join(home, ".sapgui", javaLandscapeFileName),
				filepath.Join(home, ".sapgui", landscapeFileName))
		}
	}

	found := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			found = append(found, c)
		}
	}
	return found
}

// ReadLandscapeInclude fetches the contents an <Include url="..."> points at.
//
// Only file:// includes are followed. A landscape can also name an http(s)
// address, and fetching that would have this tool make a network request to
// whatever a config file told it to — worth doing deliberately, never as a side
// effect of listing systems.
//
// The shared landscape usually lives on a company file share, so the URL is a
// Windows UNC path. WSL cannot mount one, and the contents are returned rather
// than a path precisely so that reaching it never means leaving a copy of a
// company's system list in a temporary directory.
func ReadLandscapeInclude(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("landscape include %q: %w", rawURL, err)
	}
	if !strings.EqualFold(u.Scheme, "file") {
		return nil, fmt.Errorf("landscape include %q is not a file:// URL", rawURL)
	}

	// file:///path is an ordinary local file; file://server/share/path is UNC.
	if u.Host == "" {
		return os.ReadFile(filepath.FromSlash(u.Path))
	}
	winPath := `\\` + u.Host + strings.ReplaceAll(u.Path, "/", `\`)

	switch {
	case runtime.GOOS == "windows":
		return os.ReadFile(winPath)
	case IsWSL():
		return readWindowsFile(ctx, winPath)
	}
	return nil, fmt.Errorf("landscape include %q is a Windows share, unreachable from this platform", rawURL)
}

// readWindowsFile reads a file that only the Windows side can reach, returning
// its bytes without staging a copy anywhere. The content crosses base64-encoded
// so that a byte-order mark or a UTF-16 encoding survives the pipe intact.
func readWindowsFile(ctx context.Context, winPath string) ([]byte, error) {
	script := fmt.Sprintf(
		"[Convert]::ToBase64String([System.IO.File]::ReadAllBytes(%s))",
		powerShellQuote(winPath))
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Dir = "/mnt/c"
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s from the Windows side: %w", winPath, err)
	}
	encoded := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, string(out))
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", winPath, err)
	}
	return blob, nil
}

// powerShellQuote renders a string as a PowerShell single-quoted literal.
func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

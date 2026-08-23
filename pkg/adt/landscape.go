package adt

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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
	// Server is "host:port" for a direct application-server connection — and,
	// for a load-balanced one, the logon group instead. SAP Logon shows it in a
	// column headed "Group/Server" for exactly that reason. Reading it as a
	// host turns a group named "Users" into a hostname, which is how most of
	// this landscape came out addressed to https://Users.
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
	UUID        string `xml:"uuid,attr"`
	Name        string `xml:"name,attr"`
	Host        string `xml:"host,attr"`
	Port        string `xml:"port,attr"`
	Description string `xml:"description,attr"`
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
	Group       string // logon group, for a load-balanced system
	LoadBalance bool   // reached through a message server rather than an app server
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

	seenSID := make(map[string]bool, len(lf.Services))
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
		server := strings.TrimSpace(s.Server)
		if m, ok := byMS[s.MessageServerID]; ok {
			// Load balanced: the message server's port carries the instance,
			// 3600 + nn, and the server attribute names the logon group.
			sys.Host = strings.TrimSpace(m.Host)
			sys.InstanceNr = instanceFromPort(m.Port, 3600)
			sys.LoadBalance = true
			sys.Group = server
		} else if host, port := splitHostPort(server); port != "" {
			// Direct: "host:port", where the dispatcher port is 3200 + nn. The
			// port is what identifies this as an address at all — without one
			// the value is a group name and says nothing about where to
			// connect.
			sys.Host = strings.TrimSpace(host)
			sys.InstanceNr = instanceFromPort(port, 3200)
		} else {
			sys.Group = server
		}
		if sys.Host == "" {
			continue
		}
		seenSID[sys.SystemID] = true
		out = append(out, sys)
	}

	// A system can be declared by its message server alone, with no service
	// entry — SAP Logon lists those under "All SAP Systems" and they are as
	// real as any other. Walking only the services loses them silently: this
	// landscape has 185 message servers behind 153 services.
	for _, m := range lf.MessageServers {
		sid := strings.ToUpper(strings.TrimSpace(m.Name))
		host := strings.TrimSpace(m.Host)
		if sid == "" || host == "" || seenSID[sid] {
			continue
		}
		seenSID[sid] = true
		out = append(out, LandscapeSystem{
			SystemID:    sid,
			Name:        strings.TrimSpace(m.Description),
			Host:        host,
			InstanceNr:  instanceFromPort(m.Port, 3600),
			LoadBalance: true,
			Source:      source,
		})
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
		// The default address comes first, because that is what answers. SAP's
		// port convention — HTTPS at 443nn, HTTP at 80nn — describes an ICM
		// exposed directly, and a corporate landscape rarely is: measured
		// across eight systems here, 443 answered on five and the derived
		// ports on none. They stay in the list because a system whose ICM is
		// reachable does answer there, and one such system is why this
		// derivation exists at all.
		urls = append(urls, fmt.Sprintf("https://%s", host))
		if s.InstanceNr != "" {
			urls = append(urls,
				fmt.Sprintf("https://%s:443%s", host, s.InstanceNr),
				fmt.Sprintf("http://%s:80%s", host, s.InstanceNr),
			)
		}
	}
	return urls
}

// CanonicalHost returns the name a certificate will match.
//
// A landscape records a short host name and the workstation's DNS suffix
// completes it — but the suffix Windows reports is not always the one the host
// lives under. Here the short name resolves through the reported suffix to the
// right address and then fails TLS, because the certificate names the host's
// real domain and not the alias used to reach it. The resolver knows the
// canonical name; asking it costs one lookup and removes the guesswork.
//
// The host is returned unchanged when nothing resolves, so a caller still gets
// something to try rather than an empty string.
func CanonicalHost(ctx context.Context, host string, domains []string) string {
	if host == "" {
		return host
	}
	candidates := []string{host}
	if !strings.Contains(host, ".") {
		for _, d := range domains {
			if d = strings.Trim(strings.TrimSpace(d), "."); d != "" {
				candidates = append(candidates, host+"."+d)
			}
		}
	}
	// A name that does not exist costs the full resolver timeout, and a
	// landscape has a hundred of them. Cap each lookup so an unknown host is
	// answered in a moment rather than after several seconds of waiting for a
	// nameserver that has nothing to say.
	resolver := net.DefaultResolver
	for _, name := range candidates {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		canonical, err := resolver.LookupCNAME(lookupCtx, name)
		cancel()
		if err == nil {
			if canonical = strings.TrimSuffix(canonical, "."); canonical != "" {
				return canonical
			}
		}

		lookupCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
		addrs, err := resolver.LookupHost(lookupCtx, name)
		cancel()
		if err != nil || len(addrs) == 0 {
			continue
		}
		lookupCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
		names, err := resolver.LookupAddr(lookupCtx, addrs[0])
		cancel()
		if err == nil && len(names) > 0 {
			return strings.TrimSuffix(names[0], ".")
		}
		return name
	}
	return host
}

// CanonicalHosts resolves a whole landscape at once.
//
// Sequentially this is a hundred lookups behind one another, and the ones that
// fail are the slow ones — a listing that took a moment becomes a listing that
// looks hung.
func CanonicalHosts(ctx context.Context, systems []LandscapeSystem, domains []string) {
	const workers = 16
	type job struct{ index int }

	jobs := make(chan job)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				systems[j.index].Host = CanonicalHost(ctx, systems[j.index].Host, domains)
			}
		}()
	}
	for i := range systems {
		jobs <- job{index: i}
	}
	close(jobs)
	wg.Wait()
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

	// The cases add up rather than exclude each other. Under WSL the Windows
	// side is where SAP Logon writes, but SAP GUI for Java may also be
	// installed inside the WSL distribution — looking only across the boundary
	// missed it.
	var candidates []string
	// SAP Logon caches the shared landscape it fetches from the company file
	// server, and reads that cache on every start. Taking it too means the
	// systems everyone shares are found on a local disk rather than over a
	// share that may be slow, reached through interop, or simply unavailable —
	// and it is byte for byte the file the include names.
	appDataDirs := func(appData string) {
		candidates = append(candidates, filepath.Join(appData, "SAP", "Common", landscapeFileName))
		candidates = append(candidates, cacheGlob(filepath.Join(appData, "SAP", "LogonServerConfigCache"))...)
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			appDataDirs(appData)
		}
	}

	if IsWSL() {
		if appData, err := windowsEnvVar(ctx, "APPDATA"); err == nil {
			if linux, err := windowsPathToLinux(ctx, appData); err == nil {
				appDataDirs(linux)
			}
		}
	}

	if runtime.GOOS != "windows" {
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

// cacheGlob lists the cached landscape files in a directory. They are named by
// a UUID, so they are found by extension rather than by name.
func cacheGlob(dir string) []string {
	var out []string
	for _, pattern := range []string{"*.XML", "*.xml"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue
		}
		out = append(out, matches...)
	}
	sort.Strings(out)
	return out
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

// LandscapeSource is one place a landscape can be read from, with enough
// detail to decide whether it is the one worth using.
type LandscapeSource struct {
	// Ref is what to pass to --file: a path, or a "parallels:<vm>:<path>".
	Ref string `json:"ref"`
	// Kind says who wrote it: "sapgui-java", "sapgui-windows", "parallels".
	Kind string `json:"kind"`
	// Systems and Includes describe the file without disclosing its contents.
	Systems  int    `json:"systems"`
	Includes int    `json:"includes"`
	Err      string `json:"error,omitempty"`
}

// ReadLandscapeSource reads a source by reference, whether it is a local file
// or one inside a VM.
func ReadLandscapeSource(ctx context.Context, ref string) ([]byte, error) {
	if vm, winPath, ok := ParseParallelsRef(ref); ok {
		return ReadParallelsFile(ctx, vm, winPath)
	}
	return os.ReadFile(ref)
}

// ScanLandscapeSources finds every landscape this machine can reach: the ones
// SAP GUI wrote locally, and the ones inside running VMs. Includes are counted
// but not followed — a scan reports what is here, and following a company
// share is a separate, slower decision.
func ScanLandscapeSources(ctx context.Context) []LandscapeSource {
	var out []LandscapeSource

	add := func(ref, kind string) {
		src := LandscapeSource{Ref: ref, Kind: kind}
		blob, err := ReadLandscapeSource(ctx, ref)
		if err != nil {
			src.Err = err.Error()
			out = append(out, src)
			return
		}
		lf, err := ParseLandscapeBytes(blob, ref)
		if err != nil {
			src.Err = err.Error()
			out = append(out, src)
			return
		}
		src.Systems = len(lf.Systems(ref))
		src.Includes = len(lf.Includes)
		out = append(out, src)
	}

	for _, p := range FindLandscapeFiles(ctx, "") {
		kind := "sapgui-windows"
		if strings.HasSuffix(p, javaLandscapeFileName) {
			kind = "sapgui-java"
		}
		add(p, kind)
	}

	// A discovery step that fails becomes a source that could not be read,
	// rather than a guest that quietly is not there. `landscape scan` already
	// has a column for exactly this, and its "No landscape found" line is what
	// a silent drop turned into.
	guests, err := ParallelsGuests(ctx)
	if err != nil {
		out = append(out, LandscapeSource{Kind: "parallels", Ref: "(guest discovery)", Err: err.Error()})
	}
	for _, vm := range guests {
		paths, err := ParallelsLandscapeFiles(ctx, vm)
		if err != nil {
			out = append(out, LandscapeSource{Kind: "parallels", Ref: ParallelsRef(vm, ""), Err: err.Error()})
			continue
		}
		for _, winPath := range paths {
			add(ParallelsRef(vm, winPath), "parallels")
		}
	}

	return out
}

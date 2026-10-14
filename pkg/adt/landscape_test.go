package adt

import (
	"strings"
	"testing"
)

// A landscape in the shape SAP GUI writes, with synthetic systems only.
const sampleLandscape = `<?xml version="1.0"?>
<Landscape updated="2026-01-01T00:00:00Z" version="1">
  <Includes>
    <Include url="file://fileserver.example/public/SAPUILandscape.XML" index="0"/>
  </Includes>
  <Services>
    <Service type="SAPGUI" uuid="s1" name="Dev direct" systemid="DEV" mode="1" server="devsys.example:3201"/>
    <Service type="SAPGUI" uuid="s2" name="Prod balanced" systemid="PRD" msid="m1" routerid="r1"
             sncname="p/krb5:sapsvc@EXAMPLE.LOCAL" sncop="9"/>
    <Service type="SAPGUI" uuid="s3" name="No system id" systemid="   " server="ghost.example:3200"/>
    <Service type="SAPGUI" uuid="s4" name="Nowhere" systemid="NIL"/>
  </Services>
  <Messageservers>
    <Messageserver uuid="m1" name="PRD" host="prodsys-a.example" port="3610"/>
  </Messageservers>
  <Routers>
    <Router uuid="r1" name="edge" router="/H/router.example/S/3299"/>
  </Routers>
</Landscape>`

func parseSample(t *testing.T) []LandscapeSystem {
	t.Helper()
	lf, err := ParseLandscapeBytes([]byte(sampleLandscape), "sample")
	if err != nil {
		t.Fatalf("ParseLandscapeBytes: %v", err)
	}
	return lf.Systems("sample")
}

func TestLandscapeSystemsResolveTheirServers(t *testing.T) {
	systems := parseSample(t)

	// Two of the four services describe a reachable system. One has a system id
	// of spaces, which is not a system; one names no server at all.
	if len(systems) != 2 {
		t.Fatalf("got %d systems, want 2: %+v", len(systems), systems)
	}

	byID := map[string]LandscapeSystem{}
	for _, s := range systems {
		byID[s.SystemID] = s
	}

	dev, ok := byID["DEV"]
	if !ok {
		t.Fatal("DEV missing")
	}
	if dev.Host != "devsys.example" {
		t.Errorf("DEV host = %q, want devsys.example", dev.Host)
	}
	// A direct connection names the dispatcher port, 3200 + instance.
	if dev.InstanceNr != "01" {
		t.Errorf("DEV instance = %q, want 01", dev.InstanceNr)
	}
	if dev.LoadBalance {
		t.Error("DEV is a direct connection but was marked load balanced")
	}

	prd, ok := byID["PRD"]
	if !ok {
		t.Fatal("PRD missing")
	}
	if prd.Host != "prodsys-a.example" {
		t.Errorf("PRD host = %q, want the message server's host", prd.Host)
	}
	// A load-balanced one names the message server port, 3600 + instance.
	if prd.InstanceNr != "10" {
		t.Errorf("PRD instance = %q, want 10", prd.InstanceNr)
	}
	if !prd.LoadBalance {
		t.Error("PRD goes through a message server and was not marked load balanced")
	}
	if prd.Router != "/H/router.example/S/3299" {
		t.Errorf("PRD router = %q, want the referenced router", prd.Router)
	}
	if !strings.HasPrefix(prd.SNCName, "p/krb5:") {
		t.Errorf("PRD SNC name = %q, want the one on the service", prd.SNCName)
	}
}

func TestLandscapeIncludesAreReported(t *testing.T) {
	// The shared landscape is where most systems live; losing the include means
	// finding only whatever the local file happens to hold.
	lf, err := ParseLandscapeBytes([]byte(sampleLandscape), "sample")
	if err != nil {
		t.Fatal(err)
	}
	if len(lf.Includes) != 1 || !strings.HasPrefix(lf.Includes[0].URL, "file://") {
		t.Errorf("includes = %+v, want the one file:// entry", lf.Includes)
	}
}

func TestInstanceFromPort(t *testing.T) {
	tests := []struct {
		port string
		base int
		want string
	}{
		{"3200", 3200, "00"},
		{"3201", 3200, "01"},
		{"3242", 3200, "42"},
		{"3610", 3600, "10"},
		// Outside the instance range, or not a port at all: an instance number
		// guessed wrong builds a URL that fails for a reason nobody can see.
		{"3199", 3200, ""},
		{"3400", 3200, ""},
		{"", 3200, ""},
		{"http", 3200, ""},
	}
	for _, tc := range tests {
		if got := instanceFromPort(tc.port, tc.base); got != tc.want {
			t.Errorf("instanceFromPort(%q, %d) = %q, want %q", tc.port, tc.base, got, tc.want)
		}
	}
}

func TestCandidateURLsFollowThePortConvention(t *testing.T) {
	s := LandscapeSystem{SystemID: "DEV", Host: "devsys.example", InstanceNr: "01"}
	got := s.CandidateURLs()
	// The default address first: it is what answers on a landscape fronted by
	// anything, and the derived ports only on a directly exposed ICM.
	want := []string{
		"https://devsys.example",
		"https://devsys.example:44301",
		"http://devsys.example:8001",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCandidateURLsWithoutAnInstanceNumber(t *testing.T) {
	// No instance number means no derived ports — but the default address is
	// still worth offering, since that is where a web dispatcher answers.
	s := LandscapeSystem{SystemID: "DEV", Host: "devsys.example"}
	got := s.CandidateURLs()
	if len(got) != 1 || got[0] != "https://devsys.example" {
		t.Errorf("got %v, want just the default address", got)
	}
}

func TestCandidateURLsWithoutAHost(t *testing.T) {
	if got := (LandscapeSystem{SystemID: "DEV"}).CandidateURLs(); got != nil {
		t.Errorf("got %v, want none — there is nowhere to connect", got)
	}
}

func TestPowerShellQuote(t *testing.T) {
	// A UNC path reaches PowerShell as a literal; an apostrophe in a share name
	// would otherwise end the string and change the command.
	if got := powerShellQuote(`\\server\share\file.xml`); got != `'\\server\share\file.xml'` {
		t.Errorf("got %s", got)
	}
	if got := powerShellQuote(`\\server\o'brien\f.xml`); got != `'\\server\o''brien\f.xml'` {
		t.Errorf("got %s, want the apostrophe doubled", got)
	}
}

// SAP GUI for Java writes no systemid at all: the system lives in name, and
// the file is called SAPGUILandscape.xml rather than SAPUILandscape.xml. Its
// entries were dropped entirely before, so on macOS the tool found nothing
// even with SAP GUI installed and configured.
const sampleJavaLandscape = `<?xml version="1.0" encoding="UTF-8"?>
<Landscape updated="2026-01-01T00:00:00Z" version="1" generator="SAP GUI for Java 7.80 rev 7">
  <Services>
    <Service client="001" user="TESTUSER" name="DEV" expert="1" uuid="j1" type="SAPGUI" server="devsys.example:3200" mode="1"/>
    <Service client="001" user="TESTUSER" name="A description, not a system" uuid="j2" type="SAPGUI" server="ghost.example:3200"/>
  </Services>
</Landscape>`

func TestLandscapeJavaFlavourUsesNameAsSystemID(t *testing.T) {
	lf, err := ParseLandscapeBytes([]byte(sampleJavaLandscape), "java")
	if err != nil {
		t.Fatalf("ParseLandscapeBytes: %v", err)
	}
	systems := lf.Systems("java")

	if len(systems) != 1 {
		t.Fatalf("got %d systems, want 1 — only the entry whose name is shaped like a system id", len(systems))
	}
	got := systems[0]
	if got.SystemID != "DEV" {
		t.Errorf("SystemID = %q, want %q", got.SystemID, "DEV")
	}
	if got.Host != "devsys.example" {
		t.Errorf("Host = %q, want %q", got.Host, "devsys.example")
	}
	if got.InstanceNr != "00" {
		t.Errorf("InstanceNr = %q, want %q — 3200 is instance 00", got.InstanceNr, "00")
	}
}

// The name is only a fallback for a missing system id, never a rename: a
// Windows file's blank systemid still drops the entry rather than promoting
// its description.
func TestLandscapeBlankSystemIDStillDropsTheEntry(t *testing.T) {
	for _, s := range parseSample(t) {
		if strings.EqualFold(s.SystemID, "NO SYSTEM ID") || s.Host == "ghost.example" {
			t.Errorf("entry with a blank systemid survived as %+v", s)
		}
	}
}

func TestLooksLikeSystemID(t *testing.T) {
	for _, ok := range []string{"DEV", "A4H", "PRD", "S4H", "123"} {
		if !looksLikeSystemID(ok) {
			t.Errorf("looksLikeSystemID(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "DE", "DEVS", "A description", "D-V", "a4h"} {
		if looksLikeSystemID(bad) {
			t.Errorf("looksLikeSystemID(%q) = true, want false", bad)
		}
	}
}

func TestParallelsRefRoundTrip(t *testing.T) {
	const winPath = `C:\Users\testuser\AppData\Roaming\SAP\Common\SAPUILandscape.xml`
	ref := ParallelsRef("Windows 11", winPath)

	vm, got, ok := ParseParallelsRef(ref)
	if !ok {
		t.Fatalf("ParseParallelsRef(%q) not recognised", ref)
	}
	if vm != "Windows 11" {
		t.Errorf("vm = %q, want %q — a VM name may contain spaces", vm, "Windows 11")
	}
	if got != winPath {
		t.Errorf("path = %q, want %q — the drive colon must survive the split", got, winPath)
	}

	for _, bad := range []string{"/Users/testuser/SAPUILandscape.xml", "parallels:", "parallels:vm", "parallels::x"} {
		if _, _, ok := ParseParallelsRef(bad); ok {
			t.Errorf("ParseParallelsRef(%q) = ok, want not recognised", bad)
		}
	}
}

func TestWindowsPathFromIncludeURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"file://fileserver.example/public/SAPUILandscape.XML", `\\fileserver.example\public\SAPUILandscape.XML`, true},
		{"file:///C:/Users/testuser/SAPUILandscapeGlobal.xml", `C:\Users\testuser\SAPUILandscapeGlobal.xml`, true},
		{"https://intranet.example/landscape.xml", "", false},
		{"file://", "", false},
	}
	for _, tt := range tests {
		got, ok := WindowsPathFromIncludeURL(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("WindowsPathFromIncludeURL(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

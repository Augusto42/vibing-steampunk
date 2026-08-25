package mcp

// Ten capabilities were found this August that had been advertised in the
// README, registered as tools, reachable by a user, and had never once
// returned a correct answer. Not one of them was visible by reading the code:
// each needed a live system to find, and each was found by hand, by somebody
// who happened to be looking.
//
// The eleventh will ship the same way unless the looking is automated. That is
// what this is.
//
// It walks the advertised surface and asks two different questions, because
// they fail in two different ways:
//
//	Reach.  Is the capability registered and routed at all? Ten gCTS tools sat
//	        in the whitelist for months behind a registration function that
//	        nothing called. This question needs no SAP system and belongs in
//	        CI.
//
//	Answer. Called against a real system with an input that has an answer, does
//	        it produce one? This is the question that found the other ten, and
//	        it cannot be asked offline.
//
// The distinction that carries the whole design is between an empty answer that
// is true and an empty answer that is a failure wearing a truthful face. A
// probe therefore may carry an *oracle*: a second, independent way to find out
// whether there is anything to find. When the oracle says there are twelve and
// the capability says none, that is not an empty result. It is a dead feature,
// and the sweep says so in those words.
//
// Two rules this file is built to obey, both learned by breaking them:
//
//  1. A sweep that cannot cover everything must say what it did not cover.
//     A clean report over a third of the surface is the health report that
//     said GOOD over a scan that never ran.
//  2. The sweep never writes. Every probe is a read, enforced below rather
//     than merely intended, because a verification tool that mutates a
//     customer's system to verify itself is not a verification tool.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Verdict is what one capability did when it was called.
//
// The values are deliberately more numerous than pass/fail. "Nothing came back"
// is four different situations — the release does not have it, the user may not
// see it, there is genuinely nothing there, or the feature is dead — and
// collapsing them is how the dead ones stayed hidden.
type Verdict string

const (
	// VerdictAnswered — a real answer came back.
	VerdictAnswered Verdict = "answered"
	// VerdictDead — the call succeeded and returned nothing, and an
	// independent oracle says there was something to find. This is the finding
	// the sweep exists for.
	VerdictDead Verdict = "dead"
	// VerdictEmpty — nothing came back and nothing was expected to. True on
	// this system, today, and not evidence either way about the capability.
	VerdictEmpty Verdict = "empty"
	// VerdictRefused — the system said no, out loud: an authorisation, a
	// safety block, a validation error. The capability works; the answer is no.
	VerdictRefused Verdict = "refused"
	// VerdictAbsent — the resource is not on this release. A statement about
	// the system, not about us.
	VerdictAbsent Verdict = "absent"
	// VerdictBroken — our fault. An unroutable action, a malformed call, a
	// parse failure, a panic.
	VerdictBroken Verdict = "broken"
	// VerdictUnreachable — advertised somewhere and registered or routed
	// nowhere. Found without a system.
	VerdictUnreachable Verdict = "unreachable"
	// VerdictSkipped — no object of the required kind was available to probe
	// with. The probe's fault, not the system's, and reporting it as absent
	// would put a missing feature on the system's record.
	VerdictSkipped Verdict = "skipped"
	// VerdictUnprobed — advertised, reachable, and this sweep has no probe for
	// it. Counted and named, never quietly omitted.
	VerdictUnprobed Verdict = "unprobed"
	// VerdictTimedOut — the call did not finish inside the budget. Not a defect
	// and not an absence: on a large system a heavy target can exceed thirty
	// seconds while an ordinary one answers, which says the resource is alive
	// and the target is expensive. It is reported separately so a freeze run
	// does not put a working capability on the defect list.
	VerdictTimedOut Verdict = "timed-out"
	// VerdictMisprobed — the handler read the call and said what was missing
	// from it. That is the capability working, and the probe calling it wrong.
	//
	// This verdict exists because the first live run produced nine "broken"
	// rows that were all mine: parameters named differently from what I
	// guessed. A sweep whose findings are mostly its own mistakes teaches its
	// reader to skim, and then the one real finding goes past unread. So the
	// probe's fault is separated from the product's, loudly enough to be fixed
	// and never counted as a defect in what is being swept.
	VerdictMisprobed Verdict = "misprobed"
)

// Bad reports whether a verdict is a finding a maintainer must act on, as
// opposed to a fact about the system or a gap in the sweep.
func (v Verdict) Bad() bool {
	return v == VerdictDead || v == VerdictBroken || v == VerdictUnreachable
}

// OurFault reports whether the sweep, rather than the thing swept, is wrong.
// These belong in the report — an unmaintained probe table is how a sweep
// stops being evidence — but never among the findings.
func (v Verdict) OurFault() bool {
	return v == VerdictMisprobed || v == VerdictSkipped
}

// SweepTargets names the objects probes may read. A probe whose requirements
// are not met is skipped by name rather than failed.
type SweepTargets struct {
	// Class is any class that exists. Used for reads and structure.
	Class string
	// Program is any program that exists.
	Program string
	// Package is a package that exists and holds objects.
	Package string
	// Group is a function group that exists.
	Group string
	// Table is a table that exists and has rows — T000 on every system.
	Table string
	// Referenced is an object known to be referenced by other code. It is the
	// input for every probe about callers, and choosing one at random is how
	// a dead where-used check passes: most custom objects have no callers, so
	// an empty answer is true and proves nothing.
	Referenced string
	// References is an object known to reference other code — the down
	// direction of the same problem.
	References string
	// ReferencesType is that object's type, and it travels with the name
	// because a probe that assumes one asks at the wrong address. A program
	// include asked for at /oo/classes/ answers 404, and a 404 caused by us is
	// indistinguishable from a capability the release does not have.
	ReferencesType string
	// Dump is the id of a runtime error that exists on this system. The
	// post-mortem types default to "latest" and refuse when the feed is empty,
	// which is correct and is not a defect — so a probe without one must be
	// skipped rather than counted as a death. A quiet system is quiet.
	Dump string
	// Trace is the id of a recorded trace, for the same reason.
	Trace string
	// Versioned is an object the version directory demonstrably holds history
	// for, and VersionedType is its type. Picking any class instead is the
	// mistake this struct already documents twice: most objects on a stock
	// system have never been changed, so an empty revision list is true and the
	// probe proves nothing.
	Versioned     string
	VersionedType string
	// VersionURI and VersionURI2 are two of that object's versions, resolved
	// before the sweep runs because they cannot be constructed by hand — the
	// URI is issued by the server. Reading one version and comparing two are
	// separate capabilities and both need one.
	VersionURI  string
	VersionURI2 string
	// DataElement is a data element that has labels in English, and
	// MessageClass a message class that has texts. Both are read from the
	// dictionary rather than assumed, because "the labels are empty" and "the
	// capability returns nothing" look identical from the outside.
	DataElement  string
	MessageClass string
	// TextPoolProgram is a program that has a text pool. Most do not.
	TextPoolProgram string
	// CRAttribute is the transport attribute this landscape groups change
	// requests by. Without it the CR types cannot be asked anything, and say so
	// — which is them working. A probe that reads that as a failure is wrong.
	CRAttribute string
}

// Have reports whether every named target kind is available.
func (t SweepTargets) Have(kinds ...string) (string, bool) {
	for _, k := range kinds {
		if t.get(k) == "" {
			return k, false
		}
	}
	return "", true
}

func (t SweepTargets) get(kind string) string {
	switch kind {
	case "class":
		return t.Class
	case "program":
		return t.Program
	case "package":
		return t.Package
	case "group":
		return t.Group
	case "table":
		return t.Table
	case "referenced":
		return t.Referenced
	case "references":
		return t.References
	case "references_type":
		return t.ReferencesType
	case "dump":
		return t.Dump
	case "trace":
		return t.Trace
	case "cr_attribute":
		return t.CRAttribute
	case "versioned":
		return t.Versioned
	case "versioned_type":
		return t.VersionedType
	case "version_uri":
		return t.VersionURI
	case "version_uri2":
		return t.VersionURI2
	case "data_element":
		return t.DataElement
	case "message_class":
		return t.MessageClass
	case "text_pool_program":
		return t.TextPoolProgram
	}
	return ""
}

// expand substitutes {class}, {program}, {package}, {group}, {table},
// {referenced}, {references}, {dump} and {trace} in a string.
func (t SweepTargets) expand(s string) string {
	// URI forms first, because {references_uri} contains {references} and the
	// plain substitution would eat its prefix.
	//
	// These exist because two types take only an object_uri and leave the
	// caller to build it. Building one by concatenation is a trap: a namespaced
	// name carries slashes, so /sap/bc/adt/oo/classes/ + /BOBF/CL_X yields a
	// path with an empty segment and the object loses its name. That defect was
	// fixed in the handler on 2026-08-24; a probe that concatenates recreates it
	// on the caller's side and then blames the capability.
	for _, k := range []string{"class", "referenced", "references"} {
		s = strings.ReplaceAll(s, "{"+k+"_uri}", classURI(t.get(k)))
	}
	// Longer keys before the shorter ones they contain. {references_type} does
	// not literally contain "{references}" — the closing brace is in the way —
	// but relying on that is relying on a brace, and the next compound key may
	// not be so lucky. Ordering costs nothing and removes the class.
	for _, k := range []string{"references_type", "versioned_type", "version_uri2", "version_uri", "text_pool_program", "message_class", "data_element", "versioned", "class", "program", "package", "group", "table", "referenced", "references", "dump", "trace"} {
		s = strings.ReplaceAll(s, "{"+k+"}", t.get(k))
	}
	return s
}

// classURI builds the ADT path of a class. It delegates rather than formatting,
// because the escaping is the whole point and there should be one copy of it:
// adt.GetObjectURL is what every other caller uses, and writing a fourth
// version here is how the third one came to disagree with the other two.
//
// Empty in, empty out — a probe with no target is skipped before it gets here.
func classURI(name string) string {
	if name == "" {
		return ""
	}
	return adt.GetObjectURL(adt.ObjectTypeClass, name, "")
}

// Oracle is a second, independent route to the same fact.
//
// It returns how many results *must* exist and one sentence saying how it
// knows. The sweep never trusts the oracle's number as an answer — only its
// zero-or-more — because the point is not to check arithmetic, it is to know
// whether an empty answer could possibly be true.
type Oracle func(ctx context.Context, c *adt.Client, t SweepTargets) (count int, how string, err error)

// Probe is one question to put to the product surface.
type Probe struct {
	// ID is stable across runs so two sweeps can be diffed.
	ID string
	// Capability is what this probe is evidence about, in the vocabulary the
	// user sees: an analyze type, an action, a tool name.
	Capability string
	// Why says, in a few words, what a reader learns from the answer.
	Why string
	// Action and Params are the call, exactly as an agent would make it.
	Action string
	Target string
	Params map[string]any
	// Needs lists target kinds the call interpolates — {class}, {references_uri}
	// and so on. Without them there is nothing to substitute.
	Needs []string
	// Requires lists target kinds the call does not interpolate but cannot mean
	// anything without: a precondition of the system rather than an input.
	// cr_boundaries takes only a cr_id, but on a landscape with no change-request
	// attribute configured it can only answer "not configured" — which is the
	// capability working, and counting it as broken is the probe's fault.
	//
	// The distinction is not pedantry. A probe that names a target it never uses
	// would be skipped forever and its capability never checked, so the test
	// insists Needs are substituted; Requires exist to keep that test strict
	// rather than to escape it.
	Requires []string
	// Oracle, when set, decides whether an empty answer is a dead capability.
	Oracle Oracle
	// MustContain is a substring the answer has to carry to count. Use it
	// where "not empty" is too weak — a handler that returns its own help text
	// on a bad input is not answering.
	MustContain string
	// EmptyIsFine marks a capability where nothing found is the ordinary case
	// on a quiet system: no dumps today, no traces recorded. Without it every
	// clean system would read as a wall of failures.
	EmptyIsFine bool
}

// probedCapabilities counts distinct advertised names with at least one probe.
//
// The distinction matters because probes and capabilities are not one to one in
// either direction: several probes may examine one capability from different
// angles, and a probe skipped for want of a target still names the capability
// it would have examined. What a reader wants from a coverage line is how much
// of the surface was looked at, and that is the count of names.
func (r *SweepReport) probedCapabilities() int {
	seen := map[string]bool{}
	for _, f := range r.Answer {
		seen[baseCapability(f.Capability)] = true
	}
	return len(seen)
}

// baseCapability strips the parenthesised angle a probe may add to say which
// input it used — "analyze type=graph_stats (package)" is still graph_stats.
func baseCapability(c string) string {
	if i := strings.Index(c, " ("); i > 0 {
		return c[:i]
	}
	return c
}

// SweepFinding is one probe's answer.
type SweepFinding struct {
	ID         string  `json:"id"`
	Capability string  `json:"capability"`
	Why        string  `json:"why,omitempty"`
	Verdict    Verdict `json:"verdict"`
	Detail     string  `json:"detail,omitempty"`
	// Oracle records what the second route said, when one was consulted. It is
	// the difference between "we think this is dead" and "this is dead, and
	// here is the count it should have returned".
	OracleCount int           `json:"oracleCount,omitempty"`
	OracleHow   string        `json:"oracleHow,omitempty"`
	Evidence    string        `json:"evidence,omitempty"`
	Elapsed     time.Duration `json:"-"`
}

// SweepReport is one system's answers, plus an honest account of what was not
// asked.
type SweepReport struct {
	System string `json:"system"`
	// Reach is the offline pass: registration and routing.
	Reach []SweepFinding `json:"reach"`
	// Answer is the live pass.
	Answer []SweepFinding `json:"answer,omitempty"`
	// Unprobed names every advertised capability this sweep has no probe for.
	// It is the coverage denominator, and printing the numerator without it
	// would be the defect this tool exists to find.
	Unprobed []string `json:"unprobed,omitempty"`
	// Advertised is how many capabilities were counted as the surface.
	Advertised int `json:"advertised"`
	// Missed lists targets the sweep could not obtain, which is why some
	// probes were skipped.
	Missed []adt.Unsearched `json:"missed,omitempty"`
	// ReachChecked counts the names the offline pass examined, so that pass can
	// state its own coverage too rather than borrowing the live pass's.
	ReachChecked int `json:"reachChecked"`
	// Targets names the objects the probes were run against.
	//
	// Same reason as Build. A verdict is only as good as what it was asked
	// about: "callees returned nothing" means one thing for an object the
	// cross-reference tables have rows for and nothing at all for one they do
	// not. A reader who cannot see the target cannot tell which report they
	// are holding.
	Targets SweepTargets `json:"targets"`
	// Release and Database name what the run was against, in terms that are
	// facts about the software rather than about the installation.
	//
	// The record this feeds says things like "absent on C" — and "absent on a
	// release" is a claim, while "absent on a system" is an anecdote. Without
	// the number the sentence is half of one.
	//
	// SystemInfo also carries the system id and the host name, and they are
	// deliberately not taken. The report already travels to places where
	// identity must not go, and the release answers the question while the host
	// only answers "whose". Adding them would make every consumer of this
	// report responsible for stripping them.
	Release  string `json:"release,omitempty"`
	Database string `json:"database,omitempty"`
	// Build names the binary that was exercised.
	//
	// This is not decoration. A sweep of fifteen capabilities was once run
	// against an MCP server started the previous evening and nearly recorded as
	// the state of the release: two of its "refusals" were ghosts of code that
	// had already been fixed. The corollary bit from the other side on the same
	// day — a neighbouring project dismissed a real defect, alive since
	// January, as a stale image. A report that cannot say which build it
	// exercised can be read either way, and both readings cost a night.
	Build string `json:"build,omitempty"`
	Live  bool   `json:"live"`
}

// --- the advertised surface ----------------------------------------------

// AnalyzeTypes returns every analyze type the server routes, from the routing
// tables themselves rather than from a list kept alongside them.
//
// A list kept alongside is a claim; this is the surface. Four types were
// advertised for months on a namespace that does not exist on any release, and
// nothing could tell the difference because nothing could enumerate them.
func (s *Server) AnalyzeTypes() []string {
	seen := map[string]bool{}
	for _, table := range []map[string]any{
		toAny(s.analysisTypes()),
		toAny(s.dumpAnalysisTypes()),
		toAny(s.traceAnalysisTypes()),
		toAny(s.sqlTraceAnalysisTypes()),
		toAny(s.contextAnalysisTypes()),
		toAny(s.lintTypes()),
	} {
		for k := range table {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toAny[T any](m map[string]T) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// RegisteredTools returns the tool names this server registers, sorted.
func (s *Server) RegisteredTools() []string {
	registered := s.mcpServer.ListTools()
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SweepReach runs the offline pass: is every advertised capability actually
// reachable? No network, so it belongs in CI.
func SweepReach() []SweepFinding {
	var out []SweepFinding

	// Every name the focused whitelist advertises must be a tool that some
	// mode registers. This is the check that found ten gCTS tools whitelisted
	// behind a registration function nothing called.
	expert := map[string]bool{}
	expertSrv := NewServer(&Config{BaseURL: "https://example.invalid", Mode: "expert"})
	for _, n := range expertSrv.RegisteredTools() {
		expert[n] = true
	}
	for _, n := range focusedToolNames() {
		if expert[n] {
			continue
		}
		out = append(out, SweepFinding{
			ID:         "reach.focused." + n,
			Capability: n,
			Why:        "whitelisted for focused mode",
			Verdict:    VerdictUnreachable,
			Detail:     "named in the focused whitelist and registered by no mode; a user selecting focused sees a tool that is not there",
		})
	}

	// Every analyze type this sweep knows how to probe must still be routed.
	// The reverse direction — a routed type nobody probes — is reported as
	// coverage, not as a defect.
	routed := map[string]bool{}
	for _, t := range expertSrv.AnalyzeTypes() {
		routed[t] = true
	}
	for _, p := range SweepProbes() {
		if p.Action != "analyze" {
			continue
		}
		typ, _ := p.Params["type"].(string)
		if typ == "" || routed[typ] {
			continue
		}
		out = append(out, SweepFinding{
			ID:         "reach.analyze." + typ,
			Capability: "analyze type=" + typ,
			Why:        "probed by this sweep",
			Verdict:    VerdictUnreachable,
			Detail:     "no router claims this type, so the call falls through to \"no handler found\"",
		})
	}

	// Every mode must be able to reach the analyze surface.
	//
	// This check exists because the reach pass did not catch the defect it is
	// named after. `SAP()` was registered only in hyperfocused, and the
	// thirty-eight analyze types are routed through it and registered as tools
	// nowhere — so two of the three modes advertised a capability surface
	// missing a third of itself, and the sweep reported everything reachable.
	//
	// The old check asked "is every whitelisted name registered somewhere",
	// which is a question about names. This asks whether a capability can be
	// reached from a mode that claims to offer it, which is the question a user
	// has. A sweep blind to the difference is how the gap survived being swept.
	for _, mode := range []string{"hyperfocused", "focused", "expert"} {
		srv := NewServer(&Config{BaseURL: "https://example.invalid", Mode: mode})
		tools := srv.RegisteredTools()
		if len(tools) == 0 {
			out = append(out, SweepFinding{
				ID: "reach.mode." + mode, Capability: "mode " + mode, Verdict: VerdictUnreachable,
				Detail: "this mode registers no tool at all",
			})
			continue
		}
		universal := false
		for _, n := range tools {
			if n == "SAP" {
				universal = true
				break
			}
		}
		if !universal {
			out = append(out, SweepFinding{
				ID:         "reach.analyze." + mode,
				Capability: "the analyze surface in " + mode + " mode",
				Why:        "every analyze type is routed through SAP() and registered as a tool nowhere",
				Verdict:    VerdictUnreachable,
				Detail: fmt.Sprintf("%s registers %d tools and not SAP, so none of the %d analyze types can be called from it",
					mode, len(tools), len(srv.AnalyzeTypes())),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ReachChecked counts the names the offline pass examines, so the pass can
// print its own denominator instead of borrowing the live pass's.
func ReachChecked() int {
	srv := NewServer(&Config{BaseURL: "https://example.invalid", Mode: "expert"})
	return len(srv.RegisteredTools()) + len(focusedToolNames()) + len(srv.AnalyzeTypes())
}

// --- the live pass --------------------------------------------------------

// writeActions never run. The sweep verifies; it does not modify. This is a
// list and not a comment because the difference has to survive somebody adding
// a probe in a hurry.
var writeActions = map[string]bool{
	"edit": true, "create": true, "delete": true, "deploy": true,
	"install": true, "activate": true, "transport": true, "git": true,
	"write": true, "rename": true, "move": true, "copy": true,
}

// SweepOptions shape one run.
type SweepOptions struct {
	// Only restricts the run to probes whose id or capability contains it.
	Only string
	// PerProbe caps a single probe.
	//
	// Without a cap one capability that never returns takes the whole sweep
	// with it, and a sweep that produces no report is worse than one that
	// reports a timeout: the first says nothing, the second names the
	// capability that hung. A probe that runs out of time is a finding, not a
	// skip — something a user calls would have hung on them too.
	PerProbe time.Duration
	// Progress, when set, is called before each probe so a person watching a
	// long run can see which capability is being asked.
	Progress func(p Probe)
}

// RunSweep asks the live pass and assembles the report.
func (s *Server) RunSweep(ctx context.Context, system string, targets SweepTargets, opts SweepOptions) *SweepReport {

	if opts.PerProbe <= 0 {
		opts.PerProbe = 45 * time.Second
	}
	only := opts.Only
	report := &SweepReport{System: system, Reach: SweepReach(), ReachChecked: ReachChecked(), Live: true}

	// Asked first, so a report that fails halfway still says what it was
	// talking to. A failure here is not fatal: the release is context for the
	// verdicts, not a precondition for producing them.
	if info, err := s.adtClient.GetSystemInfo(ctx); err == nil && info != nil {
		report.Release = strings.TrimSpace(info.SAPRelease)
		report.Database = strings.TrimSpace(info.DatabaseSystem)
	}

	probes := SweepProbes()
	probed := map[string]bool{}
	for _, p := range probes {
		probed[baseCapability(p.Capability)] = true
	}

	for _, p := range probes {
		if only != "" && !strings.Contains(p.ID, only) && !strings.Contains(p.Capability, only) {
			continue
		}
		if opts.Progress != nil {
			opts.Progress(p)
		}
		probeCtx, cancel := context.WithTimeout(ctx, opts.PerProbe)
		report.Answer = append(report.Answer, s.runProbe(probeCtx, p, targets))
		cancel()
	}

	// Coverage, stated rather than implied — and both halves derived from one
	// set, so they cannot drift apart.
	//
	// They had. The numerator counted distinct capabilities and the denominator
	// counted probe rows, so two probes of one action inflated the denominator
	// and the report said "39 of 40" while naming nothing as unprobed. A
	// coverage figure that can be internally inconsistent is the health report
	// saying GOOD over a scan that never ran, in the tool built to catch it.
	advertised := s.advertisedCapabilities()
	for _, c := range advertised {
		if !probed[c] {
			report.Unprobed = append(report.Unprobed, c)
		}
	}
	sort.Strings(report.Unprobed)
	report.Advertised = len(advertised)
	return report
}

// advertisedCapabilities is the population coverage is measured against.
//
// One function, so the count and the unprobed list are answers to the same
// question. Identities are the base ones: a type probed three ways — source,
// object, package — is one capability, not three.
func (s *Server) advertisedCapabilities() []string {
	seen := map[string]bool{}
	for _, t := range s.AnalyzeTypes() {
		seen["analyze type="+t] = true
	}
	for _, p := range coreActionProbes() {
		seen[baseCapability(p.Capability)] = true
	}

	// The routers outside analysisTypes() count too, or the figure overstates.
	//
	// When eleven capabilities were routed into the universal tool, three
	// actions and one analyze type became reachable — and none of them entered
	// this set, because it was built from the analyze tables and the core
	// probes alone. The report would have printed "39 of 39 capabilities
	// probed" with four capabilities outside the count entirely: not probed,
	// not named as unprobed, not counted. Arithmetically true and reading as
	// complete, which is the shape this whole command exists to refuse.
	//
	// Taken from the routers' own tables, so the list cannot drift from what is
	// dispatched. The lint router was the exception and was named here by hand,
	// which is how `analyze type=lint` came to be advertised by this function,
	// matched by hand in the router, and absent from AnalyzeTypes() all at
	// once. It has a table now. The durable fix is still one registry every
	// router registers into.
	for t := range s.i18nTypes() {
		seen["action=i18n op="+t] = true
	}
	for t := range s.revisionTypes() {
		seen["action=revisions op="+t] = true
	}
	for t := range s.lintTypes() {
		seen["action="+t] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// runProbe calls one capability through the same dispatch an agent uses, so a
// capability that is implemented but unrouted fails here exactly as it would
// for a user.
func (s *Server) runProbe(ctx context.Context, p Probe, t SweepTargets) SweepFinding {
	f := SweepFinding{ID: p.ID, Capability: p.Capability, Why: p.Why}

	if writeActions[strings.ToLower(p.Action)] {
		f.Verdict = VerdictSkipped
		f.Detail = "the sweep does not run write actions"
		return f
	}
	if kind, ok := t.Have(append(append([]string{}, p.Needs...), p.Requires...)...); !ok {
		f.Verdict = VerdictSkipped
		f.Detail = "no " + kind + " was available to probe with"
		return f
	}

	args := map[string]any{"action": p.Action}
	if p.Target != "" {
		args["target"] = t.expand(p.Target)
	}
	if len(p.Params) > 0 {
		params := make(map[string]any, len(p.Params))
		for k, v := range p.Params {
			if sv, ok := v.(string); ok {
				v = t.expand(sv)
			}
			params[k] = v
		}
		args["params"] = params
	}

	start := time.Now()
	result, err := s.handleUniversalTool(ctx, newRequest(args))
	f.Elapsed = time.Since(start)

	if err != nil {
		f.Verdict = VerdictBroken
		f.Detail = err.Error()
		if ctx.Err() != nil {
			f.Detail = "did not answer within the probe timeout; a user calling this would have hung too"
		}
		return f
	}

	text := resultText(result)
	f.Evidence = firstLine(text)

	if result != nil && result.IsError {
		f.Verdict, f.Detail = classifyError(text)
		return f
	}

	if p.MustContain != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(p.MustContain)) {
		f.Verdict = VerdictBroken
		f.Detail = "the answer does not contain " + strconv.Quote(p.MustContain) + ", so it is not an answer to this question"
		return f
	}

	if !looksEmpty(text) {
		f.Verdict = VerdictAnswered
		return f
	}

	// Nothing came back. Whether that is true is the whole question.
	if p.Oracle == nil {
		if p.EmptyIsFine {
			f.Verdict = VerdictEmpty
			f.Detail = "nothing found, and on this capability that is an ordinary answer"
			return f
		}
		f.Verdict = VerdictEmpty
		f.Detail = "nothing found, and this sweep has no second route to say whether that is true"
		return f
	}

	count, how, oerr := p.Oracle(ctx, s.adtClient, t)
	f.OracleCount, f.OracleHow = count, how
	switch {
	case oerr != nil:
		f.Verdict = VerdictEmpty
		f.Detail = "nothing found, and the oracle could not run either: " + oerr.Error()
	case count > 0:
		f.Verdict = VerdictDead
		f.Detail = fmt.Sprintf("returned nothing while %s says there are %d", how, count)
	default:
		f.Verdict = VerdictEmpty
		f.Detail = "nothing found, and " + how + " agrees there is nothing"
	}
	return f
}

// classifyError separates what the system said from what we did wrong.
func classifyError(text string) (Verdict, string) {
	low := strings.ToLower(text)
	switch {
	// A budget exceeded is not a broken capability. On a large system the
	// where-used list of a class every other class references does not fit in
	// thirty seconds, while the same call on an ordinary class answers — so the
	// resource is alive and this target is heavy. Reporting that as a defect
	// puts a working capability on the list, and the freeze run is exactly
	// where that costs somebody an afternoon.
	case strings.Contains(low, "deadline exceeded"),
		strings.Contains(low, "client.timeout exceeded"),
		strings.Contains(low, "context canceled"):
		return VerdictTimedOut, firstLine(text)
	// A handler that names the parameter it wanted has read the call and
	// answered it. Nothing is wrong with the capability; the probe asked
	// badly, and saying so is what keeps the findings list worth reading.
	case strings.Contains(low, "there is no verdict to give"),
		strings.Contains(low, "no source-bearing objects"),
		strings.Contains(low, "is required"),
		strings.Contains(low, "are required"),
		strings.Contains(low, "needs a target"),
		strings.Contains(low, "provide '"),
		strings.Contains(low, "example: sap("):
		return VerdictMisprobed, firstLine(text)
	case strings.Contains(low, "no handler found"),
		strings.Contains(low, "unknown action"),
		strings.Contains(low, "unknown analysis type"),
		strings.Contains(low, "not supported"):
		return VerdictUnreachable, firstLine(text)
	case strings.Contains(low, "404"),
		strings.Contains(low, "not found on this"),
		strings.Contains(low, "does not exist on this release"):
		return VerdictAbsent, firstLine(text)
	case strings.Contains(low, "403"),
		strings.Contains(low, "not authorized"),
		strings.Contains(low, "forbidden"),
		strings.Contains(low, "blocked"),
		strings.Contains(low, "read-only"):
		return VerdictRefused, firstLine(text)
	default:
		return VerdictBroken, firstLine(text)
	}
}

// looksEmpty decides whether an answer carries anything.
//
// It is a heuristic and is treated as one: an answer it calls empty is then put
// to an oracle rather than reported as a failure. The phrases are the ones the
// handlers actually emit.
func looksEmpty(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
		return true
	}
	low := strings.ToLower(trimmed)
	for _, phrase := range []string{
		"no results", "no matches", "nothing found", "no objects",
		"no callers", "no callees", "no references", "no dumps",
		"no traces", "no entries", "no examples", "0 results",
		"found 0 ", "no output captured", "empty",
	} {
		if strings.Contains(low, phrase) {
			return true
		}
	}
	return false
}

func resultText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// focusedToolNames returns the focused whitelist as a sorted slice.
func focusedToolNames() []string {
	set := focusedToolSet()
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// --- rendering ------------------------------------------------------------

// Counts tallies verdicts across both passes.
func (r *SweepReport) Counts() map[Verdict]int {
	out := map[Verdict]int{}
	for _, f := range append(append([]SweepFinding{}, r.Reach...), r.Answer...) {
		out[f.Verdict]++
	}
	return out
}

// Findings returns only what a maintainer must act on.
func (r *SweepReport) Findings() []SweepFinding {
	var out []SweepFinding
	for _, f := range append(append([]SweepFinding{}, r.Reach...), r.Answer...) {
		if f.Verdict.Bad() {
			out = append(out, f)
		}
	}
	return out
}

// Text renders the report for a person.
//
// The findings come first and the tally second, because a reader who stops
// after one screen must have seen the dead capabilities rather than the score.
// The coverage line is not optional: a sweep that reports six clean answers out
// of a surface of thirty-one, without saying so, is the defect it was built to
// find.
func (r *SweepReport) Text() string {
	var b strings.Builder

	findings := r.Findings()
	if len(findings) == 0 {
		if r.Live {
			fmt.Fprintf(&b, "No dead capabilities found on %s.\n", r.System)
		} else {
			fmt.Fprintf(&b, "Every advertised capability is registered and routed.\n")
		}
	} else {
		fmt.Fprintf(&b, "%d finding(s) on %s\n\n", len(findings), r.System)
		for _, f := range findings {
			fmt.Fprintf(&b, "  %-11s %s\n", f.Verdict, f.Capability)
			if f.Detail != "" {
				fmt.Fprintf(&b, "              %s\n", f.Detail)
			}
			if f.OracleHow != "" && f.OracleCount > 0 {
				fmt.Fprintf(&b, "              oracle: %s\n", f.OracleHow)
			}
			if f.Why != "" {
				fmt.Fprintf(&b, "              why it is probed: %s\n", f.Why)
			}
			b.WriteString("\n")
		}
	}

	counts := r.Counts()
	if len(counts) > 0 {
		b.WriteString("\nVerdicts\n\n")
	}
	for _, v := range []Verdict{
		VerdictAnswered, VerdictDead, VerdictBroken, VerdictUnreachable,
		VerdictEmpty, VerdictRefused, VerdictAbsent, VerdictTimedOut, VerdictSkipped, VerdictMisprobed,
	} {
		if counts[v] > 0 {
			fmt.Fprintf(&b, "  %-11s %d\n", v, counts[v])
		}
	}

	// What was not asked, stated in the same breath as what was. Two passes
	// have two denominators and printing the wrong one would be its own small
	// lie: the offline pass covers names, the live pass covers behaviour.
	b.WriteString("\nCoverage\n\n")
	if !r.Live {
		fmt.Fprintf(&b, "  %d advertised names checked for registration and routing\n", r.ReachChecked)
		r.writeBuild(&b)
		b.WriteString("  This pass cannot tell whether a reachable capability answers.\n")
		b.WriteString("  Run it against a system for that.\n")
		return b.String()
	}
	// Capabilities, not probes. One capability may be probed several ways — a
	// type that takes source, an object or a package deserves one probe each —
	// and counting probes against advertised names produced "40 of 38", which
	// is not a coverage figure at all.
	fmt.Fprintf(&b, "  %d capabilities probed of %d advertised\n", r.probedCapabilities(), r.Advertised)
	// Two different absences, and merging them was making the report say
	// something false about itself. A capability nobody has written a probe for
	// is a gap somebody should close. A capability this sweep will never probe
	// — because it writes, and every probe here is a read — is the design, and
	// listing it as a shortfall invites exactly the fix that would ruin the
	// command: probing writers, or counting a refusal as a pass.
	var byDesign, unwritten []string
	for _, u := range r.Unprobed {
		if unprobableReason(u) != "" {
			byDesign = append(byDesign, u)
			continue
		}
		unwritten = append(unwritten, u)
	}
	if len(byDesign) > 0 {
		fmt.Fprintf(&b, "  %d excluded by design — the sweep never writes:\n", len(byDesign))
		for _, u := range byDesign {
			fmt.Fprintf(&b, "    %s — %s\n", u, unprobableReason(u))
		}
	}
	if len(unwritten) > 0 {
		fmt.Fprintf(&b, "  %d advertised and not probed by this sweep:\n", len(unwritten))
		for _, u := range unwritten {
			fmt.Fprintf(&b, "    %s\n", u)
		}
		b.WriteString("  A clean result above is a statement about the probed ones only.\n")
	} else if len(byDesign) > 0 {
		// Said out loud, because "49 of 51" with nothing else on the line reads
		// as two capabilities somebody forgot.
		b.WriteString("  Everything else advertised was probed.\n")
	}
	r.writeBuild(&b)
	b.WriteString("\nProbed against\n\n")
	r.writeTargets(&b)
	var ours []SweepFinding
	for _, f := range append(append([]SweepFinding{}, r.Reach...), r.Answer...) {
		if f.Verdict.OurFault() {
			ours = append(ours, f)
		}
	}
	if len(ours) > 0 {
		fmt.Fprintf(&b, "\nThe sweep's own gaps (%d) — these are not findings about the product\n\n", len(ours))
		for _, f := range ours {
			fmt.Fprintf(&b, "  %-11s %-34s %s\n", f.Verdict, f.Capability, f.Detail)
		}
	}

	if note := adt.UnsearchedNote(r.Missed, len(r.Missed)+4, "probe target"); note != "" {
		fmt.Fprintf(&b, "\n%s\n", note)
	}
	return b.String()
}

// writeBuild names the binary the report describes, or says plainly that it
// cannot — which is itself the warning, because a reader who does not know
// which build answered cannot tell a fixed defect from a live one.
// writeTargets names what the probes were run against.
func (r *SweepReport) writeTargets(b *strings.Builder) {
	pairs := [][2]string{
		{"class", r.Targets.Class}, {"program", r.Targets.Program},
		{"package", r.Targets.Package}, {"table", r.Targets.Table},
		{"referenced", r.Targets.Referenced},
		{"references", r.Targets.References + " (" + r.Targets.ReferencesType + ")"},
		{"versioned", r.Targets.Versioned + " (" + r.Targets.VersionedType + ")"},
		{"data element", r.Targets.DataElement},
		{"message class", r.Targets.MessageClass},
		{"text pool", r.Targets.TextPoolProgram},
	}
	for _, p := range pairs {
		if p[1] == "" {
			fmt.Fprintf(b, "  %-13s (none found — probes needing it were skipped)\n", p[0])
			continue
		}
		fmt.Fprintf(b, "  %-13s %s\n", p[0], p[1])
	}
}

func (r *SweepReport) writeBuild(b *strings.Builder) {
	if r.Build == "" {
		b.WriteString("  build: unknown — this report cannot be dated against a fix\n")
	} else {
		fmt.Fprintf(b, "  build: %s\n", r.Build)
	}
	switch {
	case r.Release != "" && r.Database != "":
		fmt.Fprintf(b, "  release: %s on %s\n", r.Release, r.Database)
	case r.Release != "":
		fmt.Fprintf(b, "  release: %s\n", r.Release)
	case r.Live:
		// Said rather than omitted: "absent" without a release is half a
		// sentence, and a reader has to know which half is missing.
		b.WriteString("  release: unknown — an absence here cannot be attributed to one\n")
	}
}

// unprobableReason explains an advertised capability this sweep will never
// probe, as opposed to one nobody has written a probe for yet.
//
// Keyed on the capability rather than carried on a probe, because the thing
// being described is the absence of a probe.
func unprobableReason(capability string) string {
	switch capability {
	case "action=i18n op=write_message_texts":
		return "built against the shape ADT serves, and unverified; checking it needs a scratch message class to write to"
	case "action=i18n op=write_labels":
		return "not implemented and refuses; see WriteDataElementLabels for what a correct one has to do"
	}
	return ""
}

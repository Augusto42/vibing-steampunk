package adt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// A dump says what failed. This file answers the other half of the question:
// who else runs the code that failed.
//
// It is deliberately not a rung on the ladder in correlate.go, and the reason
// is worth writing down because the ladder is right there and inviting. That
// ladder ranks application log entries by how well each argues it explains
// *this* failure. A caller that took part in this failure is on the dump's own
// stack, so scoreOnStack already has it. A caller that is not on the stack did
// not run — by construction — so nothing it ever wrote can be evidence for this
// dump, and giving it a rung would dress a coincidence up as structure, which
// is the one thing correlate.go was written to avoid.
//
// Blast radius also has no timestamp. It is a static fact about who *could*
// reach the broken code, true yesterday and true next month, and it does not
// belong in a list sorted against a five-minute window. So it gets its own
// answer, under its own flag.

// The other reason this file exists rather than a call to GetCallersOf: the
// call-graph resource those wrappers use, /sap/bc/adt/cai/callgraph, answers
// 404 "No suitable resource found" on 7.58 — in both directions, checked with
// a CSRF token in hand so it is the resource that is missing and not the
// request. Building on it would have produced a command that reports "nobody
// calls this" on every system, which is the worst possible failure mode for an
// impact query: silent, plausible and wrong.
//
// What does answer is the where-used list behind SE84,
// /sap/bc/adt/repository/informationsystem/usageReferences, which FindReferences
// already speaks. It is also the better source — it grades each reference and
// says which package the caller lives in.

// ExposedCaller is one object that can reach the failing code by a path other
// than the one this dump took.
type ExposedCaller struct {
	Name string `json:"name"`
	// Type is SAP's own code — CLAS/OC, FUGR/FF, PROG/P — kept as it arrives
	// rather than flattened, because the second half distinguishes a function
	// module from its group and that distinction is the useful part.
	Type string `json:"type,omitempty"`
	// URI is the caller's own ADT path, taken from the container row rather
	// than rebuilt from the name — a namespaced object or a function module is
	// not addressable by any rule this side could apply.
	URI       string `json:"uri,omitempty"`
	Package   string `json:"package,omitempty"`
	Component string `json:"component,omitempty"` // the method or routine holding the reference
	IsTest    bool   `json:"is_test"`
	// Distance counts units between the failing statement and this caller: 0
	// means it calls the unit that died, 1 means it calls that unit's caller,
	// and so on outward along the dump's stack.
	Distance int    `json:"distance"`
	Via      string `json:"via"` // the unit it reaches
}

// ImpactUnit is one compiled unit taken from the dump, with who calls it.
type ImpactUnit struct {
	Object   string          `json:"object"`
	Type     string          `json:"type"`
	URI      string          `json:"uri"`
	Distance int             `json:"distance"`
	Frame    *DumpFrame      `json:"frame,omitempty"`
	Callers  []ExposedCaller `json:"callers,omitempty"`
	// Total is how many direct callers the system reported, before any cap.
	Total int `json:"total"`
	// Err records a unit whose where-used list could not be read. An impact
	// answer that quietly drops a unit is worse than one that says which unit
	// it could not ask about.
	Err string `json:"error,omitempty"`
	// Note records a unit the query reached but cannot answer for, which is a
	// different and more dangerous thing than an error: it comes back 200 with
	// an empty list, and an empty list reads as "nobody calls this".
	Note string `json:"note,omitempty"`
}

// DumpImpactResult is the blast radius of one dump.
type DumpImpactResult struct {
	Dump  Dump         `json:"dump"`
	Units []ImpactUnit `json:"units"`
	// Exposed is the ranked, deduplicated answer: callers that are *not* on
	// this dump's stack, nearest the failing statement first.
	Exposed []ExposedCaller `json:"exposed"`
	// OnPath is the callers that are on the stack. They are the route this
	// dump actually took, so they are not additional exposure — they are kept
	// apart rather than dropped because seeing the known path confirms the
	// query aimed at the right object.
	OnPath []ExposedCaller `json:"on_path,omitempty"`
	// StackUnavailable says the release served the dump but not its stack, so
	// only the dump's own program could be asked about.
	StackUnavailable bool `json:"stack_unavailable,omitempty"`
}

// Answerable reports whether any unit produced a where-used list this query can
// stand behind. It exists so the caller can tell "nothing else calls this code"
// apart from "nothing here could be asked", which look identical in the numbers
// and mean opposite things.
func (r *DumpImpactResult) Answerable() bool {
	for _, u := range r.Units {
		if u.Err == "" && u.Note == "" {
			return true
		}
	}
	return false
}

// DumpImpactOptions tunes how far out and how much.
type DumpImpactOptions struct {
	// MaxUnits is how many units to walk outward from the failing statement.
	// Small on purpose: the further out you go the more the answer becomes
	// "everything reaches everything", and a caller of frame nine is exposed to
	// this bug only in the sense that it is exposed to the whole system.
	MaxUnits int
	// Limit caps the callers reported per unit. The true count is kept in
	// ImpactUnit.Total either way.
	Limit int
}

// DumpImpact answers "who else is exposed to this bug".
func (c *Client) DumpImpact(ctx context.Context, dump Dump, opts DumpImpactOptions) (*DumpImpactResult, error) {
	if opts.MaxUnits <= 0 {
		opts.MaxUnits = 3
	}
	if opts.Limit <= 0 {
		opts.Limit = 25
	}

	result := &DumpImpactResult{Dump: dump}

	var stack []DumpFrame
	if dump.ID != "" {
		frames, err := c.DumpStack(ctx, dump.ID)
		switch {
		case errors.Is(err, ErrDumpDetailUnavailable):
			// 7.50 serves the feed and not the detail resource. The dump still
			// names its own program, so the query narrows rather than fails.
			result.StackUnavailable = true
		case err != nil:
			result.StackUnavailable = true
		default:
			stack = frames
		}
	}

	units := impactUnits(dump, stack, opts.MaxUnits)
	if len(units) == 0 {
		return nil, fmt.Errorf("this dump names no program, so there is nothing to ask a where-used list about")
	}

	for i := range units {
		if note := unanswerable(units[i]); note != "" {
			units[i].Note = note
			continue
		}
		refs, err := c.FindReferences(ctx, units[i].URI, 0, 0)
		if err != nil {
			units[i].Err = err.Error()
			continue
		}
		callers := exposedCallers(refs, units[i].Object)
		units[i].Total = len(callers)
		for j := range callers {
			callers[j].Distance = units[i].Distance
			callers[j].Via = units[i].Object
		}
		if len(callers) > opts.Limit {
			callers = callers[:opts.Limit]
		}
		units[i].Callers = callers
	}

	result.Units = units
	result.Exposed, result.OnPath = rankExposure(units, dump, stack)
	return result, nil
}

// impactUnits picks the objects to ask about, nearest the failure first.
//
// The dump's own program leads even when the stack is readable, because the
// innermost frame is not always the unit that failed: an RFC refused at the
// door dumps with %_RFC_START on the stack and names the module it could not
// reach only in its header. Where the two agree — the usual case — the
// deduplication makes it a no-op.
func impactUnits(dump Dump, stack []DumpFrame, max int) []ImpactUnit {
	var units []ImpactUnit
	seen := map[string]bool{}

	add := func(u repoUnit, frame *DumpFrame) {
		if u.URI == "" || seen[u.URI] || len(units) >= max {
			return
		}
		seen[u.URI] = true
		units = append(units, ImpactUnit{
			Object:   u.Object,
			Type:     u.Type,
			URI:      u.URI,
			Distance: len(units),
			Frame:    frame,
		})
	}

	if u, ok := unitForFrame(DumpFrame{Program: dump.Program}); ok {
		add(u, nil)
	}
	for i := range stack {
		if u, ok := unitForFrame(stack[i]); ok {
			frame := stack[i]
			add(u, &frame)
		}
	}
	return units
}

// unanswerable says why a unit's where-used list would come back empty for a
// reason that has nothing to do with how many callers it has.
//
// Found live, and it is the worst kind of wrong answer: asking about a function
// group returns 200 with zero results and a description reading "SBAL_DB -
// SAPLSBAL_DB (Include)". The group URI resolves to the group's main include,
// and nothing references a main include — so the list is empty by construction
// whether the group has one caller or a thousand. Callers live on the modules.
// A dump frame that names its module gets asked about the module and never
// lands here; a frame that only names the group has nothing askable, and saying
// so is the only honest option.
func unanswerable(unit ImpactUnit) string {
	if unit.Type == "FUGR" {
		return "a function group's where-used list resolves to its main include and comes back empty whatever the truth is; the callers are on the modules (vsp graph FUNC <module> --direction callers)"
	}
	return ""
}

// repoUnit is what a dump frame points at in the repository: an addressable
// object, not a compiled program name.
type repoUnit struct {
	Object string
	Type   string
	URI    string
}

// unitForFrame maps one dump stack frame to the object a where-used list can be
// asked about.
//
// The trap is that a dump names *compiled programs*, and only some of those are
// repository objects. A class pool arrives as ZCL_X========CP and a function
// group as SAPLZFOO; neither is addressable under /programs/programs, and
// asking for one there is a 404 that reads exactly like "nobody calls this".
// (correlate.go's programURI had the second half of that bug: it unwrapped
// class pools and sent function groups to the program path.)
//
// A FUNCTION frame is the good case. It names the module, the module has its
// own where-used list, and that list is much narrower than the whole group's —
// which is the difference between "who calls BAL_DB_SEARCH" and "who calls
// anything in SBAL_DB".
func unitForFrame(frame DumpFrame) (repoUnit, bool) {
	program := strings.TrimSpace(frame.Program)
	if program == "" {
		return repoUnit{}, false
	}

	// A class or interface pool: the name, padded with '=', then a two-letter
	// pool suffix. IP and IU are the interface ones.
	if i := strings.Index(program, "="); i > 0 {
		name := strings.TrimRight(program[:i], "=")
		suffix := strings.TrimLeft(program[i:], "=")
		if name == "" {
			return repoUnit{}, false
		}
		if strings.HasPrefix(suffix, "IP") || strings.HasPrefix(suffix, "IU") {
			return repoUnit{name, "INTF", "/sap/bc/adt/oo/interfaces/" + adtSegment(name)}, true
		}
		return repoUnit{name, "CLAS", "/sap/bc/adt/oo/classes/" + adtSegment(name)}, true
	}

	if group, ok := functionGroupOf(program, frame.Include); ok {
		base := "/sap/bc/adt/functions/groups/" + adtSegment(group)
		if module := functionModuleOf(frame); module != "" {
			return repoUnit{module, "FUNC", base + "/fmodules/" + adtSegment(module)}, true
		}
		return repoUnit{group, "FUGR", base}, true
	}

	return repoUnit{program, "PROG", "/sap/bc/adt/programs/programs/" + adtSegment(program)}, true
}

// functionGroupOf recovers the group behind a function pool. The main pool is
// SAPL<group>; the pieces are L<group>U01, L<group>F02, L<group>TOP and so on,
// and a dump frame often names one of those rather than the pool.
func functionGroupOf(program, include string) (string, bool) {
	if group, ok := groupFromPool(program); ok {
		return group, true
	}
	return groupFromPool(include)
}

func groupFromPool(name string) (string, bool) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if strings.HasPrefix(name, "SAPL") && len(name) > 4 {
		return name[4:], true
	}
	// L<group><section>: the section is a letter and two more characters, or
	// the literal TOP. Anything shorter is not a function pool include.
	if len(name) > 4 && name[0] == 'L' {
		if strings.HasSuffix(name, "TOP") {
			return name[1 : len(name)-3], true
		}
		tail := name[len(name)-3:]
		if isPoolSection(tail) {
			return name[1 : len(name)-3], true
		}
	}
	return "", false
}

// isPoolSection recognises the U01/F02/I03/E01 suffix of a function pool
// include: one letter then two digits.
func isPoolSection(tail string) bool {
	if len(tail) != 3 {
		return false
	}
	if tail[0] < 'A' || tail[0] > 'Z' {
		return false
	}
	for _, ch := range tail[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// functionModuleOf returns the module a FUNCTION frame names. A method name
// carries => or ~ and is not a module, so those are refused rather than turned
// into a URI that would 404.
func functionModuleOf(frame DumpFrame) string {
	if !strings.EqualFold(strings.TrimSpace(frame.Type), "FUNCTION") {
		return ""
	}
	name := strings.TrimSpace(frame.Name)
	if name == "" || strings.Contains(name, "=>") || strings.Contains(name, "~") {
		return ""
	}
	return name
}

// adtSegment lowercases a name for an ADT path and escapes it, which matters
// for namespaced objects: /SDF/GET_APP_LOG has to arrive as %2Fsdf%2Fget_app_log.
func adtSegment(name string) string {
	return url.PathEscape(strings.ToLower(strings.TrimSpace(name)))
}

// WhereUsed answers "who calls this" for one ADT object, over the where-used
// list SE84 uses. It is the same filtering the dump impact query relies on, and
// it is exported because "who calls this" is not a question only a dump asks.
//
// The name the object goes by is taken from its own URI, which is what lets the
// self-references be dropped.
func (c *Client) WhereUsed(ctx context.Context, objectURI string) ([]ExposedCaller, error) {
	refs, err := c.FindReferences(ctx, objectURI, 0, 0)
	if err != nil {
		return nil, err
	}
	return exposedCallers(refs, objectNameFromURI(objectURI)), nil
}

// objectNameFromURI recovers the object's own name from its ADT path. The
// escaping matters: a namespaced object arrives as %2Fdemo%2Fzreport and would
// otherwise never match itself.
func objectNameFromURI(objectURI string) string {
	path := objectURI
	if i := strings.IndexAny(path, "#?"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	segment := path[i+1:]
	if unescaped, err := url.PathUnescape(segment); err == nil {
		segment = unescaped
	}
	return strings.ToUpper(segment)
}

// exposedCallers turns a where-used response into callers.
//
// Two filters carry the whole result, and both are easy to leave out and be
// badly wrong:
//
// The list is flat but two-level. A row with no usageInformation is a container
// — the object or package the rows under it belong to — and only the rows
// beneath it are references. Counting containers as callers doubles the answer.
//
// Grade separates a real reference from the target's own parts.
// gradeComponent rows are the object describing itself: every method of the
// class you asked about is listed as a component of it. Only gradeDirect is
// somebody else's code reaching in, so only gradeDirect is blast radius.
//
// One more thing gets dropped, found by running this against a live system:
// packages come back as containers of their own, with the package interfaces
// listed under them as direct references. A package naming an object in its
// interface is visibility, not a call — it cannot reach the broken code and it
// cannot be paged about — so DEVC containers are not callers.
func exposedCallers(refs []UsageReference, target string) []ExposedCaller {
	containers := map[string]UsageReference{}
	for _, r := range refs {
		if r.UsageInformation == "" && r.URI != "" {
			containers[r.URI] = r
		}
	}

	// An index rather than a pointer: appending to out reallocates, and a
	// pointer into the old backing array would silently update nothing.
	byName := map[string]int{}
	var out []ExposedCaller
	for _, r := range refs {
		if !strings.Contains(r.UsageInformation, "gradeDirect") {
			continue
		}
		owner := containers[r.ParentURI]
		if isPackaging(owner.Type) || isPackaging(r.Type) {
			continue
		}
		name := strings.TrimSpace(owner.Name)
		if name == "" {
			name = strings.TrimSpace(r.Name)
		}
		if name == "" || equalFoldTrim(name, target) {
			// A class listing a reference to itself is not exposure.
			continue
		}
		caller := ExposedCaller{
			Name:      name,
			Type:      owner.Type,
			URI:       strings.TrimSpace(owner.URI),
			Package:   firstNonEmpty(owner.PackageName, r.PackageName),
			Component: strings.TrimSpace(r.Name),
			IsTest:    strings.Contains(strings.ToLower(r.UsageInformation), "test"),
		}
		key := trimUpper(caller.Name)
		if at, seen := byName[key]; seen {
			// One object can reference the target from several routines; the
			// object is the unit of exposure, so the extra rows only add to the
			// component list rather than becoming separate callers.
			if caller.Component != "" && !strings.Contains(out[at].Component, caller.Component) {
				out[at].Component += ", " + caller.Component
			}
			continue
		}
		out = append(out, caller)
		byName[key] = len(out) - 1
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Productive callers before tests: a test that exercises the broken
		// code is real exposure but not the one anybody is paged about.
		if out[i].IsTest != out[j].IsTest {
			return !out[i].IsTest
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// rankExposure flattens the per-unit answers into one ranked list and splits
// off the callers that are on the dump's own stack.
//
// Those are not additional exposure — they are the route this dump already
// took, and the stack printed above them says so. They are kept rather than
// dropped because seeing the known path in the answer is what confirms the
// query aimed at the object that actually failed.
func rankExposure(units []ImpactUnit, dump Dump, stack []DumpFrame) (exposed, onPath []ExposedCaller) {
	onStack := map[string]bool{}
	for _, name := range StackPrograms(stack) {
		if u, ok := unitForFrame(DumpFrame{Program: name}); ok {
			onStack[trimUpper(u.Object)] = true
		}
	}
	if u, ok := unitForFrame(DumpFrame{Program: dump.Program}); ok {
		onStack[trimUpper(u.Object)] = true
	}

	// Non-nil so the JSON says "no exposure" with [] rather than null, which a
	// consumer has to special-case and a person reads as "not computed".
	exposed = []ExposedCaller{}

	seen := map[string]bool{}
	for _, unit := range units {
		for _, caller := range unit.Callers {
			key := trimUpper(caller.Name)
			if seen[key] {
				// Nearest the failure wins, and units are walked outward, so
				// the first sighting is already the shallowest.
				continue
			}
			seen[key] = true
			if onStack[key] {
				onPath = append(onPath, caller)
				continue
			}
			exposed = append(exposed, caller)
		}
	}
	return exposed, onPath
}

// isPackaging recognises the rows that describe where an object is allowed to
// be seen from rather than who reaches it: packages (DEVC) and the package
// interfaces (PINF) that list their contents. SAPMSSY1's only direct reference
// on a live system is one of these, and reporting it would have said a kernel
// dispatcher has exactly one caller, which is a package.
func isPackaging(adtType string) bool {
	t := strings.ToUpper(strings.TrimSpace(adtType))
	return strings.HasPrefix(t, "DEVC") || strings.HasPrefix(t, "PINF")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

package ctxcomp

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// AnalysisLayer identifies which analysis method found a dependency.
type AnalysisLayer int

const (
	LayerRegex     AnalysisLayer = iota // 3b: Go regex (ctxcomp) — filesystem, fastest
	LayerParser                         // 3:  abaplint parser — filesystem or SAP, deep
	LayerScan                           // 2:  SCAN ABAP-SOURCE — SAP kernel tokenizer
	LayerCross                          // 1b: CROSS/WBCROSSGT — SAP index, instant
	LayerWhereUsed                      // 1:  ADT Where-Used — SAP full cross-ref
)

func (l AnalysisLayer) String() string {
	switch l {
	case LayerRegex:
		return "regex"
	case LayerParser:
		return "parser"
	case LayerScan:
		return "scan"
	case LayerCross:
		return "cross"
	case LayerWhereUsed:
		return "where-used"
	}
	return "unknown"
}

// AnalyzedDep is a dependency found by one or more layers.
type AnalyzedDep struct {
	Name       string
	Kind       DependencyKind
	Line       int
	FoundBy    []AnalysisLayer // which layers found it
	Confidence float64         // 0.0-1.0, higher = more certain
	InString   bool            // true if found inside a string literal (false positive)
	InComment  bool            // true if found inside a comment (false positive)
}

// AnalysisResult holds the combined output.
type AnalysisResult struct {
	ObjectName     string
	TotalLines     int
	Dependencies   []AnalyzedDep
	TrueDeps       int // confirmed by 2+ layers or high confidence
	FalsePositives int
	Layers         []AnalysisLayer // which layers were used
	Duration       time.Duration
	LayerDurations map[AnalysisLayer]time.Duration
}

// ADTProvider abstracts SAP-side operations (layers 1, 1b, 2).
type ADTProvider interface {
	// ScanSource runs SCAN ABAP-SOURCE on SAP (layer 2)
	ScanSource(ctx context.Context, source string) ([]ScanToken, error)
	// GetCrossReferences queries CROSS/WBCROSSGT (layer 1b)
	GetCrossReferences(ctx context.Context, objectName string) ([]string, error)
	// GetWhereUsed runs ADT where-used (layer 1)
	GetWhereUsed(ctx context.Context, objectURL string) ([]string, error)
}

// ScanToken represents a token from SCAN ABAP-SOURCE.
type ScanToken struct {
	Type string // I=identifier, S=string, C=comment, K=keyword
	Str  string
	Row  int
	Col  int
}

// Analyzer combines all layers for comprehensive code intelligence.
//
// Confidence model:
//
//	1.0  — parser + SAP layer (scan or cross) agree
//	0.95 — parser + regex agree (no SAP needed)
//	0.9  — parser only (authoritative — reads actual source)
//	0.85 — SCAN ABAP-SOURCE only (SAP kernel, reliable)
//	0.8  — CROSS index + regex agree
//	0.6  — CROSS index only (may be stale — only updated on activation)
//	0.3  — regex only (likely false positive — found in string/comment)
//
// Key insight: CROSS/WBCROSSGT tables can be stale (inactive objects,
// $TMP, unactivated changes). The abaplint parser is the real-time
// ground truth — it parses actual source, not an index snapshot.
// Use parser as primary harness, CROSS as supplementary confirmation.
type Analyzer struct {
	adtProvider ADTProvider // nil = offline mode (layers 3, 3b only)
}

// NewAnalyzer creates a new analyzer. If adtProvider is nil, only offline layers are used.
func NewAnalyzer(adtProvider ADTProvider) *Analyzer {
	return &Analyzer{adtProvider: adtProvider}
}

// Analyze runs all available layers and combines results.
func (a *Analyzer) Analyze(ctx context.Context, source, objectName string) *AnalysisResult {
	start := time.Now()
	result := &AnalysisResult{
		ObjectName:     objectName,
		TotalLines:     strings.Count(source, "\n") + 1,
		LayerDurations: make(map[AnalysisLayer]time.Duration),
	}

	// Merge map: name → AnalyzedDep
	merged := make(map[string]*AnalyzedDep)

	// Layer 3b: Go Regex (always available, fastest)
	t0 := time.Now()
	regexDeps := ExtractDependencies(source)
	result.LayerDurations[LayerRegex] = time.Since(t0)
	result.Layers = append(result.Layers, LayerRegex)

	for _, d := range regexDeps {
		addOrMerge(merged, d.Name, d.Kind, d.Line, LayerRegex)
	}

	// Layer 3: the ABAP statement parser.
	//
	// This layer was named for the parser and did not use it: it ran the same
	// regex again with string and comment filtering, and the comment said
	// "simulate". That made the corroboration below a fiction — an answer
	// marked found_by [regex, parser] told a reader two independent analyses
	// agreed, when it was one analysis run twice and filtered once.
	//
	// It is the parser now. Measured on one real class before the change, the
	// two disagreed by three dependencies out of nine; the parser also sees
	// what a regex cannot — a static call on the right of an assignment, the
	// exception classes in a CATCH, the RAISING in a signature.
	t0 = time.Now()
	parserDeps := parserLayerDeps(source, objectName)
	result.LayerDurations[LayerParser] = time.Since(t0)
	result.Layers = append(result.Layers, LayerParser)

	for _, d := range parserDeps {
		addOrMerge(merged, d.name, d.kind, d.line, LayerParser)
	}

	// A dependency found by one layer and not the other is not thereby false.
	//
	// The rule here used to be "found by regex, not by parser ⇒ it is in a
	// string or a comment", which assumes the parser is a superset of the
	// regex. It is not. The parser layer is the graph's edge extractor: it
	// models calls and references it draws edges for. The regex layer reads
	// declarations — INHERITING FROM, INTERFACES, TYPE REF TO, CREATE OBJECT.
	// The two answer different questions, so their disagreement is not
	// evidence about either one.
	//
	// What that rule produced, measured on this repo's own ABAP: in
	// ZCL_VSP_APC_HANDLER, whose whole job is to dispatch to them, every one of
	// ZCL_VSP_GIT_SERVICE, ZCL_VSP_DEBUG_SERVICE, ZCL_VSP_RFC_SERVICE,
	// ZCL_VSP_AMDP_SERVICE and ZCL_VSP_REPORT_SERVICE was reported as a likely
	// false positive at confidence 0.3 — along with its superclass and every
	// interface it implements. Twenty such across thirteen files, and the names
	// dismissed were in every case the most important ones in the file.
	//
	// So the claim is now made from evidence rather than from absence: a name is
	// marked as being in a string or a comment when every occurrence of it in
	// the source is, which is exactly what the field says.
	for _, dep := range merged {
		// A function module is named in a literal by construction — CALL
		// FUNCTION 'SSFC_BASE64_DECODE' — so "it only ever appears in quotes"
		// is true of every real one. Applying the check to them turned four
		// genuine dependencies in this repo's own ABAP into false positives,
		// which is the check making the same mistake as the rule it replaced,
		// in the other direction.
		if dep.Kind == KindFunction {
			continue
		}
		if occursOnlyInStringsOrComments(source, dep.Name) {
			dep.InComment = true
			dep.Confidence = 0.3
			result.FalsePositives++
			continue
		}
		// Corroboration still raises confidence — two layers seeing the same
		// name is worth more than one — but its absence no longer lowers it
		// below what a single layer's own evidence supports.
		if len(dep.FoundBy) > 1 {
			dep.Confidence = 0.95
		}
	}

	// Layer 2: SCAN ABAP-SOURCE (if SAP connected)
	if a.adtProvider != nil {
		t0 = time.Now()
		scanTokens, err := a.adtProvider.ScanSource(ctx, source)
		result.LayerDurations[LayerScan] = time.Since(t0)
		if err == nil {
			result.Layers = append(result.Layers, LayerScan)
			scanDeps := extractFromScanTokens(scanTokens)
			for _, d := range scanDeps {
				addOrMerge(merged, d.name, d.kind, 0, LayerScan)
			}
		}
	}

	// Layer 1b: CROSS/WBCROSSGT (if SAP connected)
	if a.adtProvider != nil && objectName != "" {
		t0 = time.Now()
		crossRefs, err := a.adtProvider.GetCrossReferences(ctx, objectName)
		result.LayerDurations[LayerCross] = time.Since(t0)
		if err == nil {
			result.Layers = append(result.Layers, LayerCross)
			for _, ref := range crossRefs {
				addOrMerge(merged, ref, inferKind(ref), 0, LayerCross)
			}
		}
	}

	// Confidence: corroboration raises it, and no single layer's silence lowers
	// it below what that layer's own evidence supports.
	//
	// The regex-only case used to end `dep.Confidence = 0.3; dep.InString =
	// true` — the same inference as the false-positive pass above, written
	// twice. It is wrong for the same reason: the parser layer is the graph's
	// edge extractor and models calls, while the regex reads declarations, so
	// "the parser did not see it" is a fact about what the parser looks for.
	// An INHERITING FROM is not a suspected string literal.
	for _, dep := range merged {
		if dep.InString || dep.InComment {
			continue // decided by the direct check, on evidence
		}
		if dep.Confidence > 0 {
			if dep.Confidence >= 0.5 {
				result.TrueDeps++
			}
			continue
		}

		hasParser := containsLayer(dep.FoundBy, LayerParser)
		hasRegex := containsLayer(dep.FoundBy, LayerRegex)
		hasScan := containsLayer(dep.FoundBy, LayerScan)
		hasCross := containsLayer(dep.FoundBy, LayerCross)

		switch {
		case hasParser && (hasScan || hasCross):
			dep.Confidence = 1.0 // parser plus a SAP-side layer
		case hasParser && hasRegex:
			dep.Confidence = 0.95 // two independent readers agree
		case hasScan && hasRegex:
			dep.Confidence = 0.95 // the kernel tokenizer and the regex agree
		case hasParser:
			dep.Confidence = 0.9 // a call the parser resolved
		case hasScan:
			dep.Confidence = 0.85 // the SAP kernel tokenizer saw it
		case hasCross && hasRegex:
			dep.Confidence = 0.8 // the index and the source agree
		case hasRegex:
			// A declaration — INHERITING FROM, INTERFACES, TYPE REF TO,
			// CREATE OBJECT. Only this layer reads them, so only this layer can
			// report them, and that is evidence rather than the absence of it.
			dep.Confidence = 0.8
		case hasCross:
			// The index says so and the source does not show it. Notable, and
			// possibly stale — the one case where a single layer earns less.
			dep.Confidence = 0.6
		default:
			dep.Confidence = 0.1
		}

		if dep.Confidence >= 0.5 {
			result.TrueDeps++
		}
	}

	// Sort by confidence (highest first), then name
	deps := make([]AnalyzedDep, 0, len(merged))
	for _, d := range merged {
		deps = append(deps, *d)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Confidence != deps[j].Confidence {
			return deps[i].Confidence > deps[j].Confidence
		}
		return deps[i].Name < deps[j].Name
	})

	result.Dependencies = deps
	result.Duration = time.Since(start)
	return result
}

// --- Internal helpers ---

type parsedDep struct {
	name string
	kind DependencyKind
	line int
}

// parserLayerDeps runs the shared statement parser and speaks its answer in the
// terms this package merges in.
//
// It is deliberately a translation and not a second implementation. The reason
// this layer had its own extraction at all was that pkg/graph carried no line
// numbers; the lexer always knew them and nothing asked, so the fix was one
// field there rather than a parser here.
func parserLayerDeps(source, objectName string) []parsedDep {
	name := strings.ToUpper(strings.TrimSpace(objectName))
	if name == "" {
		name = "SOURCE"
	}
	var out []parsedDep
	seen := map[string]bool{}
	for _, e := range graph.ExtractDepsFromSource(source, graph.NodeID("CLAS", name)) {
		parts := strings.SplitN(e.To, ":", 2)
		if len(parts) != 2 {
			continue
		}
		kind, target := parts[0], strings.ToUpper(parts[1])
		if target == "" || target == name || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, parsedDep{name: target, kind: dependencyKindOf(kind), line: e.Line})
	}
	return out
}

// dependencyKindOf maps a graph node type to the three kinds this package
// distinguishes. Anything else is a class as far as a contract lookup is
// concerned, and guessing more precisely would be inventing.
func dependencyKindOf(nodeType string) DependencyKind {
	switch strings.ToUpper(nodeType) {
	case "INTF":
		return KindInterface
	case "FUGR", "FUNC":
		return KindFunction
	default:
		return KindClass
	}
}

// extractWithValidation does regex extraction but skips matches inside strings and comments.
func extractWithValidation(source string) []parsedDep {
	var deps []parsedDep
	seen := make(map[string]bool)
	lines := strings.Split(source, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)

		// Skip full-line comments
		if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"") {
			continue
		}

		// Remove inline comments (everything after unquoted ")
		cleanLine := removeInlineComment(line)

		// Remove string literals (content between ' ' and ` `)
		// But preserve CALL FUNCTION 'name' pattern first
		cleanLine = removeStringLiterals(cleanLine)

		// CALL FUNCTION needs the original line (name is in string)
		for _, m := range reCallFunction.FindAllStringSubmatch(line, -1) {
			addParsedDep(&deps, seen, m[1], KindFunction, lineNum)
		}

		// Now extract from cleaned line
		for _, m := range reTypeRefTo.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reNew.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
		for _, m := range reStaticCall.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reIntfMethod.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindInterface, lineNum)
		}
		for _, m := range reInheriting.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
		for _, m := range reInterfaces.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindInterface, lineNum)
		}
		for _, m := range reCallFunction.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindFunction, lineNum)
		}
		for _, m := range reCast.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reRaising.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
	}

	return deps
}

func removeInlineComment(line string) string {
	inString := false
	for i := 0; i < len(line); i++ {
		if line[i] == '\'' {
			inString = !inString
		}
		if !inString && line[i] == '"' {
			return line[:i]
		}
	}
	return line
}

func removeStringLiterals(line string) string {
	var result strings.Builder
	inSingle := false
	inBacktick := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '\'' && !inBacktick {
			inSingle = !inSingle
			result.WriteByte(ch)
			continue
		}
		if ch == '`' && !inSingle {
			inBacktick = !inBacktick
			result.WriteByte(ch)
			continue
		}
		if inSingle || inBacktick {
			result.WriteByte('_') // replace content with placeholder
		} else {
			result.WriteByte(ch)
		}
	}
	return result.String()
}

func addParsedDep(deps *[]parsedDep, seen map[string]bool, name string, kind DependencyKind, line int) {
	upper := strings.ToUpper(name)
	if shouldSkip(upper) || seen[upper] {
		return
	}
	// Skip placeholder strings (from removeStringLiterals)
	if strings.Trim(upper, "_") == "" {
		return
	}
	seen[upper] = true
	*deps = append(*deps, parsedDep{name: upper, kind: kind, line: line})
}

func addOrMerge(merged map[string]*AnalyzedDep, name string, kind DependencyKind, line int, layer AnalysisLayer) {
	upper := strings.ToUpper(name)
	if shouldSkip(upper) {
		return
	}
	if existing, ok := merged[upper]; ok {
		if !containsLayer(existing.FoundBy, layer) {
			existing.FoundBy = append(existing.FoundBy, layer)
		}
		if kind == KindInterface && existing.Kind == KindClass {
			existing.Kind = KindInterface
		}
	} else {
		merged[upper] = &AnalyzedDep{
			Name:    upper,
			Kind:    kind,
			Line:    line,
			FoundBy: []AnalysisLayer{layer},
		}
	}
}

func containsLayer(layers []AnalysisLayer, target AnalysisLayer) bool {
	for _, l := range layers {
		if l == target {
			return true
		}
	}
	return false
}

// extractFromScanTokens extracts dependencies from SCAN ABAP-SOURCE tokens.
func extractFromScanTokens(tokens []ScanToken) []parsedDep {
	var deps []parsedDep
	seen := make(map[string]bool)
	prev := ""
	prevPrev := ""

	for _, tok := range tokens {
		if tok.Type != "I" { // only identifiers
			prevPrev = prev
			prev = ""
			continue
		}

		upper := strings.ToUpper(tok.Str)

		// TYPE REF TO <name>
		if prev == "TO" && prevPrev == "REF" {
			addParsedDep(&deps, seen, upper, inferKind(upper), tok.Row)
		}

		// <name>=>
		if strings.Contains(upper, "=>") {
			parts := strings.SplitN(upper, "=>", 2)
			if len(parts[0]) > 0 {
				addParsedDep(&deps, seen, parts[0], inferKind(parts[0]), tok.Row)
			}
		}

		// NEW <name>
		if prev == "NEW" && upper != "(" {
			// Remove trailing ( if present
			clean := strings.TrimSuffix(upper, "(")
			if len(clean) > 0 {
				addParsedDep(&deps, seen, clean, KindClass, tok.Row)
			}
		}

		prevPrev = prev
		prev = upper
	}

	return deps
}

// occursOnlyInStringsOrComments reports whether every occurrence of name in the
// source is inside a comment or a string literal.
//
// This is the direct measurement of what InString and InComment claim, and it
// replaces an inference from one layer not having seen the name. It is
// deliberately conservative: a single occurrence in real code is enough to
// treat the dependency as real, because dropping a genuine dependency from a
// reader's context is worse than carrying an extra one.
func occursOnlyInStringsOrComments(source, name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	found := false
	for _, line := range strings.Split(source, "\n") {
		upper := strings.ToUpper(line)
		idx := strings.Index(upper, name)
		if idx < 0 {
			continue
		}
		found = true
		if !occurrenceIsQuotedOrCommented(line, upper, name) {
			return false
		}
	}
	// A name that does not occur at all came from an index rather than from the
	// text — CROSS or where-used — and says nothing about strings.
	return found
}

// occurrenceIsQuotedOrCommented reports whether every occurrence of name on this
// one line sits inside a comment or a string literal.
func occurrenceIsQuotedOrCommented(line, upper, name string) bool {
	// A full-line comment: '*' in the first column.
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(line, "*") {
		return true
	}
	_ = trimmed

	for start := 0; ; {
		idx := strings.Index(upper[start:], name)
		if idx < 0 {
			return true
		}
		at := start + idx
		if !quotedOrCommentedAt(line, at) {
			return false
		}
		start = at + len(name)
	}
}

// quotedOrCommentedAt walks the line to the given offset, tracking whether it is
// inside a string literal, and reports the state there.
//
// ABAP has two quoting characters and one of them doubles as the comment
// marker: a double quote outside a string starts a comment that runs to the end
// of the line, and a single quote delimits a literal in which ” is an escaped
// quote.
func quotedOrCommentedAt(line string, offset int) bool {
	inLiteral := false
	for i := 0; i < offset && i < len(line); i++ {
		switch line[i] {
		case '\'':
			if inLiteral && i+1 < len(line) && line[i+1] == '\'' {
				i++ // an escaped quote inside the literal
				continue
			}
			inLiteral = !inLiteral
		case '"':
			if !inLiteral {
				return true // everything from here is a comment
			}
		case '`':
			inLiteral = !inLiteral
		}
	}
	return inLiteral
}

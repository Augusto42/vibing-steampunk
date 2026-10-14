package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// GetIncludeMerged returns the include's raw source with each referenced
// ENHO implementation spliced in at its anchor. The output is not valid
// ABAP you can re-upload — it's an annotated view that matches what SE80
// shows with "Display Source (Modified)" enabled.
//
// Anchor resolution is best-effort. When an anchor cannot be located, the
// corresponding ENHO source is appended at the end with an explanatory
// comment header; no error is returned for a single unresolved anchor.
//
// When the include has no enhancements this is equivalent to GetInclude.
func (c *Client) GetIncludeMerged(ctx context.Context, includeName string) (string, error) {
	raw, err := c.GetInclude(ctx, includeName)
	if err != nil {
		return "", fmt.Errorf("merged include %s: %w", includeName, err)
	}

	refs, err := c.ListEnhancementsForInclude(ctx, includeName)
	if err != nil {
		// Non-fatal: return the raw include with a warning banner so the
		// caller still gets something useful.
		return raw + "\n\n* === enhancement lookup failed: " + err.Error() + " ===\n", nil
	}
	if len(refs) == 0 {
		return raw, nil
	}

	type enhWithSource struct {
		ref    EnhancementRef
		source string
	}
	enhanced := make([]enhWithSource, 0, len(refs))
	for _, ref := range refs {
		// Preserve table-fallback metadata such as the exact REPOSRC include.
		// Re-resolving by name would discard it and can make classic-system
		// bridge reads fail for padded enhancement include names.
		refCopy := ref
		src, ferr := c.GetEnhancementByRef(ctx, &refCopy)
		if ferr != nil {
			// Record the failure inline but keep going.
			enhanced = append(enhanced, enhWithSource{
				ref:    ref,
				source: fmt.Sprintf("* <failed to fetch ENHO %s: %v>", ref.Name, ferr),
			})
			continue
		}
		enhanced = append(enhanced, enhWithSource{ref: ref, source: src})
	}

	merged := raw
	unresolved := make([]enhWithSource, 0)
	for _, e := range enhanced {
		ok, spliced := spliceAtAnchor(merged, e.ref, e.source)
		if ok {
			merged = spliced
			continue
		}
		unresolved = append(unresolved, e)
	}

	if len(unresolved) > 0 {
		var b strings.Builder
		b.WriteString(merged)
		if !strings.HasSuffix(merged, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n* === unresolved enhancements (anchor not found in include) ===\n")
		for _, e := range unresolved {
			b.WriteString(renderEnhBlock(e.ref, e.source, "anchor unresolved"))
		}
		merged = b.String()
	}

	return merged, nil
}

// anchorStartPattern matches the "$*$\SE:(N) Form X, Start" marker that the
// enhancement framework writes into the host include at each plug-in point.
// N is the anchor index inside the form/include. We use this for splicing
// because it is the most reliable, machine-readable anchor SAP emits.
var anchorStartPattern = regexp.MustCompile(`(?i)\$\*\$\\SE:\((\d+)\)\s+Form\s+([A-Z0-9_/]+),\s+Start`)

// spliceAtAnchor finds the first unoccupied anchor line in src that could
// host the given enhancement and inserts the enhancement's source after it.
// Returns (false, src) unchanged if no suitable anchor was found.
//
// Heuristic: we insert after the first anchor Start marker we encounter
// that does not already have an ENHANCEMENT ... ENDENHANCEMENT block
// directly below it. For includes with multiple anchors we may not pick
// the exact correct one without parsing the ENHO metadata — which is OK
// because both produce a readable merged view; the caller accepts this
// trade-off (documented on GetIncludeMerged).
func spliceAtAnchor(src string, ref EnhancementRef, enhSource string) (bool, string) {
	loc := anchorStartPattern.FindStringIndex(src)
	if loc == nil {
		return false, src
	}

	// Insert immediately after the line that contains the match.
	lineEnd := strings.IndexByte(src[loc[1]:], '\n')
	insertAt := loc[1]
	if lineEnd >= 0 {
		insertAt += lineEnd + 1
	}

	// Skip if an ENHANCEMENT block is already present just below this
	// anchor — we already rendered this one on a previous iteration, or
	// the include author inlined it manually.
	tail := src[insertAt:]
	if looksLikeEnhancementBlock(tail) {
		// Try to find the next anchor instead.
		rest := src[insertAt:]
		nextLoc := anchorStartPattern.FindStringIndex(rest)
		if nextLoc == nil {
			return false, src
		}
		// Recurse into the tail.
		ok, splicedTail := spliceAtAnchor(rest, ref, enhSource)
		if !ok {
			return false, src
		}
		return true, src[:insertAt] + splicedTail
	}

	block := renderEnhBlock(ref, enhSource, "")
	return true, src[:insertAt] + block + src[insertAt:]
}

func looksLikeEnhancementBlock(s string) bool {
	// Scan at most ~200 bytes; ENHANCEMENT/ENDENHANCEMENT are the markers.
	head := s
	if len(head) > 400 {
		head = head[:400]
	}
	return regexp.MustCompile(`(?i)^\s*ENHANCEMENT\s+\d+`).MatchString(head)
}

func renderEnhBlock(ref EnhancementRef, source, note string) string {
	var b strings.Builder
	kind := string(ref.Kind)
	if kind == "" {
		kind = "?"
	}
	b.WriteString("\n* vvv ENHO/")
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(ref.Name)
	if ref.PackageName != "" {
		b.WriteString(" (package ")
		b.WriteString(ref.PackageName)
		b.WriteString(")")
	}
	if note != "" {
		b.WriteString(" — ")
		b.WriteString(note)
	}
	b.WriteString(" vvv\n")
	b.WriteString(source)
	if !strings.HasSuffix(source, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("* ^^^ end of ")
	b.WriteString(ref.Name)
	b.WriteString(" ^^^\n")
	return b.String()
}

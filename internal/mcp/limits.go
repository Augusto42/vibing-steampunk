package mcp

import "fmt"

// How much of an answer to hand back by default.
//
// Measured against a live 7.58 rather than guessed. Thirteen ordinary calls
// returned 207,138 bytes, and five of them were 92% of it —
//
//	analyze callers        52,305    200 rows, and 200 was the default
//	analyze call_graph     52,305    the same answer; call_graph defaults to callers
//	analyze list_dumps     39,954    no MCP default at all, so pkg/adt's 100
//	search CL_*            24,809    100 hits
//	read CLAS              23,653    source and contracts, which is the point
//
// Bytes, because bytes are what was measured. The figure that matters is
// tokens and no tokeniser was run: at the usual four-bytes-a-token rule those
// 207,138 bytes are about 52,000 tokens, and for this material that rule
// probably undercounts — JSON repeats its keys, and a name like
// /BOBF/CL_CONF_ACT_GEN_HTML does not split the way prose does. Both columns
// are measured the same way, so the ratio holds even where the absolute number
// does not.
//
// None of that was a bug. Each default was chosen for a terminal, where a
// screenful costs nothing and scrolling is free. In an MCP session every one of
// those results stays in the context window for the rest of the conversation,
// so the same number is a very different price — and nobody had ever converted
// it.
//
// The defaults below are for the caller who did not say. A caller who names
// max_results still gets what it asks for, and truncation always says it
// happened and how to lift it. That last part is the whole discipline: a
// bounded answer that does not say it was bounded is a clean verdict over a
// list read in part.

// defaultRows is what a list-shaped answer returns when nobody asked for a
// number.
//
// Forty, because it is enough to see the shape of an answer and decide whether
// to widen it, and because forty rows of the widest of these answers is 11,296
// bytes where two hundred was 52,305. It is a default, not a limit:
// max_results overrides it in every handler that reads this.
const defaultRows = 40

// defaultDumps is smaller because a dump row is the widest of them all — a
// runtime error carries a program, a class, a user, a timestamp and a URI — and
// because the useful question about dumps is nearly always "the recent ones",
// not "all of them".
const defaultDumps = 20

// truncationNote is the sentence every capped answer owes its reader.
//
// One function so the wording cannot drift between handlers, and so the phrase
// naming the way out — the parameter, by its real name — is never omitted.
func truncationNote(shown, total int, param string) string {
	return fmt.Sprintf("showing %d of %d; raise %s to see the rest", shown, total, param)
}

// truncationNoteUnknownTotal is for the answers where counting the rest would
// cost another request. It promises less, deliberately: an invented total is
// worse than an admitted one.
// narrower names the way to ask a smaller question, which differs per answer:
// a search is narrowed by its pattern, a dump feed by its time window.
func truncationNoteUnknownTotal(shown int, param, narrower string) string {
	return fmt.Sprintf("showing %d, and there are more; raise %s, or %s", shown, param, narrower)
}

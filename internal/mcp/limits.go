package mcp

import "fmt"

// How much of an answer to hand back by default.
//
// Measured against a live 7.58 rather than guessed: thirteen ordinary calls
// came to 52,000 tokens, and five of them were 92% of it —
//
//	analyze callers        13,000    200 rows, and 200 was the default
//	analyze call_graph     13,000    the same answer; call_graph defaults to callers
//	analyze list_dumps     10,000    no MCP default at all, so 100 dumps
//	search CL_*             6,200    100 hits
//	read CLAS               5,900    source and contracts, which is the point
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
// to widen it, and because forty rows of the widest of these answers is about
// 2,600 tokens rather than 13,000. It is a default, not a limit: max_results
// overrides it in every handler that reads this.
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

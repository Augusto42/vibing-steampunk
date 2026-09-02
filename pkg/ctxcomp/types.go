// Package ctxcomp provides ABAP context compression.
// It extracts dependencies from ABAP source code and produces compressed
// "prologue" text containing only public API contracts of referenced objects.
package ctxcomp

import "context"

// DependencyKind classifies a dependency.
type DependencyKind string

const (
	KindClass     DependencyKind = "CLAS"
	KindInterface DependencyKind = "INTF"
	KindFunction  DependencyKind = "FUNC"
)

// Dependency represents a reference to an external ABAP object.
type Dependency struct {
	Name string
	Kind DependencyKind
	Line int // 1-based line where first reference was found
	// Methods are the methods this source calls on the dependency, when they
	// could be determined. Empty means nothing is known — which is different
	// from "none are called", and the contract is then kept whole rather than
	// narrowed on no information.
	Methods []string
}

// Contract is the compressed public API of a dependency.
type Contract struct {
	Name   string
	Kind   DependencyKind
	Source string // compressed public API text
	// MethodsTotal and MethodsShown say how much of the surface this contract
	// carries. A reader owed the knowledge that there is more is owed a number,
	// not a list of forty-seven names they did not ask for.
	MethodsTotal int
	MethodsShown int
	FromCache    bool
	Error        string // non-empty if resolution failed
}

// ContextResult is the output of context compression.
type ContextResult struct {
	SourceName   string
	Dependencies []Dependency
	Contracts    []Contract
	Prologue     string
	Stats        ContextStats
}

// ContextStats holds compression statistics.
type ContextStats struct {
	DepsFound    int
	DepsResolved int
	DepsFailed   int
	TotalLines   int // lines in prologue
}

// SourceProvider fetches full source code for a given object.
type SourceProvider interface {
	GetSource(ctx context.Context, kind DependencyKind, name string) (string, error)
}

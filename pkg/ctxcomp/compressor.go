package ctxcomp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Compressor orchestrates dependency extraction, source fetching, and contract generation.
type Compressor struct {
	provider SourceProvider
	maxDeps  int
	maxDepth int // 1 = direct deps only, 2 = deps of deps, etc.
}

// NewCompressor creates a new Compressor. maxDeps limits how many dependencies are resolved (default 20).
func NewCompressor(provider SourceProvider, maxDeps int) *Compressor {
	if maxDeps <= 0 {
		maxDeps = 20
	}
	return &Compressor{provider: provider, maxDeps: maxDeps, maxDepth: 1}
}

// WithDepth sets the dependency expansion depth (1=direct only, 2=deps of deps, max 3).
func (c *Compressor) WithDepth(depth int) *Compressor {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	c.maxDepth = depth
	return c
}

// Compress extracts dependencies from source, fetches their contracts, and builds a prologue.
func (c *Compressor) Compress(ctx context.Context, source, objectName, objectType string) (*ContextResult, error) {
	seen := map[string]bool{strings.ToUpper(objectName): true}
	var allDeps []Dependency
	var allContracts []Contract

	// Level 1: extract deps from main source
	pendingSources := []string{source}
	pendingNames := []string{objectName}

	for level := 1; level <= c.maxDepth; level++ {
		var levelDeps []Dependency
		for i, src := range pendingSources {
			deps := ExtractDependencies(src)
			deps = filterSelf(deps, pendingNames[i])
			// What this source calls on each of them, so the contract can be
			// narrowed to it. Unknown for a dependency reached only through a
			// declaration, and then the contract stays whole.
			called := MethodsCalledOn(src)
			for j := range deps {
				deps[j].Methods = called[deps[j].Name]
			}
			// Filter already-seen
			for _, d := range deps {
				if !seen[d.Name] {
					levelDeps = append(levelDeps, d)
					seen[d.Name] = true
				}
			}
		}

		if len(levelDeps) == 0 {
			break
		}

		// Rank by what a reader needs first — obligations, then signature
		// types, then collaborators by how often they are used, then
		// exceptions. See candidates.go for why line order was the wrong
		// proxy: it put a type used once above the superclass.
		//
		// Ranked against the source that named them, which for level one is
		// the object being read and for deeper levels is the dependency whose
		// own source was just fetched.
		ranked := RankCandidates(strings.Join(pendingSources, "\n"), levelDeps)
		levelDeps = levelDeps[:0]
		for _, r := range ranked {
			levelDeps = append(levelDeps, r.Dependency)
		}

		// The budget is spent on contracts that arrive, not on attempts.
		//
		// It used to be a slice off the front of the candidate list, fetched
		// once: five candidates, five slots, and however many of them failed
		// were five slots gone. The old line ordering hid that by accident —
		// it happened to put fetchable classes first — and ranking exposed it,
		// with three of five slots going to names that have no contract to
		// fetch at all. A budget of five that delivers two is not a budget of
		// five.
		//
		// So candidates are taken in ranked order, in batches, until the budget
		// is filled or the list runs out. A failure costs a fetch and not a
		// slot.
		remaining := c.maxDeps - len(allDeps)
		if remaining <= 0 {
			break
		}
		var levelKept []Dependency
		var levelContracts []Contract
		var levelSources []string
		for offset := 0; offset < len(levelDeps) && len(levelKept) < remaining; {
			batch := levelDeps[offset:]
			if want := (remaining - len(levelKept)) * 2; len(batch) > want && want > 0 {
				batch = batch[:want]
			}
			contracts, sources := c.fetchContractsWithSources(ctx, batch)
			for i, ct := range contracts {
				if len(levelKept) >= remaining {
					break
				}
				if ct.Error != "" {
					// Kept in the answer: a dependency that could not be
					// resolved is a gap the reader should see, and dropping it
					// silently is how a partial context reads as a whole one.
					allDeps = append(allDeps, batch[i])
					allContracts = append(allContracts, ct)
					continue
				}
				levelKept = append(levelKept, batch[i])
				levelContracts = append(levelContracts, ct)
				levelSources = append(levelSources, sources[i])
			}
			offset += len(batch)
		}

		allDeps = append(allDeps, levelKept...)
		allContracts = append(allContracts, levelContracts...)
		levelDeps = levelKept
		fullSources := levelSources

		// Prepare next level: extract deps from fetched full sources
		if level < c.maxDepth {
			pendingSources = nil
			pendingNames = nil
			for i, src := range fullSources {
				if src != "" {
					pendingSources = append(pendingSources, src)
					pendingNames = append(pendingNames, levelDeps[i].Name)
				}
			}
		}
	}

	prologue := formatPrologue(objectName, allContracts)
	lines := strings.Count(prologue, "\n") + 1

	stats := ContextStats{
		DepsFound:  len(allDeps),
		TotalLines: lines,
	}
	for _, ct := range allContracts {
		if ct.Error != "" {
			stats.DepsFailed++
		} else {
			stats.DepsResolved++
		}
	}

	return &ContextResult{
		SourceName:   objectName,
		Dependencies: allDeps,
		Contracts:    allContracts,
		Prologue:     prologue,
		Stats:        stats,
	}, nil
}

// fetchContractsWithSources fetches contracts and returns full sources for deeper expansion.
func (c *Compressor) fetchContractsWithSources(ctx context.Context, deps []Dependency) ([]Contract, []string) {
	contracts := make([]Contract, len(deps))
	fullSources := make([]string, len(deps))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // bounded parallelism

	for i, dep := range deps {
		wg.Add(1)
		go func(idx int, d Dependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fullSource, err := c.provider.GetSource(ctx, d.Kind, d.Name)
			if err != nil {
				contracts[idx] = Contract{
					Name:  d.Name,
					Kind:  d.Kind,
					Error: err.Error(),
				}
				return
			}

			fullSources[idx] = fullSource
			compressed := ExtractContract(fullSource, d.Kind)
			narrowed, total, shown := NarrowContract(compressed, d.Methods)
			contracts[idx] = Contract{
				Name:         d.Name,
				Kind:         d.Kind,
				Source:       narrowed,
				MethodsTotal: total,
				MethodsShown: shown,
			}
		}(i, dep)
	}

	wg.Wait()
	return contracts, fullSources
}

func filterSelf(deps []Dependency, objectName string) []Dependency {
	upper := strings.ToUpper(objectName)
	filtered := make([]Dependency, 0, len(deps))
	for _, d := range deps {
		if d.Name != upper {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func isCustom(name string) bool {
	return strings.HasPrefix(name, "Z") || strings.HasPrefix(name, "Y") ||
		strings.HasPrefix(name, "/Z") || strings.HasPrefix(name, "/Y")
}

func formatPrologue(objectName string, contracts []Contract) string {
	if len(contracts) == 0 {
		return ""
	}

	var resolved []Contract
	var unresolved []string
	for _, c := range contracts {
		if c.Error == "" && c.Source != "" {
			resolved = append(resolved, c)
			continue
		}
		unresolved = append(unresolved, c.Name)
	}

	if len(resolved) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("* === Dependency context for %s (%d deps) ===\n", objectName, len(resolved)))

	// Named, not dropped. The unresolved ones used to be filtered out here and
	// the header then said "(5 deps)" as though five were all there was — a
	// reader given a context with nine names missing from it, and no way to
	// know. Most are data elements and structures, which have no public section
	// to compress and never had a contract to fetch; a few are objects that
	// could not be read. Both are things the reader is entitled to know the
	// names of, and one line carries them without crowding out the contracts.
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		sb.WriteString(fmt.Sprintf("* %d referenced without a contract here (types, structures, or unreadable): %s\n",
			len(unresolved), strings.Join(unresolved, ", ")))
	}

	for _, c := range resolved {
		kindLabel := "class"
		switch c.Kind {
		case KindInterface:
			kindLabel = "interface"
		case KindFunction:
			kindLabel = "function module"
		}

		// The header carries what the narrowing dropped, as a number. That is
		// the whole of what a reader is owed about the rest of the surface —
		// the surface itself is one explicit request away, and padding every
		// context with it is the opposite of compressing.
		methodCount := c.MethodsTotal
		if methodCount == 0 {
			methodCount = strings.Count(strings.ToUpper(c.Source), "METHODS ")
		}
		info := kindLabel
		if methodCount > 0 {
			info = fmt.Sprintf("%s, %d methods", kindLabel, methodCount)
			if c.MethodsShown > 0 && c.MethodsShown < c.MethodsTotal {
				info += fmt.Sprintf("; %d called here", c.MethodsShown)
			}
		}

		sb.WriteString(fmt.Sprintf("\n* --- %s (%s) ---\n", c.Name, info))
		sb.WriteString(c.Source)
		sb.WriteString("\n")
	}

	return sb.String()
}

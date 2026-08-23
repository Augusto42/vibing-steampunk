package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var dumpsCmd = &cobra.Command{
	Use:   "dumps",
	Short: "Read runtime errors (ST22), group them, and see what was logged around one",
	Long: `Read ABAP runtime errors over plain ADT.

The feed carries the error type and the terminated program as structured
fields, so listing and grouping need no HTML parsing and no Z code.

  vsp dumps --since 2026-08-01                 # newest first
  vsp dumps --group                            # what keeps failing, not what failed once
  vsp dumps --program SAPLSBAL_DB
  vsp dumps --explain latest --tolerance 5m    # and what the application log said around it
  vsp dumps --impact latest                    # who else calls the code that failed

--explain ranks log entries by the argument for them, not by the clock. An
entry written by the program that dumped is connected structurally; one that is
merely nearby in time is a coincidence until something says otherwise.

--impact asks the opposite direction and is not evidence of anything. A caller
that took part in this failure is already on the stack --explain prints; the
callers listed here did not run, which is precisely why they are worth knowing
about. It is the blast radius: who else reaches the broken code, and would.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		ctx := context.Background()

		filter := adt.DumpFilter{}
		filter.ErrorType, _ = cmd.Flags().GetString("error")
		filter.Program, _ = cmd.Flags().GetString("program")
		filter.User, _ = cmd.Flags().GetString("user")
		filter.Limit, _ = cmd.Flags().GetInt("top")
		if since, _ := cmd.Flags().GetString("since"); strings.TrimSpace(since) != "" {
			when, perr := time.Parse("2006-01-02", strings.TrimSpace(since))
			if perr != nil {
				return fmt.Errorf("--since wants a date as YYYY-MM-DD, got %q", since)
			}
			filter.From = when
		}

		dumps, err := client.Dumps(ctx, filter)
		if err != nil {
			return err
		}
		if len(dumps) == 0 {
			fmt.Fprintln(os.Stderr, "no runtime errors match")
			return nil
		}

		asJSON, _ := cmd.Flags().GetBool("json")

		if impact, _ := cmd.Flags().GetString("impact"); impact != "" {
			dump, derr := pickDump(dumps, impact)
			if derr != nil {
				return derr
			}
			return impactOfDump(ctx, client, cmd, dump, asJSON)
		}

		if explain, _ := cmd.Flags().GetString("explain"); explain != "" {
			return explainDump(ctx, client, cmd, dumps, explain, asJSON)
		}

		if grouped, _ := cmd.Flags().GetBool("group"); grouped {
			groups := adt.GroupDumps(dumps)
			if asJSON {
				return emitJSON(groups)
			}
			fmt.Printf("%-5s %-34s %-30s %s\n", "COUNT", "RUNTIME ERROR", "PROGRAM", "LAST SEEN")
			fmt.Println(strings.Repeat("-", 100))
			for _, g := range groups {
				fmt.Printf("%-5d %-34s %-30s %s\n", g.Count, g.ErrorType, g.Program, stamp(g.Last))
			}
			fmt.Fprintf(os.Stderr, "\n%d distinct failures across %d dumps\n", len(groups), len(dumps))
			return nil
		}

		if asJSON {
			return emitJSON(dumps)
		}
		fmt.Printf("%-19s %-30s %-26s %s\n", "WHEN", "RUNTIME ERROR", "PROGRAM", "USER")
		fmt.Println(strings.Repeat("-", 100))
		for _, d := range dumps {
			fmt.Printf("%-19s %-30s %-26s %s\n", stamp(d.At), d.ErrorType, d.Program, d.User)
		}
		fmt.Fprintf(os.Stderr, "\n%d dumps\n", len(dumps))
		return nil
	},
}

// explainDump shows one dump and what the application log said around it.
func explainDump(ctx context.Context, client *adt.Client, cmd *cobra.Command, dumps []adt.Dump, which string, asJSON bool) error {
	dump, err := pickDump(dumps, which)
	if err != nil {
		return err
	}

	tolerance, _ := cmd.Flags().GetDuration("tolerance")
	matches, err := client.CorrelateDump(ctx, dump, tolerance, 20)
	if err != nil {
		return err
	}
	// Read separately for display; the correlation already used it for ranking.
	stack, stackErr := client.DumpStack(ctx, dump.ID)

	if asJSON {
		return emitJSON(struct {
			Dump    adt.Dump        `json:"dump"`
			Stack   []adt.DumpFrame `json:"stack,omitempty"`
			Matches []adt.LogMatch  `json:"matches"`
		}{dump, stack, matches})
	}

	fmt.Printf("%s  %s\n", stamp(dump.At), dump.ErrorType)
	fmt.Printf("  program %s, user %s\n", dump.Program, dump.User)
	if dump.Message != "" {
		fmt.Printf("  %s\n", dump.Message)
	}
	fmt.Println()

	switch {
	case errors.Is(stackErr, adt.ErrDumpDetailUnavailable):
		// Not a fault: the release has the feed and not the detail resource.
		fmt.Fprintf(os.Stderr, "%v\n\n", stackErr)
	case stackErr != nil:
		fmt.Fprintf(os.Stderr, "the call stack could not be read: %v\n\n", stackErr)
	case len(stack) > 0:
		fmt.Println("Call stack at the failure:")
		for _, f := range stack {
			where := f.Program
			if f.Include != "" && f.Include != f.Program {
				where += "/" + f.Include
			}
			fmt.Printf("  %3d %-12s %s:%d\n", f.Position, f.Type, where, f.Line)
			if f.Name != "" {
				fmt.Printf("      %s\n", f.Name)
			}
		}
		fmt.Println()
	}

	if len(matches) == 0 {
		fmt.Println("Nothing was written to the application log in that window.")
		return nil
	}
	fmt.Println("Application log entries around it, best argument first:")
	fmt.Println()
	for _, m := range matches {
		fmt.Printf("  %s  %s/%s\n", stamp(m.Entry.At), m.Entry.Object, m.Entry.SubObject)
		fmt.Printf("      %s\n", m.Why)
	}
	fmt.Fprintln(os.Stderr, "\nRanked by the argument for each, not by nearness. A match is a candidate, not a cause.")
	return nil
}

// pickDump resolves 'latest' or a fragment of an id against a listing.
func pickDump(dumps []adt.Dump, which string) (adt.Dump, error) {
	if strings.EqualFold(which, "latest") {
		return dumps[0], nil
	}
	for _, d := range dumps {
		if strings.Contains(d.ID, which) {
			return d, nil
		}
	}
	return adt.Dump{}, fmt.Errorf("no dump in this range has an id containing %q; pass 'latest' or part of an id from the listing", which)
}

// impactOfDump answers the question --explain does not: not what caused this
// failure, but who else runs into it.
//
// The two are separate flags on purpose. --explain ranks candidates for a
// cause, and every row it prints is arguable; this prints static facts about
// the repository, and none of them are. Merging the two lists would let the
// confidence of the second leak into how the first is read.
func impactOfDump(ctx context.Context, client *adt.Client, cmd *cobra.Command, dump adt.Dump, asJSON bool) error {
	frames, _ := cmd.Flags().GetInt("impact-frames")
	top, _ := cmd.Flags().GetInt("impact-top")

	result, err := client.DumpImpact(ctx, dump, adt.DumpImpactOptions{MaxUnits: frames, Limit: top})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(result)
	}

	fmt.Printf("%s  %s\n", stamp(dump.At), dump.ErrorType)
	fmt.Printf("  program %s, user %s\n", dump.Program, dump.User)
	if dump.Message != "" {
		fmt.Printf("  %s\n", dump.Message)
	}
	fmt.Println()

	if result.StackUnavailable {
		fmt.Fprintln(os.Stderr, "the call stack could not be read, so only the dump's own program was asked about")
		fmt.Fprintln(os.Stderr)
	}

	fmt.Println("Asked of:")
	for _, u := range result.Units {
		if u.Err != "" {
			fmt.Printf("  %-4s %-34s no where-used list: %s\n", u.Type, u.Object, u.Err)
			continue
		}
		if u.Note != "" {
			fmt.Printf("  %-4s %-34s not asked - %s\n", u.Type, u.Object, u.Note)
			continue
		}
		where := ""
		if u.Frame != nil {
			where = fmt.Sprintf("   frame %d, line %d", u.Frame.Position, u.Frame.Line)
		}
		fmt.Printf("  %-4s %-34s %4d direct callers%s\n", u.Type, u.Object, u.Total, where)
	}
	fmt.Println()

	switch {
	case !result.Answerable():
		// An empty answer and an unaskable question look the same in the
		// numbers and mean opposite things.
		fmt.Println("No unit of this dump has a where-used list that can answer. This is not a finding of zero callers.")
	case len(result.Exposed) == 0:
		fmt.Println("Nothing else calls this code. It is reachable only by the path the dump took.")
	default:
		fmt.Printf("Exposed - reaches the failing code by another path (%d):\n\n", len(result.Exposed))
		for _, e := range result.Exposed {
			marker := " "
			if e.IsTest {
				marker = "t"
			}
			fmt.Printf("  %s %-34s %-9s %-20s via %s\n", marker, e.Name, e.Type, e.Package, e.Via)
			if e.Component != "" {
				fmt.Printf("      in %s\n", e.Component)
			}
		}
	}

	if len(result.OnPath) > 0 {
		fmt.Println()
		fmt.Printf("On the dump's own stack, so not additional exposure (%d): %s\n",
			len(result.OnPath), strings.Join(callerNamesOf(result.OnPath), ", "))
	}

	fmt.Fprintln(os.Stderr, "\nWho can reach the bug, not who caused it. Object level: the where-used list resolves a method to its class, so a caller here reaches the class and not necessarily the failing method.")
	return nil
}

func callerNamesOf(callers []adt.ExposedCaller) []string {
	names := make([]string, 0, len(callers))
	for _, c := range callers {
		names = append(names, c.Name)
	}
	return names
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func init() {
	dumpsCmd.Flags().String("since", "", "Earliest date, YYYY-MM-DD")
	dumpsCmd.Flags().String("error", "", "Only this runtime error type")
	dumpsCmd.Flags().String("program", "", "Only this terminated program")
	dumpsCmd.Flags().String("user", "", "Only this user")
	dumpsCmd.Flags().Int("top", 100, "Maximum dumps to read")
	dumpsCmd.Flags().Bool("group", false, "Group by what failed rather than when")
	dumpsCmd.Flags().String("explain", "", "Show one dump ('latest' or part of an id) with the log around it")
	dumpsCmd.Flags().String("impact", "", "Show who else calls the code that failed in one dump ('latest' or part of an id)")
	dumpsCmd.Flags().Int("impact-frames", 3, "How many units to walk outward from the failing statement for --impact")
	dumpsCmd.Flags().Int("impact-top", 25, "Maximum callers to list per unit for --impact")
	dumpsCmd.Flags().Duration("tolerance", 5*time.Minute, "Window on each side of the dump for --explain")
	dumpsCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(dumpsCmd)
}

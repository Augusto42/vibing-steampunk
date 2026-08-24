package main

// `vsp sweep` calls everything the product advertises and reports what did not
// answer.
//
// It exists because ten capabilities were found dead in one week — advertised,
// registered, reachable, and never once correct — and every one was found by
// hand. A person looking is not a mechanism, and the eleventh would ship the
// same way. This is the mechanism.
//
// It is deliberately a sibling of `vsp compat` rather than part of it. compat
// asks the *system* what it supports; sweep asks *us* whether what we claim to
// support answers. They fail differently and a reader needs to know which one
// is talking.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

func init() {
	sweepCmd.Flags().Bool("json", false, "Emit the report as JSON, for diffing or for a record")
	sweepCmd.Flags().Bool("reach-only", false, "Run only the offline pass: is every advertised capability registered and routed")
	sweepCmd.Flags().String("only", "", "Run probes whose id or capability contains this")
	sweepCmd.Flags().String("class", "CL_ABAP_TYPEDESCR", "Class to probe reads and structure with")
	sweepCmd.Flags().String("program", "", "Program to probe with (default: found automatically)")
	sweepCmd.Flags().String("package", "", "Package to probe with (default: found automatically)")
	sweepCmd.Flags().String("table", "T000", "Table to probe free SQL with")
	sweepCmd.Flags().String("referenced", "CL_ABAP_TYPEDESCR", "An object known to be referenced by other code — the input for every caller probe")
	sweepCmd.Flags().String("references", "CL_ABAP_TYPEDESCR", "An object known to reference other code — the input for every callee probe")
	sweepCmd.Flags().Duration("timeout", 45*time.Second, "Cap one probe; a capability that exceeds it is reported, not waited on")
	sweepCmd.Flags().Bool("strict", false, "Exit non-zero when there is any finding, for CI")
	rootCmd.AddCommand(sweepCmd)
}

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Call everything this tool advertises, and report what did not answer",
	Long: `Walk the advertised capability surface and say which parts of it work.

Two passes, because a capability fails in two different ways:

  reach   Is it registered and routed at all? Needs no system, runs in CI, and
          is the check that finds a tool whitelisted behind a registration
          function nobody calls.

  answer  Called against a live system with an input that has an answer, does
          it produce one? This is the pass that finds a feature which has been
          returning an empty list since the day it shipped.

The distinction that matters is between an empty answer that is true and an
empty answer that is a failure wearing a truthful face. Probes that could not
tell the difference on their own carry an oracle — an independent second route
to the same fact — and when the oracle says there are twelve and the capability
says none, the report says "dead" rather than "no results".

The sweep never writes. Every probe is a read.

  vsp sweep --reach-only          # offline; no system needed
  vsp -s dev sweep                # the full pass
  vsp -s dev sweep --only graph   # one area
  vsp -s dev sweep --json         # a record to keep or to diff
  vsp -s dev sweep --strict       # non-zero exit on any finding`,
	RunE: runSweep,
}

func runSweep(cmd *cobra.Command, args []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	reachOnly, _ := cmd.Flags().GetBool("reach-only")
	strict, _ := cmd.Flags().GetBool("strict")
	only, _ := cmd.Flags().GetString("only")

	if reachOnly {
		report := &mcp.SweepReport{
			System:       "(no system)",
			Reach:        mcp.SweepReach(),
			ReachChecked: mcp.ReachChecked(),
		}
		if asJSON {
			return emitJSON(report)
		}
		fmt.Print(report.Text())
		return sweepExit(report, strict)
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	targets := mcp.SweepTargets{}
	targets.Class, _ = cmd.Flags().GetString("class")
	targets.Program, _ = cmd.Flags().GetString("program")
	targets.Package, _ = cmd.Flags().GetString("package")
	targets.Table, _ = cmd.Flags().GetString("table")
	targets.Referenced, _ = cmd.Flags().GetString("referenced")
	targets.References, _ = cmd.Flags().GetString("references")
	missed := fillSweepTargets(cmd, client, &targets)

	// A probe skipped because nothing was found to run it against says nothing
	// about the capability, and a reader who takes it for a clean result has
	// been misled by the report rather than by the code.
	if note := adt.UnsearchedNote(missed, len(missed)+4, "probe target"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}

	srv := mcp.NewServerWithClient(sweepConfig(), client)
	fmt.Fprintf(os.Stderr, "sweeping %s...\n", params.Name)
	perProbe, _ := cmd.Flags().GetDuration("timeout")
	report := srv.RunSweep(cmd.Context(), params.Name, targets, mcp.SweepOptions{
		Only:     only,
		PerProbe: perProbe,
		Progress: func(p mcp.Probe) { fmt.Fprintf(os.Stderr, "  %-28s %s\n", p.ID, p.Capability) },
	})
	report.Missed = missed

	if asJSON {
		if err := emitJSON(report); err != nil {
			return err
		}
		return sweepExit(report, strict)
	}
	fmt.Print(report.Text())
	return sweepExit(report, strict)
}

// sweepExit turns findings into an exit code, but only when asked.
//
// The default is to exit zero with the findings printed, because a person
// running this by hand wants to read it. --strict is for the build, where a
// finding must stop something.
func sweepExit(report *mcp.SweepReport, strict bool) error {
	findings := report.Findings()
	if strict && len(findings) > 0 {
		return fmt.Errorf("%d capability finding(s); see the report above", len(findings))
	}
	return nil
}

// fillSweepTargets finds a program and a package to probe with when the caller
// did not name them.
//
// The defaults for class, table, referenced and references are SAP-standard
// objects present on every release, so they need no search. A program and a
// package do: their names differ on every system, and probing with one that
// does not exist would turn a 404 we caused into a finding about the product.
func fillSweepTargets(cmd *cobra.Command, client *adt.Client, targets *mcp.SweepTargets) []adt.Unsearched {
	var missed []adt.Unsearched
	find := func(objType string, patterns ...string) string {
		var lastErr error
		for _, pattern := range patterns {
			results, err := client.SearchObject(cmd.Context(), pattern, 100)
			if err != nil {
				lastErr = err
				continue
			}
			for _, r := range results {
				if strings.EqualFold(r.Type, objType) {
					return r.Name
				}
			}
			lastErr = nil
		}
		if lastErr != nil {
			missed = append(missed, adt.Unsearched{Object: objType, Reason: lastErr.Error()})
		}
		return ""
	}
	if targets.Program == "" {
		targets.Program = find("PROG/P", "Z*", "R*")
	}
	if targets.Package == "" {
		targets.Package = find("DEVC/K", "Z*", "*")
	}
	return missed
}

// sweepConfig supplies the settings that shape registration, and nothing about
// the connection: the client is handed over already built, with this system's
// authentication and safety on it.
func sweepConfig() *mcp.Config {
	c := *cfg
	// Expert registers the widest surface, which is what a sweep should walk.
	// The universal router is reached directly whatever the mode, so this
	// affects only the reach pass's view of tool registration.
	c.Mode = "expert"
	return &c
}

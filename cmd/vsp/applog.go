package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var appLogCmd = &cobra.Command{
	Use:   "applog",
	Short: "Read the application log (SLG1) — who logged what, and from which program",
	Long: `Read application log headers over plain ADT.

SAP's own way in is the BAL_* function group, which cannot be called remotely by
any transport. The header table is an ordinary table, so this reads it with free
SQL instead — no RFC, no gateway, no Z code.

Message bodies are not here: they live in a cluster table that ADT's data
preview refuses. What is here is enough to answer which program logged what,
for which log object, and when — the part that connects a log to a dump.

  vsp applog --program ZCL_ORDER_POST --top 20
  vsp applog --user TESTUSER --since 2026-08-01
  vsp applog --object ZDEMO_LOG --json`,
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

		filter := adt.AppLogFilter{}
		filter.Program, _ = cmd.Flags().GetString("program")
		filter.User, _ = cmd.Flags().GetString("user")
		filter.Object, _ = cmd.Flags().GetString("object")
		filter.SubObject, _ = cmd.Flags().GetString("subobject")
		filter.Limit, _ = cmd.Flags().GetInt("top")

		for _, spec := range []struct {
			flag string
			into *time.Time
		}{{"since", &filter.From}, {"until", &filter.To}} {
			raw, _ := cmd.Flags().GetString(spec.flag)
			if strings.TrimSpace(raw) == "" {
				continue
			}
			when, perr := time.Parse("2006-01-02", strings.TrimSpace(raw))
			if perr != nil {
				return fmt.Errorf("--%s wants a date as YYYY-MM-DD, got %q", spec.flag, raw)
			}
			*spec.into = when
		}

		entries, err := client.ApplicationLog(context.Background(), filter)
		if err != nil {
			return err
		}

		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			out, merr := json.MarshalIndent(entries, "", "  ")
			if merr != nil {
				return merr
			}
			fmt.Println(string(out))
			return nil
		}

		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "no log entries match")
			return nil
		}
		fmt.Printf("%-19s %-20s %-20s %-14s %s\n", "WHEN", "OBJECT", "SUBOBJECT", "USER", "PROGRAM")
		fmt.Println(strings.Repeat("-", 100))
		for _, e := range entries {
			when := "-"
			if !e.At.IsZero() {
				when = e.At.Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-19s %-20s %-20s %-14s %s\n", when, e.Object, e.SubObject, e.User, e.Program)
		}
		fmt.Fprintf(os.Stderr, "\n%d entries\n", len(entries))
		return nil
	},
}

func init() {
	appLogCmd.Flags().String("program", "", "Only entries written by this program (ALPROG)")
	appLogCmd.Flags().String("user", "", "Only entries logged by this user")
	appLogCmd.Flags().String("object", "", "Log object (SLG0)")
	appLogCmd.Flags().String("subobject", "", "Log subobject")
	appLogCmd.Flags().String("since", "", "Earliest date, YYYY-MM-DD")
	appLogCmd.Flags().String("until", "", "Latest date, YYYY-MM-DD")
	appLogCmd.Flags().Int("top", 100, "Maximum entries to read")
	appLogCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(appLogCmd)
}

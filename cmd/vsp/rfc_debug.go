package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp rfc debug` drives the ABAP debugger through the ZADT_DEBUG_RFC facade on
// a pinned RFC conversation. Everything here runs inside ONE process for one
// reason: the ABAP session must survive between attach and step, and it only
// does so on a pinned connection. That is also why the interactive form exists
// — a short-lived command per step would lose the debuggee each time.

var rfcDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Drive the ABAP debugger over a pinned RFC session (needs the ZADT_DEBUG facade)",
	Long: `Drive the ABAP debugger over one pinned RFC conversation.

Without arguments it opens an interactive session; with -c it runs a
semicolon-separated script and exits. Commands:

  state              where this session landed, and whether it is pinned
  bp <PROG> <LINE>   set an external line breakpoint
  bps                list external breakpoints (with program and line)
  unbp [PROG [LINE]] delete breakpoints, or "unbp all"
  listen [SECONDS]   block until a debuggee stops (default 60)
  attach <ID>        attach to a waiting debuggee
  step [KIND]        into (default) | over | out | continue
  stack              the attached debuggee's call stack
  detach             end the debugger session and stop the listener`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		script, _ := cmd.Flags().GetString("command")
		timeout, _ := cmd.Flags().GetInt("timeout")

		return withRFCTimeout(cmd, time.Duration(timeout)*time.Second, func(ctx context.Context, c *rfc.Client) error {
			dbg, err := saprfc.NewDebugger(ctx, c, user)
			if err != nil {
				return err
			}
			defer func() { _ = dbg.Close(ctx) }()

			if script != "" {
				for _, line := range strings.Split(script, ";") {
					if err := runDebugCommand(ctx, dbg, strings.TrimSpace(line)); err != nil {
						return err
					}
				}
				return nil
			}

			fmt.Fprintln(os.Stderr, "pinned debug session — 'help' for commands, 'quit' to end")
			in := bufio.NewScanner(os.Stdin)
			for {
				fmt.Fprint(os.Stderr, "dbg> ")
				if !in.Scan() {
					return nil
				}
				line := strings.TrimSpace(in.Text())
				if line == "quit" || line == "exit" {
					return nil
				}
				if err := runDebugCommand(ctx, dbg, line); err != nil {
					fmt.Fprintln(os.Stderr, "!", err)
				}
			}
		})
	},
}

// runDebugCommand executes one line of the little command language.
func runDebugCommand(ctx context.Context, dbg *saprfc.Debugger, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	arg := func(i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}
	num := func(i int) int {
		n, _ := strconv.Atoi(arg(i))
		return n
	}

	var (
		out json.RawMessage
		err error
	)
	switch strings.ToLower(fields[0]) {
	case "help":
		fmt.Fprintln(os.Stderr, "state | bp <PROG> <LINE> | bps | unbp [PROG [LINE]|all] | "+
			"listen [SECONDS] | attach <ID> | step [into|over|out|continue] | stack | detach | quit")
		return nil
	case "state":
		out, err = dbg.State(ctx)
	case "bp":
		if len(fields) < 3 {
			return fmt.Errorf("usage: bp <PROGRAM> <LINE>")
		}
		out, err = dbg.SetBreakpoint(ctx, arg(1), num(2), strings.Join(fields[3:], " "))
	case "bps":
		out, err = dbg.Breakpoints(ctx)
	case "unbp":
		if strings.EqualFold(arg(1), "all") {
			out, err = dbg.DeleteBreakpoints(ctx, "", 0, true)
		} else {
			out, err = dbg.DeleteBreakpoints(ctx, arg(1), num(2), false)
		}
	case "listen":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 60
		}
		fmt.Fprintf(os.Stderr, "waiting up to %ds for a debuggee…\n", seconds)
		out, err = dbg.Listen(ctx, seconds)
	case "attach":
		if arg(1) == "" {
			return fmt.Errorf("usage: attach <DEBUGGEE_ID>")
		}
		out, err = dbg.Attach(ctx, arg(1))
	case "step":
		out, err = dbg.Step(ctx, arg(1))
	case "stack":
		out, err = dbg.Stack(ctx)
	case "detach":
		out, err = dbg.Detach(ctx)
	default:
		return fmt.Errorf("unknown command %q — try 'help'", fields[0])
	}
	if err != nil {
		return err
	}
	if len(out) > 0 {
		var pretty any
		if json.Unmarshal(out, &pretty) == nil {
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(string(out))
		}
	}
	return nil
}

func init() {
	rfcDebugCmd.Flags().String("user", "", "Whose debuggees to listen for (default: the logon user)")
	rfcDebugCmd.Flags().StringP("command", "c", "", "Run a semicolon-separated script instead of going interactive")
	rfcDebugCmd.Flags().Int("timeout", 300, "Seconds a single RFC call may take; must exceed the listen timeout")
	rfcCmd.AddCommand(rfcDebugCmd)
}

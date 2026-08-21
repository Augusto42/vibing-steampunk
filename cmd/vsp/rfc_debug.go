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
  bp <PROG>[/<INCL>] <LINE> [CONDITION]
                     set an external line breakpoint; name the include when
                     the line is inside a function module or class method
  bps                list external breakpoints (with program and line)
  unbp [PROG [LINE]] delete breakpoints, or "unbp all"
  listen [SECONDS]   block until a debuggee stops (default 60)
  catch [SECONDS]    listen, attach to the first debuggee, and show the stack
  attach <ID>        attach to a waiting debuggee
  step [KIND]        into (default) | over | out | continue
  stack              the attached debuggee's call stack
  detach             end the debugger session and stop the listener
  adt <METHOD> <URI> [NAME=VALUE …] [@bodyfile]
                     tunnel an ADT REST call through this same pinned session
  eclipse [SECONDS]  the same loop through SAP's own ADT debugger resources,
                     with no Z code on the server: listen, attach, stack
  estep [KIND]       ADT step: into (default) | over | return | continue
  estack             ADT call stack`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		script, _ := cmd.Flags().GetString("command")
		timeout, _ := cmd.Flags().GetInt("timeout")

		return withRFCDestTimeout(cmd, time.Duration(timeout)*time.Second, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
			dbg, err := saprfc.NewDebugger(ctx, c, user)
			if err != nil {
				return err
			}
			// ADT wants the user named in the query string; the ABAP facade can
			// default it to SY-UNAME server-side, /debugger/listeners cannot.
			rfcDebugUser = user
			if rfcDebugUser == "" {
				rfcDebugUser = dest.User
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

// adtTerminalID identifies this client to ADT's listener registry. ADT wants a
// 32-character id; it only has to be stable and distinct, not meaningful.
const adtTerminalID = "56535000000000000000000000006462"

// rfcDebugUser is whose debuggees the ADT flow listens for; the REPL sets it
// from --user, defaulting to the connection's logon user.
var rfcDebugUser string

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
		fmt.Fprintln(os.Stderr, "state | bp <PROG>[/<INCL>] <LINE> [COND] | bps | unbp [PROG [LINE]|all] | "+
			"listen [SECONDS] | catch [SECONDS] | attach <ID> | step [into|over|out|continue] | stack | detach | quit")
		return nil
	case "state":
		out, err = dbg.State(ctx)
	case "bp":
		if len(fields) < 3 {
			return fmt.Errorf("usage: bp <PROGRAM>[/<INCLUDE>] <LINE> [CONDITION]")
		}
		program, include, _ := strings.Cut(arg(1), "/")
		out, err = dbg.SetBreakpoint(ctx, program, include, num(2), strings.Join(fields[3:], " "))
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
	case "catch":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 60
		}
		fmt.Fprintf(os.Stderr, "waiting up to %ds for a debuggee…\n", seconds)
		who, attached, cerr := dbg.ListenAndAttach(ctx, seconds)
		if cerr != nil {
			return cerr
		}
		if who == nil {
			fmt.Fprintln(os.Stderr, "nobody stopped")
			return nil
		}
		fmt.Fprintf(os.Stderr, "attached to %s (%s) at %s/%s:%d\n",
			who.ID, who.User, who.Program, who.Include, who.Line)
		if len(attached) > 0 {
			printDebugJSON(attached)
		}
		out, err = dbg.Stack(ctx)
	case "attach":
		if arg(1) == "" {
			return fmt.Errorf("usage: attach <DEBUGGEE_ID>")
		}
		out, err = dbg.Attach(ctx, arg(1))
	case "step":
		out, err = dbg.Step(ctx, arg(1))
	case "stack":
		out, err = dbg.Stack(ctx)
	case "eclipse":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 120
		}
		fmt.Fprintf(os.Stderr, "ADT listener, up to %ds…\n", seconds)
		who, stack, cerr := dbg.ADTCatch(ctx, rfcDebugUser, "vsp", adtTerminalID, seconds)
		if who == nil && cerr == nil {
			fmt.Fprintln(os.Stderr, "nobody stopped")
			return nil
		}
		if who != nil {
			fmt.Fprintf(os.Stderr, "%s (%s) at %s/%s:%d — %s %s\n",
				who.ID, who.User, who.Program, who.Include, who.Line, who.Type, who.Name)
		}
		if cerr != nil {
			return cerr
		}
		fmt.Println(string(stack.Body))
		return nil
	case "estep":
		kind := map[string]string{
			"": "stepInto", "into": "stepInto", "over": "stepOver",
			"out": "stepReturn", "return": "stepReturn", "continue": "stepContinue",
		}[strings.ToLower(arg(1))]
		if kind == "" {
			return fmt.Errorf("step kinds: into, over, out, continue")
		}
		res, serr := dbg.ADTStep(ctx, kind)
		if serr != nil {
			return serr
		}
		fmt.Println(string(res.Body))
		return nil
	case "estack":
		res, serr := dbg.ADTStack(ctx)
		if serr != nil {
			return serr
		}
		fmt.Println(string(res.Body))
		return nil
	case "adt":
		if len(fields) < 3 {
			return fmt.Errorf("usage: adt <METHOD> <URI> [NAME=VALUE …]")
		}
		// A missing Accept is filled in by the tunnel itself.
		var headers []saprfc.ADTHeader
		var body []byte
		for _, raw := range fields[3:] {
			if strings.HasPrefix(raw, "@") {
				// @path — the request body, read from a file, because a source
				// document does not fit on one prompt line.
				content, rerr := os.ReadFile(strings.TrimPrefix(raw, "@"))
				if rerr != nil {
					return rerr
				}
				body = content
				continue
			}
			name, value, ok := strings.Cut(raw, "=")
			if !ok {
				return fmt.Errorf("a header must be NAME=VALUE (or @file for a body), got %q", raw)
			}
			headers = append(headers, saprfc.ADTHeader{Name: name, Value: value})
		}
		res, aerr := dbg.ADT(ctx, arg(1), arg(2), headers, body)
		if aerr != nil {
			return aerr
		}
		fmt.Fprintf(os.Stderr, "%d %s · %d bytes · %s\n",
			res.Status, res.ReasonPhrase, len(res.Body), res.Header("content-type"))
		fmt.Println(string(res.Body))
		return nil
	case "detach":
		out, err = dbg.Detach(ctx)
	default:
		return fmt.Errorf("unknown command %q — try 'help'", fields[0])
	}
	if err != nil {
		return err
	}
	if len(out) > 0 {
		printDebugJSON(out)
	}
	return nil
}

func printDebugJSON(raw json.RawMessage) {
	var pretty any
	if json.Unmarshal(raw, &pretty) == nil {
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Println(string(raw))
}

func init() {
	rfcDebugCmd.Flags().String("user", "", "Whose debuggees to listen for (default: the logon user)")
	rfcDebugCmd.Flags().StringP("command", "c", "", "Run a semicolon-separated script instead of going interactive")
	rfcDebugCmd.Flags().Int("timeout", 300, "Seconds a single RFC call may take; must exceed the listen timeout")
	rfcCmd.AddCommand(rfcDebugCmd)
}

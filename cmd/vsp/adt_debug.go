package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp adt debug` is `vsp rfc debug` for systems that have no RFC channel — a
// cookie or a single sign-on, HTTPS only, no gateway port and no RFC password.
//
// It works because the debugger was never an RFC feature: listen, attach, stack
// and step are SAP's own ADT resources, and RFC was one way to carry them. What
// they actually need is a *session* — ADT keeps the debug session in an ABAP
// roll area — and over HTTPS that is the stateful ICF session selected by
// sap-contextid. So the requirement here is the same as there, expressed
// differently: one process, one session, held for the whole loop.

var adtCmd = &cobra.Command{
	Use:   "adt",
	Short: "Talk to ADT directly over HTTPS",
}

var adtDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Drive the ABAP debugger over a stateful ADT session (no RFC needed)",
	Long: `Drive the ABAP debugger over ADT's own resources on one stateful HTTPS session.

The same commands as 'vsp rfc debug', minus the ones that need the ZADT_DEBUG
function group — those are function modules and need an RFC channel:

  eclipse [SECONDS]  listen, attach to the first debuggee, show the stack
  estep [KIND]       into (default) | over | out | continue
  estack             the call stack
  ebp <OBJECT> <LINE> [COND]
                     set a line breakpoint through ADT — no Z code needed
  ebps               the breakpoints this client has registered
  esys               toggle breakpoints inside SAP standard code (off by default)
  eunbp <ID|all>     remove one breakpoint, or all of them
  elocals            the current frame's own variables, with values
  evars [NAME …]     variable values (default roots @ROOT @DATAAGING)
  echildren <ID>     expand a structure, a table or a synthetic root
  erec [MAX]         record from here: one JSON object per stop
  evalues            record real values instead of «type:length» placeholders
  eraw               print the next e-command as the XML SAP sent
  adt <METHOD> <URI> [NAME=VALUE …] [@bodyfile]
                     any ADT request on this same session`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		script, _ := cmd.Flags().GetString("command")
		timeout, _ := cmd.Flags().GetInt("timeout")

		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		transport, err := statefulADTTransport(params, time.Duration(timeout)*time.Second)
		if err != nil {
			return err
		}

		rfcDebugUser = strings.ToUpper(strings.TrimSpace(user))
		if rfcDebugUser == "" {
			rfcDebugUser = strings.ToUpper(params.User)
		}
		if rfcDebugUser == "" {
			return fmt.Errorf("ADT needs the user named explicitly: pass --user")
		}

		ctx := context.Background()
		dbg := saprfc.NewADTDebugger(transport, rfcDebugUser)
		defer func() { _ = dbg.Close(ctx) }()

		if script != "" {
			for _, line := range strings.Split(script, ";") {
				if err := runDebugCommand(ctx, dbg, strings.TrimSpace(line)); err != nil {
					return err
				}
			}
			return nil
		}

		fmt.Fprintf(os.Stderr, "ADT debug session as %s — 'help' for commands, 'quit' to end\n", rfcDebugUser)
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
	},
}

// statefulADTTransport builds one ADT transport and keeps it: a new transport
// is a new session, and a new session has no debuggee attached.
func statefulADTTransport(params *systemParams, timeout time.Duration) (saprfc.ADTTransport, error) {
	opts := []adt.Option{
		adt.WithClient(params.Client),
		adt.WithLanguage(params.Language),
		adt.WithSessionType(adt.SessionStateful),
		// The debugger's listener is a request that deliberately does not answer
		// until something stops, so the client timeout has to outlast it. The
		// stock 60s turns a 90s listen into "context deadline exceeded" and the
		// caller never learns that the debuggee was fine.
		adt.WithTimeout(timeout),
	}
	if params.Insecure {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}

	user, password := params.User, params.Password
	switch {
	case params.CookieFile != "":
		cookies, err := adt.LoadCookiesFromFile(params.CookieFile)
		if err != nil {
			return nil, fmt.Errorf("loading cookies from %s: %w", params.CookieFile, err)
		}
		opts = append(opts, adt.WithCookies(cookies))
		user, password = "", ""
	case params.CookieString != "":
		opts = append(opts, adt.WithCookies(adt.ParseCookieString(params.CookieString)))
		user, password = "", ""
	}

	cfg := adt.NewConfig(params.URL, user, password, opts...)
	return saprfc.HTTPSession(adt.NewTransport(cfg)), nil
}

func init() {
	adtDebugCmd.Flags().String("user", "", "Whose debuggees to listen for (default: the logon user)")
	adtDebugCmd.Flags().StringP("command", "c", "", "Run a semicolon-separated script instead of going interactive")
	adtDebugCmd.Flags().Int("timeout", 300, "Seconds a single HTTP request may take; must exceed the listen timeout")
	adtCmd.AddCommand(adtDebugCmd)
	rootCmd.AddCommand(adtCmd)
}

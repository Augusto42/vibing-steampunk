package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

func init() {
	detectCmd.Flags().String("client", "", "SAP client to ask for")
	detectCmd.Flags().String("instance", "", "Instance number, if known — adds the conventional ports to the scan")
	detectCmd.Flags().IntSlice("port", nil, "Scan only these ports")
	detectCmd.Flags().Bool("insecure", false, "Accept a certificate that does not match the hostname")
	detectCmd.Flags().Bool("json", false, "Emit the findings as JSON")
	detectCmd.Flags().Bool("all", false, "Exhaustive: every conventional port for all hundred instances, plus the install defaults")
	detectCmd.Flags().Bool("show-open", false, "List ports that accepted a connection but served nothing recognisable")
	rootCmd.AddCommand(detectCmd)
}

var detectCmd = &cobra.Command{
	Use:   "detect <host|SID>",
	Short: "Find which port a system serves ADT on",
	Long: `Scan a host for the port that answers ADT, and print the configuration to use.

Nothing on this machine knows that port. A landscape file describes SAP GUI
connectivity and carries no HTTP at all; Eclipse ADT asks the person setting up
the project. The convention — HTTPS at 443nn, HTTP at 80nn — is a guess that is
often wrong: a system behind a web dispatcher answers on 443, and one measured
here answers on 8422, which follows no rule.

So this asks. Give it a hostname, or a system id that appears in the landscape,
and it reports which ports answer and how far each got — which separates "wrong
port" from "right port, and the ADT node is switched off".

  vsp detect sap.example.com
  vsp detect D15 --client 100
  vsp detect sap.example.com --port 44300 --port 8000

Run it before writing a system into .vsp.json or .mcp.json, so the address in
there is one that answered rather than one that ought to work.`,
	Args: cobra.ExactArgs(1),
	RunE: runDetect,
}

func runDetect(cmd *cobra.Command, args []string) error {
	target := strings.TrimSpace(args[0])
	clientNr, _ := cmd.Flags().GetString("client")
	instance, _ := cmd.Flags().GetString("instance")
	explicitPorts, _ := cmd.Flags().GetIntSlice("port")
	insecure, _ := cmd.Flags().GetBool("insecure")
	asJSON, _ := cmd.Flags().GetBool("json")
	exhaustive, _ := cmd.Flags().GetBool("all")
	showOpen, _ := cmd.Flags().GetBool("show-open")

	// A system id is the more useful thing to type, and the landscape knows its
	// host and instance — which is where the conventional ports come from.
	// The landscape knows the instance number, and that is what shapes the
	// shortlist — 443nn and 80nn mean nothing without an nn. Looking the target
	// up by host as well as by system id means a caller who types the hostname
	// does not lose the conventional ports for it.
	host, sid := target, ""
	if found := findInLandscape(cmd, target); found != nil {
		domains := adt.DNSSearchDomains(cmd.Context())
		host = adt.CanonicalHost(cmd.Context(), found.Host, domains)
		sid = found.SystemID
		if instance == "" {
			instance = found.InstanceNr
		}
		fmt.Fprintf(os.Stderr, "%s in the landscape: %s, instance %s\n",
			found.SystemID, host, orDash(found.InstanceNr))
	}

	ports := explicitPorts
	switch {
	case len(ports) > 0:
	case exhaustive:
		ports = adt.ExhaustivePorts()
	default:
		ports = adt.CandidatePorts(instance)
	}

	fmt.Fprintf(os.Stderr, "scanning %s on %d ports...\n", host, len(ports))
	result := adt.ScanForADT(cmd.Context(), host, ports, clientNr, insecure)

	// A mismatched certificate names the host this port is served under, and
	// that is usually the HTTPS address the caller wanted. Following it costs
	// one more scan and turns "use plain HTTP" into "use TLS, over there".
	if lead := result.CertificateLead(); lead != "" && !strings.EqualFold(lead, host) {
		if best := result.Best(); best == nil || !best.Secure || best.Kind != adt.PortADT {
			fmt.Fprintf(os.Stderr, "a certificate here names %s — scanning it too...\n", lead)
			if viaCert := adt.ScanForADT(cmd.Context(), lead, ports, clientNr, insecure); viaCert.Best() != nil {
				result.Findings = append(result.Findings, viaCert.Findings...)
				result.Host = lead
			}
		}
	}

	if asJSON {
		blob, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(blob))
		return nil
	}

	printFindings(result, showOpen)
	printSuggestion(result, sid, clientNr)
	return nil
}

func printFindings(result *adt.PortScanResult, showOpen bool) {
	if len(result.Findings) == 0 {
		fmt.Printf("Nothing answered on %s.\n", result.Host)
		fmt.Println("The host may be firewalled from here, or the name may not be the one that serves HTTP —")
		fmt.Println("a system fronted by a web dispatcher answers on a different host than its application server.")
		fmt.Println("If it should be reachable, --all sweeps every conventional port rather than the shortlist.")
		return
	}

	fmt.Printf("%-7s %-18s %-6s %s\n", "PORT", "WHAT", "HTTP", "URL / NOTE")
	fmt.Println(strings.Repeat("-", 88))
	shown := 0
	for _, f := range result.Findings {
		if !showOpen && f.Kind == adt.PortOpen {
			continue
		}
		shown++
		note := f.URL
		if f.Detail != "" {
			note = strings.TrimSpace(note + "  " + f.Detail)
		}
		fmt.Printf("%-7d %-18s %-6s %s\n", f.Port, f.Kind, statusOrDash(f.Status), note)
	}
	if shown == 0 {
		fmt.Println("(ports are open but none served anything recognisable — rerun with --show-open)")
	}
}

// printSuggestion turns the scan into the line the caller came for.
func printSuggestion(result *adt.PortScanResult, sid, clientNr string) {
	best := result.Best()
	if best == nil || best.URL == "" {
		return
	}

	fmt.Println()
	switch best.Kind {
	case adt.PortADT:
		fmt.Printf("Use %s\n", best.URL)
		if !best.Secure {
			fmt.Println("This is plain HTTP: the session cookie travels in clear, and a system that")
			fmt.Println("sets login/ticket_only_by_https will refuse to issue one at all. No TLS port")
			fmt.Println("answered here — try --all if this was the shortlist.")
		}
	case adt.PortSAPNoADT:
		fmt.Printf("SAP answers on %s, but the ADT node did not.\n", best.URL)
		fmt.Println("Ask basis to activate /sap/bc/adt in SICF; the port itself is right.")
		return
	case adt.PortTLSMismatch:
		fmt.Printf("A server answers on %s and its certificate names another host.\n", best.URL)
		fmt.Println("Use the name the certificate carries, or rerun with --insecure to look past it.")
		return
	default:
		return
	}

	name := strings.ToLower(sid)
	if name == "" {
		name = "system"
	}
	entry := map[string]any{"url": best.URL, "auth": "sso"}
	if clientNr != "" {
		entry["client"] = clientNr
	}
	blob, err := json.MarshalIndent(map[string]any{
		"systems": map[string]any{name: entry},
	}, "", "  ")
	if err != nil {
		return
	}
	fmt.Println("\nFor .vsp.json:")
	fmt.Println(string(blob))
	fmt.Println("\nThen: vsp config vsp-to-mcp   — to write the same into .mcp.json")
}

// findInLandscape looks the target up by system id or by host, and says nothing
// if there is no landscape to look in — the scan still works from a bare
// hostname, only without the conventional ports for its instance.
func findInLandscape(cmd *cobra.Command, target string) *adt.LandscapeSystem {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	// A hostname may be typed fully qualified while the landscape records the
	// short form, so they are compared on the first label.
	shortHost := strings.ToLower(strings.SplitN(target, ".", 2)[0])

	for _, path := range adt.FindLandscapeFiles(cmd.Context(), "") {
		lf, err := adt.ParseLandscape(path)
		if err != nil {
			continue
		}
		for _, sys := range lf.Systems(path) {
			if strings.EqualFold(sys.SystemID, target) {
				return &sys
			}
			if strings.EqualFold(strings.SplitN(sys.Host, ".", 2)[0], shortHost) {
				return &sys
			}
		}
	}
	return nil
}

func statusOrDash(status int) string {
	if status == 0 {
		return "-"
	}
	return strconv.Itoa(status)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

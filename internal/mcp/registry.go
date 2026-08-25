package mcp

// One declaration per capability, and everything else derived from it.
//
// Every drift this project found in a week sat between two hand-kept lists.
// The routing table said one thing and the tool description another; the
// advertised set counted analyze types and missed four actions; help documented
// twelve actions of sixteen and nobody noticed until a test compared the two.
// Each was fixed by adding a test that holds two lists together — which works,
// and leaves two lists.
//
// A capability declared once cannot drift from itself. From this the router
// dispatches, help answers, the advertised set counts, and the parameter
// documentation is written — none of them a separate list to forget.
//
// Parameters are declared as a struct with tags rather than as prose, so the
// names in the documentation are the names the handler reads. Prose describing
// a parameter is a fourth list.

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

// Param is one argument, derived from a struct field's tags.
type Param struct {
	Name     string
	Required bool
	Doc      string
}

// Capability is one thing the tool can be asked to do.
type Capability struct {
	// Action is what SAP(action=…) takes. Op narrows it where one action
	// carries several operations — i18n, revisions — and is empty otherwise.
	Action string
	Op     string
	// Summary is one line, in the imperative. It is what a list of
	// capabilities shows and what help leads with.
	Summary string
	// Params is a zero value of a struct whose fields carry `vsp` tags:
	//     Language string `vsp:"language,required,two-letter code"`
	// Nil means the capability takes no arguments worth naming.
	Params any
	// Examples are calls that work. At least one, because a parameter list is
	// not documentation — the shortest correct documentation is a call.
	Examples []string
	// Writes marks a capability that changes the system. The sweep refuses to
	// probe these, and help says so, from one declaration rather than two.
	Writes  bool
	Handler server.ToolHandlerFunc
}

// Key is the registry key: the action, or action/op where one exists.
func (c Capability) Key() string {
	if c.Op == "" {
		return c.Action
	}
	return c.Action + "/" + c.Op
}

// ParamList reads the parameter declarations off the struct.
func (c Capability) ParamList() []Param {
	if c.Params == nil {
		return nil
	}
	t := reflect.TypeOf(c.Params)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []Param
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("vsp")
		if tag == "" || tag == "-" {
			continue
		}
		// The documentation may itself contain commas — "CLAS, PROG, INTF" —
		// so only the name and an optional `required` are split off, and the
		// remainder is the doc, joined back. The first version split on every
		// comma and kept the last piece, which turned that list into "INTF".
		parts := strings.SplitN(tag, ",", 2)
		p := Param{Name: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			rest := strings.TrimSpace(parts[1])
			if rest == "required" {
				p.Required = true
			} else if after, ok := strings.CutPrefix(rest, "required,"); ok {
				p.Required = true
				p.Doc = strings.TrimSpace(after)
			} else {
				p.Doc = rest
			}
		}
		out = append(out, p)
	}
	return out
}

// Help renders the capability the way a reader needs it.
func (c Capability) Help() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", c.callForm(), c.Summary)
	if c.Writes {
		b.WriteString("\nThis changes the system.\n")
	}
	if params := c.ParamList(); len(params) > 0 {
		b.WriteString("\nParameters:\n")
		for _, p := range params {
			req := ""
			if p.Required {
				req = "  (required)"
			}
			doc := ""
			if p.Doc != "" {
				doc = " — " + p.Doc
			}
			fmt.Fprintf(&b, "  %s%s%s\n", p.Name, req, doc)
		}
	}
	if len(c.Examples) > 0 {
		b.WriteString("\n")
		for _, e := range c.Examples {
			fmt.Fprintf(&b, "  %s\n", e)
		}
	}
	return b.String()
}

func (c Capability) callForm() string {
	if c.Op == "" {
		return fmt.Sprintf("SAP(action=%q)", c.Action)
	}
	return fmt.Sprintf("SAP(action=%q, params={\"op\": %q})", c.Action, c.Op)
}

// --- the registry ---------------------------------------------------------

// capabilities is filled by the register* functions below at construction, so
// the set is a property of the server rather than a package global that a test
// can leave dirty.
type registry struct {
	byKey  map[string]Capability
	byAct  map[string][]Capability
	sorted []Capability
}

func newRegistry(caps ...Capability) *registry {
	r := &registry{byKey: map[string]Capability{}, byAct: map[string][]Capability{}}
	for _, c := range caps {
		r.byKey[c.Key()] = c
		r.byAct[c.Action] = append(r.byAct[c.Action], c)
		r.sorted = append(r.sorted, c)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i].Key() < r.sorted[j].Key() })
	return r
}

// Lookup finds the capability for an action and op.
func (r *registry) Lookup(action, op string) (Capability, bool) {
	if c, ok := r.byKey[action+"/"+op]; ok {
		return c, true
	}
	c, ok := r.byKey[action]
	return c, ok
}

// Ops lists the operations an action carries, sorted, for the message a wrong
// one earns — derived, so it cannot name an op that is not routed.
func (r *registry) Ops(action string) []string {
	var out []string
	for _, c := range r.byAct[action] {
		if c.Op != "" {
			out = append(out, c.Op)
		}
	}
	sort.Strings(out)
	return out
}

// Actions lists every action the registry claims.
func (r *registry) Actions() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range r.sorted {
		if !seen[c.Action] {
			seen[c.Action] = true
			out = append(out, c.Action)
		}
	}
	return out
}

// All returns every capability, sorted by key.
func (r *registry) All() []Capability { return r.sorted }

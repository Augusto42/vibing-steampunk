package ctxcomp

// Narrowing a contract to the methods the code actually calls.
//
// A contract is the public section of a dependency, and a wide interface used
// narrowly turns most of it into noise. Measured on CL_ATO_CHANGELIST against a
// live system: IF_ATO_DB_ACCESS carries **56** methods and that class calls
// **nine** of them. Eighty-four per cent of the space that contract occupies
// crowds out other dependencies entirely, inside a budget of five.
//
// It is not uniformly worth doing, which is why the numbers matter: the same
// class uses twelve of IF_ATO_ZIP_FILE's thirteen methods, and there is nothing
// to cut. So the narrowing applies where there is something to narrow and gets
// out of the way where there is not.
//
// # What is lost, and where it went
//
// A reader who sees only the called methods stops seeing the rest of the
// surface, and "what else could I call here" is a fair question. It is not
// answered by padding every context with forty-seven unused signatures: the
// context is *compressed* by definition, and the full surface is one explicit
// request away.
//
// What the narrowed contract owes the reader is that there is more — not what
// it is. The header carries it in a number:
//
//	--- IF_ATO_DB_ACCESS (interface, 56 methods; 9 called here) ---
//
// One line, no list, and nothing silently dropped.

import (
	"regexp"
	"strings"
)

var (
	// x->method( and x=>method( and if_x~method(. The receiver is captured so
	// a caller can tell which dependency the method belongs to; the parser
	// layer resolves that properly, and here the receiver's own declaration is
	// what links a variable to its type.
	reInstanceCall = regexp.MustCompile(`(?i)\b([A-Z_][A-Z_0-9]*)\s*->\s*([A-Z_][A-Z_0-9]*)\s*\(`)
	reStaticMethod = regexp.MustCompile(`(?i)\b([A-Z_/][A-Z_0-9/]*)\s*=>\s*([A-Z_][A-Z_0-9]*)\s*\(`)
	reIntfCall     = regexp.MustCompile(`(?i)\b([A-Z_/][A-Z_0-9/]*)\s*~\s*([A-Z_][A-Z_0-9]*)\s*\(`)
	// DATA mo_x TYPE REF TO zcl_y — what links a receiver variable to a type.
	reTypedRef = regexp.MustCompile(`(?i)\b([A-Z_][A-Z_0-9]*)\s+TYPE\s+REF\s+TO\s+([A-Z_/][A-Z_0-9/]*)`)
)

// MethodsCalledOn returns, per dependency name, the methods this source calls
// on it.
//
// Instance calls go through a variable, so the variable's declared type is
// looked up first. A receiver whose type is not declared in this source is
// skipped rather than guessed — attributing a method to the wrong class would
// put a signature in the context that does not exist there, which is worse than
// a contract that is too wide.
func MethodsCalledOn(source string) map[string][]string {
	upper := strings.ToUpper(source)

	// Receiver variable → declared type.
	typeOf := map[string]string{}
	for _, m := range reTypedRef.FindAllStringSubmatch(upper, -1) {
		typeOf[m[1]] = m[2]
	}

	out := map[string]map[string]bool{}
	add := func(owner, method string) {
		if owner == "" || method == "" {
			return
		}
		if out[owner] == nil {
			out[owner] = map[string]bool{}
		}
		out[owner][method] = true
	}

	for _, m := range reInstanceCall.FindAllStringSubmatch(upper, -1) {
		if owner, ok := typeOf[m[1]]; ok {
			add(owner, m[2])
		}
	}
	for _, m := range reStaticMethod.FindAllStringSubmatch(upper, -1) {
		add(m[1], m[2])
	}
	for _, m := range reIntfCall.FindAllStringSubmatch(upper, -1) {
		add(m[1], m[2])
	}

	result := make(map[string][]string, len(out))
	for owner, methods := range out {
		list := make([]string, 0, len(methods))
		for m := range methods {
			list = append(list, m)
		}
		result[owner] = list
	}
	return result
}

// NarrowContract keeps the declarations of the named methods and drops the
// rest, returning the narrowed text, how many methods the contract held, and
// how many survived.
//
// With no names it returns the contract unchanged: nothing is known about what
// is used, and narrowing on no information would be guessing. Everything that
// is not a method declaration — the class or interface header, types, constants,
// data — is kept whatever happens, because those are what the surviving
// signatures are written in terms of.
func NarrowContract(contract string, want []string) (narrowed string, total, kept int) {
	if strings.TrimSpace(contract) == "" {
		return contract, 0, 0
	}
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[strings.ToUpper(strings.TrimSpace(w))] = true
	}

	lines := strings.Split(contract, "\n")
	var out []string
	inMethod := false
	keepThis := false

	for _, line := range lines {
		upper := strings.ToUpper(line)
		trimmed := strings.TrimSpace(upper)

		if name, ok := methodDeclName(trimmed); ok {
			total++
			inMethod = true
			keepThis = len(wanted) == 0 || wanted[name]
			if keepThis {
				kept++
				out = append(out, line)
			}
			if strings.HasSuffix(trimmed, ".") {
				inMethod = false
			}
			continue
		}

		if inMethod {
			// A continuation line of the declaration just decided on.
			if keepThis {
				out = append(out, line)
			}
			if strings.HasSuffix(trimmed, ".") {
				inMethod = false
			}
			continue
		}

		out = append(out, line)
	}

	if len(wanted) == 0 {
		return contract, total, total
	}
	return strings.Join(out, "\n"), total, kept
}

// methodDeclName reports whether a line opens a method declaration, and its
// name.
func methodDeclName(trimmed string) (string, bool) {
	for _, kw := range []string{"METHODS ", "CLASS-METHODS "} {
		if !strings.HasPrefix(trimmed, kw) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(kw):])
		// METHODS: a, b, c — the chained form declares several at once, and
		// narrowing one out of a chain would produce syntax that is not ABAP.
		// Left whole.
		if strings.HasPrefix(rest, ":") {
			return "", false
		}
		name := rest
		if i := strings.IndexAny(name, " \t."); i > 0 {
			name = name[:i]
		}
		if name == "" {
			return "", false
		}
		return name, true
	}
	return "", false
}

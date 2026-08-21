package saprfc

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// A password reaches a log by accident, never on purpose — usually because a
// struct grew the field long after the format string was written. These pin
// that no rendering path prints it.
func TestSecretNeverRenders(t *testing.T) {
	const password = "hunter2-do-not-print"
	p := Params{Host: "sap.example", User: "DEVELOPER", Password: Secret(password)}

	for _, rendered := range []string{
		fmt.Sprint(p.Password),
		fmt.Sprintf("%v", p),
		fmt.Sprintf("%+v", p),
		fmt.Sprintf("%#v", p.Password),
		fmt.Sprintf("%s", p.Password),
	} {
		if strings.Contains(rendered, password) {
			t.Fatalf("the password rendered: %q", rendered)
		}
	}

	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), password) {
		t.Fatalf("the password was serialised: %s", encoded)
	}
	if p.Password.Reveal() != password {
		t.Fatal("Reveal must return the secret itself")
	}
}

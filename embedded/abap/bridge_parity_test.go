package embedded

import (
	"os"
	"strings"
	"testing"
)

func TestRFCServiceMatchesAbapGitSource(t *testing.T) {
	file, err := os.ReadFile("../../src/zcl_vsp_rfc_service.clas.abap")
	if err != nil {
		t.Fatal(err)
	}
	normalize := func(source string) string {
		return strings.TrimSpace(strings.ReplaceAll(source, "\r\n", "\n"))
	}
	if normalize(string(file)) != normalize(ZclVspRfcService) {
		t.Fatal("embedded RFC bridge and abapGit source differ; update both before release")
	}
}

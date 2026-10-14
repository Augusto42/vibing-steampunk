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

func TestBridgeServicesMatchAbapGitSource(t *testing.T) {
	for _, object := range GetObjects() {
		// This installer-only helper has no abapGit counterpart.
		if object.Name == "ZADT_CL_TADIR_MOVE" {
			continue
		}
		extension := ".clas.abap"
		if object.Type == "INTF" {
			extension = ".intf.abap"
		}
		path := "../../src/" + strings.ToLower(object.Name) + extension
		file, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		normalize := func(source string) string {
			// AMDP sample names follow their respective packaging conventions.
			if object.Name == "ZCL_VSP_AMDP_SERVICE" {
				source = strings.ReplaceAll(source, "zcl_adt_00_amdp_test", "zcl_vsp_00_amdp_test")
			}
			return strings.TrimSpace(strings.ReplaceAll(source, "\r\n", "\n"))
		}
		if normalize(string(file)) != normalize(object.Source) {
			t.Errorf("%s differs between embedded installer and abapGit package", object.Name)
		}
	}
}

// Command fetchdeps builds the embedded dependency archives from their upstream
// source.
//
// They used to be produced by exporting from a SAP system, which meant a
// released binary carried whatever happened to be installed on one developer's
// machine — there was no reproducible build of that artefact, and no way for
// anyone else to make the same one. abapGit publishes its standalone report as
// a single file, so it can be fetched by revision and checked.
//
//	go run ./tools/fetchdeps            # fetch and report the checksum
//	go run ./tools/fetchdeps -check     # verify what is embedded, change nothing
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// standaloneURL is abapGit's published single-file build. The project ships it
// from a separate repository that holds only build output, which is why this
// points there and not at the source tree.
const standaloneURL = "https://raw.githubusercontent.com/abapGit/build/main/zabapgit_standalone.prog.abap"

// reportName is the ABAP program the archive installs.
const reportName = "zabapgit_standalone"

func main() {
	check := flag.Bool("check", false, "verify the embedded archive without rewriting it")
	out := flag.String("out", filepath.Join("embedded", "deps", "abapgit-standalone.zip"), "archive to write")
	flag.Parse()

	if *check {
		if err := verify(*out); err != nil {
			fmt.Fprintf(os.Stderr, "fetchdeps: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := fetch(*out); err != nil {
		fmt.Fprintf(os.Stderr, "fetchdeps: %v\n", err)
		os.Exit(1)
	}
}

func fetch(out string) error {
	fmt.Printf("fetching %s\n", standaloneURL)
	source, err := download(standaloneURL)
	if err != nil {
		return err
	}
	// A report this small is not the real one — the standalone build is several
	// megabytes, and a short body here means a redirect page or an error was
	// served with a 200.
	if len(source) < 1<<20 {
		return fmt.Errorf("downloaded %d bytes, which is too small to be the standalone report", len(source))
	}

	archive, err := packArchive(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(out, archive, 0644); err != nil {
		return err
	}

	fmt.Printf("wrote %s (%d bytes)\n", out, len(archive))
	fmt.Printf("  report   %s, %d bytes\n", reportName, len(source))
	fmt.Printf("  sha256   %s\n", sum(source))
	fmt.Println("\nRecord that checksum with the change: it is what makes a later build")
	fmt.Println("comparable to this one.")
	return nil
}

// packArchive wraps the report in the layout abapGit itself uses, so the result
// deploys through the same path as any exported package.
func packArchive(source []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	files := map[string][]byte{
		"src/" + reportName + ".prog.abap": source,
		"src/" + reportName + ".prog.xml":  []byte(programMetadata()),
		".abapgit.xml":                     []byte(repositoryMetadata()),
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func programMetadata() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<abapGit version="v1.0.0" serializer="LCL_OBJECT_PROG" serializer_version="v1.0.0">
 <asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
   <PROGDIR>
    <NAME>` + upper(reportName) + `</NAME>
    <SUBC>1</SUBC>
    <FIXPT>X</FIXPT>
    <UCCHECK>X</UCCHECK>
   </PROGDIR>
   <TPOOL>
    <item>
     <ID>R</ID>
     <ENTRY>abapGit standalone</ENTRY>
     <LENGTH>18</LENGTH>
    </item>
   </TPOOL>
  </asx:values>
 </asx:abap>
</abapGit>
`
}

func repositoryMetadata() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
 <asx:values>
  <DATA>
   <MASTER_LANGUAGE>E</MASTER_LANGUAGE>
   <STARTING_FOLDER>/src/</STARTING_FOLDER>
   <FOLDER_LOGIC>PREFIX</FOLDER_LOGIC>
  </DATA>
 </asx:values>
</asx:abap>
`
}

func verify(path string) error {
	archive, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(archive) == 0 {
		return fmt.Errorf("%s is empty — this build ships an archive that installs nothing; run `make fetch-deps`", path)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("%s is not a readable archive: %w", path, err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == reportName+".prog.abap" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			source, err := io.ReadAll(rc)
			if err != nil {
				return err
			}
			fmt.Printf("%s: %s, %d bytes\n  sha256 %s\n", path, f.Name, len(source), sum(source))
			return nil
		}
	}
	return fmt.Errorf("%s contains no %s", path, reportName+".prog.abap")
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}

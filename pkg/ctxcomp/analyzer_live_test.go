//go:build integration

// Live tests: they dial a real SAP system, so they are not part of the default
// `go test ./...` — run them with `go test -tags integration ./pkg/ctxcomp/`.
// Same convention as pkg/adt/integration_test.go.

package ctxcomp

import (
	"context"
	"os"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestAnalyzerLive(t *testing.T) {
	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")
	if url == "" {
		t.Skip("SAP_URL not set")
	}

	client := adt.NewClient(url, user, pass, adt.WithInsecureSkipVerify())
	ctx := context.Background()

	// Read a real class
	source, err := client.GetClassSource(ctx, "ZCL_ABAPGIT_AJSON")
	if err != nil {
		t.Fatalf("GetClassSource: %v", err)
	}

	analyzer := NewAnalyzer(nil) // offline layers only for now
	result := analyzer.Analyze(ctx, source, "ZCL_ABAPGIT_AJSON")

	t.Logf("=== ZCL_ABAPGIT_AJSON ===")
	t.Logf("Lines: %d", result.TotalLines)
	t.Logf("Layers: %v", result.Layers)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("True deps: %d, False positives: %d, Total: %d", result.TrueDeps, result.FalsePositives, len(result.Dependencies))
	t.Logf("")

	for _, dep := range result.Dependencies {
		status := ""
		if dep.InString {
			status = " [FALSE POSITIVE]"
		}
		layers := ""
		for _, l := range dep.FoundBy {
			layers += l.String() + "+"
		}
		t.Logf("  %-30s conf=%.2f  %s  [%s]%s", dep.Name, dep.Confidence, dep.Kind, layers, status)
	}
}

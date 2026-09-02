package cache

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSQLiteWorksWithoutCGO is the regression guard for #157. Every released
// binary is cross-compiled, which implies CGO_ENABLED=0; under the old cgo
// binding that made the cache return "This is a stub" at runtime while the
// build stayed green, so nothing caught it.
func TestSQLiteWorksWithoutCGO(t *testing.T) {
	c, err := NewSQLiteCache(Config{Path: filepath.Join(t.TempDir(), "probe.db")})
	if err != nil {
		t.Fatalf("opening the SQLite cache failed: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if putErr := c.PutNode(ctx, &Node{ID: "ZCL_DEMO_ONE", ObjectType: "CLAS", ObjectName: "ZCL_DEMO_ONE", Valid: true}); putErr != nil {
		t.Fatalf("PutNode: %v", putErr)
	}
	got, err := c.GetNode(ctx, "ZCL_DEMO_ONE")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got == nil || got.ObjectName != "ZCL_DEMO_ONE" {
		t.Fatalf("round-trip failed: %+v", got)
	}
}

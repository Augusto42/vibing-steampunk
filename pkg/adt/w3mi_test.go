package adt

import (
	"strings"
	"testing"
)

func TestW3MIRowBytes(t *testing.T) {
	b, err := w3miRowBytes("0300FF7F")
	if err != nil || string(b) != string([]byte{0x03, 0x00, 0xFF, 0x7F}) {
		t.Errorf("hex row: got % x, err %v", b, err)
	}
	if b, err := w3miRowBytes([]byte{1, 2}); err != nil || len(b) != 2 {
		t.Errorf("byte row: got % x, err %v", b, err)
	}
	if _, err := w3miRowBytes(42); err == nil {
		t.Error("an int row should be an error, not silently empty")
	}
	if _, err := w3miRowBytes("nothex"); err == nil {
		t.Error("a non-hex string should be an error")
	}
}

// A quoted object id would break out of the WHERE clause this builds. The check
// is not about SQL injection from a hostile caller so much as about a name with
// an apostrophe producing a confusing server error instead of a clear one.
func TestGetW3MIRejectsQuotedName(t *testing.T) {
	c := &Client{}
	if _, err := c.GetW3MI(nil, "X'Y"); err == nil || !strings.Contains(err.Error(), "quote") {
		t.Errorf("want a quote error, got %v", err)
	}
	if _, err := c.GetW3MI(nil, "  "); err == nil {
		t.Error("an empty object id should be an error")
	}
	if _, err := c.ListW3MI(nil, "X'Y", 10); err == nil || !strings.Contains(err.Error(), "quote") {
		t.Errorf("want a quote error from ListW3MI, got %v", err)
	}
}

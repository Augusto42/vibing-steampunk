package adt

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestEditSource_NoPackageLookupBetweenLockAndWriteAndUnlocksOnFailure(t *testing.T) {
	mock := &methodPathMock{routes: []routedResponse{
		resp(http.MethodGet, "informationsystem/search", http.StatusOK, `<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zsynthetic" adtcore:type="PROG/P" adtcore:name="ZSYNTHETIC" adtcore:packageName="$TMP"/></adtcore:objectReferences>`),
		resp(http.MethodGet, "/source/main", http.StatusOK, "REPORT zsynthetic."),
		resp(http.MethodPost, "/programs/programs/ZSYNTHETIC", http.StatusOK, syntheticLocalLockXML),
		resp(http.MethodPut, "/source/main", http.StatusLocked, "synthetic lock failure"),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
	result, err := client.EditSourceWithOptions(
		context.Background(),
		"/sap/bc/adt/programs/programs/ZSYNTHETIC",
		"REPORT zsynthetic.",
		"REPORT zsynthetic_changed.",
		&EditSourceOptions{SyntaxCheck: false},
	)
	if err != nil {
		t.Fatalf("EditSourceWithOptions returned error: %v", err)
	}
	if result == nil || !strings.Contains(result.Message, "Failed to update source") {
		t.Fatalf("unexpected result: %#v", result)
	}

	searches, lockIndex, putIndex, unlockIndex := 0, -1, -1, -1
	for i, call := range mock.calls {
		if strings.Contains(call.path, "informationsystem/search") {
			searches++
		}
		if call.method == http.MethodPost && call.query.Get("_action") == "LOCK" {
			lockIndex = i
		}
		if call.method == http.MethodPut {
			putIndex = i
		}
		if call.method == http.MethodPost && call.query.Get("_action") == "UNLOCK" {
			unlockIndex = i
		}
	}
	if searches != 1 || lockIndex < 0 || putIndex != lockIndex+1 || unlockIndex <= putIndex {
		t.Fatalf("unsafe request order: %#v", mock.calls)
	}
}

func TestLockObject_PreservesModificationSupport(t *testing.T) {
	const noModLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>HANDLE-X</LOCK_HANDLE>
      <CORRNR></CORRNR>
      <CORRUSER></CORRUSER>
      <CORRTEXT></CORRTEXT>
      <IS_LOCAL></IS_LOCAL>
      <IS_LINK_UP></IS_LINK_UP>
      <MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>
    </DATA>
  </asx:values>
</asx:abap>`
	mock := &methodPathMock{
		routes: []routedResponse{
			resp("", "discovery", 200, "ok"),
			resp(http.MethodPost, "/oo/classes/ZREADONLY", 200, noModLockXML),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.LockObject(
		context.Background(),
		"/sap/bc/adt/oo/classes/ZREADONLY",
		"MODIFY",
	)
	if err != nil {
		t.Fatalf("LockObject returned an error for valid metadata: %v", err)
	}
	if result.LockHandle != "HANDLE-X" {
		t.Errorf("LockHandle = %q, want HANDLE-X", result.LockHandle)
	}
	if result.ModificationSupport != "NoModification" {
		t.Errorf("ModificationSupport = %q, want NoModification", result.ModificationSupport)
	}
}

func TestMutationPackageMarkerDoesNotBypassOperationOrTransportPolicy(t *testing.T) {
	ctx := withMutationPackageChecked(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC")
	t.Run("operation", func(t *testing.T) {
		cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithSafety(SafetyConfig{DisallowedOps: "U"}))
		client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &methodPathMock{}))
		if err := client.checkMutation(ctx, MutationContext{Op: OpUpdate, OpName: "synthetic update"}); err == nil {
			t.Fatal("marked context bypassed operation policy")
		}
	})
	t.Run("transport", func(t *testing.T) {
		cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
		client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &methodPathMock{}))
		if err := client.checkMutation(ctx, MutationContext{Op: OpUpdate, OpName: "synthetic update", Transport: "REQ-SYN-001"}); err == nil {
			t.Fatal("marked context bypassed transport policy")
		}
	})
}

func TestPackageExistsUsesDirectResource(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantExists bool
		wantErr    bool
	}{
		{name: "existing empty package", status: http.StatusOK, wantExists: true},
		{name: "missing package", status: http.StatusNotFound},
		{name: "inconclusive server error", status: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &methodPathMock{routes: []routedResponse{
				resp(http.MethodGet, "/packages/$SYNTHETIC", tt.status, "synthetic response"),
			}}
			client := newReconcileClient(t, mock)
			exists, err := client.PackageExists(context.Background(), "$SYNTHETIC")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr=%t", err, tt.wantErr)
			}
			if exists != tt.wantExists {
				t.Fatalf("exists = %t, want %t", exists, tt.wantExists)
			}
			if len(mock.calls) != 1 || mock.calls[0].method != http.MethodGet {
				t.Fatalf("unexpected calls: %#v", mock.calls)
			}
		})
	}
}

func TestUpdateSource_LockCorrelationStillHonorsSafety(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
	}{
		{name: "transportable edits disabled"},
		{name: "transport not allowed", options: []Option{WithAllowTransportableEdits(), WithAllowedTransports("REQ-OTHER-*")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &methodPathMock{routes: []routedResponse{
				resp("", "discovery", http.StatusOK, ""),
				resp(http.MethodPost, "/programs/programs/ZSYNTHETIC", http.StatusOK, syntheticTransportLockXML),
			}}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass", tt.options...)
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
			lock, err := client.LockObject(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC", "MODIFY")
			if err != nil {
				t.Fatalf("LockObject failed: %v", err)
			}
			err = client.UpdateSource(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC/source/main", "REPORT zsynthetic.", lock.LockHandle, "")
			if err == nil {
				t.Fatal("UpdateSource should be blocked by transport safety")
			}
			for _, call := range mock.calls {
				if call.method == http.MethodPut {
					t.Fatal("blocked mutation sent a PUT request")
				}
			}
		})
	}
}

func TestUpdateSource_PropagatesWriteFailuresWithoutBlindRetry(t *testing.T) {
	for _, status := range []int{http.StatusLocked, http.StatusConflict, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mock := &methodPathMock{routes: []routedResponse{
				resp("", "discovery", http.StatusOK, ""),
				resp(http.MethodPut, "/source/main", status, "synthetic write failure"),
			}}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
			err := client.UpdateSource(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC/source/main", "REPORT zsynthetic.", "SYNTHETIC-HANDLE", "")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
				t.Fatalf("error = %v, want API status %d", err, status)
			}
			putCount := 0
			for _, call := range mock.calls {
				if call.method == http.MethodPut {
					putCount++
				}
			}
			if putCount != 1 {
				t.Fatalf("PUT count = %d, want 1", putCount)
			}
		})
	}
}

func TestUpdateSource_RejectsTransportThatDiffersFromLockCorrelation(t *testing.T) {
	mock := &methodPathMock{routes: []routedResponse{
		resp("", "discovery", http.StatusOK, ""),
		resp(http.MethodPost, "/programs/programs/ZSYNTHETIC", http.StatusOK, syntheticTransportLockXML),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowTransportableEdits(),
		WithAllowedTransports("REQ-EXPLICIT-*", "REQ-SYN-*"),
	)
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	lock, err := client.LockObject(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC", "MODIFY")
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	err = client.UpdateSource(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC/source/main", "REPORT zsynthetic.", lock.LockHandle, "REQ-EXPLICIT-001")
	if err == nil {
		t.Fatal("UpdateSource should reject a transport that differs from the lock correlation")
	}
	if !strings.Contains(err.Error(), "REQ-EXPLICIT-001") || !strings.Contains(err.Error(), "REQ-SYN-001") {
		t.Fatalf("mismatch error should identify both transports, got: %v", err)
	}

	for _, call := range mock.calls {
		if call.method == http.MethodPut {
			t.Fatal("transport mismatch sent a PUT request")
		}
	}
}

func TestUpdateSource_UsesLockCorrelationTransport(t *testing.T) {
	mock := &methodPathMock{routes: []routedResponse{
		resp("", "discovery", http.StatusOK, ""),
		resp(http.MethodPost, "/programs/programs/ZSYNTHETIC", http.StatusOK, syntheticTransportLockXML),
		resp(http.MethodPut, "/source/main", http.StatusOK, ""),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass",
		WithAllowTransportableEdits(),
		WithAllowedTransports("REQ-SYN-*"),
	)
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	lock, err := client.LockObject(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC", "MODIFY")
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	if err := client.UpdateSource(context.Background(), "/sap/bc/adt/programs/programs/ZSYNTHETIC/source/main", "REPORT zsynthetic.", lock.LockHandle, ""); err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	for _, call := range mock.calls {
		if call.method == http.MethodPut {
			if got := call.query.Get("corrNr"); got != "REQ-SYN-001" {
				t.Fatalf("PUT corrNr = %q, want REQ-SYN-001", got)
			}
			return
		}
	}
	t.Fatal("no PUT request was sent")
}

const syntheticLocalLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values><DATA>
    <LOCK_HANDLE>SYNTHETIC-HANDLE</LOCK_HANDLE>
  </DATA></asx:values>
</asx:abap>`

const syntheticTransportLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values><DATA>
    <LOCK_HANDLE>SYNTHETIC-HANDLE</LOCK_HANDLE>
    <CORRNR>REQ-SYN-001</CORRNR>
    <MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>
  </DATA></asx:values>
</asx:abap>`

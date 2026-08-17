package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCheckMutation_NoPolicy_Passes(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "TestOp",
		ObjectURL: "/sap/bc/adt/programs/programs/ZTEST",
	})
	if err != nil {
		t.Fatalf("expected no error when no policy configured, got: %v", err)
	}
}

func TestCheckMutation_OpType_Blocked(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithReadOnly())
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:     OpUpdate,
		OpName: "TestOp",
	})
	if err == nil {
		t.Fatal("expected read-only mode to block OpUpdate")
	}
}

func TestCheckMutation_ExplicitPackage_NotInWhitelist(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:      OpCreate,
		OpName:  "CreateObject",
		Package: "ZOTHER",
	})
	if err == nil {
		t.Fatal("expected explicit package outside whitelist to be blocked")
	}
	if !strings.Contains(err.Error(), "ZOTHER") {
		t.Fatalf("expected error to mention blocked package, got: %v", err)
	}
}

func TestCheckMutation_ObjectURL_ResolvesADTPackage(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newSearchResponse("/sap/bc/adt/programs/programs/ztest", "PROG/P", "ZTEST", "ZOTHER"),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: "/sap/bc/adt/programs/programs/ZTEST/source/main",
	})
	if err == nil {
		t.Fatal("expected object URL resolution to block non-whitelisted package")
	}
	if !strings.Contains(err.Error(), "ZOTHER") {
		t.Fatalf("expected error to mention resolved package, got: %v", err)
	}
}

func TestCheckMutation_UI5Surface_BlockedWhenPolicyActive(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UI5UploadFile",
		ObjectURL: "MYAPP",
		Surface:   SurfaceUI5,
	})
	if err == nil {
		t.Fatal("expected UI5 surface to be blocked until app→package resolution lands")
	}
	if !strings.Contains(err.Error(), "UI5") {
		t.Fatalf("expected error to mention UI5, got: %v", err)
	}
}

func TestCheckMutation_UI5Surface_AllowedWhenNoPolicy(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UI5UploadFile",
		ObjectURL: "MYAPP",
		Surface:   SurfaceUI5,
	})
	if err != nil {
		t.Fatalf("expected UI5 surface to pass when no package policy, got: %v", err)
	}
}

func TestCheckMutation_MissingObjectURLAndPackage_FailsClosed(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:     OpUpdate,
		OpName: "MysteryOp",
	})
	if err == nil {
		t.Fatal("expected gate to fail closed when neither ObjectURL nor Package is provided under policy")
	}
	if !strings.Contains(err.Error(), "MysteryOp") {
		t.Fatalf("expected error to mention op name, got: %v", err)
	}
}

func TestPackageTransportRequirement_TransportablePackageWithoutRequestFailsClosed(t *testing.T) {
	packageXML := `<?xml version="1.0" encoding="utf-8"?>
<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZSYNTHETIC">
  <pak:attributes pak:recordChanges="true"/>
  <pak:transport><pak:softwareComponent pak:name="HOME"/></pak:transport>
</pak:package>`
	mock := &mockTransportClient{responses: map[string]*http.Response{
		"/sap/bc/adt/packages/ZSYNTHETIC": newTestResponse(packageXML),
		"discovery":                       newTestResponse("OK"),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	err := client.checkPackageTransportRequirement(context.Background(), "ZSYNTHETIC", "", "WriteSource")
	if err == nil {
		t.Fatal("transportable package without request should be blocked")
	}
	if !strings.Contains(err.Error(), "blocked before mutation") || !strings.Contains(err.Error(), "--transport") {
		t.Fatalf("unexpected preflight error: %v", err)
	}
}

func TestPackageTransportRequirement_NamedLocalPackageDoesNotRequireRequest(t *testing.T) {
	packageXML := `<?xml version="1.0" encoding="utf-8"?>
<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZLOCAL">
  <pak:attributes pak:recordChanges="false"/>
  <pak:transport><pak:softwareComponent pak:name="LOCAL"/></pak:transport>
</pak:package>`
	mock := &mockTransportClient{responses: map[string]*http.Response{
		"/sap/bc/adt/packages/ZLOCAL": newTestResponse(packageXML),
		"discovery":                   newTestResponse("OK"),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if err := client.checkPackageTransportRequirement(context.Background(), "ZLOCAL", "", "WriteSource"); err != nil {
		t.Fatalf("named local package should not require a request: %v", err)
	}
}

func TestWriteSource_TransportableCreateWithoutRequestSendsNoMutation(t *testing.T) {
	packageXML := `<?xml version="1.0" encoding="utf-8"?>
<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZSYNTHETIC">
  <pak:attributes pak:recordChanges="true"/>
  <pak:transport><pak:softwareComponent pak:name="HOME"/></pak:transport>
</pak:package>`
	mock := &mockWorkflowTransport{responses: map[string]*http.Response{
		"GET /sap/bc/adt/programs/programs/ZNO_REQUEST/source/main": newWorkflowStatusResponse(http.StatusNotFound, "not found"),
		"/sap/bc/adt/packages/ZSYNTHETIC":                           newWorkflowTestResponse(packageXML),
		"discovery":                                                 newWorkflowTestResponse("OK"),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	result, err := client.WriteSource(context.Background(), "PROG", "ZNO_REQUEST", "REPORT zno_request.", &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: "Synthetic transport guard",
		Package:     "ZSYNTHETIC",
	})
	if err == nil {
		t.Fatalf("transportable create should fail before mutation, got result: %#v", result)
	}
	for _, req := range mock.requests {
		if req.Method != http.MethodGet {
			t.Fatalf("preflight failure sent a mutation: %s %s", req.Method, req.URL.Path)
		}
	}
}

func TestClient_UI5UploadFile_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5UploadFile(context.Background(), "MYAPP", "/index.html", []byte("x"), "text/html")
	if err == nil {
		t.Fatal("expected UI5UploadFile to be blocked under AllowedPackages policy")
	}
}

func TestClient_UI5DeleteFile_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5DeleteFile(context.Background(), "MYAPP", "/index.html")
	if err == nil {
		t.Fatal("expected UI5DeleteFile to be blocked under AllowedPackages policy")
	}
}

func TestClient_UI5DeleteApp_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5DeleteApp(context.Background(), "MYAPP", "")
	if err == nil {
		t.Fatal("expected UI5DeleteApp to be blocked under AllowedPackages policy")
	}
}

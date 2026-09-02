package saprfc

import (
	"context"
	"strings"
	"testing"
)

func TestWhoAmIReadsTheOwnerOutOfAnEmptyTree(t *testing.T) {
	// The shape SAP returns when you own no transport requests at all: a root
	// element, no children, and your name on it. This is the case that matters,
	// because it is the one that looks like "no answer" and is not.
	empty := []byte(`<?xml version="1.0" encoding="utf-8"?><tm:root adtcore:name="TESTUSER" ` +
		`adtcore:changedBy="TESTUSER" adtcore:createdBy="TESTUSER" ` +
		`xmlns:tm="http://www.sap.com/cts/adt/tm" xmlns:adtcore="http://www.sap.com/adt/core"/>`)

	if got := userFromTransportTree(empty); got != "TESTUSER" {
		t.Fatalf("owner should be TESTUSER, got %q", got)
	}
}

func TestWhoAmIFallsBackWhenTheRootIsUnnamed(t *testing.T) {
	// Some releases leave the root unnamed and say it only in createdBy.
	body := []byte(`<tm:root adtcore:createdBy="testuser" ` +
		`xmlns:tm="http://www.sap.com/cts/adt/tm" xmlns:adtcore="http://www.sap.com/adt/core"/>`)
	if got := userFromTransportTree(body); got != "TESTUSER" {
		t.Fatalf("should fall back to createdBy and upper-case it, got %q", got)
	}
}

func TestWhoAmIReportsWhenTheSystemNamesNobody(t *testing.T) {
	transport := &scriptedTransport{answers: map[string]string{
		"GET " + whoAmIResource + "|": `<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm"/>`,
	}}
	_, err := CurrentUser(context.Background(), transport)
	if err == nil {
		t.Fatal("an answer that names nobody is not an answer")
	}
	if !strings.Contains(err.Error(), "without naming a user") {
		t.Fatalf("the failure should say what was missing, got: %v", err)
	}
}

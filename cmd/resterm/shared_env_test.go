package main

import (
	"strings"
	"testing"
)

func TestParseCompareTargetsRejectsShared(t *testing.T) {
	_, err := parseCompareTargets("dev $shared")
	if err == nil {
		t.Fatalf("expected parseCompareTargets to reject $shared")
	}
	if !strings.Contains(err.Error(), "reserved for shared defaults") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

func TestValidateReservedEnvironmentSelection(t *testing.T) {
	if err := validateReservedEnvironment("dev", "--env"); err != nil {
		t.Fatalf("expected dev to be accepted, got %v", err)
	}
	if err := validateReservedEnvironment("", "--env"); err != nil {
		t.Fatalf("expected empty value to be accepted, got %v", err)
	}

	err := validateReservedEnvironment("$shared", "--env")
	if err == nil {
		t.Fatalf("expected $shared to be rejected")
	}
	if !strings.Contains(err.Error(), "reserved for shared defaults") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

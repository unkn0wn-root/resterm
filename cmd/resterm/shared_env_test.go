package main

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/runx/check"
)

func TestParseCompareTargetsRejectsShared(t *testing.T) {
	_, err := cli.ParseCompareTargets("dev $shared")
	if err == nil {
		t.Fatalf("expected parseCompareTargets to reject $shared")
	}
	if !strings.Contains(err.Error(), "reserved for shared defaults") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

func TestValidateConcreteEnvironmentSelection(t *testing.T) {
	if err := runcheck.ValidateConcreteEnvironment("dev", "--env"); err != nil {
		t.Fatalf("expected dev to be accepted, got %v", err)
	}
	if err := runcheck.ValidateConcreteEnvironment("", "--env"); err != nil {
		t.Fatalf("expected empty value to be accepted, got %v", err)
	}

	err := runcheck.ValidateConcreteEnvironment("$shared", "--env")
	if err == nil {
		t.Fatalf("expected $shared to be rejected")
	}
	if !strings.Contains(err.Error(), "reserved for shared defaults") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

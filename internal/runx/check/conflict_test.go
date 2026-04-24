package runcheck

import (
	"strings"
	"testing"
)

func TestValidateConcreteEnvironment(t *testing.T) {
	if err := ValidateConcreteEnvironment("", "--env"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateConcreteEnvironment("dev", "--env"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := ValidateConcreteEnvironment("$shared", "--env")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `--env "$shared" is reserved for shared defaults. Choose a concrete environment` {
		t.Fatalf("error = %q", got)
	}
	if !strings.Contains(err.Error(), "reserved for shared defaults") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProfileCompare(t *testing.T) {
	n := Names{
		Profile:  "profile.enabled",
		Compare:  "compare.targets",
		Workflow: "selection.workflow",
	}

	err := ValidateProfileCompare(true, true, n)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "profile.enabled cannot be combined with compare.targets" {
		t.Fatalf("error = %q", got)
	}
	if err := ValidateProfileCompare(true, false, n); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWorkflowMode(t *testing.T) {
	n := Names{
		Profile:  "--profile",
		Compare:  "--compare",
		Workflow: "--workflow",
	}

	err := ValidateWorkflowMode(true, false, true, n)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "--workflow cannot be combined with --compare or --profile" {
		t.Fatalf("error = %q", got)
	}
	if err := ValidateWorkflowMode(false, true, true, n); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

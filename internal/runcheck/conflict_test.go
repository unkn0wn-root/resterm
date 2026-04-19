package runcheck

import "testing"

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

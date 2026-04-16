package cli

import "testing"

func TestStringVarTrimsDefaultAndParsedValue(t *testing.T) {
	fs := NewFlagSet("trim")
	var got string
	StringVar(fs, &got, "name", "  dev  ", "name")
	if got != "dev" {
		t.Fatalf("default value = %q, want %q", got, "dev")
	}

	if err := fs.Parse([]string{"-name", "  prod  "}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "prod" {
		t.Fatalf("parsed value = %q, want %q", got, "prod")
	}
}

func TestStringVarSupportsAliasBinding(t *testing.T) {
	fs := NewFlagSet("trim")
	var got string
	StringVar(fs, &got, "request", "", "request")
	StringVar(fs, &got, "r", "", "request")

	if err := fs.Parse([]string{"-r", "  sample  "}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "sample" {
		t.Fatalf("alias value = %q, want %q", got, "sample")
	}
}

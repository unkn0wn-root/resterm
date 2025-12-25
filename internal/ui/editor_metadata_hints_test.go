package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/ui/hint"
)

func TestMetadataHintCatalogContainsRequiredDirectives(t *testing.T) {
	required := []string{"@body", "@const", "@variables", "@query", "@trace"}
	labels := make(map[string]struct{}, len(hint.MetaCatalog))
	for _, option := range hint.MetaCatalog {
		labels[option.Label] = struct{}{}
	}
	for _, label := range required {
		if _, ok := labels[label]; !ok {
			t.Fatalf("metadata hint catalog missing %s", label)
		}
	}
}

func TestFilterMetadataHintOptionsForSubcommands(t *testing.T) {
	options := hint.MetaOptions("ws", "")
	if len(options) == 0 {
		t.Fatal("expected ws subcommand options")
	}
	expected := []string{
		"send",
		"send-json",
		"send-base64",
		"send-file",
		"ping",
		"pong",
		"wait",
		"close",
	}
	for _, label := range expected {
		if !hintOptionsContain(options, label) {
			t.Fatalf("expected ws subcommand %q", label)
		}
	}

	filtered := hint.MetaOptions("ws", "send-")
	if len(filtered) == 0 {
		t.Fatal("expected filtered subcommand results for prefix")
	}
	for _, option := range filtered {
		if !strings.HasPrefix(option.Label, "send") {
			t.Fatalf("expected send* suggestion, got %q", option.Label)
		}
	}

	if opts := hint.MetaOptions("unknown", ""); opts != nil {
		t.Fatalf("expected nil suggestions for unknown directive, got %v", opts)
	}

	traceOptions := hint.MetaOptions("trace", "")
	if len(traceOptions) == 0 {
		t.Fatal("expected trace subcommand options")
	}
	for _, label := range []string{"enabled=true", "dns<=", "tolerance="} {
		if !hintOptionsContain(traceOptions, label) {
			t.Fatalf("missing trace subcommand %q", label)
		}
	}
	filteredTrace := hint.MetaOptions("trace", "tot")
	if len(filteredTrace) == 0 {
		t.Fatal("expected filtered trace subcommand results")
	}
	for _, option := range filteredTrace {
		if !strings.HasPrefix(option.Label, "tot") {
			t.Fatalf("expected tot* suggestion, got %q", option.Label)
		}
	}
}

func TestTraceMetadataHintsProvidePlaceholders(t *testing.T) {
	options := hint.MetaOptions("trace", "")
	var dns hint.Hint
	found := false
	for _, option := range options {
		if option.Label == "dns<=" {
			dns = option
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dns<= trace option")
	}
	if dns.Insert != "dns<=50ms" {
		t.Fatalf("expected dns<= insert with placeholder, got %q", dns.Insert)
	}
	if dns.CursorBack != len("50ms") {
		t.Fatalf("expected dns<= cursor back %d, got %d", len("50ms"), dns.CursorBack)
	}
}

func TestTraceMetadataHintsFilterByPrefix(t *testing.T) {
	filtered := hint.MetaOptions("trace", "d")
	if len(filtered) == 0 {
		t.Fatal("expected filtered trace hints for prefix 'd'")
	}
	if !hintOptionsContain(filtered, "dns<=") {
		t.Fatalf("expected dns<= in filtered hints, got %v", filtered)
	}
}

func hintOptionsContain(options []hint.Hint, label string) bool {
	for _, option := range options {
		if option.Label == label {
			return true
		}
	}
	return false
}

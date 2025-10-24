package ui

import (
	"strings"
	"testing"
)

func TestMetadataHintCatalogContainsRequiredDirectives(t *testing.T) {
	required := []string{"@body", "@const", "@variables", "@query"}
	labels := make(map[string]struct{}, len(metadataHintCatalog))
	for _, option := range metadataHintCatalog {
		labels[option.Label] = struct{}{}
	}
	for _, label := range required {
		if _, ok := labels[label]; !ok {
			t.Fatalf("metadata hint catalog missing %s", label)
		}
	}
}

func TestFilterMetadataHintOptionsForSubcommands(t *testing.T) {
	options := filterMetadataHintOptions("ws", "")
	if len(options) == 0 {
		t.Fatal("expected ws subcommand options")
	}
	expected := []string{"send", "send-json", "send-base64", "send-file", "ping", "pong", "wait", "close"}
	for _, label := range expected {
		if !hintOptionsContain(options, label) {
			t.Fatalf("expected ws subcommand %q", label)
		}
	}

	filtered := filterMetadataHintOptions("ws", "send-")
	if len(filtered) == 0 {
		t.Fatal("expected filtered subcommand results for prefix")
	}
	for _, option := range filtered {
		if !strings.HasPrefix(option.Label, "send") {
			t.Fatalf("expected send* suggestion, got %q", option.Label)
		}
	}

	if opts := filterMetadataHintOptions("unknown", ""); opts != nil {
		t.Fatalf("expected nil suggestions for unknown directive, got %v", opts)
	}
}

func hintOptionsContain(options []metadataHintOption, label string) bool {
	for _, option := range options {
		if option.Label == label {
			return true
		}
	}
	return false
}

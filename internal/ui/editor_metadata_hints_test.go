package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/ui/hint"
)

func TestMetadataHintCatalogContainsRequiredDirectives(t *testing.T) {
	required := []string{
		"@body",
		"@const",
		"@variables",
		"@query",
		"@trace",
		"@patch",
		"@k8s",
		"@rts",
	}
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

func TestRTSMetadataHintInsertsExplicitPreRequestMode(t *testing.T) {
	for _, option := range hint.MetaCatalog {
		if option.Label != "@rts" {
			continue
		}
		if option.Insert != "@rts pre-request" {
			t.Fatalf("expected @rts hint to insert explicit mode, got %q", option.Insert)
		}
		return
	}
	t.Fatal("metadata hint catalog missing @rts")
}

func TestFilterMetadataHintOptionsForSubcommands(t *testing.T) {
	authOptions := hint.MetaOptions("auth", "")
	if len(authOptions) == 0 {
		t.Fatal("expected auth subcommand options")
	}
	for _, label := range []string{
		"basic",
		"bearer",
		"apikey",
		"oauth2",
		"command",
		"token_url=",
		"argv=",
		"cache_key=",
	} {
		if !hintOptionsContain(authOptions, label) {
			t.Fatalf("missing auth subcommand %q", label)
		}
	}
	filteredAuth := hint.MetaOptions("auth", "com")
	if len(filteredAuth) == 0 {
		t.Fatal("expected filtered auth subcommand results")
	}
	for _, option := range filteredAuth {
		if !strings.HasPrefix(option.Label, "com") {
			t.Fatalf("expected com* suggestion, got %q", option.Label)
		}
	}

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

	rtsOptions := hint.MetaOptions("rts", "")
	if len(rtsOptions) == 0 {
		t.Fatal("expected rts subcommand options")
	}
	for _, label := range []string{"pre-request", "test"} {
		if !hintOptionsContain(rtsOptions, label) {
			t.Fatalf("missing rts subcommand %q", label)
		}
	}
	filteredRTS := hint.MetaOptions("rts", "pre")
	if len(filteredRTS) == 0 {
		t.Fatal("expected filtered rts subcommand results")
	}
	for _, option := range filteredRTS {
		if !strings.HasPrefix(option.Label, "pre") {
			t.Fatalf("expected pre* suggestion, got %q", option.Label)
		}
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

	applyOptions := hint.MetaOptions("apply", "")
	if len(applyOptions) == 0 {
		t.Fatal("expected apply subcommand options")
	}
	if !hintOptionsContain(applyOptions, "use=") {
		t.Fatalf("expected apply use= suggestion, got %v", applyOptions)
	}

	k8sOptions := hint.MetaOptions("k8s", "")
	if len(k8sOptions) == 0 {
		t.Fatal("expected k8s subcommand options")
	}
	for _, label := range []string{
		"target=",
		"namespace=",
		"pod=",
		"service=",
		"deployment=",
		"statefulset=",
		"port=",
		"use=",
	} {
		if !hintOptionsContain(k8sOptions, label) {
			t.Fatalf("missing k8s subcommand %q", label)
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

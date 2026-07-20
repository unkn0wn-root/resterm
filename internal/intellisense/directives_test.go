package intellisense

import (
	"strings"
	"testing"
)

func contains(items []Item, label string) bool {
	for _, it := range items {
		if it.Label == label {
			return true
		}
	}
	return false
}

func argOptions(directive, query string) []Item {
	return directiveSource{}.Provide(
		Context{Kind: KindDirectiveArg, Directive: directive, Query: query},
		Scope{},
	)
}

func TestDirectiveCatalogContainsRequiredDirectives(t *testing.T) {
	required := []string{
		"@body",
		"@const",
		"@variables",
		"@query",
		"@trace",
		"@patch",
		"@k8s",
		"@rts",
		"@sse",
		"@expect",
	}
	for _, label := range required {
		if !contains(directives, label) {
			t.Fatalf("directive catalog missing %s", label)
		}
	}
}

func TestRTSDirectiveInsertsExplicitPreRequestMode(t *testing.T) {
	for _, it := range directives {
		if it.Label != "@rts" {
			continue
		}
		if it.Insert != "@rts pre-request" {
			t.Fatalf("expected @rts to insert explicit mode, got %q", it.Insert)
		}
		return
	}
	t.Fatal("directive catalog missing @rts")
}

func TestDirectiveArgsFilterByPrefix(t *testing.T) {
	auth := argOptions("auth", "")
	for _, label := range []string{"basic", "bearer", "apikey", "oauth2", "command", "token_url=", "argv=", "cache_key="} {
		if !contains(auth, label) {
			t.Fatalf("missing auth arg %q", label)
		}
	}
	for _, it := range argOptions("auth", "com") {
		if !strings.HasPrefix(it.Label, "com") {
			t.Fatalf("expected com* suggestion, got %q", it.Label)
		}
	}

	ws := argOptions("ws", "")
	for _, label := range []string{"send", "send-json", "send-base64", "send-file", "ping", "pong", "wait", "close"} {
		if !contains(ws, label) {
			t.Fatalf("missing ws arg %q", label)
		}
	}

	sse := argOptions("sse", "")
	for _, label := range []string{"timeout=", "duration=", "idle=", "idle-timeout=", "max-events=", "max-bytes=", "limit-bytes=", "off"} {
		if !contains(sse, label) {
			t.Fatalf("missing sse arg %q", label)
		}
	}

	k8s := argOptions("k8s", "")
	for _, label := range []string{"target=", "namespace=", "pod=", "service=", "deployment=", "statefulset=", "port=", "use="} {
		if !contains(k8s, label) {
			t.Fatalf("missing k8s arg %q", label)
		}
	}

	rts := argOptions("rts", "")
	if !contains(rts, "pre-request") || contains(rts, "test") {
		t.Fatalf("rts args = %v", rts)
	}

	mock := argOptions("mock", "")
	for _, label := range []string{"sequence=", "sequence-key=", "interpolate=false"} {
		if !contains(mock, label) {
			t.Fatalf("mock args missing %q: %v", label, mock)
		}
	}
	if !contains(argOptions("expect", ""), "calls=") {
		t.Fatal("expect args missing calls=")
	}

	if opts := argOptions("unknown", ""); opts != nil {
		t.Fatalf("expected nil for unknown directive, got %v", opts)
	}
}

func TestTraceArgsProvidePlaceholders(t *testing.T) {
	var dns Item
	for _, it := range argOptions("trace", "") {
		if it.Label == "dns<=" {
			dns = it
		}
	}
	if dns.Insert != "dns<=50ms" {
		t.Fatalf("expected dns<= insert with placeholder, got %q", dns.Insert)
	}
	if dns.CursorBack != len("50ms") {
		t.Fatalf("expected dns<= cursor back %d, got %d", len("50ms"), dns.CursorBack)
	}
	if !contains(argOptions("trace", "d"), "dns<=") {
		t.Fatal("expected dns<= in filtered trace args")
	}
}

func TestCompareArgsIncludeEnvironments(t *testing.T) {
	sc := Scope{Environments: []string{"dev", "prod"}}
	ctx := Context{Kind: KindDirectiveArg, Directive: "compare", Query: ""}
	items := directiveSource{}.Provide(ctx, sc)
	for _, label := range []string{"base=", "baseline=", "dev", "prod"} {
		if !contains(items, label) {
			t.Fatalf("compare suggestions missing %q: %v", label, items)
		}
	}
	// Static options must not be mutated by appended environments.
	if got := len(directiveArgs["compare"]); got != 2 {
		t.Fatalf("compare static args mutated, len = %d", got)
	}
}

func TestUseValueOffersProfileNames(t *testing.T) {
	sc := Scope{
		Profiles: ProfileSet{
			Patch: []string{"jsonApi"},
			SSH:   []string{"edge"},
			K8s:   []string{"cluster"},
		},
	}

	apply := directiveSource{}.Provide(
		Context{Kind: KindDirectiveArg, Directive: "apply", ArgKey: "use"},
		sc,
	)
	if !contains(apply, "jsonApi") {
		t.Fatalf("apply use= missing patch profile: %v", apply)
	}
	ssh := directiveSource{}.Provide(
		Context{Kind: KindDirectiveArg, Directive: "ssh", ArgKey: "use"},
		sc,
	)
	if !contains(ssh, "edge") {
		t.Fatalf("ssh use= missing profile: %v", ssh)
	}
	k8s := directiveSource{}.Provide(
		Context{Kind: KindDirectiveArg, Directive: "k8s", ArgKey: "use"},
		sc,
	)
	if !contains(k8s, "cluster") {
		t.Fatalf("k8s use= missing profile: %v", k8s)
	}
}

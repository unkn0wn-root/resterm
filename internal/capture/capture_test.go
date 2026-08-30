package capture

import "testing"

func TestStrictEnabledKeyPriority(t *testing.T) {
	s := map[string]string{
		"capture_strict": "true",
		"capture-strict": "false",
		"capture.strict": "true",
	}
	if !StrictEnabled(s) {
		t.Fatalf("expected capture.strict to take precedence over aliases")
	}
}

func TestStrictEnabledScopeOverride(t *testing.T) {
	file := map[string]string{"capture.strict": "true"}
	req := map[string]string{"capture.strict": "false"}
	if StrictEnabled(file, req) {
		t.Fatalf("expected later scope to override earlier scope")
	}
}

func TestStrictEnabledAcceptsAliases(t *testing.T) {
	for _, s := range []map[string]string{
		{"capture.strict": "true"},
		{"capture-strict": "true"},
		{"capture_strict": "true"},
	} {
		if !StrictEnabled(s) {
			t.Fatalf("expected strict alias to enable strict mode: %v", s)
		}
	}
}

func TestStrictEnabledConflictingCanonicalizedKeysSafeDefault(t *testing.T) {
	s := map[string]string{
		" capture.strict ": "true",
		"CAPTURE.STRICT":   "false",
	}
	if StrictEnabled(s) {
		t.Fatalf("expected conflicting canonicalized keys to resolve to safe default false")
	}
}

func TestHasJSONPathDoubleDotIgnoresQuoted(t *testing.T) {
	if HasJSONPathDoubleDot(`contains("response.json..token", "x")`) {
		t.Fatalf("expected quoted content not to trigger double-dot detection")
	}
	if !HasJSONPathDoubleDot(`response.json..token`) {
		t.Fatalf("expected direct double-dot path to be detected")
	}
}

func TestHasUnquotedTemplateMarker(t *testing.T) {
	tests := []struct {
		name string
		ex   string
		want bool
	}{
		{name: "plain text", ex: `Bearer {{response.json.token}}`, want: true},
		{name: "quoted marker", ex: `contains(response.text(), "{{token}}")`},
		{name: "no marker", ex: `response.json.token`},
		{name: "unmatched bracket", ex: `prefix[{{response.status}}`, want: true},
		{name: "unmatched paren", ex: `prefix({{response.status}}`, want: true},
		{name: "hash before the marker", ex: `anchor#{{response.status}}`, want: true},
		{name: "hash after the marker", ex: `{{response.status}}#anchor`, want: true},
		{name: "open marker", ex: `prefix {{response.status`},
		{name: "closed then open", ex: `{{a}} and {{b`, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HasUnquotedTemplateMarker(tt.ex); got != tt.want {
				t.Fatalf("HasUnquotedTemplateMarker(%q) = %v, want %v", tt.ex, got, tt.want)
			}
		})
	}
}

func TestOpenMarker(t *testing.T) {
	tests := []struct {
		name string
		ex   string
		want string
	}{
		{name: "empty"},
		{name: "no marker", ex: `response.json.token`},
		{name: "closed marker", ex: `Bearer {{token}}`},
		{name: "open marker", ex: `Bearer {{token`, want: "}}"},
		{name: "open expression marker", ex: `{{=`, want: "}}"},
		{name: "open after a closed one", ex: `{{a}} and {{b`, want: "}}"},
		{name: "open across lines", ex: "{{\n  response.status", want: "}}"},
		{name: "closed across lines", ex: "{{\n  response.status\n}}"},
		{name: "marker in a string", ex: `contains(response.text(), "{{token")`},
		{name: "unmatched bracket", ex: `prefix[{{token}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OpenMarker(tt.ex); got != tt.want {
				t.Fatalf("OpenMarker(%q) = %q, want %q", tt.ex, got, tt.want)
			}
		})
	}
}

func TestTemplateScannerMatchesBatchScanAtEveryChunkBoundary(t *testing.T) {
	inputs := []string{
		`response.json.token`,
		`Bearer {{token}}`,
		`Bearer {{token`,
		`{{a}} and {{b`,
		`contains(response.text(), "{{token")`,
		"contains(\"escaped\\\n{{token}}\")",
		"{{\n response.status\n}}",
		`{{a}"{{b}}"{{c}}`,
	}

	for _, input := range inputs {
		for cut := range len(input) + 1 {
			var scanner TemplateScanner
			scanner.Feed(input[:cut])
			scanner.Feed(input[cut:])
			if got, want := scanner.State(), templateState(input); got != want {
				t.Fatalf("chunks [%q, %q] have state %d, whole read gives %d", input[:cut], input[cut:], got, want)
			}
		}
	}
}

func templateState(input string) TemplateState {
	scan := scanTemplates(input)
	if scan.open {
		return TemplateOpen
	}
	if scan.has {
		return TemplateClosed
	}
	return TemplateNone
}

func TestMixedTemplateRTSCall(t *testing.T) {
	if !MixedTemplateRTSCall(`contains({{name}}, "x")`) {
		t.Fatalf("expected mixed template+call form to be detected")
	}
	if !MixedTemplateRTSCall(`contains({{name}})`) {
		t.Fatalf("expected single-arg mixed template+call form to be detected")
	}
	if MixedTemplateRTSCall(`Bearer {{name}}`) {
		t.Fatalf("did not expect plain template literal to be flagged")
	}
	if MixedTemplateRTSCall(`contains(response.text(), "{{token}}")`) {
		t.Fatalf("did not expect quoted marker to be flagged")
	}
}

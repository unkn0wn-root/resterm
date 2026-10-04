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

func TestMarkerState(t *testing.T) {
	tests := []struct {
		name string
		ex   string
		want TemplateState
	}{
		{name: "empty"},
		{name: "no marker", ex: `response.json.token`},
		{name: "closed marker", ex: `Bearer {{token}}`, want: TemplateClosed},
		{name: "open marker", ex: `Bearer {{token`, want: TemplateOpen},
		{name: "open expression marker", ex: `{{=`, want: TemplateOpen},
		{name: "open after a closed one", ex: `{{a}} and {{b`, want: TemplateOpen},
		{name: "open across lines", ex: "{{\n  response.status", want: TemplateOpen},
		{name: "closed across lines", ex: "{{\n  response.status\n}}", want: TemplateClosed},
		{name: "marker in a string", ex: `contains(response.text(), "{{token")`},
		{name: "unmatched bracket", ex: `prefix[{{token}}`, want: TemplateClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MarkerState(tt.ex); got != tt.want {
				t.Fatalf("MarkerState(%q) = %d, want %d", tt.ex, got, tt.want)
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
			if got, want := scanner.State(), MarkerState(input); got != want {
				t.Fatalf("chunks [%q, %q] have state %d, whole read gives %d", input[:cut], input[cut:], got, want)
			}
			if got, want := scanner.HasMarker(), HasUnquotedTemplateMarker(input); got != want {
				t.Fatalf("chunks [%q, %q] HasMarker = %t, whole read gives %t", input[:cut], input[cut:], got, want)
			}
		}
	}
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
	if !MixedTemplateRTSCall(`{{a}} contains({{b`) {
		t.Fatalf("expected a call before an open marker to be detected")
	}
}

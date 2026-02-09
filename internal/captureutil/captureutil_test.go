package captureutil

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
	if !HasUnquotedTemplateMarker(`Bearer {{response.json.token}}`) {
		t.Fatalf("expected unquoted marker to be detected")
	}
	if HasUnquotedTemplateMarker(`contains(response.text(), "{{token}}")`) {
		t.Fatalf("expected quoted marker not to be detected")
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
}

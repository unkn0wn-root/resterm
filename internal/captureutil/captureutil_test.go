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

func TestSuspiciousJSONDoubleDotIgnoresQuoted(t *testing.T) {
	if SuspiciousJSONDoubleDot(`contains("response.json..token", "x")`) {
		t.Fatalf("expected quoted content not to trigger suspicious lint")
	}
	if !SuspiciousJSONDoubleDot(`response.json..token`) {
		t.Fatalf("expected direct double-dot path to trigger suspicious lint")
	}
}

func TestIsLegacyTemplate(t *testing.T) {
	if !IsLegacyTemplate(`{{response.json.token}}`) {
		t.Fatalf("expected legacy template syntax to be detected")
	}
	if IsLegacyTemplate(`response.json.token`) {
		t.Fatalf("expected plain RST expression not to be detected as legacy")
	}
}

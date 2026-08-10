package directive

import "testing"

func TestParseScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  Scope
		ok    bool
	}{
		{input: "request", want: ScopeRequest, ok: true},
		{input: " FILE ", want: ScopeFile, ok: true},
		{input: "global", want: ScopeGlobal, ok: true},
		{input: "file-secret"},
		{input: "unsupported"},
		{input: ""},
	}

	for _, tt := range tests {
		got, ok := ParseScope(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("ParseScope(%q) = (%v, %t), want (%v, %t)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseSecretScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input  string
		want   Scope
		secret bool
		ok     bool
	}{
		{input: "file", want: ScopeFile, ok: true},
		{input: "file-secret", want: ScopeFile, secret: true, ok: true},
		{input: " GLOBAL-SECRET ", want: ScopeGlobal, secret: true, ok: true},
		{input: "request-secret", want: ScopeRequest, secret: true, ok: true},
		{input: "unsupported-secret", secret: true},
		{input: "unsupported"},
		{input: ""},
	}

	for _, tt := range tests {
		got, secret, ok := ParseSecretScope(tt.input)
		if got != tt.want || secret != tt.secret || ok != tt.ok {
			t.Fatalf("ParseSecretScope(%q) = (%v, %t, %t), want (%v, %t, %t)",
				tt.input, got, secret, ok, tt.want, tt.secret, tt.ok)
		}
	}
}

// The scope names round-trip because they are also the spelling used in files.
func TestScopeStringRoundTrips(t *testing.T) {
	t.Parallel()

	for _, want := range []Scope{ScopeRequest, ScopeFile, ScopeGlobal} {
		got, ok := ParseScope(want.String())
		if !ok || got != want {
			t.Fatalf("ParseScope(%q) = (%v, %t), want (%v, true)", want.String(), got, ok, want)
		}
	}
}

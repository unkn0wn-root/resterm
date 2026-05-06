package restfile

import "testing"

func TestNormalizeScriptLang(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ScriptLangJS},
		{in: "javascript", want: ScriptLangJS},
		{in: " JS ", want: ScriptLangJS},
		{in: "restermlang", want: ScriptLangRTS},
		{in: "rts", want: ScriptLangRTS},
	}

	for _, tt := range tests {
		if got := NormalizeScriptLang(tt.in); got != tt.want {
			t.Fatalf("NormalizeScriptLang(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsPreRequestScript(t *testing.T) {
	block := ScriptBlock{Kind: "Pre-Request", Lang: "restermlang"}
	if !IsPreRequestScript(block, ScriptLangRTS) {
		t.Fatalf("expected RTS pre-request script")
	}
	if IsPreRequestScript(block, ScriptLangJS) {
		t.Fatalf("did not expect JS pre-request script")
	}
}

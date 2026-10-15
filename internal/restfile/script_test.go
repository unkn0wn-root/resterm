package restfile

import "testing"

func TestScriptBlockKindAndLang(t *testing.T) {
	tests := []struct {
		block     ScriptBlock
		lang      ScriptLang
		pre, test bool
	}{
		{ScriptBlock{Kind: ScriptKindPreRequest, Lang: ScriptLangRTS}, ScriptLangRTS, true, false},
		{ScriptBlock{Kind: ScriptKindPreRequest, Lang: ScriptLangRTS}, ScriptLangJS, false, false},
		{ScriptBlock{Kind: ScriptKindTest, Lang: ScriptLangRTS}, ScriptLangRTS, false, true},
		{ScriptBlock{Kind: ScriptKindTest, Lang: ScriptLangJS}, ScriptLangJS, false, true},
		{ScriptBlock{Kind: ScriptKindTest}, ScriptLangJS, false, true},
		{ScriptBlock{Kind: ScriptKindPreRequest}, ScriptLangJS, true, false},
		{ScriptBlock{Kind: ScriptKindTest}, ScriptLangRTS, false, false},
		{ScriptBlock{Kind: "tests", Lang: ScriptLangJS}, ScriptLangJS, false, false},
		{ScriptBlock{Kind: ScriptKindTest, Lang: "python"}, ScriptLangJS, false, false},
		{ScriptBlock{Lang: ScriptLangJS}, ScriptLangJS, false, false},
	}
	for _, tt := range tests {
		if got := tt.block.Is(ScriptKindPreRequest, tt.lang); got != tt.pre {
			t.Errorf("%+v.Is(pre-request, %q) = %t", tt.block, tt.lang, got)
		}
		if got := tt.block.Is(ScriptKindTest, tt.lang); got != tt.test {
			t.Errorf("%+v.Is(test, %q) = %t", tt.block, tt.lang, got)
		}
	}
}

func TestParseScriptKindAndLang(t *testing.T) {
	kinds := []struct {
		in    string
		want  ScriptKind
		known bool
	}{
		{"test", ScriptKindTest, true},
		{" Tests ", ScriptKindTest, true},
		{"PRE-REQUEST", ScriptKindPreRequest, true},
		{"Check", "check", false},
		{"", "", false},
	}
	for _, tt := range kinds {
		if got, known := ParseScriptKind(tt.in); got != tt.want || known != tt.known {
			t.Errorf("ParseScriptKind(%q) = %q, %t, want %q, %t", tt.in, got, known, tt.want, tt.known)
		}
	}
	langs := []struct {
		in    string
		want  ScriptLang
		known bool
	}{
		{"js", ScriptLangJS, true},
		{"JavaScript", ScriptLangJS, true},
		{" RTS", ScriptLangRTS, true},
		{"restermlang", ScriptLangRTS, true},
		{"Python", "python", false},
		{"", "", false},
	}
	for _, tt := range langs {
		if got, known := ParseScriptLang(tt.in); got != tt.want || known != tt.known {
			t.Errorf("ParseScriptLang(%q) = %q, %t, want %q, %t", tt.in, got, known, tt.want, tt.known)
		}
	}
}

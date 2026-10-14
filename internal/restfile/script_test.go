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
		if got := IsPreRequestScript(tt.block, tt.lang); got != tt.pre {
			t.Errorf("IsPreRequestScript(%+v, %q) = %t", tt.block, tt.lang, got)
		}
		if got := IsTestScript(tt.block, tt.lang); got != tt.test {
			t.Errorf("IsTestScript(%+v, %q) = %t", tt.block, tt.lang, got)
		}
	}
}

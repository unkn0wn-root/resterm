package restfile

import (
	"strings"
)

const (
	ScriptKindPreRequest = "pre-request"
	ScriptLangJS         = "js"
	ScriptLangRTS        = "rts"
)

// NormalizeScriptLang returns the canonical script language token.
func NormalizeScriptLang(lang string) string {
	val := strings.ToLower(strings.TrimSpace(lang))
	switch val {
	case "", "javascript":
		return ScriptLangJS
	case "restermlang":
		return ScriptLangRTS
	default:
		return val
	}
}

// IsPreRequestScript reports whether block is a pre-request script of lang.
func IsPreRequestScript(block ScriptBlock, lang string) bool {
	return strings.EqualFold(
		block.Kind,
		ScriptKindPreRequest,
	) && NormalizeScriptLang(block.Lang) == NormalizeScriptLang(lang)
}

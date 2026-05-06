package restfile

import (
	"slices"
	"strings"
)

const (
	ScriptKindPreRequest = "pre-request"
	ScriptLangJS         = "js"
	ScriptLangRTS        = "rts"
)

// CloneScriptBlocks returns a copy of script block metadata.
func CloneScriptBlocks(src []ScriptBlock) []ScriptBlock {
	if len(src) == 0 {
		return nil
	}
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Lines = slices.Clone(src[i].Lines)
	}
	return dst
}

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

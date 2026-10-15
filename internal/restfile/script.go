package restfile

import "strings"

type (
	ScriptKind string
	ScriptLang string
)

const (
	ScriptKindPreRequest ScriptKind = "pre-request"
	ScriptKindTest       ScriptKind = "test"

	ScriptLangJS  ScriptLang = "js"
	ScriptLangRTS ScriptLang = "rts"
)

func (b ScriptBlock) Is(kind ScriptKind, lang ScriptLang) bool {
	return b.Kind == kind && (b.Lang == lang || b.Lang == "" && lang == ScriptLangJS)
}

func ParseScriptKind(s string) (ScriptKind, bool) {
	switch kind := ScriptKind(strings.ToLower(strings.TrimSpace(s))); kind {
	case ScriptKindTest, "tests":
		return ScriptKindTest, true
	case ScriptKindPreRequest:
		return kind, true
	default:
		return kind, false
	}
}

func ParseScriptLang(s string) (ScriptLang, bool) {
	switch lang := ScriptLang(strings.ToLower(strings.TrimSpace(s))); lang {
	case ScriptLangJS, "javascript":
		return ScriptLangJS, true
	case ScriptLangRTS, "restermlang":
		return ScriptLangRTS, true
	default:
		return lang, false
	}
}

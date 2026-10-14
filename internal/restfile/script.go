package restfile

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

func IsPreRequestScript(block ScriptBlock, lang ScriptLang) bool {
	return block.Kind == ScriptKindPreRequest && block.hasLang(lang)
}

func IsTestScript(block ScriptBlock, lang ScriptLang) bool {
	return block.Kind == ScriptKindTest && block.hasLang(lang)
}

func (b ScriptBlock) hasLang(lang ScriptLang) bool {
	return b.Lang == lang || b.Lang == "" && lang == ScriptLangJS
}

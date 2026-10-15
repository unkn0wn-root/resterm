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

func (b ScriptBlock) Is(kind ScriptKind, lang ScriptLang) bool {
	return b.Kind == kind && (b.Lang == lang || b.Lang == "" && lang == ScriptLangJS)
}

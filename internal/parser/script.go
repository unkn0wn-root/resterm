package parser

import (
	"errors"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type scriptKind string

const (
	scriptKindTest       scriptKind = "test"
	scriptKindTests      scriptKind = "tests"
	scriptKindPreRequest scriptKind = "pre-request"
)

func (k scriptKind) String() string {
	return string(k)
}

type scriptLang string

const (
	scriptLangJS  scriptLang = "js"
	scriptLangRTS scriptLang = "rts"
)

func (l scriptLang) String() string {
	return string(l)
}

const (
	defaultScriptKind = scriptKindTest
	defaultScriptLang = scriptLangJS
)

func normScriptKind(kind string) scriptKind {
	out := str.LowerTrim(kind)
	if out == "" {
		return defaultScriptKind
	}
	return scriptKind(out)
}

func normScriptLang(lang string) scriptLang {
	out := str.LowerTrim(lang)
	switch out {
	case "":
		return defaultScriptLang
	case "javascript":
		return defaultScriptLang
	case "restermlang":
		return scriptLangRTS
	default:
		return scriptLang(out)
	}
}

func (b *documentBuilder) flushScriptIfNeeded(ln line) {
	if b.inRequest && b.request != nil && !ln.hasScriptMarker() {
		b.request.flushPendingScript()
	}
}

func (b *documentBuilder) handleScriptLine(ln line) bool {
	if !ln.hasScriptMarker() {
		return false
	}
	if body, col, ok := ln.cutScriptMarker(); ok {
		b.ensureRequest(ln.no)
		if p, ok := scriptInc(body); ok {
			b.request.appendScriptInclude(b.request.currentScriptKind, b.request.currentScriptLang, p)
		} else {
			b.addScriptLine(ln.no, col, body)
		}
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) addScriptLine(no, col int, body string) {
	b.ensureRequest(no)
	r := b.request
	r.appendScriptLine(
		r.currentScriptKind,
		r.currentScriptLang,
		body,
		b.doc.Path,
		restfile.ScriptLine{Line: no, Col: col},
	)
}

func scriptInc(body string) (string, bool) {
	h := str.TrimLeft(body)
	if !strings.HasPrefix(h, "<") {
		return "", false
	}
	p := strings.TrimSpace(strings.TrimPrefix(h, "<"))
	if p == "" {
		return "", false
	}
	return p, true
}

func (b *documentBuilder) handleScriptBlockStart(ln line) bool {
	if !ln.isScriptBlockStart() {
		return false
	}
	b.ensureRequest(ln.no)
	b.inScriptBlock = true
	b.scriptBlockStartLine = ln.no
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleScriptBlockLine(ln line) bool {
	if ln.isScriptBlockEnd() {
		b.appendLine(ln.raw)
		b.endScriptBlock()
		return true
	}

	if b.handleSeparator(ln) {
		return true
	}

	body, col, ok := ln.cutScriptMarker()
	if !ok {
		body, col = str.TrimRight(ln.raw), 1
	}
	b.addScriptLine(ln.no, col, body)
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) endScriptBlock() {
	if !b.inScriptBlock {
		return
	}
	b.inScriptBlock = false
	b.scriptBlockStartLine = 0
	if b.request != nil {
		b.request.flushPendingScript()
	}
}

func (b *documentBuilder) setScript(rest, lang string) {
	k, l := parseScriptSpec(rest)
	if lang != "" {
		l = normScriptLang(lang)
	}
	b.request.currentScriptKind = k
	b.request.currentScriptLang = l
	b.request.discardScript = false
}

func (b *documentBuilder) setRTSScript(rest string) error {
	k, l, err := parseRTSScriptSpec(rest)
	if err != nil {
		b.request.discardScript = true
		b.request.flushPendingScript()
		return err
	}
	b.request.currentScriptKind = k
	b.request.currentScriptLang = l
	b.request.discardScript = false
	return nil
}

func parseScriptSpec(rest string) (scriptKind, scriptLang) {
	fields := lexer.Fields(rest)
	kind := scriptKind("")
	lang := scriptLang("")
	for _, field := range fields {
		if strings.Contains(field, "=") {
			continue
		}
		if kind == "" {
			kind = scriptKind(field)
			continue
		}
		if lang == "" {
			if v, ok := scriptLangToken(field); ok {
				lang = v
			}
		}
	}
	params := options.ParseFields(fields)
	if v := params["lang"]; v != "" {
		lang = scriptLang(v)
	}
	if v := params["language"]; v != "" && lang == "" {
		lang = scriptLang(v)
	}
	return normScriptKind(kind.String()), normScriptLang(lang.String())
}

func parseRTSScriptSpec(rest string) (scriptKind, scriptLang, error) {
	fields := lexer.Fields(rest)
	var kind scriptKind
	kindSet := false

	for _, field := range fields {
		if strings.Contains(field, "=") {
			continue
		}
		if lang, ok := scriptLangToken(field); ok {
			if lang != scriptLangRTS {
				return "", "", errRTSLangUnsupported()
			}
			continue
		}

		next, err := parseRTSScriptKind(field)
		if err != nil {
			return "", "", err
		}
		if kindSet {
			return "", "", errRTSMultipleModes()
		}
		kind = next
		kindSet = true
	}

	if err := validateRTSScriptLangOptions(fields); err != nil {
		return "", "", err
	}
	if !kindSet {
		return "", "", errRTSModeRequired()
	}

	return kind, scriptLangRTS, nil
}

func parseRTSScriptKind(field string) (scriptKind, error) {
	switch kind := normScriptKind(field); kind {
	case scriptKindPreRequest:
		return kind, nil
	case scriptKindTest, scriptKindTests:
		return "", errRTSTestUnsupported()
	default:
		return "", errRTSModeUnsupported()
	}
}

func validateRTSScriptLangOptions(fields []string) error {
	params := options.ParseFields(fields)
	for _, opt := range []string{"lang", "language"} {
		if val := params[opt]; val != "" && normScriptLang(val) != scriptLangRTS {
			return errRTSLangUnsupported()
		}
	}
	return nil
}

func errRTSTestUnsupported() error {
	return errors.New(
		"@rts test is not supported, use @assert for RTS response checks or @script test for JavaScript tests",
	)
}

func errRTSLangUnsupported() error {
	return errors.New(
		"@rts only supports RestermScript, remove lang=js or use @script for JavaScript",
	)
}

func errRTSModeRequired() error {
	return errors.New("@rts requires a mode, use '@rts pre-request'")
}

func errRTSModeUnsupported() error {
	return errors.New("@rts supports only pre-request mode, use '@rts pre-request'")
}

func errRTSMultipleModes() error {
	return errors.New("@rts accepts only one mode, use '@rts pre-request'")
}

func scriptLangToken(tok string) (scriptLang, bool) {
	out := strings.ToLower(strings.TrimSpace(tok))
	switch out {
	case "js", "javascript":
		return scriptLangJS, true
	case "rts", "restermlang":
		return scriptLangRTS, true
	default:
		return "", false
	}
}

func (r *requestBuilder) appendScriptLine(
	kind scriptKind,
	lang scriptLang,
	body string,
	path string,
	loc restfile.ScriptLine,
) {
	if r.discardScript {
		return
	}
	if r.scriptBufferKind != "" &&
		(r.scriptBufferKind != kind ||
			r.scriptBufferLang != lang ||
			r.scriptSourcePath != path) {
		r.flushPendingScript()
	}
	if r.scriptBufferKind == "" {
		r.scriptBufferKind = kind
		r.scriptBufferLang = lang
		r.scriptSourcePath = path
	}
	r.scriptBuffer = append(r.scriptBuffer, body)
	r.scriptBufferLines = append(r.scriptBufferLines, loc)
}

func (r *requestBuilder) flushPendingScript() {
	if len(r.scriptBuffer) == 0 {
		return
	}
	script := strings.Join(r.scriptBuffer, "\n")
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:       r.scriptBufferKind.String(),
		Lang:       r.scriptBufferLang.String(),
		Body:       script,
		SourcePath: r.scriptSourcePath,
		Lines:      append([]restfile.ScriptLine(nil), r.scriptBufferLines...),
	})
	r.scriptBuffer = nil
	r.scriptBufferKind = ""
	r.scriptBufferLang = ""
	r.scriptSourcePath = ""
	r.scriptBufferLines = nil
}

func (r *requestBuilder) appendScriptInclude(kind scriptKind, lang scriptLang, path string) {
	if r.discardScript {
		return
	}
	r.flushPendingScript()
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:     kind.String(),
		Lang:     lang.String(),
		FilePath: path,
	})
}

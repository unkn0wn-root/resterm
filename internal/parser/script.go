package parser

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/str"
)

func scriptKindToken(tok string) (restfile.ScriptKind, bool) {
	switch kind := restfile.ScriptKind(str.LowerTrim(tok)); kind {
	case restfile.ScriptKindTest, "tests":
		return restfile.ScriptKindTest, true
	case restfile.ScriptKindPreRequest:
		return kind, true
	default:
		return kind, false
	}
}

func scriptLangToken(tok string) (restfile.ScriptLang, bool) {
	switch lang := restfile.ScriptLang(str.LowerTrim(tok)); lang {
	case restfile.ScriptLangJS, "javascript":
		return restfile.ScriptLangJS, true
	case restfile.ScriptLangRTS, "restermlang":
		return restfile.ScriptLangRTS, true
	default:
		return lang, false
	}
}

type scriptFieldError struct {
	msg        string
	start, end int
}

func (e *scriptFieldError) Error() string {
	return e.msg
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
			b.request.appendScriptInclude(p)
		} else {
			b.addScriptLine(ln.no, col, body)
		}
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) addScriptLine(no, col int, body string) {
	b.ensureRequest(no)
	b.request.appendScriptLine(body, b.doc.Path, restfile.ScriptLine{Line: no, Col: col})
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

	body, col := ln.scriptBlockBody()
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

// Shared by the document parser and SourceSyntax so they agree on script settings.
type scriptMode struct {
	kind    restfile.ScriptKind
	lang    restfile.ScriptLang
	name    string
	discard bool
}

var defaultScriptMode = scriptMode{kind: restfile.ScriptKindTest, lang: restfile.ScriptLangJS}

// Keep the parsed settings after an option error so the script body is not lost.
func (m *scriptMode) setScript(args string) error {
	m.discard, m.name = false, ""
	if args == "" {
		return nil
	}
	k, l, err := parseScriptSpec(args)
	m.kind, m.lang = k, l
	return err
}

// Ignore later script lines if @rts is invalid.
func (m *scriptMode) setRTS(args string) error {
	k, name, err := parseRTSScriptSpec(args)
	if err != nil {
		m.discard = true
		return err
	}
	*m = scriptMode{kind: k, lang: restfile.ScriptLangRTS, name: name}
	return nil
}

var scriptLangKeys = []string{"lang", "language"}

func parseScriptSpec(rest string) (restfile.ScriptKind, restfile.ScriptLang, error) {
	fields := slices.Collect(directive.ScanFields(rest))
	var (
		head    directive.Field
		lang    restfile.ScriptLang
		ignored []directive.Field
	)
	for _, field := range fields {
		if !field.Positional() {
			continue
		}
		if head.Value == "" {
			head = field
			continue
		}
		if v, ok := scriptLangToken(field.Value); ok && lang == "" {
			lang = v
			continue
		}
		ignored = append(ignored, field)
	}
	params, err := directive.OptionFields(directive.Script, fields)
	errs := []error{err}

	kind, known := scriptKindToken(head.Value)
	switch {
	case kind == "":
		kind = restfile.ScriptKindTest
	case !known:
		errs = append(errs, &scriptFieldError{
			msg:   fmt.Sprintf("@script kind %q is not supported. Use pre-request or test", head.Value),
			start: head.Start,
			end:   head.End,
		})
	}
	for _, f := range ignored {
		errs = append(errs, &scriptFieldError{
			msg:   fmt.Sprintf("@script ignores %q. Use lang=js or lang=rts to choose a language", f.Value),
			start: f.Start,
			end:   f.End,
		})
	}

	if key, v, ok := params.PopKey(scriptLangKeys...); ok {
		lang, known = scriptLangToken(v)
		if !known {
			// PopKey drops source positions; find the field for the diagnostic.
			i := slices.IndexFunc(fields, func(f directive.Field) bool {
				k, val, _ := strings.Cut(f.Value, "=")
				return f.Op == directive.OpEq && strings.EqualFold(k, key) && strings.TrimSpace(val) == v
			})
			errs = append(errs, &scriptFieldError{
				msg:   fmt.Sprintf("@script language %q is not supported. Use js or rts", v),
				start: fields[i].ValueStart(),
				end:   fields[i].End,
			})
		}
	}
	if lang == "" {
		lang = restfile.ScriptLangJS
	}
	errs = append(errs, params.Conflicts(directive.Script))
	return kind, lang, errors.Join(errs...)
}

func parseRTSScriptSpec(rest string) (restfile.ScriptKind, string, error) {
	fields := slices.Collect(directive.ScanFields(rest))
	var (
		kind restfile.ScriptKind
		name string
	)
	for _, f := range fields {
		if !f.Positional() {
			continue
		}
		quoted := kind != "" && strings.ContainsRune(`"'`, rune(rest[f.Start]))
		if lang, ok := scriptLangToken(f.Value); ok && !quoted {
			if lang != restfile.ScriptLangRTS {
				return "", "", errRTSLangUnsupported
			}
			continue
		}
		next, known := scriptKindToken(f.Value)
		switch {
		case kind == "" && !known:
			return "", "", errRTSModeUnsupported
		case kind == "":
			kind = next
		case known && !quoted:
			return "", "", errRTSMultipleModes
		case kind == restfile.ScriptKindPreRequest:
			return "", "", &scriptFieldError{msg: "@rts pre-request takes no name", start: f.Start, end: f.End}
		case name != "":
			return "", "", &scriptFieldError{
				msg:   "@rts test takes one name. Quote a name with spaces",
				start: f.Start,
				end:   f.End,
			}
		default:
			name = f.Value
		}
	}

	if err := validateRTSScriptLangOptions(fields); err != nil {
		return "", "", err
	}
	if kind == "" {
		return "", "", errRTSModeRequired
	}
	return kind, name, nil
}

func validateRTSScriptLangOptions(fields []directive.Field) error {
	params, err := directive.OptionFields(directive.RTS, fields)
	if err != nil {
		return err
	}
	if val, ok := params.First(scriptLangKeys...); ok {
		if lang, _ := scriptLangToken(val); lang != restfile.ScriptLangRTS {
			return errRTSLangUnsupported
		}
	}
	return params.Conflicts(directive.RTS)
}

var errRTSLangUnsupported = errors.New("@rts only supports RestermScript. Remove lang=js or use @script for JavaScript")

var errRTSModeRequired = errors.New("@rts requires a mode. Use '@rts pre-request' or '@rts test'")

var errRTSModeUnsupported = errors.New(
	"@rts supports only pre-request and test modes. Use '@rts pre-request' or '@rts test'",
)

var errRTSMultipleModes = errors.New("@rts accepts only one mode. Use '@rts pre-request' or '@rts test'")

func (r *requestBuilder) appendScriptLine(body, path string, loc restfile.ScriptLine) {
	if r.script.discard {
		return
	}
	if len(r.scriptBuffer) > 0 && (r.scriptBufferMode != r.script || r.scriptSourcePath != path) {
		r.flushPendingScript()
	}
	if len(r.scriptBuffer) == 0 {
		r.scriptBufferMode = r.script
		r.scriptSourcePath = path
	}
	r.scriptBuffer = append(r.scriptBuffer, body)
	r.scriptBufferLines = append(r.scriptBufferLines, loc)
}

func (r *requestBuilder) flushPendingScript() {
	if len(r.scriptBuffer) == 0 {
		return
	}
	body := strings.Join(r.scriptBuffer, "\n")
	lines := append([]restfile.ScriptLine(nil), r.scriptBufferLines...)
	if s := strings.TrimSpace(body); len(s) >= len("{%%}") && strings.HasPrefix(s, "{%") && strings.HasSuffix(s, "%}") {
		inner := s[2 : len(s)-2]
		off := len(body) - len(str.TrimLeft(body)) + 2 + len(inner) - len(str.TrimLeft(inner))
		cut := body[:off]
		body = strings.TrimSpace(inner)
		skip := strings.Count(cut, "\n")
		lines = lines[skip : skip+strings.Count(body, "\n")+1]
		lines[0].Col += len(cut) - strings.LastIndexByte(cut, '\n') - 1
	}
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:       r.scriptBufferMode.kind,
		Lang:       r.scriptBufferMode.lang,
		Name:       r.scriptBufferMode.name,
		Body:       body,
		SourcePath: r.scriptSourcePath,
		Lines:      lines,
	})
	r.scriptBuffer = nil
	r.scriptBufferMode = scriptMode{}
	r.scriptSourcePath = ""
	r.scriptBufferLines = nil
}

func (r *requestBuilder) appendScriptInclude(path string) {
	if r.script.discard {
		return
	}
	r.flushPendingScript()
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:     r.script.kind,
		Lang:     r.script.lang,
		Name:     r.script.name,
		FilePath: path,
	})
}

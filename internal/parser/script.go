package parser

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type scriptFieldError struct {
	msg        string
	start, end int
}

func (e *scriptFieldError) Error() string {
	return e.msg
}

func fieldErr(f directive.Field, msg string) error {
	return &scriptFieldError{msg: msg, start: f.Start, end: f.End}
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
		} else if code, off, ok := cutInlineBlock(body); ok {
			b.addScriptLine(ln.no, col+off, code)
		} else {
			b.addScriptLine(ln.no, col, body)
		}
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) addScriptLine(no, col int, body string) {
	b.ensureRequest(no)
	b.request.appendScriptLine(body, restfile.ScriptLine{Line: no, Col: col})
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
	for i := 0; i < len(fields); i++ {
		if _, n := directive.SpacedOption(fields, i); n > 0 {
			i += n - 1
			continue
		}
		field := fields[i]
		if !field.Positional() {
			continue
		}
		if head.Value == "" {
			head = field
			continue
		}
		if v, ok := restfile.ParseScriptLang(field.Value); ok && lang == "" {
			lang = v
			continue
		}
		ignored = append(ignored, field)
	}
	params, err := directive.OptionFields(directive.Script, fields)
	errs := []error{err}

	kind, known := restfile.ParseScriptKind(head.Value)
	switch {
	case kind == "":
		kind = restfile.ScriptKindTest
	case !known:
		errs = append(
			errs,
			fieldErr(head, fmt.Sprintf("@script kind %q is not supported. Use pre-request or test", head.Value)),
		)
	}
	for _, f := range ignored {
		errs = append(
			errs,
			fieldErr(f, fmt.Sprintf("@script ignores %q. Use lang=js or lang=rts to choose a language", f.Value)),
		)
	}

	if key, v, ok := params.PopKey(scriptLangKeys...); ok {
		lang, known = restfile.ParseScriptLang(v)
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
	errs = append(errs, params.Leftover(directive.Script))
	return kind, lang, errors.Join(errs...)
}

func parseRTSScriptSpec(rest string) (restfile.ScriptKind, string, error) {
	if closer := directive.FieldsOpen(rest); closer != 0 {
		return "", "", &directive.UnclosedError{Directive: directive.RTS, Closer: string(closer)}
	}
	fields := slices.Collect(directive.ScanFields(rest))
	var (
		kind restfile.ScriptKind
		name string
	)
	for i := 0; i < len(fields); i++ {
		if _, n := directive.SpacedOption(fields, i); n > 0 {
			i += n - 1
			continue
		}
		f := fields[i]
		if !f.Positional() {
			continue
		}
		quoted := kind != "" && strings.ContainsRune(`"'`, rune(rest[f.Start]))
		if lang, ok := restfile.ParseScriptLang(f.Value); ok && !quoted {
			if lang != restfile.ScriptLangRTS {
				return "", "", errRTSLangUnsupported
			}
			continue
		}
		next, known := restfile.ParseScriptKind(f.Value)
		switch {
		case kind == "" && !known:
			return "", "", errRTSModeUnsupported
		case kind == "":
			kind = next
		case known && !quoted:
			return "", "", errRTSMultipleModes
		case kind == restfile.ScriptKindPreRequest:
			return "", "", fieldErr(f, "@rts pre-request takes no name. Remove it")
		case name != "":
			return "", "", fieldErr(f, "@rts test takes one name. Quote a name with spaces")
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
	if val, ok := params.PopAny(scriptLangKeys...); ok {
		if lang, _ := restfile.ParseScriptLang(val); lang != restfile.ScriptLangRTS {
			return errRTSLangUnsupported
		}
	}
	return params.Leftover(directive.RTS)
}

var errRTSLangUnsupported = errors.New("@rts only supports RestermScript. Remove lang=js or use @script for JavaScript")

var errRTSModeRequired = errors.New("@rts requires a mode. Use '@rts pre-request' or '@rts test'")

var errRTSModeUnsupported = errors.New(
	"@rts supports only pre-request and test modes. Use '@rts pre-request' or '@rts test'",
)

var errRTSMultipleModes = errors.New("@rts accepts only one mode. Use '@rts pre-request' or '@rts test'")

func (r *requestBuilder) appendScriptLine(body string, loc restfile.ScriptLine) {
	if r.script.discard {
		return
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
		Kind:       r.script.kind,
		Lang:       r.script.lang,
		Name:       r.script.name,
		Body:       script,
		SourcePath: r.sourcePath,
		Lines:      append([]restfile.ScriptLine(nil), r.scriptBufferLines...),
	})
	r.scriptBuffer = nil
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

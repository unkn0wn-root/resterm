package parser

import (
	"slices"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (b *documentBuilder) warnUnclosed(text string, start diag.Pos) {
	for _, u := range vars.UnclosedPlaceholders(text, start) {
		b.pushWarning(restfile.ParseDiagnostic{Message: unclosedMessage(u), Span: u.Span})
	}
}

// Scan the full body because placeholders can span lines.
// Skip warnings for bodies whose builders do not record source lines.
func (b *documentBuilder) warnUnclosedBody(req *restfile.Request) {
	for _, u := range vars.UnclosedPlaceholdersLocated(req.Body.Text, req.LocateBody) {
		if u.Span.Start.Line <= 0 {
			continue
		}
		b.pushWarning(restfile.ParseDiagnostic{Message: unclosedMessage(u), Span: u.Span})
	}
}

func (b *documentBuilder) warnUnclosedArgs(d parsedDirective) {
	for _, u := range d.unclosedPlaceholders() {
		item := restfile.ParseDiagnostic{Message: unclosedMessage(u), Span: d.nameSpan}
		if span, ok := d.argumentSpan(u.Off, u.Off+len(u.Text)); ok {
			item.Span = span
		}
		b.pushWarning(item)
	}
}

func (d parsedDirective) unclosedPlaceholders() []vars.Unclosed {
	args := d.Args
	if d.scriptArgs() {
		args = rts.MaskText(args)
	}
	return vars.UnclosedPlaceholders(args, diag.Pos{})
}

// Template captures expand placeholders even inside quotes.
func (d parsedDirective) scriptArgs() bool {
	if d.Name == directive.Capture {
		_, _, expr := cutCapture(d.Args)
		return captureMode(expr) == restfile.CaptureExprModeRTS
	}
	return d.Name.ScriptArgs()
}

func unclosedMessage(u vars.Unclosed) string {
	return "placeholder " + u.Text + " is not closed with }}"
}

func Diagnostics(doc *restfile.Document) diag.Report {
	rep := diag.Report{Path: doc.Path, Source: doc.Raw}
	lines := strings.Split(string(doc.Raw), "\n")

	for _, item := range doc.Errors {
		rep.Items = append(rep.Items, reportItem(item, diag.SeverityError, doc.Path, lines))
	}

	for _, item := range doc.Warnings {
		rep.Items = append(rep.Items, reportItem(item, diag.SeverityWarning, doc.Path, lines))
	}
	return rep
}

func reportItem(item restfile.ParseDiagnostic, severity diag.Severity, path string, lines []string) diag.Diagnostic {
	span := item.Span
	if span.Start.Col == 0 {
		span = lineSpan(lines, span.Start.Line)
	}

	labels := slices.Clone(item.Labels)
	for i := range labels {
		labels[i].Span = withPath(labels[i].Span, path)
	}

	return diag.Diagnostic{
		Class: diag.ClassParse, Component: diag.ComponentParser, Severity: severity,
		Message: item.Message, Span: withPath(span, path), Labels: labels,
	}
}

func withPath(span diag.Span, path string) diag.Span {
	span.Start.Path, span.End.Path = path, path
	return span
}

func lineSpan(lines []string, line int) diag.Span {
	span := diag.Span{Start: diag.Pos{Line: line, Col: 1}}
	span.End = span.Start

	if line <= 0 || line > len(lines) {
		return span
	}

	raw := strings.TrimSuffix(lines[line-1], "\r")
	start := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
	end := len(strings.TrimRightFunc(raw, unicode.IsSpace))
	span.Start.Col = start + 1
	span.End.Col = max(start, end) + 1
	return span
}

// Maps argument bytes to their source line. Padding between continued lines has no source.
type argumentPart struct {
	start, end int
	pos        diag.Pos
}

func (d parsedDirective) argumentSpan(start, end int) (diag.Span, bool) {
	for _, part := range d.argParts {
		if start >= part.start && end <= part.end {
			pos := part.pos
			pos.Col += start - part.start
			last := pos
			last.Col += end - start
			return diag.Span{Start: pos, End: last}, true
		}
	}
	return diag.Span{}, false
}

func (d parsedDirective) diagnostic(msg string, cause error) restfile.ParseDiagnostic {
	item := restfile.ParseDiagnostic{Message: msg}
	item.Span, item.Labels = d.locate(cause)
	return item
}

// argFields caches option spans across diagnostics and rebuilds when
// continuation lines change the arguments.
type argFields struct {
	args  string
	parts int
	spans []diag.Span
	byKey map[string][]int
}

func (f *argFields) load(d parsedDirective) {
	if f.byKey != nil && f.args == d.Args && f.parts == len(d.argParts) {
		return
	}
	f.args, f.parts = d.Args, len(d.argParts)
	f.spans, f.byKey = nil, make(map[string][]int)
	for _, field := range directive.FieldSpans(d.Args) {
		end := field.Eq
		if end < 0 {
			end = field.End // ParseOptions also accepts bare switches.
		}

		span, ok := d.argumentSpan(field.Start, end)
		if !ok {
			continue
		}

		key := strings.ToLower(d.Args[field.Start:end])
		f.byKey[key] = append(f.byKey[key], len(f.spans))
		f.spans = append(f.spans, span)
	}
}

// Fall back to the directive name unless every option key maps to source text;
// custom grammars may report normalized names that do not appear in the source.
func (d parsedDirective) locate(cause error) (diag.Span, []diag.Label) {
	keys := directive.OptionKeys(cause)
	if len(keys) == 0 {
		return d.nameSpan, nil
	}

	f := d.fields
	if f == nil {
		f = &argFields{}
	}

	f.load(d)
	var at []int
	for _, key := range keys {
		found := f.byKey[key]
		if len(found) == 0 {
			return d.nameSpan, nil
		}
		at = append(at, found...)
	}

	// Use the first option in source order as the primary span.
	slices.Sort(at)
	var labels []diag.Label
	for _, i := range at[1:] {
		labels = append(labels, diag.Label{Span: f.spans[i]})
	}
	return f.spans[at[0]], labels
}

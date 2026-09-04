package parser

import (
	"slices"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Diagnostics returns editor annotations. Check and WarningTexts still report
// the opening line of continued directives.
func Diagnostics(doc *restfile.Document) diag.Report {
	rep := diag.Report{Path: doc.Path, Source: slices.Clone(doc.Raw)}
	lines := strings.Split(string(doc.Raw), "\n")
	add := func(items []restfile.ParseDiagnostic, severity diag.Severity) {
		for _, item := range items {
			span := item.Span
			if span.Start.Line == 0 {
				span = lineSpan(lines, item.Line)
			}
			labels := slices.Clone(item.Labels)
			for i := range labels {
				labels[i].Span = withPath(labels[i].Span, doc.Path)
			}
			rep.Items = append(rep.Items, diag.Diagnostic{
				Class: diag.ClassParse, Component: diag.ComponentParser, Severity: severity,
				Message: item.Message, Span: withPath(span, doc.Path), Labels: labels,
			})
		}
	}
	add(doc.Errors, diag.SeverityError)
	add(doc.Warnings, diag.SeverityWarning)
	return rep
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

// argumentPart maps argument bytes back to a physical source line. Separators
// and padding between continued lines have no source.
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
	item := restfile.ParseDiagnostic{Line: d.lines.Start, Message: msg}
	item.Span, item.Labels = d.spans(cause)
	return item
}

// Fall back to the directive name unless every option key maps to source text;
// custom grammars may report normalized names that do not appear in the source.
func (d parsedDirective) spans(cause error) (diag.Span, []diag.Label) {
	keys := directive.OptionKeys(cause)
	if len(keys) == 0 {
		return d.nameSpan, nil
	}
	var spans []diag.Span
	found := make(map[string]bool, len(keys))
	for _, field := range directive.FieldSpans(d.Args) {
		end := field.Eq
		if end < 0 {
			end = field.End // ParseOptions also accepts bare switches.
		}
		key := strings.ToLower(d.Args[field.Start:end])
		if !slices.Contains(keys, key) {
			continue
		}
		if span, ok := d.argumentSpan(field.Start, end); ok {
			spans = append(spans, span)
			found[key] = true
		}
	}
	if len(found) != len(keys) {
		return d.nameSpan, nil
	}
	var labels []diag.Label
	for _, span := range spans[1:] {
		labels = append(labels, diag.Label{Span: span})
	}
	return spans[0], labels
}

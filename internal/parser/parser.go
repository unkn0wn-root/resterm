package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/eol"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// eol.ScanLines retains line endings, so reserve CRLF separately from the
// one-megabyte content limit.
const (
	maxLine      = 1024 * 1024
	maxScanToken = maxLine + len(eol.CRLF)
)

func Parse(path string, data []byte) *restfile.Document {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), maxScanToken)
	scanner.Split(eol.ScanLines)

	doc := &restfile.Document{Path: path, Raw: data}
	builder := &documentBuilder{doc: doc}

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text, term := eol.Cut(scanner.Text())
		builder.processLine(lineNumber, text, term)
	}

	if err := scanner.Err(); err != nil {
		if builder.mock != nil {
			builder.mock.endLine = lineNumber + 1
		}
		msg := fmt.Sprintf("parse error: %v", err)
		if errors.Is(err, bufio.ErrTooLong) {
			msg = fmt.Sprintf("parse error: line exceeds %d bytes", maxLine)
		}
		builder.addError(lineNumber+1, msg)
	}

	builder.finish()

	return doc
}

// WarningTexts renders each parse warning with its source location. Warnings are
// not fatal, so callers report them beside their results instead of failing.
func WarningTexts(doc *restfile.Document) []string {
	if doc == nil || len(doc.Warnings) == 0 {
		return nil
	}
	out := make([]string, 0, len(doc.Warnings))
	for _, warn := range doc.Warnings {
		line := warn.Span.Start.Line
		if doc.Path == "" {
			out = append(out, fmt.Sprintf("line %d: %s", line, warn.Message))
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d: %s", doc.Path, line, warn.Message))
	}
	return out
}

func Check(doc *restfile.Document) error {
	if doc == nil || len(doc.Errors) == 0 {
		return nil
	}

	rep := Diagnostics(doc)
	rep.Items = slices.DeleteFunc(rep.Items, func(d diag.Diagnostic) bool { return d.Severity != diag.SeverityError })
	note := diag.Note{Kind: diag.NoteInfo, Message: parseNote(len(rep.Items))}
	for i := range rep.Items {
		rep.Items[i].Notes = []diag.Note{note}
	}
	return diag.FromReport(rep, nil)
}

func parseNote(n int) string {
	if n > 1 {
		return fmt.Sprintf("Fix these %d request file parse errors before running.", n)
	}
	return "Fix the request file parse error before running."
}

package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const maxScanToken = 1024 * 1024

func Parse(path string, data []byte) *restfile.Document {
	src := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 0, 1024), maxScanToken)

	doc := &restfile.Document{Path: path, Raw: data}
	builder := &documentBuilder{doc: doc}

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		builder.processLine(lineNumber, line)
	}

	if err := scanner.Err(); err != nil {
		if builder.mock != nil {
			builder.mock.endLine = lineNumber + 1
		}
		msg := fmt.Sprintf("parse error: %v", err)
		if errors.Is(err, bufio.ErrTooLong) {
			msg = fmt.Sprintf("parse error: line exceeds %d bytes", maxScanToken)
		}
		builder.addError(lineNumber+1, msg)
	}

	builder.finish()

	return doc
}

func Check(doc *restfile.Document) error {
	if doc == nil || len(doc.Errors) == 0 {
		return nil
	}

	items := make([]diag.Diagnostic, 0, len(doc.Errors))
	for _, e := range doc.Errors {
		msg := e.Message
		if msg == "" {
			msg = "invalid request file"
		}
		items = append(items, diag.Diagnostic{
			Class:    diag.ClassParse,
			Severity: diag.SeverityError,
			Message:  msg,
			Span: diag.Span{
				Start: diag.Pos{
					Path: doc.Path,
					Line: e.Line,
					Col:  e.Column,
				},
			},
			Notes: []diag.Note{{
				Kind:    diag.NoteInfo,
				Message: parseNote(len(doc.Errors)),
			}},
		})
	}
	return diag.FromReport(diag.Report{
		Path:   doc.Path,
		Source: doc.Raw,
		Items:  items,
	}, nil)
}

func parseNote(n int) string {
	if n > 1 {
		return fmt.Sprintf("Fix these %d request file parse errors before running.", n)
	}
	return "Fix the request file parse error before running."
}

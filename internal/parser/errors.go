package parser

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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

package diag_test

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestRenderCaretFollowsEscapedCharacters(t *testing.T) {
	for _, src := range []string{
		"const soft\u00ad = 1;",
		"const bell \x07 x = 1;",
		"const tab\tbed = 1;",
		"const plain = 1;",
	} {
		col := strings.Index(src, "= 1;") + 1
		assertCaretUnder(t, diag.Report{
			Path:   "sample.js",
			Source: []byte(src + "\n"),
			Items: []diag.Diagnostic{{
				Class:    diag.ClassScript,
				Severity: diag.SeverityError,
				Message:  "bad assignment",
				Span: diag.Span{
					Start: diag.Pos{Line: 1, Col: col},
					End:   diag.Pos{Line: 1, Col: col},
				},
			}},
		}, "= 1;")
	}
}

func TestRenderEscapesUntrustedText(t *testing.T) {
	rep := diag.Report{
		Path:   "sample.js",
		Source: []byte("const a = 1;\n"),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassScript,
			Severity: diag.SeverityError,
			Message:  "cursor \x1b[2J moved",
			Span: diag.Span{
				Start: diag.Pos{Line: 1, Col: 7},
				End:   diag.Pos{Line: 1, Col: 7},
				Label: "here soft\u00ad",
			},
			Notes:  []diag.Note{{Kind: diag.NoteHelp, Message: "note \x1b[2J here"}},
			Frames: []diag.StackFrame{{Name: "fn soft\u00ad"}},
		}},
	}
	for _, l := range diag.Lines(rep) {
		if strings.ContainsAny(l.Text, "\x1b\u00ad") {
			t.Errorf("%v line kept a terminal control: %q", l.Kind, l.Text)
		}
	}
}

func TestRenderExcerptExpandsTabIndentation(t *testing.T) {
	const src = "\tconst value = missing;"
	rep := diag.Report{
		Path:   "sample.js",
		Source: []byte(src + "\n"),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassScript,
			Severity: diag.SeverityError,
			Message:  "undefined variable",
			Span: diag.Span{
				Start: diag.Pos{Line: 1, Col: strings.Index(src, "missing") + 1},
				End:   diag.Pos{Line: 1, Col: strings.Index(src, "missing") + 1},
			},
		}},
	}
	for _, l := range diag.Lines(rep) {
		if l.Kind == diag.LineSrc && strings.Contains(l.Text, `\t`) {
			t.Fatalf("excerpt shows a tab escape instead of indentation: %q", l.Text)
		}
	}
	assertCaretUnder(t, rep, "missing")
}

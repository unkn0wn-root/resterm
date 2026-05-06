package diag

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type LineKind string

const (
	LineBlank LineKind = "blank"
	LineHead  LineKind = "head"
	LineLoc   LineKind = "loc"
	LineBar   LineKind = "bar"
	LineSrc   LineKind = "src"
	LineMark  LineKind = "mark"
	LineChain LineKind = "chain"
	LineNote  LineKind = "note"
	LineHelp  LineKind = "help"
	LineStack LineKind = "stack"
)

type Line struct {
	Kind LineKind
	Text string
}

func Render(err error) string {
	return RenderReport(ReportOf(err))
}

func RenderReport(rep Report) string {
	return text(Lines(rep))
}

func Write(w io.Writer, err error) error {
	if w == nil {
		return nil
	}
	out := Render(err)
	if out == "" {
		return nil
	}
	_, err = fmt.Fprintln(w, out)
	return err
}

func Lines(rep Report) []Line {
	rep = prepareReport(rep)
	if len(rep.Items) == 0 {
		return nil
	}
	var out []Line
	for i, it := range rep.Items {
		if i > 0 {
			out = append(out, Line{Kind: LineBlank})
		}
		out = append(out, itemLines(rep, it)...)
	}
	return out
}

func itemLines(rep Report, it Diagnostic) []Line {
	sev := severityOrError(it.Severity)
	msg := it.Message
	if msg == "" {
		msg = "operation failed"
	}
	class := classOrUnknown(it.Class)
	head := string(sev) + "[" + string(class) + "]"
	ls := []Line{{Kind: LineHead, Text: head + ": " + msg}}

	if loc := it.Span.Start.String(); loc != "" {
		ls = append(ls, Line{Kind: LineLoc, Text: "--> " + loc})
	}
	if src, ok := lineText(sourceFor(rep, it), it.Span.Start.Line); ok {
		width := len(strconv.Itoa(it.Span.Start.Line))
		if width < 4 {
			width = 4
		}
		bar := strings.Repeat(" ", width) + " |"
		col := it.Span.Start.Col
		if col <= 0 {
			col = 1
		}
		ls = append(
			ls,
			Line{Kind: LineBar, Text: bar},
			Line{Kind: LineSrc, Text: fmt.Sprintf("%*d | %s", width, it.Span.Start.Line, src)},
			Line{
				Kind: LineMark,
				Text: strings.Repeat(" ", width+3+col-1) + "^" + label(it.Span.Label),
			},
		)
	}
	if len(it.Chain) > 0 {
		ls = append(ls, chainLines(it.Chain)...)
	}
	for _, note := range it.Notes {
		ls = append(ls, noteLine(note))
	}
	if len(it.Frames) > 0 {
		ls = append(ls, Line{Kind: LineStack, Text: "Stack:"})
		for _, f := range it.Frames {
			name := f.Name
			if name == "" {
				name = "<fn>"
			}
			if pos := f.Pos.String(); pos != "" {
				ls = append(
					ls,
					Line{Kind: LineStack, Text: fmt.Sprintf("  at %s in %s", pos, name)},
				)
				continue
			}
			ls = append(ls, Line{Kind: LineStack, Text: "  at " + name})
		}
	}
	return ls
}

func noteLine(note Note) Line {
	switch noteKindOrInfo(note.Kind) {
	case NoteHelp, NoteSuggestion:
		return Line{Kind: LineHelp, Text: string(note.Kind) + ": " + note.Message}
	case NoteWarning:
		return Line{Kind: LineNote, Text: "warning: " + note.Message}
	default:
		return Line{Kind: LineNote, Text: "note: " + note.Message}
	}
}

func chainLines(entries []ChainEntry) []Line {
	entries = prepareChain(entries)
	if len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 {
		root := entries[0]
		out := []Line{{Kind: LineChain, Text: root.Message}}
		appendTreeChainLines(&out, "", root.Children)
		return out
	}
	var out []Line
	appendTreeChainLines(&out, "", entries)
	return out
}

func appendTreeChainLines(out *[]Line, prefix string, entries []ChainEntry) {
	for i, entry := range entries {
		last := i == len(entries)-1
		connector := treeChainConnector(i, len(entries))
		*out = append(*out, Line{
			Kind: LineChain,
			Text: prefix + connector + " " + entry.Message,
		})
		if len(entry.Children) == 0 {
			continue
		}
		appendTreeChainLines(out, prefix+treeChildPrefix(last), entry.Children)
	}
}

func treeChainConnector(i, n int) string {
	switch {
	case n <= 1:
		return "╰─>"
	case i == n-1:
		return "╰─>"
	default:
		return "├─>"
	}
}

func treeChildPrefix(last bool) string {
	if last {
		return "    "
	}
	return "│   "
}

func text(ls []Line) string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func sourceFor(rep Report, it Diagnostic) []byte {
	if len(it.Source) > 0 {
		return it.Source
	}
	return rep.Source
}

func lineText(src []byte, line int) (string, bool) {
	if line <= 0 || len(src) == 0 {
		return "", false
	}
	data := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if line > len(lines) {
		return "", false
	}
	return lines[line-1], true
}

func label(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

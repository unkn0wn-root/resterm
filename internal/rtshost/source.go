package rtshost

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

// source is RTS script text plus the metadata diagnostics point back at.
type source struct {
	Text string
	Path string
	Raw  []byte
	Pos  rts.Pos
}

// loadSource returns RTS source text and diagnostic metadata for a script block.
func loadSource(doc *restfile.Document, block restfile.ScriptBlock, base string) (source, error) {
	if block.FilePath == "" {
		return inlineSource(doc, block), nil
	}
	path := block.FilePath
	if !filepath.IsAbs(path) && base != "" {
		path = filepath.Join(base, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return source{}, err
	}
	return source{
		Text: string(data),
		Path: path,
		Raw:  data,
		Pos:  rts.Pos{Path: path, Line: 1, Col: 1},
	}, nil
}

// annotate attaches the script source to err so diagnostics can render the offending line.
func (s source) annotate(err error) error {
	if err == nil {
		return nil
	}
	if s.Path == "" && len(s.Raw) == 0 {
		return err
	}
	// Empty operation keeps the original diagnostic message and adds no chain entry.
	return diag.Wrap(err, "", diag.WithSource(s.Path, s.Raw))
}

func inlineSource(doc *restfile.Document, block restfile.ScriptBlock) source {
	path := block.SourcePath
	var raw []byte
	if doc != nil {
		if path == "" {
			path = doc.Path
		}
		if path == doc.Path {
			raw = doc.Raw
		}
	}
	pos := rts.Pos{Path: path, Line: 1, Col: 1}
	if len(block.Lines) > 0 && block.Lines[0].Line > 0 {
		pos.Line = block.Lines[0].Line
	}
	// Keep Col at 1: bodySource pads each inline line to its source column.
	// Setting Pos.Col from block.Lines would double-count the first line offset.
	return source{
		Text: bodySource(block.Body, block.Lines),
		Path: path,
		Raw:  raw,
		Pos:  pos,
	}
}

// bodySource rebuilds the block body at its document coordinates so RTS
// positions point at the .http file.
func bodySource(body string, lines []restfile.ScriptLine) string {
	if len(lines) == 0 {
		return body
	}

	var b strings.Builder
	line := 1
	if lines[0].Line > 0 {
		line = lines[0].Line
	}
	for i, part := range strings.Split(body, "\n") {
		if i > 0 {
			b.WriteByte('\n')
			line++
		}
		// Lines without metadata or with stale metadata behind the cursor stay verbatim.
		if i >= len(lines) || lines[i].Line < line {
			b.WriteString(part)
			continue
		}
		loc := lines[i]
		for line < loc.Line {
			b.WriteByte('\n')
			line++
		}
		if loc.Col > 1 {
			b.WriteString(strings.Repeat(" ", loc.Col-1))
		}
		b.WriteString(part)
	}
	return b.String()
}

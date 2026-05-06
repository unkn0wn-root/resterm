package rtssrc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

// Source is RTS script text plus source metadata for diagnostics.
type Source struct {
	Text string
	Path string
	Raw  []byte
	Pos  rts.Pos
}

// Load returns RTS source text and diagnostic metadata for a script block.
func Load(doc *restfile.Document, block restfile.ScriptBlock, base string) (Source, error) {
	if block.FilePath == "" {
		return inline(doc, block), nil
	}
	path := block.FilePath
	if !filepath.IsAbs(path) && base != "" {
		path = filepath.Join(base, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	return Source{
		Text: string(data),
		Path: path,
		Raw:  data,
		Pos:  rts.Pos{Path: path, Line: 1, Col: 1},
	}, nil
}

// Annotate attaches source metadata to err when available.
func Annotate(err error, src Source) error {
	if err == nil {
		return nil
	}
	if src.Path == "" && len(src.Raw) == 0 {
		return err
	}
	// Empty operation keeps the original diagnostic message and adds no chain entry.
	return diag.Wrap(err, "", diag.WithSource(src.Path, src.Raw))
}

func inline(doc *restfile.Document, block restfile.ScriptBlock) Source {
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
	return Source{
		Text: bodySource(block.Body, block.Lines),
		Path: path,
		Raw:  raw,
		Pos:  pos,
	}
}

func bodySource(body string, lines []restfile.ScriptLine) string {
	if len(lines) == 0 {
		return body
	}

	parts := strings.Split(body, "\n")
	var b strings.Builder
	line := 1
	if lines[0].Line > 0 {
		line = lines[0].Line
	}
	for i, part := range parts {
		if i > 0 {
			b.WriteByte('\n')
			line++
		}
		// Extra body lines have no source metadata
		if i >= len(lines) {
			b.WriteString(part)
			continue
		}
		loc := lines[i]
		if loc.Line < line {
			b.WriteString(part)
			continue
		}
		for line < loc.Line {
			b.WriteByte('\n')
			line++
		}
		if col := loc.Col; col > 1 {
			b.WriteString(strings.Repeat(" ", col-1))
		}
		b.WriteString(part)
	}
	return b.String()
}

package httpx

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/eol"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func newFileLookup(baseDir string, opts Options) filelookup.Lookup {
	return filelookup.For(baseDir, opts.FallbackBaseDirs, opts.NoFallback)
}

func (c *Client) readFile(lookup filelookup.Lookup, path, label string) ([]byte, string, error) {
	if c == nil || c.fs == nil {
		return nil, "", diag.New(diag.ClassFilesystem, "file reader unavailable")
	}

	if path == "" {
		return nil, "", diag.Newf(
			diag.ClassFilesystem,
			"%s path is empty",
			strings.ToLower(label),
		)
	}

	data, tried, err := lookup.Read(c.fs, path)
	if err == nil {
		return data, tried, nil
	}

	if filepath.IsAbs(path) || filelookup.Fatal(err) {
		return nil, "", diag.WrapAsf(
			diag.ClassFilesystem, err,
			"read %s %s",
			strings.ToLower(label),
			tried,
		)
	}
	return nil, "", diag.WrapAsf(
		diag.ClassFilesystem, err,
		"read %s %s (last tried %s)",
		strings.ToLower(label),
		path,
		tried,
	)
}

// bodyExpand expands a run of body text that starts on the given body line.
type bodyExpand func(text string, line int) (string, error)

type bodyPart struct {
	text string
	path string
	term string
}

// injectBodyIncludes expands body and replaces its "@path" lines with file
// contents. "@{...}" templates are not includes. Include lines are found in
// body before expansion, so an expanded value cannot add one. A nil expand
// leaves text as it is. Other bodies keep their original line endings.
// Multipart bodies use CRLF throughout and end with CRLF.
func (c *Client) injectBodyIncludes(
	body string,
	expand bodyExpand,
	lookup filelookup.Lookup,
	crlf bool,
) ([]byte, error) {
	if expand == nil {
		expand = func(text string, _ int) (string, error) { return text, nil }
	}
	phs := vars.Placeholders(body)
	var parts []bodyPart
	var firstErr error
	add := func(text string, line int) {
		out, err := expand(text, line)
		if err != nil {
			firstErr = vars.PreferStructural(firstErr, err)
			return
		}
		parts = append(parts, bodyPart{text: out})
	}

	off, start, n, runLine := 0, 0, 1, 1
	for line, term := range eol.Lines(body) {
		// A line inside a placeholder that spans lines is part of the placeholder.
		for len(phs) > 0 && phs[0][1] <= off {
			phs = phs[1:]
		}
		inside := len(phs) > 0 && phs[0][0] < off
		if _, ok := bodyref.IncludeLine(line); ok && !inside {
			if start < off {
				add(body[start:off], runLine)
			}
			out, err := expand(line, n)
			if err != nil {
				firstErr = vars.PreferStructural(firstErr, err)
			} else if path, ok := bodyref.IncludeLine(out); ok {
				parts = append(parts, bodyPart{path: path, term: term})
			} else {
				parts = append(parts, bodyPart{text: out + term})
			}
			start, runLine = off+len(line)+len(term), n+1
		}
		off += len(line) + len(term)
		n++
	}
	if start < len(body) {
		add(body[start:], runLine)
	}
	if firstErr != nil {
		return nil, firstErr
	}

	var b bytes.Buffer
	b.Grow(len(body))
	for _, p := range parts {
		if p.path == "" {
			writeBodyText(&b, p.text, crlf)
			continue
		}
		data, _, err := c.readFile(lookup, p.path, "include body file")
		if err != nil {
			return nil, err
		}
		b.Write(data)
		if crlf {
			p.term = eol.CRLF
		}
		b.WriteString(p.term)
	}
	if crlf && !bytes.HasSuffix(b.Bytes(), []byte(eol.CRLF)) {
		b.WriteString(eol.CRLF)
	}
	return b.Bytes(), nil
}

func writeBodyText(b *bytes.Buffer, text string, crlf bool) {
	if !crlf {
		b.WriteString(text)
		return
	}
	for line := range eol.Lines(text) {
		b.WriteString(line)
		b.WriteString(eol.CRLF)
	}
}

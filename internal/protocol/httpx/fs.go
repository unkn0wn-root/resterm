package httpx

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/eol"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
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

// injectBodyIncludes replaces "@path" lines with file contents and leaves
// "@{...}" templates unchanged. Other bodies keep their original line endings.
// Multipart bodies use CRLF throughout and end with CRLF.
func (c *Client) injectBodyIncludes(body string, lookup filelookup.Lookup, crlf bool) ([]byte, error) {
	var b bytes.Buffer
	b.Grow(len(body))
	for line, term := range eol.Lines(body) {
		if path, ok := bodyref.IncludeLine(line); ok {
			data, _, err := c.readFile(lookup, path, "include body file")
			if err != nil {
				return nil, err
			}
			b.Write(data)
		} else {
			b.WriteString(line)
		}
		if crlf {
			term = eol.CRLF
		}
		b.WriteString(term)
	}
	if crlf && !bytes.HasSuffix(b.Bytes(), []byte(eol.CRLF)) {
		b.WriteString(eol.CRLF)
	}
	return b.Bytes(), nil
}

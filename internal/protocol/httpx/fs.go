package httpx

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
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

// injectBodyIncludes replaces each "@path" line with the referenced file's
// bytes. "@{...}" template lines are left alone. crlf rejoins lines with CRLF
// and always terminates the body with CRLF, like curl -F: readline-based
// multipart parsers (e.g. Python's cgi) block without it.
func (c *Client) injectBodyIncludes(body string, lookup filelookup.Lookup, crlf bool) ([]byte, error) {
	eol := "\n"
	if crlf {
		eol = "\r\n"
	}

	var b bytes.Buffer
	b.Grow(len(body))
	for i, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if i > 0 {
			b.WriteString(eol)
		}
		line = strings.TrimSuffix(line, "\r")
		if path, ok := bodyref.IncludeLine(line); ok {
			data, _, err := c.readFile(lookup, path, "include body file")
			if err != nil {
				return nil, err
			}
			b.Write(data)
			continue
		}
		b.WriteString(line)
	}
	if crlf {
		b.WriteString(eol)
	}
	return b.Bytes(), nil
}

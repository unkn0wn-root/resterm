package httpclient

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

type fileLookup struct {
	baseDir   string
	fallbacks []string
	allowRaw  bool
}

func newFileLookup(baseDir string, opts Options) fileLookup {
	fallbacks, allowRaw := resolveFileLookup(baseDir, opts)
	return fileLookup{baseDir: baseDir, fallbacks: fallbacks, allowRaw: allowRaw}
}

func (lookup fileLookup) read(c *Client, path, label string) ([]byte, string, error) {
	return c.readFileWithFallback(path, lookup.baseDir, lookup.fallbacks, lookup.allowRaw, label)
}

func (c *Client) readFileWithFallback(
	path string,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
	label string,
) ([]byte, string, error) {
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

	if filepath.IsAbs(path) {
		data, err := c.fs.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		return nil, "", diag.WrapAsf(
			diag.ClassFilesystem, err,
			"read %s %s",
			strings.ToLower(label),
			path,
		)
	}

	candidates := buildPathCandidates(path, baseDir, fallbacks, allowRaw)

	var lastErr error
	var lastPath string
	for _, candidate := range candidates {
		data, err := c.fs.ReadFile(candidate)
		if err == nil {
			return data, candidate, nil
		}
		if stopReadFallback(err) {
			return nil, "", diag.WrapAsf(
				diag.ClassFilesystem, err,
				"read %s %s",
				strings.ToLower(label),
				candidate,
			)
		}
		lastErr = err
		lastPath = candidate
	}

	if lastErr == nil {
		lastErr = os.ErrNotExist
		lastPath = path
	}
	return nil, "", diag.WrapAsf(
		diag.ClassFilesystem, lastErr,
		"read %s %s (last tried %s)",
		strings.ToLower(label),
		path,
		lastPath,
	)
}

// injectBodyIncludes replaces each "@path" line with the referenced file's
// bytes. "@{...}" template lines are left alone. crlf rejoins lines with CRLF
// and always terminates the body with CRLF, like curl -F: readline-based
// multipart parsers (e.g. Python's cgi) block without it.
func (c *Client) injectBodyIncludes(body string, lookup fileLookup, crlf bool) ([]byte, error) {
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
			data, _, err := lookup.read(c, path, "include body file")
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

func buildPathCandidates(path, baseDir string, fallbacks []string, allowRaw bool) []string {
	list := make([]string, 0, 2+len(fallbacks))
	if baseDir != "" {
		list = append(list, filepath.Join(baseDir, path))
	}
	for _, fb := range fallbacks {
		if fb == "" {
			continue
		}
		list = append(list, filepath.Join(fb, path))
	}
	if allowRaw {
		list = append(list, path)
	}
	return util.DedupeNonEmptyStrings(list)
}

func resolveFileLookup(baseDir string, opts Options) ([]string, bool) {
	if opts.NoFallback {
		return nil, baseDir == ""
	}
	return opts.FallbackBaseDirs, true
}

func stopReadFallback(err error) bool {
	return isPerm(err) || isDirErr(err) || errors.Is(err, os.ErrInvalid)
}

func isPerm(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrPermission)
}

func isDirErr(err error) bool {
	if errors.Is(err, syscall.EISDIR) {
		return true
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(pe.Err, syscall.EISDIR) {
		return true
	}
	return false
}

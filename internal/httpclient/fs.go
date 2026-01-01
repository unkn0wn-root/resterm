package httpclient

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (c *Client) readFileWithFallback(
	path string,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
	label string,
) ([]byte, string, error) {
	if c == nil || c.fs == nil {
		return nil, "", errdef.New(errdef.CodeFilesystem, "file reader unavailable")
	}

	if path == "" {
		return nil, "", errdef.New(
			errdef.CodeFilesystem,
			"%s path is empty",
			strings.ToLower(label),
		)
	}

	if filepath.IsAbs(path) {
		data, err := c.fs.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		return nil, "", errdef.Wrap(
			errdef.CodeFilesystem,
			err,
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
			return nil, "", errdef.Wrap(
				errdef.CodeFilesystem,
				err,
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
	return nil, "", errdef.Wrap(
		errdef.CodeFilesystem,
		lastErr,
		"read %s %s (last tried %s)",
		strings.ToLower(label),
		path,
		lastPath,
	)
}

// Lines starting with @ get replaced with the file contents.
// @{variable} syntax is left alone so template expansion can handle it.
func (c *Client) injectBodyIncludes(
	body string,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	var b strings.Builder
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if !first {
			b.WriteByte('\n')
		}

		first = false
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 1 && strings.HasPrefix(trimmed, "@") &&
			!strings.HasPrefix(trimmed, "@{") {
			includePath := strings.TrimSpace(trimmed[1:])
			if includePath != "" {
				data, _, err := c.readFileWithFallback(
					includePath,
					baseDir,
					fallbacks,
					allowRaw,
					"include body file",
				)
				if err != nil {
					return "", err
				}
				b.WriteString(string(data))
				continue
			}
		}
		b.WriteString(line)
	}

	if err := scanner.Err(); err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "scan body includes")
	}
	return b.String(), nil
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

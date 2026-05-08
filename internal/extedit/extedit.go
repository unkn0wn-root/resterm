package extedit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	EnvRestermEditor = "RESTERM_EDITOR"
	EnvVisual        = "VISUAL"
	EnvEditor        = "EDITOR"
)

var (
	ErrNoEditor = errors.New("no external editor configured")
	ErrNoArgs   = errors.New("editor command is empty")
)

type Cmd struct {
	Args []string
}

func (c Cmd) Exec(path string) *exec.Cmd {
	args := append([]string(nil), c.Args...)
	args = append(args, path)
	return exec.Command(args[0], args[1:]...)
}

func Resolve() (Cmd, error) {
	return ResolveWith(os.Getenv, exec.LookPath)
}

func ResolveWith(getenv func(string) string, lookPath func(string) (string, error)) (Cmd, error) {
	for _, env := range []string{EnvRestermEditor, EnvVisual, EnvEditor} {
		raw := strings.TrimSpace(getenv(env))
		if raw == "" {
			continue
		}
		args, err := split(raw)
		if err != nil {
			return Cmd{}, fmt.Errorf("%s: %w", env, err)
		}
		if len(args) == 0 {
			return Cmd{}, fmt.Errorf("%s: %w", env, ErrNoArgs)
		}
		if err := checkExe(args[0], lookPath); err != nil {
			return Cmd{}, fmt.Errorf("%s: %w", env, err)
		}
		return Cmd{Args: args}, nil
	}
	return Cmd{}, ErrNoEditor
}

func split(s string) ([]string, error) {
	var args []string
	var b strings.Builder
	quote := rune(0)
	seen := false
	rs := []rune(s)

	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if quote == 0 && unicode.IsSpace(r) {
			if seen {
				args = append(args, b.String())
				b.Reset()
				seen = false
			}
			continue
		}

		switch r {
		case '\'', '"':
			if quote == 0 {
				quote = r
				seen = true
				continue
			}
			if quote == r {
				quote = 0
				continue
			}
		case '\\':
			// inside single quotes, backslash has no escape role; keep this thing literal.
			if i+1 < len(rs) && shouldEscape(quote, rs[i+1]) {
				next := rs[i+1]
				b.WriteRune(next)
				seen = true
				i++
				continue
			}
		}

		b.WriteRune(r)
		seen = true
	}

	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if seen {
		args = append(args, b.String())
	}
	return args, nil
}

func checkExe(path string, lookPath func(string) (string, error)) error {
	if path == "" {
		return ErrNoArgs
	}
	if _, err := lookPath(path); err != nil {
		if isPath(path) {
			return fmt.Errorf("editor %q not found or not executable", path)
		}
		return fmt.Errorf("editor %q not found in PATH", path)
	}
	return nil
}

func isPath(s string) bool {
	return filepath.IsAbs(s) || strings.ContainsAny(s, `/\`)
}

func shouldEscape(quote, next rune) bool {
	switch quote {
	case '\'':
		return false
	case '"':
		return next == '"' || next == '\\'
	default:
		return unicode.IsSpace(next) || next == '\'' || next == '"' || next == '\\'
	}
}

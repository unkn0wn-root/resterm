package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func (r *runner) planGitignore() (op, error) {
	abs := filepath.Join(r.o.Dir, gitignoreFile)
	o := op{Path: gitignoreFile, Abs: abs, Mode: filePerm}

	data, err := r.fs.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		o.Action = ActionCreate
		o.Data = gitignoreEntry + "\n"
		return o, nil
	}
	if err != nil {
		return op{}, fmt.Errorf("init: read .gitignore: %w", err)
	}
	if hasGitignoreEntry(string(data), gitignoreEntry) {
		o.Action = ActionSkip
		return o, nil
	}

	info, err := r.fs.Stat(abs)
	if err != nil {
		return op{}, fmt.Errorf("init: stat .gitignore: %w", err)
	}
	o.Action = ActionAppend
	o.Mode = info.Mode().Perm()
	o.Data = appendGitignoreEntry(string(data), gitignoreEntry)
	o.Prev = &prior{Data: string(data), Mode: o.Mode}
	return o, nil
}

func appendGitignoreEntry(data, entry string) string {
	if data == "" {
		return entry + "\n"
	}
	if data[len(data)-1] != '\n' {
		return data + "\n" + entry + "\n"
	}
	return data + entry + "\n"
}

func hasGitignoreEntry(data, entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return true
	}
	entrySlash := "/" + entry
	for line := range strings.SplitSeq(data, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if matchesGitignoreEntry(trim, entry, entrySlash) {
			return true
		}
	}
	return false
}

func matchesGitignoreEntry(line, entry, entrySlash string) bool {
	if strings.HasPrefix(line, entry) {
		return trailingCommentOrEmpty(line[len(entry):])
	}
	if strings.HasPrefix(line, entrySlash) {
		return trailingCommentOrEmpty(line[len(entrySlash):])
	}
	return false
}

func trailingCommentOrEmpty(rest string) bool {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return true
	}
	return strings.HasPrefix(rest, "#")
}

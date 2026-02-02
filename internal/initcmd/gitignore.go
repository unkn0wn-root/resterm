package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func (r *runner) writeGitignore() error {
	act, err := r.ensureGitignore(gitignoreEntry)
	if err != nil {
		return err
	}
	return r.report(string(act), gitignoreFile)
}

func (r *runner) ensureGitignore(entry string) (Action, error) {
	p := filepath.Join(r.o.Dir, gitignoreFile)
	data, err := r.fs.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("init: read .gitignore: %w", err)
	}

	mode := filePerm
	if err == nil {
		info, statErr := r.fs.Stat(p)
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return "", fmt.Errorf("init: stat .gitignore: %w", statErr)
		}
		if statErr == nil {
			mode = info.Mode().Perm()
		}

		if hasGitignoreEntry(string(data), entry) {
			return ActionSkip, nil
		}
		if r.o.DryRun {
			return ActionAppend, nil
		}

		updated := appendGitignoreEntry(string(data), entry)
		if err := r.writeAtomic(p, mode, updated, true); err != nil {
			return "", fmt.Errorf("init: update .gitignore: %w", err)
		}
		return ActionAppend, nil
	}

	if r.o.DryRun {
		return ActionCreate, nil
	}
	if err = r.writeAtomic(p, mode, entry+"\n", true); err != nil {
		return "", fmt.Errorf("init: create .gitignore: %w", err)
	}
	return ActionCreate, nil
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

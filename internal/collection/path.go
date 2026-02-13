package collection

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func NormRelPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("collection path is empty")
	}
	if strings.ContainsRune(p, 0) {
		return "", errors.New("collection path contains null byte")
	}

	// Manifest paths are canonical slash-separated paths.
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("absolute path is not allowed: %q", p)
	}
	if hasWinDrive(p) {
		return "", fmt.Errorf("volume path is not allowed: %q", p)
	}

	c := path.Clean(p)
	if c == "." || c == "" {
		return "", fmt.Errorf("path resolves to empty: %q", p)
	}
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("path escapes bundle root: %q", p)
	}
	return c, nil
}

func SafeJoin(base, rel string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("base path is empty")
	}
	rel, err := NormRelPath(rel)
	if err != nil {
		return "", err
	}

	base = filepath.Clean(base)
	if !filepath.IsAbs(base) {
		abs, err := filepath.Abs(base)
		if err != nil {
			return "", fmt.Errorf("resolve base path: %w", err)
		}
		base = abs
	}
	dst := filepath.Join(base, filepath.FromSlash(rel))
	if !withinBase(base, dst) {
		return "", fmt.Errorf("path escapes target root: %q", rel)
	}
	return dst, nil
}

func withinBase(base, dst string) bool {
	if base == "" || dst == "" {
		return false
	}
	if !filepath.IsAbs(base) || !filepath.IsAbs(dst) {
		return false
	}

	rel, err := filepath.Rel(base, dst)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

// hasWinDrive rejects Windows volume paths so manifests remain portable and
// cannot escape roots on Windows after slash normalization.
func hasWinDrive(p string) bool {
	if len(p) < 2 {
		return false
	}
	a := p[0]
	if !((a >= 'a' && a <= 'z') || (a >= 'A' && a <= 'Z')) {
		return false
	}
	return p[1] == ':'
}

func cleanAbsPath(raw, label string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	return abs, nil
}

func resolveDir(path, label string) (abs, real string, err error) {
	abs, err = cleanAbsPath(path, label)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a directory: %s", label, abs)
	}
	real = abs
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		real = p
	}
	return abs, real, nil
}

func readWorkspaceFile(rootAbs, rootReal, baseDir, ref string) (string, string, []byte, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", nil, errors.New("dependency path is empty")
	}
	abs := filepath.Clean(filepath.Join(baseDir, ref))
	if filepath.IsAbs(ref) {
		abs = filepath.Clean(ref)
	}
	if !withinBase(rootAbs, abs) {
		return "", "", nil, fmt.Errorf("reference %q resolves outside workspace (%s)", ref, abs)
	}

	data, ok, err := readWorkspaceAbsFileIfExists(rootReal, abs, ref)
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		return "", "", nil, fmt.Errorf("referenced file not found: %s", abs)
	}

	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", "", nil, fmt.Errorf("make relative path for %s: %w", abs, err)
	}
	rel, err = NormRelPath(filepath.ToSlash(rel))
	if err != nil {
		return "", "", nil, fmt.Errorf("normalize bundle path for %s: %w", abs, err)
	}

	return abs, rel, data, nil
}

func readWorkspaceFileIfExists(rootAbs, rootReal, rel string) ([]byte, bool, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, false, errors.New("dependency path is empty")
	}
	abs := filepath.Clean(filepath.Join(rootAbs, rel))
	if filepath.IsAbs(rel) {
		abs = filepath.Clean(rel)
	}
	if !withinBase(rootAbs, abs) {
		return nil, false, fmt.Errorf("reference %q resolves outside workspace (%s)", rel, abs)
	}

	return readWorkspaceAbsFileIfExists(rootReal, abs, rel)
}

func readWorkspaceAbsFileIfExists(rootReal, abs, ref string) ([]byte, bool, error) {
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat referenced file %s: %w", abs, err)
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("referenced path is a directory: %s", abs)
	}

	real := abs
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		real = p
	}
	if !withinBase(rootReal, real) {
		return nil, false, fmt.Errorf(
			"reference %q resolves outside workspace via symlink (%s)",
			ref,
			real,
		)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, false, fmt.Errorf("read file %s: %w", abs, err)
	}
	return data, true, nil
}

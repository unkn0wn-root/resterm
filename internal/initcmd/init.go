package initcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Opt describes how the init command should run.
// Fields are plain values so callers can map flags directly.
type Opt struct {
	Dir         string
	Template    string
	Force       bool
	DryRun      bool
	NoGitignore bool
	List        bool
	Out         io.Writer
}

type fileSpec struct {
	Path string
	Data string
	Mode fs.FileMode
}

// template represents a named starter set that can write multiple files.
// AddGitignore controls whether resterm.env.json is added to .gitignore.
type template struct {
	Name         string
	Description  string
	Files        []fileSpec
	AddGitignore bool
}

// Run orchestrates the init flow: validate input, ensure the directory,
// write files, and optionally update .gitignore.
func Run(opt Opt) error {
	opt = withDefaults(opt)
	if opt.List {
		return listTemplates(opt.Out)
	}

	tmpl, ok := findTemplate(opt.Template)
	if !ok {
		return unknownTemplateErr(opt.Template)
	}

	if err := ensureDir(opt.Dir, opt.DryRun); err != nil {
		return err
	}

	if err := preflight(opt.Dir, tmpl.Files, opt.Force); err != nil {
		return err
	}

	for _, f := range tmpl.Files {
		act, err := writeFile(opt.Dir, f, opt.Force, opt.DryRun)
		if err != nil {
			return err
		}
		if err := report(opt.Out, act, f.Path, opt.DryRun); err != nil {
			return err
		}
	}

	if tmpl.AddGitignore && !opt.NoGitignore {
		act, err := ensureGitignore(opt.Dir, gitignoreEntry, opt.DryRun)
		if err != nil {
			return err
		}
		if err := report(opt.Out, act, gitignoreFile, opt.DryRun); err != nil {
			return err
		}
	}

	return nil
}

func withDefaults(opt Opt) Opt {
	if opt.Out == nil {
		opt.Out = os.Stdout
	}
	opt.Dir = strings.TrimSpace(opt.Dir)
	if opt.Dir == "" {
		opt.Dir = DefaultDir
	}
	opt.Template = normalizeTemplateName(opt.Template)
	if opt.Template == "" {
		opt.Template = DefaultTemplate
	}
	return opt
}

func normalizeTemplateName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func listTemplates(w io.Writer) error {
	tpls := templateList()
	width := 0
	for _, t := range tpls {
		if nameWidth := utf8.RuneCountInString(t.Name); nameWidth > width {
			width = nameWidth
		}
	}
	for _, t := range tpls {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", width, t.Name, t.Description); err != nil {
			return fmt.Errorf("init: list templates: %w", err)
		}
	}
	return nil
}

func findTemplate(name string) (template, bool) {
	for _, t := range templateList() {
		if t.Name == name {
			return t, true
		}
	}
	return template{}, false
}

func templateNames() []string {
	tpls := templateList()
	out := make([]string, 0, len(tpls))
	for _, t := range tpls {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

func unknownTemplateErr(name string) error {
	if name == "" {
		name = "(empty)"
	}
	return fmt.Errorf("init: unknown template %q (available: %s)", name, strings.Join(templateNames(), ", "))
}

func ensureDir(dir string, dry bool) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("init: %s is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: stat %s: %w", dir, err)
	}
	if dry {
		return nil
	}
	if err = os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("init: create %s: %w", dir, err)
	}
	return nil
}

func preflight(dir string, specs []fileSpec, force bool) error {
	var conflicts []string
	for _, f := range specs {
		path := filepath.Join(dir, f.Path)
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				conflicts = append(conflicts, f.Path+" (dir)")
				continue
			}
			if !force {
				conflicts = append(conflicts, f.Path)
			}
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return fmt.Errorf("init: stat %s: %w", f.Path, err)
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("init: files already exist: %s (use --force to overwrite)", strings.Join(conflicts, ", "))
}

func writeFile(dir string, f fileSpec, force bool, dry bool) (string, error) {
	path := filepath.Join(dir, f.Path)
	_, err := os.Stat(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("init: stat %s: %w", f.Path, err)
	}

	act := actionCreate
	if exists {
		act = actionOverwrite
	}
	if dry {
		return act, nil
	}

	if err = os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return "", fmt.Errorf("init: create dir for %s: %w", f.Path, err)
	}

	flag := os.O_WRONLY | os.O_CREATE
	if force {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_EXCL
	}

	file, err := os.OpenFile(path, flag, f.Mode)
	if err != nil {
		return "", fmt.Errorf("init: write %s: %w", f.Path, err)
	}

	defer func() {
		_ = file.Close()
	}()

	if _, err = io.WriteString(file, f.Data); err != nil {
		return "", fmt.Errorf("init: write %s: %w", f.Path, err)
	}
	return act, nil
}

func ensureGitignore(dir, entry string, dry bool) (string, error) {
	path := filepath.Join(dir, gitignoreFile)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("init: read .gitignore: %w", err)
	}

	if err == nil {
		if hasGitignoreEntry(string(data), entry) {
			return actionSkip, nil
		}
		if dry {
			return actionAppend, nil
		}

		add := entry + "\n"
		buf := appendGitignore(data, add)
		if err = os.WriteFile(path, buf, filePerm); err != nil {
			return "", fmt.Errorf("init: update .gitignore: %w", err)
		}
		return actionAppend, nil
	}

	if dry {
		return actionCreate, nil
	}
	if err = os.WriteFile(path, []byte(entry+"\n"), filePerm); err != nil {
		return "", fmt.Errorf("init: create .gitignore: %w", err)
	}
	return actionCreate, nil
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

func appendGitignore(data []byte, add string) []byte {
	if len(data) == 0 {
		return []byte(add)
	}
	if data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return append(data, []byte(add)...)
}

func report(w io.Writer, act, path string, dry bool) error {
	if w == nil || act == "" {
		return nil
	}
	prefix := ""
	if dry {
		prefix = "dry-run: "
	}
	if _, err := fmt.Fprintf(w, "%s%s %s\n", prefix, act, path); err != nil {
		return fmt.Errorf("init: report %s %s: %w", act, path, err)
	}
	return nil
}

package initcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

type runner struct {
	o Opt
	t template
}

// Run orchestrates the init flow: validate input, ensure the directory,
// write files, and optionally update .gitignore.
func Run(o Opt) error {
	r := newRunner(o)
	return r.run()
}

func newRunner(o Opt) *runner {
	o = withDefaults(o)
	return &runner{o: o}
}

func (r *runner) run() error {
	if r.o.List {
		return listTemplates(r.o.Out)
	}

	t, ok := findTemplate(r.o.Template)
	if !ok {
		return unknownTemplateErr(r.o.Template)
	}
	r.t = t

	if err := r.ensureDir(); err != nil {
		return err
	}

	if err := r.preflight(); err != nil {
		return err
	}

	if err := r.writeFiles(); err != nil {
		return err
	}

	if r.t.AddGitignore && !r.o.NoGitignore {
		return r.writeGitignore()
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
	width := templateWidth()
	for _, t := range tpls {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", width, t.Name, t.Description); err != nil {
			return fmt.Errorf("init: list templates: %w", err)
		}
	}
	return nil
}

func unknownTemplateErr(name string) error {
	if name == "" {
		name = "(empty)"
	}
	return fmt.Errorf(
		"init: unknown template %q (available: %s)",
		name,
		strings.Join(templateNames(), ", "),
	)
}

func (r *runner) ensureDir() error {
	d := r.o.Dir
	info, err := os.Stat(d)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("init: %s is not a directory", d)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: stat %s: %w", d, err)
	}
	if r.o.DryRun {
		return nil
	}
	if err = os.MkdirAll(d, dirPerm); err != nil {
		return fmt.Errorf("init: create %s: %w", d, err)
	}
	return nil
}

func (r *runner) preflight() error {
	d := r.o.Dir
	force := r.o.Force
	var cs []string
	for _, f := range r.t.Files {
		p := filepath.Join(d, f.Path)
		info, err := os.Stat(p)
		if err == nil {
			if info.IsDir() {
				cs = append(cs, f.Path+" (dir)")
				continue
			}
			if !force {
				cs = append(cs, f.Path)
			}
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return fmt.Errorf("init: stat %s: %w", f.Path, err)
	}
	if len(cs) == 0 {
		return nil
	}
	return fmt.Errorf(
		"init: files already exist: %s (use --force to overwrite)",
		strings.Join(cs, ", "),
	)
}

func (r *runner) writeFiles() error {
	for _, f := range r.t.Files {
		act, err := r.writeFile(f)
		if err != nil {
			return err
		}
		if err := r.report(act, f.Path); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) writeFile(f fileSpec) (string, error) {
	p := filepath.Join(r.o.Dir, f.Path)
	_, err := os.Stat(p)
	ex := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("init: stat %s: %w", f.Path, err)
	}
	if ex && !r.o.Force {
		return "", fmt.Errorf("init: write %s: %w", f.Path, os.ErrExist)
	}

	act := actionCreate
	if ex {
		act = actionOverwrite
	}
	if r.o.DryRun {
		return act, nil
	}

	if err = os.MkdirAll(filepath.Dir(p), dirPerm); err != nil {
		return "", fmt.Errorf("init: create dir for %s: %w", f.Path, err)
	}

	if err = writeAtomic(p, f.Mode, f.Data, r.o.Force); err != nil {
		return "", fmt.Errorf("init: write %s: %w", f.Path, err)
	}
	return act, nil
}

func writeAtomic(p string, m fs.FileMode, data string, force bool) (err error) {
	d := filepath.Dir(p)
	f, err := os.CreateTemp(d, ".resterm-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if err = f.Chmod(m); err != nil {
		return err
	}
	if _, err = io.WriteString(f, data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if !force {
		if _, err = os.Stat(p); err == nil {
			return os.ErrExist
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err = os.Rename(tmp, p); err == nil {
		return nil
	}
	if !force || !os.IsExist(err) {
		return err
	}
	if err = os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, p)
}

func (r *runner) writeGitignore() error {
	act, err := r.ensureGitignore(gitignoreEntry)
	if err != nil {
		return err
	}
	return r.report(act, gitignoreFile)
}

func (r *runner) ensureGitignore(entry string) (string, error) {
	p := filepath.Join(r.o.Dir, gitignoreFile)
	data, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("init: read .gitignore: %w", err)
	}

	if err == nil {
		if hasGitignoreEntry(string(data), entry) {
			return actionSkip, nil
		}
		if r.o.DryRun {
			return actionAppend, nil
		}

		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return "", fmt.Errorf("init: update .gitignore: %w", err)
		}
		defer func() {
			_ = f.Close()
		}()
		if len(data) > 0 && data[len(data)-1] != '\n' {
			if _, err = io.WriteString(f, "\n"); err != nil {
				return "", fmt.Errorf("init: update .gitignore: %w", err)
			}
		}
		if _, err = io.WriteString(f, entry+"\n"); err != nil {
			return "", fmt.Errorf("init: update .gitignore: %w", err)
		}
		return actionAppend, nil
	}

	if r.o.DryRun {
		return actionCreate, nil
	}
	if err = os.WriteFile(p, []byte(entry+"\n"), filePerm); err != nil {
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

func (r *runner) report(act, path string) error {
	if r.o.Out == nil || act == "" {
		return nil
	}
	prefix := ""
	if r.o.DryRun {
		prefix = "dry-run: "
	}
	if _, err := fmt.Fprintf(r.o.Out, "%s%s %s\n", prefix, act, path); err != nil {
		return fmt.Errorf("init: report %s %s: %w", act, path, err)
	}
	return nil
}

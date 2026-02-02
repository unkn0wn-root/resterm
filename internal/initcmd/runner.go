package initcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

type runner struct {
	fs FS
	o  Opt
	t  template
}

func (r *runner) run() error {
	if err := r.ensureDir(); err != nil {
		return err
	}
	ops, err := r.plan()
	if err != nil {
		return err
	}
	if err := r.apply(ops); err != nil {
		return err
	}
	if r.t.AddGitignore && !r.o.NoGitignore {
		return r.writeGitignore()
	}
	return nil
}

func (r *runner) ensureDir() error {
	d := r.o.Dir
	info, err := r.fs.Stat(d)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("init: %s is not a directory", d)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("init: stat %s: %w", d, err)
	}
	if r.o.DryRun {
		return nil
	}
	if err = r.fs.MkdirAll(d, dirPerm); err != nil {
		return fmt.Errorf("init: create %s: %w", d, err)
	}
	return nil
}

func (r *runner) writeAtomic(p string, m fs.FileMode, data string, force bool) (err error) {
	d := filepath.Dir(p)
	f, err := r.fs.CreateTemp(d, ".resterm-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = r.fs.Remove(tmp)
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
		if _, err = r.fs.Stat(p); err == nil {
			return fs.ErrExist
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err = r.fs.Rename(tmp, p); err == nil {
		return nil
	}
	if !force || !errors.Is(err, fs.ErrExist) {
		return err
	}
	if err = r.fs.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return r.fs.Rename(tmp, p)
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

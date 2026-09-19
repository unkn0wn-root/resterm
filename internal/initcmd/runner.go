package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
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
	return r.apply(ops)
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

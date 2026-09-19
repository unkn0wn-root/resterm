package initcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

type staged struct {
	op  op
	tmp string
}

// apply writes all files as one unit: every file is staged to a temp
// next to its target before any target changes, and a failed commit
// puts already replaced targets back.
func (r *runner) apply(ops []op) error {
	if !r.o.DryRun {
		st, err := r.stage(ops)
		if err != nil {
			return err
		}
		if err := r.commit(st); err != nil {
			return err
		}
	}
	for _, o := range ops {
		if err := r.report(string(o.Action), o.Path); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) stage(ops []op) ([]staged, error) {
	var st []staged
	for _, o := range ops {
		if o.Action == ActionSkip {
			continue
		}
		tmp, err := r.stageFile(o.Abs, o.Mode, o.Data)
		if err != nil {
			r.discard(st)
			return nil, fmt.Errorf("init: write %s: %w", o.Path, err)
		}
		st = append(st, staged{op: o, tmp: tmp})
	}
	return st, nil
}

func (r *runner) stageFile(abs string, m fs.FileMode, data string) (tmp string, err error) {
	dir := filepath.Dir(abs)
	if err := r.fs.MkdirAll(dir, dirPerm); err != nil {
		return "", err
	}
	f, err := r.fs.CreateTemp(dir, tmpPattern)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = r.fs.Remove(f.Name())
		}
	}()
	if err = f.Chmod(m); err != nil {
		return "", err
	}
	if _, err = io.WriteString(f, data); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (r *runner) commit(st []staged) error {
	for i, s := range st {
		err := r.commitFile(s)
		if err == nil {
			continue
		}
		err = fmt.Errorf("init: write %s: %w", s.op.Path, err)
		r.discard(st[i:])
		if rerr := r.rollback(st[:i]); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}
	return nil
}

func (r *runner) commitFile(s staged) error {
	if s.op.Action == ActionCreate {
		_, err := r.fs.Stat(s.op.Abs)
		if err == nil {
			return fs.ErrExist
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return r.fs.Rename(s.tmp, s.op.Abs)
}

func (r *runner) rollback(done []staged) error {
	var errs []error
	for _, s := range done {
		if err := r.undo(s.op); err != nil {
			errs = append(errs, fmt.Errorf("init: restore %s: %w", s.op.Path, err))
		}
	}
	return errors.Join(errs...)
}

func (r *runner) undo(o op) error {
	if o.Prev == nil {
		return r.fs.Remove(o.Abs)
	}
	tmp, err := r.stageFile(o.Abs, o.Prev.Mode, o.Prev.Data)
	if err != nil {
		return err
	}
	if err := r.fs.Rename(tmp, o.Abs); err != nil {
		_ = r.fs.Remove(tmp)
		return err
	}
	return nil
}

func (r *runner) discard(st []staged) {
	for _, s := range st {
		_ = r.fs.Remove(s.tmp)
	}
}

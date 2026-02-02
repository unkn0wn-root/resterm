package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func (r *runner) plan() ([]op, error) {
	var ops []op
	var conflicts []string

	for _, f := range r.t.Files {
		rel := normalizeTemplatePath(f.Path)
		abs, err := safeJoin(r.o.Dir, rel)
		if err != nil {
			return nil, err
		}

		info, err := r.fs.Stat(abs)
		switch {
		case err == nil && info.IsDir():
			conflicts = append(conflicts, rel+" (dir)")
			continue
		case err == nil && !r.o.Force:
			conflicts = append(conflicts, rel)
			continue
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("init: stat %s: %w", rel, err)
		}

		act := ActionCreate
		if err == nil {
			act = ActionOverwrite
		}

		ops = append(ops, op{
			Action: act,
			Path:   rel,
			Abs:    abs,
			Mode:   f.Mode,
			Data:   f.Data,
		})
	}

	if len(conflicts) > 0 {
		return nil, fmt.Errorf(
			"init: files already exist: %s (use --force to overwrite)",
			strings.Join(conflicts, ", "),
		)
	}

	return ops, nil
}

func (r *runner) apply(ops []op) error {
	for _, op := range ops {
		if r.o.DryRun {
			if err := r.report(string(op.Action), op.Path); err != nil {
				return err
			}
			continue
		}

		if err := r.fs.MkdirAll(filepath.Dir(op.Abs), dirPerm); err != nil {
			return fmt.Errorf("init: create dir for %s: %w", op.Path, err)
		}

		if err := r.writeAtomic(op.Abs, op.Mode, op.Data, r.o.Force); err != nil {
			return fmt.Errorf("init: write %s: %w", op.Path, err)
		}

		if err := r.report(string(op.Action), op.Path); err != nil {
			return err
		}
	}
	return nil
}

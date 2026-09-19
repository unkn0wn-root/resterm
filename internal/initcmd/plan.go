package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
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

		o := op{
			Action: ActionCreate,
			Path:   rel,
			Abs:    abs,
			Mode:   f.Mode,
			Data:   f.Data,
		}
		if err == nil {
			data, err := r.fs.ReadFile(abs)
			if err != nil {
				return nil, fmt.Errorf("init: read %s: %w", rel, err)
			}
			o.Action = ActionOverwrite
			o.Prev = &prior{Data: string(data), Mode: info.Mode().Perm()}
		}
		ops = append(ops, o)
	}

	if len(conflicts) > 0 {
		return nil, fmt.Errorf(
			"init: files already exist: %s (use --force to overwrite)",
			strings.Join(conflicts, ", "),
		)
	}

	if r.t.AddGitignore && !r.o.NoGitignore {
		o, err := r.planGitignore()
		if err != nil {
			return nil, err
		}
		ops = append(ops, o)
	}

	return ops, nil
}

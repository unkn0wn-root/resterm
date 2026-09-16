package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type fixtureSet struct {
	dir   string
	files []string
	dirs  []string
}

func (s fixtureSet) remove() {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()

	for _, name := range s.files {
		_ = root.Remove(name)
	}
	for _, name := range s.dirs {
		_ = root.Remove(name)
	}
}

func (s *fixtureSet) write(root *os.Root, fixture Fixture) error {
	if !filepath.IsLocal(fixture.Path) {
		return errors.New("fixture path must be local")
	}
	dir := filepath.Dir(fixture.Path)
	switch err := root.Mkdir(dir, 0o700); {
	case err == nil:
		s.dirs = append(s.dirs, dir)
	case !slices.Contains(s.dirs, dir):
		return fmt.Errorf("create recording fixtures: %w", err)
	}

	f, err := root.OpenFile(fixture.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create recording body: %w", err)
	}
	s.files = append(s.files, fixture.Path)

	_, writeErr := f.Write(fixture.Data)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return fmt.Errorf("write recording body: %w", err)
	}
	return nil
}

// PublishFixtures returns cleanup for only the files and directories it creates.
func (p *Plan) PublishFixtures(ctx context.Context) (func(), error) {
	dir := filepath.Dir(p.Document.Path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open recording directory: %w", err)
	}
	// Cleanup may run later. Reopen the root then to avoid holding a descriptor.
	defer func() { _ = root.Close() }()

	set := fixtureSet{dir: dir}
	for _, fixture := range p.Fixtures {
		err := ctx.Err()
		if err == nil {
			err = set.write(root, fixture)
		}
		if err != nil {
			set.remove()
			return nil, err
		}
	}
	return set.remove, nil
}

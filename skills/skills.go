package skills

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const Name = "resterm"

const fence = "---"

//go:embed resterm
var FS embed.FS

func Install(dir string) (string, error) {
	sub, err := fs.Sub(FS, Name)
	if err != nil {
		return "", err
	}

	dst := filepath.Join(dir, Name)
	// dir can hold a resterm clone, the binary, or a link to a skill that other agents share.
	fi, err := os.Lstat(dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	exists := err == nil
	if exists {
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("%s is a link. Update the skill where it points, or remove the link", dst)
		}
		if !fi.IsDir() || !isSkill(dst) {
			return "", fmt.Errorf("%s exists and is not a resterm skill. Remove it or choose another directory", dst)
		}
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", err
	}

	// The stage sits in dir so both renames stay on one file system.
	stage, err := os.MkdirTemp(dir, "."+Name+"-")
	if err != nil {
		return "", err
	}

	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()

	// MkdirTemp makes stage 0700, so the skill goes in a subfolder with normal permissions.
	src := filepath.Join(stage, Name)
	if err := os.CopyFS(src, sub); err != nil {
		return "", err
	}

	old := filepath.Join(stage, "old")
	if exists {
		if err := os.Rename(dst, old); err != nil {
			return "", err
		}
	}
	if err := os.Rename(src, dst); err != nil {
		if !exists {
			return "", err
		}
		if rerr := os.Rename(old, dst); rerr != nil {
			keep = true
			return "", errors.Join(err, fmt.Errorf("%w. The old skill is in %s", rerr, old))
		}
		return "", err
	}
	return dst, nil
}

func isSkill(dir string) bool {
	name := filepath.Join(dir, "SKILL.md")
	if fi, err := os.Lstat(name); err != nil || !fi.Mode().IsRegular() {
		return false
	}

	f, err := os.Open(name)
	if err != nil {
		return false
	}

	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	if !s.Scan() || s.Text() != fence {
		return false
	}

	named := false
	for s.Scan() {
		switch s.Text() {
		case fence:
			return named
		case "name: " + Name:
			named = true
		}
	}
	return false
}

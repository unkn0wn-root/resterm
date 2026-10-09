package sqlite

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

// The driver stops reading a plain path at "?". An escaped file URI keeps all of it.
func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", diag.WrapAs(diag.ClassFilesystem, err, "resolve db path")
	}
	p := filepath.ToSlash(abs)
	// A Windows drive path needs a leading slash, as in file:///C:/x.db.
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String(), nil
}

// History rows hold response bodies. SQLite gives new -wal and -shm files the mode of this file.
func createPrivate(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, diag.WrapAs(diag.ClassFilesystem, err, "create history file")
	}
	_ = f.Close()
	return true, nil
}

// Existing files are only changed by path and never opened.
// If this process closed a file SQLite has open, it would drop SQLite's locks on it.
func makePrivate(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		// Only the owner can change the mode. For anyone else, history opens as before.
		_ = os.Chmod(p, st.Mode().Perm()&^0o077)
	}
}

func quarantineDB(path string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	dst := nextQuarantinePath(path + ".corrupt-" + ts)
	if err := moveIfExists(path, dst); err != nil {
		return "", err
	}
	// WAL and SHM files must move with the main file so SQLite never
	// tries to replay stale pages into the replacement database.
	if err := moveIfExists(path+"-wal", dst+"-wal"); err != nil {
		return "", err
	}
	if err := moveIfExists(path+"-shm", dst+"-shm"); err != nil {
		return "", err
	}
	return dst, nil
}

func nextQuarantinePath(base string) string {
	p := base
	// Recovery can run multiple times in the same second, so numbered
	// suffixes keep each quarantined copy instead of overwriting one.
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
		p = base + "." + strconv.Itoa(i)
	}
	return base + ".x"
}

func moveIfExists(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return diag.WrapAs(diag.ClassFilesystem, err, "move corrupted history file")
	}
	return nil
}

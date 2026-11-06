package fsx

import (
	"io"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to a temp file and renames it over path, so readers never see a partial file.
// The file gets perm even if path already existed.
// A symlink at path is replaced, not followed.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".resterm-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteInPlace writes data to path like os.WriteFile, but keeps the file itself.
// A symlink at path is followed, and the inode, links, owner and mode do not change.
// perm is used only when the file is created. It needs read access to the file.
// The file is never truncated first. It grows before the old contents are overwritten and shrinks last,
// and after a failed write it makes a best effort to write the old contents back.
// It is not crash safe: a crash during the write can leave old, new or mixed bytes.
func WriteInPlace(path string, data []byte, perm os.FileMode) (err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	old, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if len(data) > len(old) {
		// Grow first, so a full disk fails before the old contents are overwritten.
		if _, err := f.WriteAt(data[len(old):], int64(len(old))); err != nil {
			_ = f.Truncate(int64(len(old)))
			return err
		}
	}
	defer func() {
		if err != nil {
			if _, werr := f.WriteAt(old, 0); werr == nil {
				_ = f.Truncate(int64(len(old)))
			}
		}
	}()
	if _, err := f.WriteAt(data[:min(len(data), len(old))], 0); err != nil {
		return err
	}
	if len(data) < len(old) {
		return f.Truncate(int64(len(data)))
	}
	return nil
}

package runner

import (
	"path/filepath"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

func absCleanPath(path string) (string, error) {
	path = str.Trim(path)
	if path == "" {
		return "", nil
	}
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

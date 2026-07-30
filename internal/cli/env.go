package cli

import (
	"os"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func LoadEnvironment(explicit, filePath, workspace string) (vars.Catalog, string, error) {
	if explicit != "" {
		cat, err := vars.LoadEnvironmentFile(explicit)
		if err != nil {
			return vars.Catalog{}, "", err
		}
		return cat, explicit, nil
	}

	var paths []string
	if filePath != "" {
		paths = append(paths, filepath.Dir(filePath))
	}
	if workspace != "" {
		paths = append(paths, workspace)
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, cwd)
	}

	return vars.ResolveEnvironment(paths)
}

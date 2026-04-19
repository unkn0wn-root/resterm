package cli

import (
	"log"
	"os"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func LoadEnvironment(explicit, filePath, workspace string) (vars.EnvironmentSet, string) {
	if explicit != "" {
		envs, err := vars.LoadEnvironmentFile(explicit)
		if err != nil {
			log.Printf("failed to load environment file %s: %v", explicit, err)
			return nil, ""
		}
		return envs, explicit
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

	envs, path, err := vars.ResolveEnvironment(paths)
	if err != nil {
		return nil, ""
	}
	return envs, path
}

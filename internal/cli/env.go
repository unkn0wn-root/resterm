package cli

import (
	"log"
	"os"
	"path/filepath"
	"sort"

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

func SelectDefaultEnvironment(envs vars.EnvironmentSet) (string, bool) {
	if len(envs) == 0 {
		return "", false
	}
	preferred := []string{"dev", "default", "local"}
	for _, name := range preferred {
		if _, ok := envs[name]; ok {
			return name, len(envs) > 1
		}
	}
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0], len(envs) > 1
}

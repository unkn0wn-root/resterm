package vars

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

const (
	SharedEnvKey  = "$shared"
	GroupsEnvKey  = "$groups"
	DefaultEnvKey = "$default"
)

type EnvironmentSet map[string]map[string]string

var environmentFileCandidates = [...]string{
	"rest-client.env.json",
	"resterm.env.json",
}

func IsReservedEnvironment(name string) bool {
	switch norm(name) {
	case SharedEnvKey, GroupsEnvKey, DefaultEnvKey:
		return true
	default:
		return false
	}
}

func LoadEnvironmentFile(path string) (Catalog, error) {
	if IsDotEnvPath(path) {
		set, err := loadDotEnvEnvironment(path)
		if err != nil {
			return Catalog{}, err
		}
		return NewCatalog(set)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, diag.WrapAsf(diag.ClassFilesystem, err, "read env file %s", path)
	}
	cat, err := parseCatalog(data)
	if err != nil {
		return Catalog{}, diag.WrapAsf(diag.ClassParse, err, "parse env file %s", path)
	}
	return cat, nil
}

func ResolveEnvironment(paths []string) (Catalog, string, error) {
	for _, dir := range paths {
		for _, name := range environmentFileCandidates {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				cat, loadErr := LoadEnvironmentFile(path)
				return cat, path, loadErr
			}
		}
	}
	return Catalog{}, "", nil
}

func norm(s string) string {
	return strings.ToLower(str.Trim(s))
}

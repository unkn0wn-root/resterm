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

	// EnvJSONSuffix is the naming convention environment files share. Only the
	// two candidate names below are ever opened by discovery; the suffix alone
	// marks files that read as environments without being discoverable.
	EnvJSONSuffix = ".env.json"
)

type EnvironmentSet map[string]map[string]string

var environmentFileCandidates = [...]string{
	"rest-client.env.json",
	"resterm.env.json",
}

// IsEnvFileName reports whether path names one of the files discovery opens.
func IsEnvFileName(path string) bool {
	base := strings.ToLower(filepath.Base(str.Trim(path)))
	for _, name := range environmentFileCandidates {
		if base == name {
			return true
		}
	}
	return false
}

// LooksLikeEnvFile reports whether a name reads as an environment file without
// being one of the names discovery opens.
func LooksLikeEnvFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, EnvJSONSuffix) && !IsEnvFileName(base)
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
		cat, err := NewCatalog(set)
		if err != nil {
			return Catalog{}, err
		}
		return cat.withSource(path), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, diag.WrapAsf(diag.ClassFilesystem, err, "read env file %s", path)
	}
	cat, err := parseCatalog(data)
	if err != nil {
		return Catalog{}, diag.WrapAsf(diag.ClassParse, err, "parse env file %s", path)
	}
	return cat.withSource(path), nil
}

// Discover loads the first environment file found directly under roots, tried
// in order. Empty roots are skipped and nothing is added implicitly, so the
// caller decides whether ambient directories such as the working directory take
// part.
func Discover(roots ...string) (Catalog, string, error) {
	path := DiscoverPath(roots...)
	if path == "" {
		return Catalog{}, "", nil
	}
	cat, err := LoadEnvironmentFile(path)
	return cat, path, err
}

// DiscoverPath reports the file Discover would load, without loading it.
func DiscoverPath(roots ...string) string {
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, name := range environmentFileCandidates {
			path := filepath.Join(root, name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

func norm(s string) string {
	return strings.ToLower(str.Trim(s))
}

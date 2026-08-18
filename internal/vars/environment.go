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

	// EnvJSONSuffix is the naming convention environment files share.
	EnvJSONSuffix = ".env.json"
)

type EnvironmentSet map[string]map[string]string

var environmentFileCandidates = [...]string{
	"http-client.env.json",
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

// PrivateEnvFileName is the JetBrains convention user-specific companion file.
// It holds passwords, tokens and other values meant to stay out of source
// control, and is only ever loaded as an overlay over its public sibling.
const PrivateEnvFileName = "http-client.private.env.json"

// IsPrivateEnvFileName reports whether path names the private companion file.
func IsPrivateEnvFileName(path string) bool {
	return strings.EqualFold(filepath.Base(str.Trim(path)), PrivateEnvFileName)
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
// in order. Nothing is added implicitly, so the caller decides whether ambient
// directories such as the working directory take part. When the resolved file
// is the JetBrains public file, its private companion is merged in.
func Discover(roots ...string) (Catalog, string, error) {
	path := DiscoverPath(roots...)
	if path == "" {
		return Catalog{}, "", nil
	}
	if strings.EqualFold(filepath.Base(str.Trim(path)), "http-client.env.json") {
		cat, err := LoadEnvironmentPair(path)
		return cat, path, err
	}
	cat, err := LoadEnvironmentFile(path)
	return cat, path, err
}

// LoadEnvironmentPath loads the environment file at path, merging the private
// companion when path names the JetBrains public file. It is the entry point
// for explicit --env-file handling.
func LoadEnvironmentPath(path string) (Catalog, error) {
	if strings.EqualFold(filepath.Base(str.Trim(path)), "http-client.env.json") {
		return LoadEnvironmentPair(path)
	}
	return LoadEnvironmentFile(path)
}

// LoadEnvironmentPair loads the public env file and overlays the private
// companion file (if present) so private values win. Both files must parse.
func LoadEnvironmentPair(publicPath string) (Catalog, error) {
	cat, err := LoadEnvironmentFile(publicPath)
	if err != nil {
		return Catalog{}, err
	}
	priv := privateSibling(publicPath)
	if priv == "" {
		return cat, nil
	}
	privCat, err := LoadEnvironmentFile(priv)
	if err != nil {
		return Catalog{}, diag.WrapAsf(diag.ClassParse, err, "parse private env file %s", priv)
	}
	return cat.mergePrivate(privCat), nil
}

// privateSibling returns the private companion path for publicPath, empty when
// the file does not exist. Only the exact basename http-client.env.json is
// paired, so resterm.env.json / rest-client.env.json never pick up a private
// overlay.
func privateSibling(publicPath string) string {
	if !strings.EqualFold(filepath.Base(str.Trim(publicPath)), "http-client.env.json") {
		return ""
	}
	priv := filepath.Join(filepath.Dir(str.Trim(publicPath)), PrivateEnvFileName)
	if _, err := os.Stat(priv); err != nil {
		return ""
	}
	return priv
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

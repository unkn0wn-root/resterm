package filesvc

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	extHTTP              = ".http"
	extREST              = ".rest"
	extRTS               = ".rts"
	extGraphQL           = ".graphql"
	extGQL               = ".gql"
	extJSON              = ".json"
	extJS                = ".js"
	extMJS               = ".mjs"
	extCJS               = ".cjs"
	defaultEnvSourceFile = "resterm.env.json"
	altEnvSourceFile     = "rest-client.env.json"
)

type FileKind int

const (
	FileKindRequest FileKind = iota
	FileKindScript
	FileKindEnv
	FileKindGraphQL
	FileKindJSON
	FileKindJavaScript
)

func (k FileKind) String() string {
	switch k {
	case FileKindRequest:
		return "request"
	case FileKindScript:
		return "script"
	case FileKindEnv:
		return "env"
	case FileKindGraphQL:
		return "graphql"
	case FileKindJSON:
		return "json"
	case FileKindJavaScript:
		return "javascript"
	default:
		return "unknown"
	}
}

func (k FileKind) BadgeLabel() string {
	switch k {
	case FileKindScript:
		return "RTS"
	case FileKindEnv:
		return "ENV"
	case FileKindGraphQL:
		return "GQL"
	case FileKindJSON:
		return "JSON"
	case FileKindJavaScript:
		return "JS"
	default:
		return ""
	}
}

type FileEntry struct {
	Name string
	Path string
	Kind FileKind
}

type ListOptions struct {
	ExplicitEnvFile string
}

func IsRequestFile(path string) bool {
	ext := fileExt(path)
	return ext == extHTTP || ext == extREST
}

func IsRTSFile(path string) bool {
	return fileExt(path) == extRTS
}

func IsGraphQLFile(path string) bool {
	ext := fileExt(path)
	return ext == extGraphQL || ext == extGQL
}

func IsJSONFile(path string) bool {
	return fileExt(path) == extJSON
}

func IsJavaScriptFile(path string) bool {
	ext := fileExt(path)
	return ext == extJS || ext == extMJS || ext == extCJS
}

func IsEnvJSONFile(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	return base == defaultEnvSourceFile || base == altEnvSourceFile
}

func IsWorkspaceFile(path string) bool {
	_, ok := ClassifyWorkspacePath(path)
	return ok
}

func ClassifyRequestPath(path string) (FileKind, bool) {
	switch {
	case IsRequestFile(path):
		return FileKindRequest, true
	case IsRTSFile(path):
		return FileKindScript, true
	default:
		return 0, false
	}
}

func ClassifyWorkspacePath(path string) (FileKind, bool) {
	switch {
	case IsRequestFile(path):
		return FileKindRequest, true
	case IsRTSFile(path):
		return FileKindScript, true
	case IsEnvJSONFile(path):
		return FileKindEnv, true
	case IsGraphQLFile(path):
		return FileKindGraphQL, true
	case IsJSONFile(path):
		return FileKindJSON, true
	case IsJavaScriptFile(path):
		return FileKindJavaScript, true
	default:
		return 0, false
	}
}

func ListRequestFiles(root string, recursive bool) ([]FileEntry, error) {
	return listFiles(root, recursive, ClassifyRequestPath, "")
}

func ListWorkspaceFiles(root string, recursive bool, opts ListOptions) ([]FileEntry, error) {
	return listFiles(root, recursive, ClassifyWorkspacePath, opts.ExplicitEnvFile)
}

func listFiles(
	root string,
	recursive bool,
	classify func(path string) (FileKind, bool),
	explicitEnvFile string,
) ([]FileEntry, error) {
	entriesByPath := make(map[string]FileEntry)
	addEntry := func(path string, kind FileKind) {
		rel := filepath.Base(path)
		if r, err := filepath.Rel(root, path); err == nil {
			rel = r
		}
		entriesByPath[filepath.Clean(path)] = FileEntry{
			Name: rel,
			Path: path,
			Kind: kind,
		}
	}

	if recursive {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			kind, ok := classify(path)
			if !ok {
				return nil
			}
			addEntry(path, kind)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		dirEntries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			kind, ok := classify(path)
			if !ok {
				continue
			}
			addEntry(path, kind)
		}
	}

	if entry, ok := explicitWorkspaceEnvEntry(root, explicitEnvFile); ok {
		entriesByPath[filepath.Clean(entry.Path)] = entry
	}

	entries := make([]FileEntry, 0, len(entriesByPath))
	for _, entry := range entriesByPath {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func explicitWorkspaceEnvEntry(root, envFile string) (FileEntry, bool) {
	envFile = strings.TrimSpace(envFile)
	if envFile == "" {
		return FileEntry{}, false
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = filepath.Clean(root)
	}
	envAbs, err := filepath.Abs(envFile)
	if err != nil {
		envAbs = filepath.Clean(envFile)
	}

	info, err := os.Stat(envAbs)
	if err != nil || info.IsDir() {
		return FileEntry{}, false
	}

	rel, err := filepath.Rel(rootAbs, envAbs)
	if err != nil {
		return FileEntry{}, false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return FileEntry{}, false
	}

	return FileEntry{
		Name: rel,
		Path: filepath.Join(root, rel),
		Kind: FileKindEnv,
	}, true
}

func fileExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

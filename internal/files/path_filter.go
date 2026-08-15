package files

import (
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type pathFilterKind uint8

const (
	pathFilterNone pathFilterKind = iota
	pathFilterAny
	pathFilterRequest
	pathFilterWorkspace
)

type PathFilter struct {
	kind    pathFilterKind
	envFile string
}

// PathFilter must remain comparable because it is used as a cache key.
var _ map[PathFilter]struct{}

func AnyPathFilter() PathFilter {
	return PathFilter{kind: pathFilterAny}
}

func RequestPathFilter() PathFilter {
	return PathFilter{kind: pathFilterRequest}
}

func WorkspacePathFilter(envFile string) PathFilter {
	return PathFilter{kind: pathFilterWorkspace, envFile: envFile}
}

func (f PathFilter) Accept(path string) bool {
	switch f.kind {
	case pathFilterAny:
		return true
	case pathFilterRequest:
		return IsRequest(path)
	case pathFilterWorkspace:
		return IsWorkspace(path) || vars.IsDotEnvPath(path) || util.SameFile(path, f.envFile)
	default: // pathFilterNone
		return false
	}
}

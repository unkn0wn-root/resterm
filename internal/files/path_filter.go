package files

import (
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type pathFilterKind uint8

const (
	pathFilterNone pathFilterKind = iota
	pathFilterAny
	pathFilterKinds
	pathFilterWorkspace
)

type PathFilter struct {
	kind    pathFilterKind
	kinds   uint // bitmask of Kind values
	envFile string
}

// PathFilter must remain comparable because it is used as a cache key.
var _ map[PathFilter]struct{}

func AnyPathFilter() PathFilter {
	return PathFilter{kind: pathFilterAny}
}

// KindPathFilter accepts files with an extension matching any of the given kinds.
func KindPathFilter(kinds ...Kind) PathFilter {
	f := PathFilter{kind: pathFilterKinds}
	for _, k := range kinds {
		f.kinds |= 1 << k
	}
	return f
}

func RequestPathFilter() PathFilter {
	return KindPathFilter(KindRequest)
}

func WorkspacePathFilter(envFile string) PathFilter {
	return PathFilter{kind: pathFilterWorkspace, envFile: envFile}
}

func (f PathFilter) Accept(path string) bool {
	switch f.kind {
	case pathFilterAny:
		return true
	case pathFilterKinds:
		kind, ok := classifyExt(path)
		return ok && f.kinds&(1<<kind) != 0
	case pathFilterWorkspace:
		return IsWorkspace(path) || vars.IsDotEnvPath(path) || util.SameFile(path, f.envFile)
	default: // pathFilterNone
		return false
	}
}

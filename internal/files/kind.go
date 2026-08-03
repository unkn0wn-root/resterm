// Package files classifies and enumerates the workspace files resterm reads:
// requests, scripts, environments, and their data companions.
package files

import (
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Kind int

const (
	KindUnknown Kind = iota
	KindRequest
	KindScript
	KindEnv
	KindGraphQL
	KindJSON
	KindJavaScript
)

// Extensions split by the scope that recognizes them. Environment files match
// on file name rather than extension, so they belong to neither table and are
// checked between the two.
var (
	requestExt = map[string]Kind{
		".http": KindRequest,
		".rest": KindRequest,
		".rts":  KindScript,
	}

	dataExt = map[string]Kind{
		".graphql": KindGraphQL,
		".gql":     KindGraphQL,
		".json":    KindJSON,
		".js":      KindJavaScript,
		".mjs":     KindJavaScript,
		".cjs":     KindJavaScript,
	}
)

func (k Kind) String() string {
	switch k {
	case KindRequest:
		return "request"
	case KindScript:
		return "script"
	case KindEnv:
		return "env"
	case KindGraphQL:
		return "graphql"
	case KindJSON:
		return "json"
	case KindJavaScript:
		return "javascript"
	default:
		return "unknown"
	}
}

func (k Kind) BadgeLabel() string {
	switch k {
	case KindEnv:
		return "ENV"
	default:
		return ""
	}
}

type Entry struct {
	Name string
	Path string
	Kind Kind
}

func IsRequest(path string) bool {
	return requestExt[fileExt(path)] == KindRequest
}

func IsRTS(path string) bool {
	return requestExt[fileExt(path)] == KindScript
}

func IsGraphQL(path string) bool {
	return dataExt[fileExt(path)] == KindGraphQL
}

func IsJSON(path string) bool {
	return dataExt[fileExt(path)] == KindJSON
}

func IsJavaScript(path string) bool {
	return dataExt[fileExt(path)] == KindJavaScript
}

func IsWorkspace(path string) bool {
	_, ok := ClassifyWorkspace(path)
	return ok
}

func ClassifyRequest(path string) (Kind, bool) {
	kind, ok := requestExt[fileExt(path)]
	return kind, ok
}

// ClassifyWorkspace checks requests first so a request extension always wins,
// then environment names, so resterm.env.json reads as an environment rather
// than as plain JSON.
func ClassifyWorkspace(path string) (Kind, bool) {
	ext := fileExt(path)
	if kind, ok := requestExt[ext]; ok {
		return kind, true
	}
	if vars.IsEnvFileName(path) {
		return KindEnv, true
	}
	kind, ok := dataExt[ext]
	return kind, ok
}

func fileExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

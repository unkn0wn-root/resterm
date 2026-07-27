package directive

import "strings"

// Scope keeps request, file, and global behavior consistent across directive parsers.
type Scope int

const (
	ScopeRequest Scope = iota
	ScopeFile
	ScopeGlobal
)

func (s Scope) String() string {
	switch s {
	case ScopeFile:
		return "file"
	case ScopeGlobal:
		return "global"
	default:
		return "request"
	}
}

func ParseScope(token string) (Scope, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "request":
		return ScopeRequest, true
	case "file":
		return ScopeFile, true
	case "global":
		return ScopeGlobal, true
	default:
		return ScopeRequest, false
	}
}

// Secret scopes carry a -secret suffix, as in @file-secret. secret comes back
// even for an unknown scope so the caller can word its own error.
func ParseSecretScope(token string) (scope Scope, secret, ok bool) {
	tok := strings.ToLower(strings.TrimSpace(token))
	if base, cut := strings.CutSuffix(tok, "-secret"); cut {
		tok, secret = base, true
	}
	scope, ok = ParseScope(tok)
	return scope, secret, ok
}

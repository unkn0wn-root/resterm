package restfile

import (
	"fmt"
	"maps"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

// A value holds the form as it was written, so it may be an alias or a form
// resterm does not support.
type AuthKind string

const (
	AuthBasic   AuthKind = "basic"
	AuthBearer  AuthKind = "bearer"
	AuthAPIKey  AuthKind = "apikey"
	AuthHeader  AuthKind = "header"
	AuthCommand AuthKind = "command"
	AuthOAuth2  AuthKind = "oauth2"
)

var authAliases = map[AuthKind]AuthKind{"api-key": AuthAPIKey}

const AuthDisableWord = "none"

// AuthHeader is absent because it has no keyword of its own. It is what the
// first word falls back to, which is why a header may be named "header".
var authKeywords = map[AuthKind]struct{}{
	AuthBasic:   {},
	AuthBearer:  {},
	AuthAPIKey:  {},
	AuthCommand: {},
	AuthOAuth2:  {},
}

// A custom header named after a scope, the disable switch, or a type has no
// bare "<header> <value>" form. Reading it back would take the name for one of
// those instead.
func ReservedAuthWord(word string) bool {
	word = strings.TrimSpace(word)
	if strings.EqualFold(word, AuthDisableWord) {
		return true
	}
	if _, ok := directive.ParseScope(word); ok {
		return true
	}
	_, ok := authKeywords[AuthKind(word).Canonical()]
	return ok
}

func (a *AuthSpec) Kind() AuthKind {
	if a == nil {
		return ""
	}
	return a.Type.Canonical()
}

// Normalizing on read rather than at construction leaves Type as it was
// written, which the writer reports when it cannot write a form. Nothing
// guarantees a hand-built AuthSpec is normalized.
func (k AuthKind) Canonical() AuthKind {
	k = AuthKind(strings.ToLower(strings.TrimSpace(string(k))))
	if alias, ok := authAliases[k]; ok {
		return alias
	}
	return k
}

func (k AuthKind) String() string { return string(k) }

func (a *AuthSpec) Clone() *AuthSpec {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Params = cloneAuthParams(a.Params)
	return &cp
}

func (a *AuthSpec) Origin() string {
	if a == nil {
		return ""
	}
	return origin(a.SourcePath, a.Line)
}

func origin(path string, line int) string {
	switch {
	case path != "" && line > 0:
		return fmt.Sprintf("%s:%d", path, line)
	case path != "":
		return path
	case line > 0:
		return fmt.Sprintf("line %d", line)
	default:
		return ""
	}
}

func cloneAuthParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

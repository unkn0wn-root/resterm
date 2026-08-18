package vars

import (
	"os"
	"strings"
)

// EnvRefKey parses an env: reference. An empty name is still a reference.
func EnvRefKey(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 4 || !strings.EqualFold(trimmed[:4], "env:") {
		return "", false
	}
	return strings.TrimSpace(trimmed[4:]), true
}

// EnvRefs caches OS environment lookups for one run and records their values
// for redaction.
type EnvRefs struct {
	seen    map[string]Value
	named   map[string]Value
	secrets Secrets
	// names excludes runtime data so a response cannot select an OS variable.
	names    *Resolver
	withheld bool
}

// NewEnvRefs starts a per-run snapshot.
func NewEnvRefs(names *Resolver) *EnvRefs {
	return &EnvRefs{names: names}
}

// Withheld hides referenced values while preserving their precedence.
func (s *EnvRefs) Withheld() *EnvRefs {
	return &EnvRefs{withheld: true}
}

// Declared resolves env: references found in configuration and request files.
// Runtime values must bypass it so response data cannot select an OS variable.
func (s *EnvRefs) Declared(text string) Value {
	ref, ok := EnvRefKey(text)
	switch {
	case !ok:
		return Value{Text: text}
	case s.withheld:
		return Value{Missing: true}
	case ref == "":
		return Value{Missing: true}
	case !HasPlaceholder(ref):
		return s.Resolve(ref)
	}

	if v, ok := s.named[ref]; ok {
		return v
	}
	v := Value{Missing: true}
	if s.names != nil {
		if expanded, err := s.names.ExpandTemplatesStatic(ref); err == nil {
			v = s.Resolve(expanded)
		}
	}
	if s.named == nil {
		s.named = make(map[string]Value)
	}
	s.named[ref] = v
	return v
}

// Resolve reads an OS environment variable at most once.
func (s *EnvRefs) Resolve(key string) Value {
	if s.withheld {
		return Value{Missing: true}
	}
	if v, ok := s.seen[key]; ok {
		return v
	}
	v := Value{Missing: true}
	if text, ok := lookupEnv(key); ok {
		v = Value{Text: text, Final: true}
		s.secrets.Add(text)
	}
	if s.seen == nil {
		s.seen = make(map[string]Value)
	}
	s.seen[key] = v
	return v
}

// Secrets returns every OS value read by this snapshot.
func (s *EnvRefs) Secrets() []string {
	return s.secrets.Values()
}

// lookupEnv tries the key as-is first, then uppercased, so lowercase variable
// names can match conventional uppercase OS environment variables.
func lookupEnv(key string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}
	return os.LookupEnv(strings.ToUpper(key))
}

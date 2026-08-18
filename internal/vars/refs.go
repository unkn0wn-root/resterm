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

type EnvRefs struct {
	seen    map[string]Value
	named   map[string]Value
	secrets Secrets
	// names excludes runtime data so a response cannot select an OS variable.
	names    *Resolver
	withheld bool
}

func NewEnvRefs(names *Resolver) *EnvRefs {
	return &EnvRefs{names: names}
}

// withhold hides referenced values but retains them for redaction.
func (s *EnvRefs) withhold() *EnvRefs {
	if s == nil {
		return &EnvRefs{withheld: true}
	}
	return &EnvRefs{secrets: s.secrets, withheld: true}
}

// ResolveDeclared resolves env: references authored in configuration or request
// files. Runtime values must not call it or they could select an OS variable.
func (s *EnvRefs) ResolveDeclared(text string) Value {
	ref, ok := EnvRefKey(text)
	if !ok {
		return Value{Text: text}
	}
	if s == nil || s.withheld || ref == "" {
		return Value{Missing: true}
	}
	if !HasPlaceholder(ref) {
		return s.resolve(ref)
	}
	return s.resolveNamed(ref)
}

func (s *EnvRefs) resolveNamed(ref string) Value {
	if v, ok := s.named[ref]; ok {
		return v
	}
	v := Value{Missing: true}
	if s.names != nil {
		if expanded, err := s.names.ExpandTemplatesStatic(ref); err == nil {
			v = s.resolve(expanded)
		}
	}
	if s.named == nil {
		s.named = make(map[string]Value)
	}
	s.named[ref] = v
	return v
}

func (s *EnvRefs) resolve(key string) Value {
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
	if s == nil {
		return nil
	}
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

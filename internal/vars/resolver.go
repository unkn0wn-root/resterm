package vars

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"
)

type Provider interface {
	Resolve(name string) (string, bool)
	Label() string
}

type Resolver struct {
	providers []Provider
}

func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers}
}

func (r *Resolver) Resolve(name string) (string, bool) {
	for _, provider := range r.providers {
		if value, ok := provider.Resolve(name); ok {
			return value, true
		}
	}
	return "", false
}

var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func (r *Resolver) ExpandTemplates(input string) (string, error) {
	var firstErr error
	result := templateVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		name := strings.TrimSpace(templateVarPattern.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}
		if strings.HasPrefix(name, "$") {
			if dynamic, ok := resolveDynamic(name); ok {
				return dynamic
			}
		}
		if value, ok := r.Resolve(name); ok {
			return value
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("undefined variable: %s", name)
		}
		return match
	})
	return result, firstErr
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	var firstErr error
	result := templateVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		submatches := templateVarPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		name := strings.TrimSpace(submatches[1])
		if name == "" {
			return match
		}
		if value, ok := r.Resolve(name); ok {
			return value
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("undefined variable: %s", name)
		}
		return match
	})
	return result, firstErr
}

func resolveDynamic(name string) (string, bool) {
	switch name {
	case "$timestamp":
		return fmt.Sprintf("%d", time.Now().Unix()), true
	case "$timestampISO8601":
		return time.Now().UTC().Format(time.RFC3339), true
	case "$randomInt":
		n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		return n.String(), true
	case "$uuid":
		return generateUUID(), true
	default:
		return "", false
	}
}

type MapProvider struct {
	values map[string]string
	label  string
}

func NewMapProvider(label string, values map[string]string) Provider {
	normalized := make(map[string]string, len(values))
	for k, v := range values {
		normalized[strings.ToLower(k)] = v
	}
	return &MapProvider{values: normalized, label: label}
}

func (p *MapProvider) Resolve(name string) (string, bool) {
	value, ok := p.values[strings.ToLower(name)]
	return value, ok
}

func (p *MapProvider) Label() string {
	return p.label
}

type EnvProvider struct{}

func (EnvProvider) Resolve(name string) (string, bool) {
	if value, ok := os.LookupEnv(name); ok {
		return value, true
	}
	return os.LookupEnv(strings.ToUpper(name))
}

func (EnvProvider) Label() string {
	return "env"
}

func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

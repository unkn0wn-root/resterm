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

type ExprPos struct {
	Path string
	Line int
	Col  int
}

type ExprEval func(expr string, pos ExprPos) (string, error)

type Resolver struct {
	providers []Provider
	expr      ExprEval
	exprPos   ExprPos
}

func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers}
}

// First tries direct lookup across all providers.
// If that fails and the name has a dot, tries to match a provider prefix -
// so "production.api_key" looks for a provider labeled "production" then asks for "api_key".
func (r *Resolver) Resolve(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", false
	}
	for _, provider := range r.providers {
		if value, ok := provider.Resolve(trimmed); ok {
			return value, true
		}
	}
	if !strings.Contains(trimmed, ".") {
		return "", false
	}
	lowered := strings.ToLower(trimmed)
	for _, provider := range r.providers {
		label := strings.TrimSpace(provider.Label())
		if label == "" {
			continue
		}
		labelLower := strings.ToLower(label)
		if idx := strings.Index(labelLower, ":"); idx >= 0 {
			labelLower = strings.TrimSpace(labelLower[:idx])
		}
		if labelLower == "" {
			continue
		}
		if strings.HasPrefix(lowered, labelLower+".") {
			subject := strings.TrimSpace(trimmed[len(labelLower)+1:])
			if subject == "" {
				continue
			}
			if value, ok := provider.Resolve(subject); ok {
				return value, true
			}
		}
	}
	return "", false
}

var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func (r *Resolver) ExpandTemplates(input string) (string, error) {
	return r.expandTemplates(input, r.exprPos, true, true)
}

func (r *Resolver) ExpandTemplatesAt(input string, pos ExprPos) (string, error) {
	return r.expandTemplates(input, pos, true, true)
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	return r.expandTemplates(input, r.exprPos, false, false)
}

func (r *Resolver) SetExprEval(fn ExprEval) {
	r.expr = fn
}

func (r *Resolver) SetExprPos(pos ExprPos) {
	r.exprPos = pos
}

func (r *Resolver) expandTemplates(
	input string,
	pos ExprPos,
	allowDynamic, allowExpr bool,
) (string, error) {
	var firstErr error
	result := templateVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		sub := templateVarPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := strings.TrimSpace(sub[1])
		if name == "" {
			return match
		}
		if strings.HasPrefix(name, "=") {
			if !allowExpr {
				if firstErr == nil {
					firstErr = fmt.Errorf("expressions not allowed")
				}
				return match
			}
			expr := strings.TrimSpace(name[1:])
			if expr == "" {
				if firstErr == nil {
					firstErr = fmt.Errorf("empty expression")
				}
				return match
			}
			if r.expr == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("expressions not enabled")
				}
				return match
			}
			val, err := r.expr(expr, pos)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return val
		}
		if allowDynamic && strings.HasPrefix(name, "$") {
			if value, ok := r.Resolve(name); ok {
				return value
			}
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

func resolveDynamic(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "$timestamp":
		return fmt.Sprintf("%d", time.Now().Unix()), true
	case "$timestampiso8601":
		return time.Now().UTC().Format(time.RFC3339), true
	case "$randomint":
		n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		return n.String(), true
	case "$uuid", "$guid":
		return generateUUID(), true
	default:
		return "", false
	}
}

type MapProvider struct {
	values map[string]string
	label  string
}

// Keys get lowercased so lookups are case-insensitive
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

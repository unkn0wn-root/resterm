package vars

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unkn0wn-root/resterm/internal/duration"
)

var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

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
	refs      []RefResolver
	expr      ExprEval
	exprPos   ExprPos
	trace     *Trace
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
	if r.trace == nil {
		for _, provider := range r.providers {
			if value, ok := provider.Resolve(trimmed); ok {
				return r.applyRefs(value)
			}
		}
	} else {
		var hits []string
		var raw string
		for _, provider := range r.providers {
			if value, ok := provider.Resolve(trimmed); ok {
				label := providerLabel(provider)
				hits = append(hits, label)
				if len(hits) == 1 {
					raw = value
				}
			}
		}
		if len(hits) > 0 {
			resolved, found := r.applyRefs(raw)
			r.traceVar(ResolveTrace{
				Name:     trimmed,
				Source:   hits[0],
				Value:    resolved,
				Shadowed: hits[1:],
				Uses:     1,
			})
			return resolved, found
		}
	}
	if !strings.Contains(trimmed, ".") {
		return "", false
	}

	lowered := strings.ToLower(trimmed)
	var hits []string
	var raw string
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
				if r.trace == nil {
					return r.applyRefs(value)
				}
				label = providerLabel(provider)
				hits = append(hits, label)
				if len(hits) == 1 {
					raw = value
				}
			}
		}
	}
	if len(hits) > 0 {
		resolved, found := r.applyRefs(raw)
		r.traceVar(ResolveTrace{
			Name:     trimmed,
			Source:   hits[0],
			Value:    resolved,
			Shadowed: hits[1:],
			Uses:     1,
		})
		return resolved, found
	}
	return "", false
}

// applyRefs runs the value through registered ref resolvers. The first
// resolver that claims the value (handled==true) wins. If no resolver
// handles the value it is returned as-is.
func (r *Resolver) applyRefs(value string) (string, bool) {
	for _, ref := range r.refs {
		resolved, handled, found := ref(value)
		if handled {
			return resolved, found
		}
	}
	return value, true
}

func providerLabel(p Provider) string {
	if p == nil {
		return ""
	}
	label := strings.TrimSpace(p.Label())
	if label == "" {
		return "provider"
	}
	return label
}

func (r *Resolver) ExpandTemplates(input string) (string, error) {
	return r.expandTemplates(input, r.exprPos, true, true)
}

func (r *Resolver) ExpandTemplatesAt(input string, pos ExprPos) (string, error) {
	return r.expandTemplates(input, pos, true, true)
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	return r.expandTemplates(input, r.exprPos, false, false)
}

func (r *Resolver) AddRefResolver(fn RefResolver) {
	r.refs = append(r.refs, fn)
}

func (r *Resolver) SetTrace(tr *Trace) {
	r.trace = tr
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
	result := ReplaceTemplateVars(input, func(match, name string) string {
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
				r.traceVar(ResolveTrace{
					Name:    name,
					Source:  "dynamic",
					Value:   dynamic,
					Dynamic: true,
					Uses:    1,
				})
				return dynamic
			}
		}
		if value, ok := r.Resolve(name); ok {
			return value
		}
		r.traceVar(ResolveTrace{Name: name, Missing: true, Uses: 1})
		if firstErr == nil {
			firstErr = fmt.Errorf("undefined variable: %s", name)
		}
		return match
	})
	return result, firstErr
}

func resolveDynamic(name string) (string, bool) {
	if base, offset, ok := splitDynamicOffset(name); ok {
		return resolveDynamicBase(base, offset)
	}
	return resolveDynamicBase(name, 0)
}

func (r *Resolver) traceVar(it ResolveTrace) {
	if r == nil || r.trace == nil {
		return
	}
	r.trace.Add(it)
}

func resolveDynamicBase(name string, offset time.Duration) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "$timestamp", "$timestampiso8601", "$timestampms":
		t := time.Now().Add(offset)
		switch lower {
		case "$timestamp":
			return fmt.Sprintf("%d", t.Unix()), true
		case "$timestampms":
			return fmt.Sprintf("%d", t.UnixNano()/int64(time.Millisecond)), true
		default:
			return t.UTC().Format(time.RFC3339), true
		}
	case "$randomint":
		if offset != 0 {
			return "", false
		}
		n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
		if err != nil {
			return "", false
		}
		return n.String(), true
	case "$uuid", "$guid":
		if offset != 0 {
			return "", false
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return "", false
		}
		return id.String(), true
	default:
		return "", false
	}
}

// splits "$helper +/- duration" into base name and signed offset.
// "$timestampISO8601 - 90m" -> base "$timestampISO8601", offset -90m.
func splitDynamicOffset(name string) (string, time.Duration, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", 0, false
	}
	for opIdx := len(trimmed) - 1; opIdx > 0; opIdx-- {
		ch := trimmed[opIdx]
		if ch != '+' && ch != '-' {
			continue
		}
		base := strings.TrimSpace(trimmed[:opIdx])
		if base == "" {
			continue
		}
		raw := strings.TrimSpace(trimmed[opIdx+1:])
		if raw == "" {
			continue
		}
		dur, ok := duration.Parse(raw)
		if !ok {
			continue
		}
		if ch == '-' {
			dur = -dur
		}
		return base, dur, true
	}
	return "", 0, false
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

func ReplaceTemplateVars(input string, fn func(match, name string) string) string {
	if fn == nil {
		return input
	}
	return templateVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		sub := templateVarPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return fn(match, strings.TrimSpace(sub[1]))
	})
}

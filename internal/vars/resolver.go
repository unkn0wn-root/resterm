package vars

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unkn0wn-root/resterm/internal/duration"
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
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}

	raw, sources, ok := r.lookup(func(p Provider) (string, bool) { return p.Resolve(name) })
	if !ok && strings.Contains(name, ".") {
		raw, sources, ok = r.lookup(func(p Provider) (string, bool) { return lookupPrefixed(p, name) })
	}
	if !ok {
		return "", false
	}

	resolved, found := r.applyRefs(raw)
	if len(sources) > 0 {
		r.traceVar(ResolveTrace{
			Name:     name,
			Source:   sources[0],
			Value:    resolved,
			Shadowed: sources[1:],
			Uses:     1,
			Missing:  !found,
		})
	}
	return resolved, found
}

// lookup returns the first value find yields across providers. Without tracing
// it stops at the first hit. With tracing it keeps scanning and collects every
// matching provider label so shadowed sources can be reported.
func (r *Resolver) lookup(find func(Provider) (string, bool)) (string, []string, bool) {
	var raw string
	var sources []string
	hit := false
	for _, p := range r.providers {
		value, ok := find(p)
		if !ok {
			continue
		}
		if !hit {
			raw, hit = value, true
			if r.trace == nil {
				return raw, nil, true
			}
		}
		sources = append(sources, providerLabel(p))
	}
	return raw, sources, hit
}

// lookupPrefixed matches "label.name" against the provider label, with any
// ":detail" suffix stripped, and resolves the remainder against that provider.
func lookupPrefixed(p Provider, name string) (string, bool) {
	label := strings.TrimSpace(strings.ToLower(p.Label()))
	if before, _, ok := strings.Cut(label, ":"); ok {
		label = strings.TrimSpace(before)
	}
	if label == "" || !strings.HasPrefix(strings.ToLower(name), label+".") {
		return "", false
	}
	subject := strings.TrimSpace(name[len(label)+1:])
	if subject == "" {
		return "", false
	}
	return p.Resolve(subject)
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
	return CompileTemplate(input).render(r, r.exprPos, true, true)
}

func (r *Resolver) ExpandTemplatesAt(input string, pos ExprPos) (string, error) {
	return CompileTemplate(input).render(r, pos, true, true)
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	return CompileTemplate(input).render(r, r.exprPos, false, false)
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

// resolveName resolves one placeholder name. A non-nil error means the
// placeholder cannot be resolved and callers keep it as literal text.
func (r *Resolver) resolveName(name string, pos ExprPos, allowDynamic, allowExpr bool) (string, error) {
	if strings.HasPrefix(name, "=") {
		if !allowExpr {
			return "", fmt.Errorf("expressions not allowed")
		}
		expr := strings.TrimSpace(name[1:])
		if expr == "" {
			return "", fmt.Errorf("empty expression")
		}
		if r.expr == nil {
			return "", fmt.Errorf("expressions not enabled")
		}
		return r.expr(expr, pos)
	}
	if allowDynamic && strings.HasPrefix(name, "$") {
		if value, ok := r.Resolve(name); ok {
			return value, nil
		}
		if dynamic, ok := resolveDynamic(name); ok {
			r.traceVar(ResolveTrace{
				Name:    name,
				Source:  "dynamic",
				Value:   dynamic,
				Dynamic: true,
				Uses:    1,
			})
			return dynamic, nil
		}
	}
	if value, ok := r.Resolve(name); ok {
		return value, nil
	}
	r.traceVar(ResolveTrace{Name: name, Missing: true, Uses: 1})
	return "", fmt.Errorf("undefined variable: %s", name)
}

func resolveDynamic(name string) (string, bool) {
	if base, offset, ok := splitDynamicOffset(name); ok {
		return resolveDynamicBase(base, offset)
	}
	return resolveDynamicBase(name, 0)
}

// IsDynamic reports whether name identifies a supported dynamic template helper.
// It defers to resolveDynamic so support and validation share one definition; the
// generated value is discarded, which is why callers use it only at compile time.
func IsDynamic(name string) bool {
	_, ok := resolveDynamic(name)
	return ok
}

func (r *Resolver) traceVar(it ResolveTrace) {
	if r.trace == nil {
		return
	}
	r.trace.Add(it)
}

func resolveDynamicBase(name string, offset time.Duration) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "$timestamp":
		return strconv.FormatInt(time.Now().Add(offset).Unix(), 10), true
	case "$timestampms":
		return strconv.FormatInt(time.Now().Add(offset).UnixMilli(), 10), true
	case "$timestampiso8601":
		return time.Now().Add(offset).UTC().Format(time.RFC3339), true
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
	return lookupEnv(name)
}

func (EnvProvider) Label() string {
	return "env"
}

// ReplaceTemplateVars rewrites every {{...}} in input through fn. The
// callback gets the raw match and the trimmed name, which is empty for a
// blank {{ }}.
func ReplaceTemplateVars(input string, fn func(match, name string) string) string {
	if fn == nil {
		return input
	}
	return CompileTemplate(input).replace(fn)
}

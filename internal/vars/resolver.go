package vars

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
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
	lenient   bool
	memo      *memoStore
}

// memoStore lives behind a pointer so derived resolvers share pinned values
// without copying the lock.
type memoStore struct {
	mu      sync.Mutex
	entries map[memoKey]memoEntry
}

// memoEntry pins the first expansion of a template-valued variable so every
// later reference in the same resolver sees the same value, which is what
// makes a declared {{$uuid}} stable across one request execution.
type memoEntry struct {
	value string
	found bool
}

// variableKey identifies the declaration selected by provider precedence.
// Keeping the provider index prevents a qualified lower-precedence variable
// from sharing a cached value with an unqualified higher-precedence variable.
type variableKey struct {
	provider int
	name     string
}

type memoKey struct {
	variable     variableKey
	allowDynamic bool
	allowExpr    bool
}

const maxExpandDepth = 16

// expandState tracks the chain of variables being expanded in one render so
// cycles and runaway nesting fail with the full reference chain.
type expandState struct {
	names map[variableKey]bool
	stack []string
}

func NewResolver(providers ...Provider) *Resolver {
	return &Resolver{providers: providers, memo: &memoStore{}}
}

// Lenient returns a resolver whose top level expansion never fails.
// Unresolved placeholders stay literal and the trace still records the
// missing names. Preview rendering uses it so display does not depend on
// every variable resolving.
func (r *Resolver) Lenient() *Resolver {
	if r == nil || r.lenient {
		return r
	}
	cp := *r
	cp.lenient = true
	return &cp
}

func (r *Resolver) Resolve(name string) (string, bool) {
	value, ok, err := r.resolve(name, r.exprPos, true, true, nil)
	if err != nil {
		return "", false
	}
	return value, ok
}

// resolve looks the name up and, when the winning provider holds authored
// template text, expands nested placeholders before refs apply. A non-nil
// error means expansion failed (cycle, depth, or an unresolvable nested
// placeholder) and the value is unusable.
func (r *Resolver) resolve(
	name string,
	pos ExprPos,
	allowDynamic, allowExpr bool,
	st *expandState,
) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, nil
	}

	hit, ok := r.lookupValue(name)
	if !ok {
		return "", false, nil
	}

	var resolved string
	var found bool
	if expandableProvider(hit.prov) && strings.Contains(hit.raw, "{{") {
		variable := hit.key()
		if err := checkExpandState(name, variable, st); err != nil {
			return "", false, err
		}

		key := memoKey{
			variable:     variable,
			allowDynamic: allowDynamic,
			allowExpr:    allowExpr,
		}
		entry, cached := r.memoized(key)
		if !cached {
			expanded, err := r.expandValue(name, hit.raw, variable, pos, allowDynamic, allowExpr, st)
			if err != nil {
				return "", false, err
			}
			value, valueFound := r.applyRefs(expanded)
			entry = r.memoize(key, memoEntry{value: value, found: valueFound})
		}
		resolved, found = entry.value, entry.found
	} else {
		resolved, found = r.applyRefs(hit.raw)
	}

	if len(hit.sources) > 0 {
		r.traceVar(ResolveTrace{
			Name:     name,
			Source:   hit.sources[0],
			Value:    resolved,
			Shadowed: hit.sources[1:],
			Uses:     1,
			Missing:  !found,
		})
	}
	return resolved, found, nil
}

func (r *Resolver) expandValue(
	name, raw string,
	key variableKey,
	pos ExprPos,
	allowDynamic, allowExpr bool,
	st *expandState,
) (string, error) {
	if st == nil {
		st = &expandState{names: make(map[variableKey]bool)}
	}

	st.names[key] = true
	st.stack = append(st.stack, name)
	out, err := CompileTemplate(raw).render(r, pos, allowDynamic, allowExpr, st)
	st.stack = st.stack[:len(st.stack)-1]
	delete(st.names, key)

	if err != nil {
		return "", fmt.Errorf("expand %s: %w", name, err)
	}
	return out, nil
}

func checkExpandState(name string, key variableKey, st *expandState) error {
	if st == nil {
		return nil
	}
	chain := strings.Join(append(st.stack, name), " -> ")
	if st.names[key] {
		return fmt.Errorf("variable cycle: %s", chain)
	}
	if len(st.stack) >= maxExpandDepth {
		return fmt.Errorf("variable nesting deeper than %d: %s", maxExpandDepth, chain)
	}
	return nil
}

func (r *Resolver) memoized(key memoKey) (memoEntry, bool) {
	r.memo.mu.Lock()
	defer r.memo.mu.Unlock()
	entry, ok := r.memo.entries[key]
	return entry, ok
}

// memoize returns the first value stored for key. Multiple goroutines may do
// the expansion work concurrently, but every caller observes the same winner.
// Avoiding a wait on another variable's expansion also keeps concurrent roots
// of a cyclic graph from deadlocking each other.
func (r *Resolver) memoize(key memoKey, candidate memoEntry) memoEntry {
	r.memo.mu.Lock()
	defer r.memo.mu.Unlock()
	if r.memo.entries == nil {
		r.memo.entries = make(map[memoKey]memoEntry)
	}
	if entry, ok := r.memo.entries[key]; ok {
		return entry
	}
	r.memo.entries[key] = candidate
	return candidate
}

// lookupHit is the provider match a lookup selected. subject is the name the
// winning provider actually resolved, which differs from the requested name
// for prefixed lookups.
type lookupHit struct {
	raw     string
	prov    Provider
	idx     int
	subject string
	sources []string
}

func (h lookupHit) key() variableKey {
	return variableKey{provider: h.idx, name: strings.ToLower(h.subject)}
}

// lookupValue first tries direct lookup across all providers.
// If that fails and the name has a dot, tries to match a provider prefix -
// so "production.api_key" looks for a provider labeled "production" then asks for "api_key".
func (r *Resolver) lookupValue(name string) (lookupHit, bool) {
	hit, ok := r.lookup(func(p Provider) (string, string, bool) {
		value, found := p.Resolve(name)
		return value, name, found
	})
	if !ok && strings.Contains(name, ".") {
		hit, ok = r.lookup(func(p Provider) (string, string, bool) {
			return lookupPrefixed(p, name)
		})
	}
	return hit, ok
}

// lookup returns the first value find yields across providers. Without tracing
// it stops at the first hit. With tracing it keeps scanning and collects every
// matching provider label so shadowed sources can be reported.
func (r *Resolver) lookup(find func(Provider) (string, string, bool)) (lookupHit, bool) {
	var hit lookupHit
	matched := false
	for idx, p := range r.providers {
		value, subject, ok := find(p)
		if !ok {
			continue
		}
		if !matched {
			hit = lookupHit{raw: value, prov: p, idx: idx, subject: subject}
			matched = true
			if r.trace == nil {
				return hit, true
			}
		}
		hit.sources = append(hit.sources, providerLabel(p))
	}
	return hit, matched
}

// lookupPrefixed matches "label.name" against the provider label, with any
// ":detail" suffix stripped, and resolves the remainder against that provider.
func lookupPrefixed(p Provider, name string) (string, string, bool) {
	label := strings.TrimSpace(strings.ToLower(p.Label()))
	if before, _, ok := strings.Cut(label, ":"); ok {
		label = strings.TrimSpace(before)
	}
	if label == "" || !strings.HasPrefix(strings.ToLower(name), label+".") {
		return "", "", false
	}
	subject := strings.TrimSpace(name[len(label)+1:])
	if subject == "" {
		return "", "", false
	}
	value, ok := p.Resolve(subject)
	return value, subject, ok
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
	return CompileTemplate(input).render(r, r.exprPos, true, true, nil)
}

func (r *Resolver) ExpandTemplatesAt(input string, pos ExprPos) (string, error) {
	return CompileTemplate(input).render(r, pos, true, true, nil)
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	return CompileTemplate(input).render(r, r.exprPos, false, false, nil)
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

// ErrUndefinedVariable marks names no provider could resolve. Lenient
// rendering suppresses exactly this class since the trace records the name
// as missing. Every other error still fails.
var ErrUndefinedVariable = errors.New("undefined variable")

// resolveName resolves one placeholder name. A non-nil error means the
// placeholder cannot be resolved and callers keep it as literal text.
func (r *Resolver) resolveName(
	name string,
	pos ExprPos,
	allowDynamic, allowExpr bool,
	st *expandState,
) (string, error) {
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
		value, ok, err := r.resolve(name, pos, allowDynamic, allowExpr, st)
		if err != nil {
			return "", err
		}
		if ok {
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
	value, ok, err := r.resolve(name, pos, allowDynamic, allowExpr, st)
	if err != nil {
		return "", err
	}
	if ok {
		return value, nil
	}
	r.traceVar(ResolveTrace{Name: name, Missing: true, Uses: 1})
	return "", fmt.Errorf("%w: %s", ErrUndefinedVariable, name)
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
	values   map[string]string
	label    string
	template bool
}

// Keys get lowercased so lookups are case-insensitive
func NewMapProvider(label string, values map[string]string) Provider {
	return newMapProvider(label, values, false)
}

// NewTemplateProvider is a MapProvider whose values are authored template
// text, so a value containing {{...}} is expanded when the variable resolves.
// Use it only for values written in source files, never for runtime data such
// as captured responses.
func NewTemplateProvider(label string, values map[string]string) Provider {
	return newMapProvider(label, values, true)
}

func newMapProvider(label string, values map[string]string, template bool) Provider {
	normalized := make(map[string]string, len(values))
	for k, v := range values {
		normalized[strings.ToLower(k)] = v
	}
	return &MapProvider{values: normalized, label: label, template: template}
}

type templateValueProvider interface {
	templateValues() bool
}

func (p *MapProvider) templateValues() bool {
	return p.template
}

func expandableProvider(p Provider) bool {
	tp, ok := p.(templateValueProvider)
	return ok && tp.templateValues()
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

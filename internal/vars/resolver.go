package vars

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/vars/dynamic"
)

type Provider interface {
	Resolve(name string) (string, bool)
	Label() string
}

// Value is a provider result. Its zero value is ordinary empty text.
type Value struct {
	Text string
	// Missing claims the name without allowing fallback to another provider.
	Missing bool
	// Final prevents another round of template expansion.
	Final bool
}

// ValueProvider returns structured provider results.
type ValueProvider interface {
	Provider
	ResolveValue(name string) (Value, bool)
}

func providerValue(p Provider, name string) (Value, bool) {
	if vp, ok := p.(ValueProvider); ok {
		return vp.ResolveValue(name)
	}
	text, ok := p.Resolve(name)
	return Value{Text: text}, ok
}

type ExprPos struct {
	Path string
	Line int
	Col  int
}

type ExprEval func(expr string, pos ExprPos) (string, error)

// Expansion is the result of rendering one template input. Lenient rendering
// preserves undefined placeholders in Value and reports their presence here.
type Expansion struct {
	Value                 string
	HasUndefinedVariables bool
}

type Resolver struct {
	providers []Provider
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
	entries map[memoKey]string
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

// Lenient returns a resolver that suppresses undefined variable errors at
// the top level. Placeholders stay literal and the trace still records the
// missing names, while cycles, nesting depth, and expression failures keep
// erroring. Derive it only after the resolver is fully configured, since the
// copy does not see later setter calls.
func (r *Resolver) Lenient() *Resolver {
	if r == nil || r.lenient {
		return r
	}
	cp := *r
	cp.lenient = true
	return &cp
}

// WithExprEval returns a copy that uses fn for expression templates.
// It shares memoized values with the original but not later configuration changes.
func (r *Resolver) WithExprEval(fn ExprEval) *Resolver {
	if r == nil {
		return r
	}
	cp := *r
	cp.expr = fn
	return &cp
}

// WithProviders reuses memoized expansions with a new provider set. Providers
// must retain their order because memoized entries use provider positions.
func (r *Resolver) WithProviders(providers ...Provider) *Resolver {
	if r == nil {
		return r
	}
	cp := *r
	cp.providers = providers
	return &cp
}

func (r *Resolver) Resolve(name string) (string, bool) {
	value, ok, err := r.resolve(name, r.exprPos, true, true, nil)
	if err != nil {
		return "", false
	}
	return value, ok
}

// A missing result still wins provider lookup, preventing fallthrough.
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
	if hit.val.Missing {
		r.traceHit(name, hit, "", true)
		return "", false, nil
	}

	resolved := hit.val.Text
	if !hit.val.Final && expandableProvider(hit.prov) && HasPlaceholder(resolved) {
		variable := hit.key()
		if err := checkExpandState(name, variable, st); err != nil {
			return "", false, err
		}

		key := memoKey{
			variable:     variable,
			allowDynamic: allowDynamic,
			allowExpr:    allowExpr,
		}
		value, cached := r.memoized(key)
		if !cached {
			expanded, err := r.expandValue(name, resolved, variable, pos, allowDynamic, allowExpr, st)
			if err != nil {
				return "", false, err
			}
			value = r.memoize(key, expanded)
		}
		resolved = value
	}

	r.traceHit(name, hit, resolved, false)
	return resolved, true, nil
}

func (r *Resolver) traceHit(name string, hit lookupHit, value string, missing bool) {
	if len(hit.sources) == 0 {
		return
	}
	r.traceVar(ResolveTrace{
		Name:     name,
		Source:   hit.sources[0],
		Value:    value,
		Shadowed: hit.sources[1:],
		Uses:     1,
		Missing:  missing,
	})
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

func (r *Resolver) memoized(key memoKey) (string, bool) {
	r.memo.mu.Lock()
	defer r.memo.mu.Unlock()
	value, ok := r.memo.entries[key]
	return value, ok
}

// memoize returns the first value stored for key. Multiple goroutines may do
// the expansion work concurrently, but every caller observes the same winner.
// Avoiding a wait on another variable's expansion also keeps concurrent roots
// of a cyclic graph from deadlocking each other.
func (r *Resolver) memoize(key memoKey, candidate string) string {
	r.memo.mu.Lock()
	defer r.memo.mu.Unlock()
	if r.memo.entries == nil {
		r.memo.entries = make(map[memoKey]string)
	}
	if value, ok := r.memo.entries[key]; ok {
		return value
	}
	r.memo.entries[key] = candidate
	return candidate
}

// lookupHit is the provider match a lookup selected. subject is the name the
// winning provider actually resolved, which differs from the requested name
// for prefixed lookups.
type lookupHit struct {
	val     Value
	prov    Provider
	idx     int
	subject string
	sources []string
}

func (h lookupHit) key() variableKey {
	return variableKey{provider: h.idx, name: NameKey(h.subject)}
}

// lookupValue first tries direct lookup across all providers.
// If that fails and the name has a dot, tries to match a provider prefix -
// so "production.api_key" looks for a provider labeled "production" then asks for "api_key".
func (r *Resolver) lookupValue(name string) (lookupHit, bool) {
	hit, ok := r.lookup(func(p Provider) (Value, string, bool) {
		value, found := providerValue(p, name)
		return value, name, found
	})
	if !ok && strings.Contains(name, ".") {
		hit, ok = r.lookup(func(p Provider) (Value, string, bool) {
			return lookupPrefixed(p, name)
		})
	}
	return hit, ok
}

// lookup returns the first value find yields across providers. Without tracing
// it stops at the first hit. With tracing it keeps scanning and collects every
// matching provider label so shadowed sources can be reported.
func (r *Resolver) lookup(find func(Provider) (Value, string, bool)) (lookupHit, bool) {
	var hit lookupHit
	matched := false
	for idx, p := range r.providers {
		value, subject, ok := find(p)
		if !ok {
			continue
		}
		if !matched {
			hit = lookupHit{val: value, prov: p, idx: idx, subject: subject}
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
func lookupPrefixed(p Provider, name string) (Value, string, bool) {
	label := strings.TrimSpace(strings.ToLower(p.Label()))
	if before, _, ok := strings.Cut(label, ":"); ok {
		label = strings.TrimSpace(before)
	}
	if label == "" || !strings.HasPrefix(strings.ToLower(name), label+".") {
		return Value{}, "", false
	}
	subject := strings.TrimSpace(name[len(label)+1:])
	if subject == "" {
		return Value{}, "", false
	}
	value, ok := providerValue(p, subject)
	return value, subject, ok
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

// ExpandTemplatesResult expands input and reports whether resolution encountered
// any undefined variables, including when a lenient resolver suppresses the error.
func (r *Resolver) ExpandTemplatesResult(input string) (Expansion, error) {
	return CompileTemplate(input).renderResult(r, r.exprPos, true, true, nil)
}

func (r *Resolver) ExpandTemplatesAt(input string, pos ExprPos) (string, error) {
	return CompileTemplate(input).render(r, pos, true, true, nil)
}

func (r *Resolver) ExpandTemplatesStatic(input string) (string, error) {
	return CompileTemplate(input).render(r, r.exprPos, false, false, nil)
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

// PreferStructural picks which expansion error to report when several occur.
// The first structural error wins, so an undefined variable seen earlier can
// never mask a cycle or broken expression seen later. Callers that classify
// on ErrUndefinedVariable depend on this order.
func PreferStructural(firstErr, err error) error {
	if firstErr == nil || (!errors.Is(err, ErrUndefinedVariable) && errors.Is(firstErr, ErrUndefinedVariable)) {
		return err
	}
	return firstErr
}

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
		// A name no helper claims falls through and is reported as undefined.
		// Anything else is a misused helper, worth saying so.
		built, err := dynamic.Resolve(name)
		if err == nil {
			r.traceVar(ResolveTrace{
				Name:    name,
				Source:  "dynamic",
				Value:   built,
				Dynamic: true,
				Uses:    1,
			})
			return built, nil
		}
		if !errors.Is(err, dynamic.ErrUnknown) {
			return "", err
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

func (r *Resolver) traceVar(it ResolveTrace) {
	if r.trace == nil {
		return
	}
	r.trace.Add(it)
}

type MapProvider struct {
	values   NameMap[string]
	label    string
	template bool
}

// NewMapProvider normalizes names case-insensitively. Equivalent keys resolve
// in deterministic name order.
func NewMapProvider(label string, values map[string]string) Provider {
	return newMapProvider(label, CollectNames(values), false)
}

// NewTemplateProvider is a MapProvider whose values are authored template
// text, so a value containing {{...}} is expanded when the variable resolves.
// Use it only for values written in source files, never for runtime data such
// as captured responses.
func NewTemplateProvider(label string, values map[string]string) Provider {
	return newMapProvider(label, CollectNames(values), true)
}

func newMapProvider(label string, values NameMap[string], template bool) Provider {
	return &MapProvider{values: values, label: label, template: template}
}

// ValueMapProvider is a provider backed by a NameMap of Values.
type ValueMapProvider struct {
	values   NameMap[Value]
	label    string
	template bool
}

// NewValueMapProvider wraps a NameMap without copying it.
func NewValueMapProvider(label string, values NameMap[Value]) Provider {
	return &ValueMapProvider{values: values, label: label}
}

// NewValueMapTemplateProvider enables template expansion for non-final values.
func NewValueMapTemplateProvider(label string, values NameMap[Value]) Provider {
	return &ValueMapProvider{values: values, label: label, template: true}
}

func (p *ValueMapProvider) ResolveValue(name string) (Value, bool) {
	return p.values.Get(name)
}

func (p *ValueMapProvider) Resolve(name string) (string, bool) {
	v, ok := p.values.Get(name)
	return v.Text, ok
}

func (p *ValueMapProvider) Label() string {
	return p.label
}

func (p *ValueMapProvider) templateValues() bool {
	return p.template
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
	return p.values.Get(name)
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

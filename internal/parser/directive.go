package parser

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// bindingKind names the directive slot a binding came from
type bindingKind string

const (
	useAlias    bindingKind = "@use alias"
	forEachName bindingKind = "@for-each name"
)

// checkRTSBinding validates a directive token that becomes an RTS binding. A
// reserved word looks like an identifier but lexes as a keyword, so the binding
// would be created and then be impossible to reference
func checkRTSBinding(kind bindingKind, name string) error {
	if !directive.IsIdent(name) {
		return fmt.Errorf("%s %q is invalid", kind, name)
	}
	if rts.IsKeyword(name) {
		return fmt.Errorf("%s %q is a reserved word", kind, name)
	}
	return nil
}

func parseApplySpec(rest string, line int) (restfile.ApplySpec, error) {
	raw := cutAssign(rest)
	if raw == "" {
		return restfile.ApplySpec{}, fmt.Errorf("@apply expression missing")
	}
	if namesProfiles(raw) {
		us, err := parseApplyUses(raw)
		if err != nil {
			return restfile.ApplySpec{}, err
		}
		return restfile.ApplySpec{
			Uses: us,
			Line: line,
			Col:  1,
		}, nil
	}
	return restfile.ApplySpec{
		Expression: raw,
		Line:       line,
		Col:        1,
	}, nil
}

func namesProfiles(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "use=")
}

func applyExpr(rest string) string {
	raw := cutAssign(rest)
	if namesProfiles(raw) {
		return ""
	}
	return raw
}

func cutAssign(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "="); ok {
		return strings.TrimSpace(after)
	}
	return s
}

func parseApplyUses(raw string) ([]string, error) {
	ps := strings.Split(raw, ",")
	us := make([]string, 0, len(ps))
	for _, p := range ps {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("@apply has an empty use token")
		}
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("@apply token %q must be use=<name>", p)
		}
		if !strings.EqualFold(strings.TrimSpace(k), "use") {
			return nil, fmt.Errorf("@apply token %q must be use=<name>", p)
		}
		n := strings.TrimSpace(directive.TrimQuotes(v))
		if !validProfileName(n) {
			return nil, fmt.Errorf("@apply use name %q is invalid", n)
		}
		us = append(us, n)
	}
	if len(us) == 0 {
		return nil, fmt.Errorf("@apply use= requires at least one profile name")
	}
	return us, nil
}

func parsePatchSpec(rest string, line int) (restfile.PatchProfile, error) {
	scTok, n, ex := cutPatch(rest)
	if scTok == "" {
		return restfile.PatchProfile{}, fmt.Errorf(
			"@patch requires '<scope> <name> <expression>'",
		)
	}
	sc, ok := parsePatchScope(scTok)
	if !ok {
		return restfile.PatchProfile{}, fmt.Errorf("@patch scope must be file or global")
	}
	if !validProfileName(n) {
		return restfile.PatchProfile{}, fmt.Errorf("@patch name %q is invalid", n)
	}
	if ex == "" {
		return restfile.PatchProfile{}, fmt.Errorf("@patch %q expression missing", n)
	}
	return restfile.PatchProfile{
		Scope:      sc,
		Name:       n,
		Expression: ex,
		Line:       line,
		Col:        1,
	}, nil
}

func cutPatch(rest string) (scope, name, expr string) {
	scope, rem := directive.CutToken(rest)
	name, rem = directive.CutToken(rem)
	return scope, name, cutAssign(rem)
}

// A patch is only useful where later requests can still see it.
func parsePatchScope(tok string) (directive.Scope, bool) {
	scope, ok := directive.ParseScope(tok)
	if !ok || scope == directive.ScopeRequest {
		return directive.ScopeRequest, false
	}
	return scope, true
}

func validProfileName(n string) bool {
	n = strings.TrimSpace(n)
	if n == "" {
		return false
	}
	for _, r := range n {
		if !directive.IsKeyRune(r) {
			return false
		}
	}
	return true
}

func parseUseSpec(rest string, line int) (restfile.UseSpec, error) {
	f := directive.Fields(rest)
	n := len(f)
	switch n {
	case 0:
		return restfile.UseSpec{}, fmt.Errorf("@use requires a path")
	case 1:
		p := strings.TrimSpace(f[0])
		if p == "" {
			return restfile.UseSpec{}, fmt.Errorf("@use requires a non-empty path")
		}
		return restfile.UseSpec{Path: p, Line: line}, nil
	case 2:
		if strings.EqualFold(f[1], "as") {
			return restfile.UseSpec{}, fmt.Errorf("@use requires an alias after 'as'")
		}
		return restfile.UseSpec{}, fmt.Errorf("@use must be '<path>' or '<path> as <alias>'")
	case 3:
		if !strings.EqualFold(f[1], "as") {
			return restfile.UseSpec{}, fmt.Errorf("@use must use 'as' to define an alias")
		}
		p := strings.TrimSpace(f[0])
		a := strings.TrimSpace(f[2])
		if p == "" || a == "" {
			return restfile.UseSpec{}, fmt.Errorf("@use requires a non-empty path and alias")
		}
		if err := checkRTSBinding(useAlias, a); err != nil {
			return restfile.UseSpec{}, err
		}
		return restfile.UseSpec{
			Path:  p,
			Alias: a,
			Line:  line,
		}, nil
	default:
		return restfile.UseSpec{}, fmt.Errorf("@use has too many tokens")
	}
}

func parseConditionSpec(rest string, line int, negate bool) (*restfile.ConditionSpec, error) {
	expr := strings.TrimSpace(rest)
	if expr == "" {
		return nil, fmt.Errorf("@when expression missing")
	}
	return &restfile.ConditionSpec{
		Expression: expr,
		Line:       line,
		Col:        1,
		Negate:     negate,
	}, nil
}

func parseForEachSpec(rest string, line int) (*restfile.ForEachSpec, error) {
	expr, name, form := cutForEach(rest)
	switch {
	case expr == "" && name == "":
		return nil, fmt.Errorf("@for-each expression missing")
	case form == forEachNone:
		return nil, fmt.Errorf("@for-each must use 'as' or 'in'")
	case expr == "" || name == "":
		return nil, fmt.Errorf("@for-each requires %s", form)
	}
	if err := checkRTSBinding(forEachName, name); err != nil {
		return nil, err
	}
	return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
}

type forEachForm uint8

const (
	forEachNone forEachForm = iota
	forEachAs
	forEachIn
)

func (f forEachForm) String() string {
	switch f {
	case forEachAs:
		return "'<expr> as <name>'"
	case forEachIn:
		return "'<name> in <expr>'"
	}
	return ""
}

// cutForEach ignores keywords inside strings, comments, and nested groups.
// The as form is read from the right so the expression may contain the keyword.
func cutForEach(rest string) (expr, name string, form forEachForm) {
	rest = strings.TrimSpace(rest)
	mask := rts.Mask(rest)
	if i := strings.LastIndex(mask, " as "); i >= 0 {
		return strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+4:]), forEachAs
	}
	if i := strings.Index(mask, " in "); i >= 0 {
		return strings.TrimSpace(rest[i+4:]), strings.TrimSpace(rest[:i]), forEachIn
	}
	return rest, "", forEachNone
}

type authDirective struct {
	Scope   directive.Scope
	Name    string
	Spec    *restfile.AuthSpec
	Disable bool
}

var errAuthSpec = errors.New("@auth requires a valid auth spec")

func parseAuthDirective(rest string) (authDirective, error) {
	dir := authDirective{Scope: directive.ScopeRequest}
	fields := slices.Collect(directive.ScanFields(rest))
	if len(fields) == 0 {
		return dir, errAuthSpec
	}

	explicitScope := false
	if scope, ok := directive.ParseScope(fields[0].Value); ok {
		dir.Scope = scope
		explicitScope = true
		fields = fields[1:]
		if len(fields) == 0 {
			return dir, fmt.Errorf("@auth %s scope requires an auth spec", scope.String())
		}
	}
	// Read the name first so a rejected line cannot become a default.
	name, nameErr := authName(dir.Scope, fields)
	dir.Name = name
	// Only quotes count. An open bracket keeps the rest of the line, as in Pa(ss.
	if closer := directive.FieldsOpen(rest); closer == '"' || closer == '\'' {
		return dir, &directive.UnclosedError{Directive: directive.Auth, Closer: string(closer)}
	}
	if nameErr != nil {
		return dir, nameErr
	}

	if strings.EqualFold(fields[0].Value, restfile.AuthDisableWord) {
		if dir.Scope != directive.ScopeRequest {
			return dir, fmt.Errorf("@auth %s scope does not support none", dir.Scope.String())
		}
		if len(fields) != 1 {
			return dir, errors.New("@auth none does not accept additional tokens")
		}
		dir.Disable = true
		return dir, nil
	}

	if namesProfiles(fields[0].Value) || (strings.EqualFold(fields[0].Value, "use") && directive.SpacedKey(fields, 0)) {
		if dir.Scope != directive.ScopeRequest {
			return dir, fmt.Errorf("@auth %s scope does not support use=", dir.Scope.String())
		}
		spec, err := parseAuthUse(fields)
		dir.Spec = spec
		return dir, err
	}

	spec, err := parseAuthSpec(fields)
	if err != nil {
		return dir, err
	}
	// A named definition needs its own command; it cannot rely on a prior cache entry.
	if dir.Name != "" && spec != nil && spec.Params["cmd"] == "" && spec.Params["argv"] == "" {
		return dir, fmt.Errorf("@auth command %s requires cmd or argv", dir.Name)
	}
	if spec == nil {
		if explicitScope {
			return dir, fmt.Errorf("@auth %s scope requires a valid auth spec", dir.Scope.String())
		}
		return dir, errAuthSpec
	}
	dir.Spec = spec
	return dir, nil
}

// authName reads name= from a command line. The name comes back even with an
// error, so a broken named definition only fails the requests that use it.
// Option errors are left for parseAuthSpec to report.
func authName(scope directive.Scope, fields []directive.Field) (string, error) {
	if len(fields) < 2 || restfile.AuthKind(fields[0].Value).Canonical() != restfile.AuthCommand {
		return "", nil
	}
	opts, _ := directive.OptionFields(directive.Auth, fields[1:])
	name, ok := opts.Lookup("name")
	if !ok {
		// Before 1.10 this word was ignored and the line was a default. Rejecting
		// it keeps an old default from quietly becoming a named definition.
		w := fields[1].Value
		if scope != directive.ScopeRequest && validProfileName(w) && !directive.SpacedKey(fields, 1) {
			return "", fmt.Errorf(
				"@auth expects key=value options but got %q. Write name=%s to name a definition",
				w,
				w,
			)
		}
		return "", nil
	}
	switch {
	case scope == directive.ScopeRequest:
		return name, fmt.Errorf("@auth %s scope does not support name=", scope.String())
	case name == "":
		return name, errors.New("@auth name= requires a profile name")
	case !validProfileName(name):
		return name, fmt.Errorf("@auth profile name %q is invalid", name)
	}
	return name, nil
}

// Reject bare words so an unquoted cmd=gh auth token cannot silently run gh.
// Report spacing errors first for options such as cmd = x.
func authOptionFields(fields []directive.Field) (directive.Options, error) {
	opts, err := directive.OptionFields(directive.Auth, fields)
	if err != nil {
		return directive.Options{}, err
	}
	for _, f := range fields {
		if !strings.Contains(f.Value, "=") {
			return directive.Options{}, fmt.Errorf(
				"@auth expects key=value options but got %q. Quote a value that has spaces",
				f.Value,
			)
		}
	}
	return opts, nil
}

func parseAuthUse(fields []directive.Field) (*restfile.AuthSpec, error) {
	opts, err := authOptionFields(fields)
	if err != nil {
		return nil, err
	}
	name := opts.Pop("use")
	switch {
	case name == "":
		return nil, errors.New("@auth use= requires a profile name")
	case !validProfileName(name):
		return nil, fmt.Errorf("@auth use name %q is invalid", name)
	}
	params := make(map[string]string)
	opts.CopyTo(params)
	spec := &restfile.AuthSpec{Use: name, Params: params}
	if bad := spec.UnknownParams(); len(bad) > 0 {
		return nil, fmt.Errorf(
			"@auth use= accepts only %s. Set %s on the definition",
			strings.Join(restfile.AuthUseParams, ", "),
			strings.Join(bad, ", "),
		)
	}
	// An empty override would clear the definition's value.
	maps.DeleteFunc(spec.Params, func(_, v string) bool { return v == "" })
	return spec, nil
}

// Fields arrive decoded. Rejoining them loses the boundary around a quoted
// option value, so scope="read write" would read back as scope=read plus an
// unrelated bare word.
func parseAuthSpec(fields []directive.Field) (*restfile.AuthSpec, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	authType := restfile.AuthKind(fields[0].Value).Canonical()
	params := make(map[string]string)
	switch authType {
	case restfile.AuthBasic, restfile.AuthDigest:
		if len(fields) >= 3 {
			params["username"] = fields[1].Value
			params["password"] = joinValues(fields[2:])
		}
	case restfile.AuthBearer:
		if len(fields) >= 2 {
			params["token"] = joinValues(fields[1:])
		}
	case restfile.AuthAPIKey:
		if len(fields) >= 4 {
			place := fields[1].Value
			if _, ok := restfile.ParseAPIKeyPlacement(place); !ok && !vars.HasPlaceholder(place) {
				return nil, fmt.Errorf("@auth apikey placement %q is not supported. Use header or query", place)
			}
			params["placement"] = strings.ToLower(place)
			params["name"] = fields[2].Value
			params["value"] = joinValues(fields[3:])
		}
	case restfile.AuthOAuth2:
		if len(fields) < 2 {
			return nil, nil
		}
		opts, err := authOptionFields(fields[1:])
		if err != nil {
			return nil, err
		}
		opts.CopyTo(params)
		if params["token_url"] == "" && params["cache_key"] == "" {
			return nil, nil
		}
	case restfile.AuthCommand:
		if len(fields) < 2 {
			return nil, nil
		}
		opts, err := authOptionFields(fields[1:])
		if err != nil {
			return nil, err
		}
		// authName reads the name. It names the definition and is not a command option.
		opts.Pop("name")
		opts.CopyTo(params)
		if params["cmd"] == "" && params["argv"] == "" && params["cache_key"] == "" {
			return nil, nil
		}
	default:
		if len(fields) >= 2 {
			params["header"] = fields[0].Value
			params["value"] = joinValues(fields[1:])
			authType = restfile.AuthHeader
		}
	}
	if len(params) == 0 {
		return nil, nil
	}
	spec := &restfile.AuthSpec{Type: authType, Params: params}
	if bad := spec.UnknownParams(); len(bad) > 0 {
		msg := fmt.Sprintf("@auth %s does not accept %s", authType, strings.Join(bad, ", "))
		if params["cmd"] != "" {
			msg += ". Quote a cmd value that has spaces"
		}
		return nil, errors.New(msg)
	}
	return spec, nil
}

func joinValues(fields []directive.Field) string {
	vals := make([]string, len(fields))
	for i, f := range fields {
		vals[i] = f.Value
	}
	return strings.Join(vals, " ")
}

// Trace budgets use "<=" syntax, so duplicate checks use normalized target
// names instead of parsed options. An unknown token is reported by the name
// before its operator, or whole when it has none.
func parseTraceSpec(rest string) (*restfile.TraceSpec, error) {
	spec := &restfile.TraceSpec{Enabled: true}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return spec, nil
	}

	fields := slices.Collect(directive.ScanFields(rest))
	var set, unknown, spaced []string
	var errs []error
	for i := 0; i < len(fields); i++ {
		if key, n := directive.SpacedOption(fields, i); n > 0 {
			spaced = append(spaced, key)
			i += n - 1
			continue
		}
		if fields[i].Value == "" {
			continue
		}
		target, err := applyTraceToken(spec, fields[i].Value)
		var unk *directive.UnknownOptionsError
		switch {
		case errors.As(err, &unk):
			unknown = append(unknown, unk.Keys...)
		case err != nil:
			errs = append(errs, err)
		case target != "":
			set = append(set, target)
		}
	}
	if len(spaced) > 0 {
		errs = append(errs, &directive.SpacedOptionsError{Directive: directive.Trace, Keys: spaced})
	}
	errs = append(errs, directive.UnknownOption(directive.Trace, unknown...))
	if err := directive.RepeatedNames(directive.Trace, set); err != nil {
		return nil, errors.Join(append([]error{err}, errs...)...)
	}

	if len(spec.Budgets.Phases) == 0 {
		spec.Budgets.Phases = nil
	}
	return spec, errors.Join(errs...)
}

// applyTraceToken returns the normalized setting name used for duplicate checks.
// An invalid duration or bool leaves the setting as it was.
func applyTraceToken(spec *restfile.TraceSpec, value string) (string, error) {
	key, val, ok := strings.Cut(value, "=")
	if !ok {
		key = strings.TrimSpace(key)
		switch strings.ToLower(key) {
		case "off", "disable", "disabled", "false":
			spec.Enabled = false
			return "", nil
		case "on", "enable", "enabled", "true":
			spec.Enabled = true
			return "", nil
		}
		return "", directive.UnknownOption(directive.Trace, cmp.Or(key, value))
	}
	key, le := strings.CutSuffix(key, "<")
	key, val = strings.TrimSpace(key), strings.TrimSpace(val)
	if kind, ok := tracebudget.NormalizePhase(key); ok {
		if dur := parseDuration(val); dur > 0 {
			setTracePhaseBudget(spec, kind, dur)
			return string(kind), nil
		}
		return "", nil
	}

	var target string
	switch strings.ToLower(key) {
	case "enabled":
		target = "enabled"
	case "tolerance", "allowance", "grace":
		target = "tolerance"
	default:
		return "", directive.UnknownOption(directive.Trace, cmp.Or(key, value))
	}
	if le {
		return "", &directive.OpOptionError{Directive: directive.Trace, Key: key, Value: val, Op: directive.OpLe}
	}
	switch target {
	case "enabled":
		if b, ok := directive.ParseBool(val); ok {
			spec.Enabled = b
			return target, nil
		}
	case "tolerance":
		if dur := parseDuration(val); dur >= 0 {
			spec.Budgets.Tolerance = dur
			return target, nil
		}
	}
	return "", nil
}

func setTracePhaseBudget(spec *restfile.TraceSpec, kind nettrace.PhaseKind, dur time.Duration) {
	if kind == nettrace.PhaseTotal {
		spec.Budgets.Total = dur
		return
	}
	if spec.Budgets.Phases == nil {
		spec.Budgets.Phases = make(map[string]time.Duration)
	}
	spec.Budgets.Phases[string(kind)] = dur
}

var compareBaselineKeys = []string{"base", "baseline", "primary", "ref"}

func parseCompareDirective(rest string) (*restfile.CompareSpec, error) {
	fields := slices.Collect(directive.ScanFields(rest))
	opts, err := directive.OptionFields(directive.Compare, fields)
	if err != nil {
		return nil, err
	}

	baseline, err := compareOption(opts, compareBaselineKeys...)
	if err != nil {
		return nil, err
	}
	group, err := compareOption(opts, "group")
	if err != nil {
		return nil, err
	}
	if err := opts.Leftover(directive.Compare); err != nil {
		return nil, err
	}
	if vars.IsReservedEnvironment(baseline) {
		return nil, fmt.Errorf("@compare baseline %q is reserved for shared defaults", baseline)
	}
	if vars.IsReservedEnvironment(group) {
		return nil, fmt.Errorf("@compare group %q is reserved", group)
	}

	envs, err := compareEnvironments(fields)
	if err != nil {
		return nil, err
	}
	spec := &restfile.CompareSpec{Environments: envs, Baseline: envs[0], Group: group}
	if baseline == "" {
		return spec, nil
	}
	for _, env := range envs {
		if strings.EqualFold(env, baseline) {
			spec.Baseline = env
			return spec, nil
		}
	}
	return nil, fmt.Errorf("@compare baseline %q must match one of the environments", baseline)
}

// Compare options reject empty values even when another alias supplies one.
func compareOption(opts directive.Options, keys ...string) (string, error) {
	for _, key := range keys {
		if raw, ok := opts.Lookup(key); ok && strings.TrimSpace(raw) == "" {
			return "", fmt.Errorf("@compare %s cannot be empty", key)
		}
	}
	value, _ := opts.PopAny(keys...)
	return value, nil
}

func compareEnvironments(fields []directive.Field) ([]string, error) {
	envs := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		env := strings.TrimSpace(field.Value)
		if env == "" || strings.Contains(env, "=") {
			continue
		}
		if vars.IsReservedEnvironment(env) {
			return nil, fmt.Errorf("@compare environment %q is reserved for shared defaults", env)
		}
		lowered := strings.ToLower(env)
		if _, exists := seen[lowered]; exists {
			return nil, fmt.Errorf("@compare duplicate environment %q", env)
		}
		seen[lowered] = struct{}{}
		envs = append(envs, env)
	}
	if len(envs) < 2 {
		return nil, fmt.Errorf("@compare requires at least two environments")
	}
	return envs, nil
}

func parseDuration(value string) time.Duration {
	dur, ok := duration.Parse(value)
	if !ok {
		return 0
	}
	return dur
}

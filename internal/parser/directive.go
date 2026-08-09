package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
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
	raw := strings.TrimSpace(rest)
	if after, ok := strings.CutPrefix(raw, "="); ok {
		raw = strings.TrimSpace(after)
	}
	if raw == "" {
		return restfile.ApplySpec{}, fmt.Errorf("@apply expression missing")
	}
	l := strings.ToLower(raw)
	if strings.HasPrefix(l, "use=") {
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
		if !validPatchName(n) {
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
	scTok, rem := directive.CutToken(rest)
	if scTok == "" {
		return restfile.PatchProfile{}, fmt.Errorf(
			"@patch requires '<scope> <name> <expression>'",
		)
	}
	sc, ok := parsePatchScope(scTok)
	if !ok {
		return restfile.PatchProfile{}, fmt.Errorf("@patch scope must be file or global")
	}
	n, rem := directive.CutToken(rem)
	n = strings.TrimSpace(n)
	if !validPatchName(n) {
		return restfile.PatchProfile{}, fmt.Errorf("@patch name %q is invalid", n)
	}
	ex := strings.TrimSpace(rem)
	if after, ok := strings.CutPrefix(ex, "="); ok {
		ex = strings.TrimSpace(after)
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

// A patch is only useful where later requests can still see it.
func parsePatchScope(tok string) (directive.Scope, bool) {
	scope, ok := directive.ParseScope(tok)
	if !ok || scope == directive.ScopeRequest {
		return directive.ScopeRequest, false
	}
	return scope, true
}

func validPatchName(n string) bool {
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
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	if idx := strings.LastIndex(rest, " as "); idx >= 0 {
		expr := strings.TrimSpace(rest[:idx])
		name := strings.TrimSpace(rest[idx+4:])
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<expr> as <name>'")
		}
		if err := checkRTSBinding(forEachName, name); err != nil {
			return nil, err
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	if before, after, ok := strings.Cut(rest, " in "); ok {
		name := strings.TrimSpace(before)
		expr := strings.TrimSpace(after)
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<name> in <expr>'")
		}
		if err := checkRTSBinding(forEachName, name); err != nil {
			return nil, err
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	return nil, fmt.Errorf("@for-each must use 'as' or 'in'")
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
	fields := directive.Fields(strings.TrimSpace(rest))
	if len(fields) == 0 {
		return dir, errAuthSpec
	}

	explicitScope := false
	if scope, ok := directive.ParseScope(fields[0]); ok {
		dir.Scope = scope
		explicitScope = true
		fields = fields[1:]
		if len(fields) == 0 {
			return dir, fmt.Errorf("@auth %s scope requires an auth spec", scope.String())
		}
	}

	if strings.EqualFold(fields[0], "none") {
		if dir.Scope != directive.ScopeRequest {
			return dir, fmt.Errorf("@auth %s scope does not support none", dir.Scope.String())
		}
		if len(fields) != 1 {
			return dir, errors.New("@auth none does not accept additional tokens")
		}
		dir.Disable = true
		return dir, nil
	}

	spec, err := parseAuthSpec(strings.Join(fields, " "))
	if err != nil {
		return dir, err
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

func parseAuthSpec(rest string) (*restfile.AuthSpec, error) {
	fields := directive.Fields(rest)
	if len(fields) == 0 {
		return nil, nil
	}
	authType := strings.ToLower(fields[0])
	params := make(map[string]string)
	switch authType {
	case "basic":
		if len(fields) >= 3 {
			params["username"] = fields[1]
			params["password"] = strings.Join(fields[2:], " ")
		}
	case "bearer":
		if len(fields) >= 2 {
			params["token"] = strings.Join(fields[1:], " ")
		}
	case "apikey", "api-key":
		if len(fields) >= 4 {
			params["placement"] = strings.ToLower(fields[1])
			params["name"] = fields[2]
			params["value"] = strings.Join(fields[3:], " ")
		}
	case "oauth2":
		if len(fields) < 2 {
			return nil, nil
		}
		opts, err := directive.OptionFields(directive.Auth, fields[1:])
		if err != nil {
			return nil, err
		}
		opts.CopyTo(params)
		if params["token_url"] == "" && params["cache_key"] == "" {
			return nil, nil
		}
		if params["grant"] == "" {
			params["grant"] = "client_credentials"
		}
		if params["client_auth"] == "" {
			params["client_auth"] = "basic"
		}
	case "command":
		if len(fields) < 2 {
			return nil, nil
		}
		opts, err := directive.OptionFields(directive.Auth, fields[1:])
		if err != nil {
			return nil, err
		}
		opts.CopyTo(params)
		if params["argv"] == "" && params["cache_key"] == "" {
			return nil, nil
		}
	default:
		if len(fields) >= 2 {
			params["header"] = fields[0]
			params["value"] = strings.Join(fields[1:], " ")
			authType = "header"
		}
	}
	if len(params) == 0 {
		return nil, nil
	}
	return &restfile.AuthSpec{Type: authType, Params: params}, nil
}

func parseProfileSpec(rest string) (*restfile.ProfileSpec, error) {
	rest = strings.TrimSpace(rest)
	spec := &restfile.ProfileSpec{}

	if rest == "" {
		spec.Count = 10
		return spec, nil
	}

	fields := directive.Fields(rest)
	params, err := directive.OptionFields(directive.Profile, fields)
	if err != nil {
		return nil, err
	}

	if spec.Count == 0 {
		if raw, ok := params.Lookup("count"); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
				spec.Count = n
			}
		}
	}

	if spec.Count == 0 && len(fields) == 1 && !strings.Contains(fields[0], "=") {
		if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
			spec.Count = n
		}
	}

	if raw, ok := params.Lookup("warmup"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
			spec.Warmup = n
		}
	}

	if raw, ok := params.Lookup("delay"); ok {
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			spec.Delay = dur
		}
	}

	if spec.Count <= 0 {
		spec.Count = 10
	}
	if spec.Warmup < 0 {
		spec.Warmup = 0
	}
	return spec, nil
}

// Trace budgets use "<=" syntax, so duplicate checks use normalized target
// names instead of parsed options.
func parseTraceSpec(rest string) (*restfile.TraceSpec, error) {
	spec := &restfile.TraceSpec{Enabled: true}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return spec, nil
	}

	var set []string
	for _, field := range directive.Fields(rest) {
		if value := strings.TrimSpace(field); value != "" {
			if target := applyTraceToken(spec, value); target != "" {
				set = append(set, target)
			}
		}
	}
	if err := directive.RepeatedNames(directive.Trace, set); err != nil {
		return nil, err
	}

	if len(spec.Budgets.Phases) == 0 {
		spec.Budgets.Phases = nil
	}
	return spec, nil
}

// applyTraceToken returns the normalized setting name used for duplicate checks.
func applyTraceToken(spec *restfile.TraceSpec, value string) string {
	switch strings.ToLower(value) {
	case "off", "disable", "disabled", "false":
		spec.Enabled = false
		return ""
	case "on", "enable", "enabled", "true":
		spec.Enabled = true
		return ""
	}

	if parts := strings.SplitN(value, "<=", 2); len(parts) == 2 {
		name := tracebudget.NormalizePhase(parts[0])
		dur := parseDuration(parts[1])
		if name == "" || dur <= 0 {
			return ""
		}
		setTracePhaseBudget(spec, name, dur)
		return name
	}

	if before, after, ok := strings.Cut(value, "="); ok {
		key := strings.ToLower(strings.TrimSpace(before))
		val := strings.TrimSpace(after)
		return applyTraceOption(spec, key, val)
	}
	return ""
}

func applyTraceOption(spec *restfile.TraceSpec, key, val string) string {
	switch key {
	case "enabled":
		if b, ok := directive.ParseBool(val); ok {
			spec.Enabled = b
			return key
		}
	case "total":
		if dur := parseDuration(val); dur > 0 {
			spec.Budgets.Total = dur
			return tracebudget.TotalPhase
		}
	case "tolerance", "allowance", "grace":
		if dur := parseDuration(val); dur >= 0 {
			spec.Budgets.Tolerance = dur
			return "tolerance"
		}
	default:
		dur := parseDuration(val)
		if dur <= 0 {
			return ""
		}
		name := tracebudget.NormalizePhase(key)
		if name == "" {
			return ""
		}
		setTracePhaseBudget(spec, name, dur)
		return name
	}
	return ""
}

func setTracePhaseBudget(spec *restfile.TraceSpec, name string, dur time.Duration) {
	if name == tracebudget.TotalPhase {
		spec.Budgets.Total = dur
		return
	}
	if spec.Budgets.Phases == nil {
		spec.Budgets.Phases = make(map[string]time.Duration)
	}
	spec.Budgets.Phases[name] = dur
}

var compareBaselineKeys = []string{"base", "baseline", "primary", "ref"}

func parseCompareDirective(rest string) (*restfile.CompareSpec, error) {
	fields := directive.Fields(rest)
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

func compareEnvironments(fields []string) ([]string, error) {
	envs := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		env := strings.TrimSpace(field)
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

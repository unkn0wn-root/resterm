package parser

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

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
		if !directive.IsIdent(a) {
			return restfile.UseSpec{}, fmt.Errorf("@use alias %q is invalid", a)
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
		if !directive.IsIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	if before, after, ok := strings.Cut(rest, " in "); ok {
		name := strings.TrimSpace(before)
		expr := strings.TrimSpace(after)
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<name> in <expr>'")
		}
		if !directive.IsIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
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

func parseAuthDirective(rest string) (authDirective, bool, error) {
	dir := authDirective{Scope: directive.ScopeRequest}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return dir, false, nil
	}

	fields := directive.Fields(rest)
	if len(fields) == 0 {
		return dir, false, nil
	}

	explicitScope := false
	if scope, ok := directive.ParseScope(fields[0]); ok {
		dir.Scope = scope
		explicitScope = true
		fields = fields[1:]
		if len(fields) == 0 {
			return dir, true, fmt.Errorf(
				"@auth %s scope requires an auth spec",
				scope.String(),
			)
		}
	}

	if strings.EqualFold(fields[0], "none") {
		if dir.Scope != directive.ScopeRequest {
			return dir, true, fmt.Errorf(
				"@auth %s scope does not support none",
				dir.Scope.String(),
			)
		}
		if len(fields) != 1 {
			return dir, true, fmt.Errorf("@auth none does not accept additional tokens")
		}
		dir.Disable = true
		return dir, true, nil
	}

	spec := parseAuthSpec(strings.Join(fields, " "))
	if spec == nil {
		if explicitScope {
			return dir, true, fmt.Errorf(
				"@auth %s scope requires a valid auth spec",
				dir.Scope.String(),
			)
		}
		return dir, false, nil
	}
	dir.Spec = spec
	return dir, true, nil
}

func parseAuthSpec(rest string) *restfile.AuthSpec {
	fields := directive.Fields(rest)
	if len(fields) == 0 {
		return nil
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
			return nil
		}
		maps.Copy(params, directive.OptionFields(fields[1:]))
		if params["token_url"] == "" && params["cache_key"] == "" {
			return nil
		}
		if params["grant"] == "" {
			params["grant"] = "client_credentials"
		}
		if params["client_auth"] == "" {
			params["client_auth"] = "basic"
		}
	case "command":
		if len(fields) < 2 {
			return nil
		}
		maps.Copy(params, directive.OptionFields(fields[1:]))
		if params["argv"] == "" && params["cache_key"] == "" {
			return nil
		}
	default:
		if len(fields) >= 2 {
			params["header"] = fields[0]
			params["value"] = strings.Join(fields[1:], " ")
			authType = "header"
		}
	}
	if len(params) == 0 {
		return nil
	}
	return &restfile.AuthSpec{Type: authType, Params: params}
}

func parseProfileSpec(rest string) *restfile.ProfileSpec {
	rest = strings.TrimSpace(rest)
	spec := &restfile.ProfileSpec{}

	if rest == "" {
		spec.Count = 10
		return spec
	}

	fields := directive.Fields(rest)
	params := directive.OptionFields(fields)

	if spec.Count == 0 {
		if raw, ok := params["count"]; ok {
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

	if raw, ok := params["warmup"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
			spec.Warmup = n
		}
	}

	if raw, ok := params["delay"]; ok {
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
	return spec
}

func parseTraceSpec(rest string) *restfile.TraceSpec {
	spec := &restfile.TraceSpec{Enabled: true}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return spec
	}

	for _, field := range directive.Fields(rest) {
		if value := strings.TrimSpace(field); value != "" {
			applyTraceToken(spec, value)
		}
	}

	if len(spec.Budgets.Phases) == 0 {
		spec.Budgets.Phases = nil
	}
	return spec
}

// applyTraceToken handles one @trace token, which is an on/off word, a
// "phase<=dur" budget or a key=value option. The budget form has to be
// checked before the plain option form because it contains "=" as well.
func applyTraceToken(spec *restfile.TraceSpec, value string) {
	switch strings.ToLower(value) {
	case "off", "disable", "disabled", "false":
		spec.Enabled = false
		return
	case "on", "enable", "enabled", "true":
		spec.Enabled = true
		return
	}

	if parts := strings.SplitN(value, "<=", 2); len(parts) == 2 {
		name := tracebudget.NormalizePhase(parts[0])
		dur := parseDuration(parts[1])
		if name != "" && dur > 0 {
			setTracePhaseBudget(spec, name, dur)
		}
		return
	}

	if before, after, ok := strings.Cut(value, "="); ok {
		key := strings.ToLower(strings.TrimSpace(before))
		val := strings.TrimSpace(after)
		applyTraceOption(spec, key, val)
	}
}

func applyTraceOption(spec *restfile.TraceSpec, key, val string) {
	switch key {
	case "enabled":
		if b, ok := directive.ParseBool(val); ok {
			spec.Enabled = b
		}
	case "total":
		if dur := parseDuration(val); dur > 0 {
			spec.Budgets.Total = dur
		}
	case "tolerance", "allowance", "grace":
		if dur := parseDuration(val); dur >= 0 {
			spec.Budgets.Tolerance = dur
		}
	default:
		dur := parseDuration(val)
		if dur <= 0 {
			return
		}
		name := tracebudget.NormalizePhase(key)
		if name == "" {
			return
		}
		setTracePhaseBudget(spec, name, dur)
	}
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

func parseCompareDirective(rest string) (*restfile.CompareSpec, error) {
	fields := directive.Fields(rest)
	envs := make([]string, 0, len(fields))
	seen := make(map[string]struct{})
	var baseline string
	var group string

	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		if before, after, ok := strings.Cut(value, "="); ok {
			key := strings.ToLower(strings.TrimSpace(before))
			val := strings.TrimSpace(after)
			switch key {
			case "base", "baseline", "primary", "ref":
				if val == "" {
					return nil, fmt.Errorf("@compare baseline cannot be empty")
				}
				if vars.IsReservedEnvironment(val) {
					return nil, fmt.Errorf(
						"@compare baseline %q is reserved for shared defaults",
						val,
					)
				}
				baseline = val
			case "group":
				if val == "" {
					return nil, fmt.Errorf("@compare group cannot be empty")
				}
				if group != "" {
					return nil, fmt.Errorf("@compare group specified more than once")
				}
				if vars.IsReservedEnvironment(val) {
					return nil, fmt.Errorf("@compare group %q is reserved", val)
				}
				group = val
			default:
				return nil, fmt.Errorf("@compare unsupported option %q", key)
			}
			continue
		}
		if vars.IsReservedEnvironment(value) {
			return nil, fmt.Errorf("@compare environment %q is reserved for shared defaults", value)
		}
		lowered := strings.ToLower(value)
		if _, exists := seen[lowered]; exists {
			return nil, fmt.Errorf("@compare duplicate environment %q", value)
		}
		seen[lowered] = struct{}{}
		envs = append(envs, value)
	}

	if len(envs) < 2 {
		return nil, fmt.Errorf("@compare requires at least two environments")
	}

	if baseline == "" {
		baseline = envs[0]
	} else {
		match := ""
		for _, env := range envs {
			if strings.EqualFold(env, baseline) {
				match = env
				break
			}
		}
		if match == "" {
			return nil, fmt.Errorf(
				"@compare baseline %q must match one of the environments",
				baseline,
			)
		}
		baseline = match
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     baseline,
		Group:        group,
	}, nil
}

func parseDuration(value string) time.Duration {
	dur, ok := duration.Parse(value)
	if !ok {
		return 0
	}
	return dur
}

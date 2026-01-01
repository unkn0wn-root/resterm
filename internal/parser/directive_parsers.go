package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func parseApplySpec(rest string, line int) (restfile.ApplySpec, bool) {
	expr := strings.TrimSpace(rest)
	if strings.HasPrefix(expr, "=") {
		expr = strings.TrimSpace(strings.TrimPrefix(expr, "="))
	}
	if expr == "" {
		return restfile.ApplySpec{}, false
	}
	return restfile.ApplySpec{
		Expression: expr,
		Line:       line,
		Col:        1,
	}, true
}

func parseUseSpec(rest string, line int) (restfile.UseSpec, error) {
	fields := splitAuthFields(rest)
	if len(fields) < 3 {
		return restfile.UseSpec{}, fmt.Errorf("@use requires a path and alias")
	}
	if !strings.EqualFold(fields[1], "as") {
		return restfile.UseSpec{}, fmt.Errorf("@use must use 'as' to define an alias")
	}
	if len(fields) > 3 {
		return restfile.UseSpec{}, fmt.Errorf("@use has too many tokens")
	}
	path := strings.TrimSpace(fields[0])
	alias := strings.TrimSpace(fields[2])
	if path == "" || alias == "" {
		return restfile.UseSpec{}, fmt.Errorf("@use requires a non-empty path and alias")
	}
	if !isIdent(alias) {
		return restfile.UseSpec{}, fmt.Errorf("@use alias %q is invalid", alias)
	}
	return restfile.UseSpec{
		Path:  path,
		Alias: alias,
		Line:  line,
	}, nil
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
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	if idx := strings.LastIndex(trimmed, " as "); idx >= 0 {
		expr := strings.TrimSpace(trimmed[:idx])
		name := strings.TrimSpace(trimmed[idx+4:])
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<expr> as <name>'")
		}
		if !isIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	if idx := strings.Index(trimmed, " in "); idx >= 0 {
		name := strings.TrimSpace(trimmed[:idx])
		expr := strings.TrimSpace(trimmed[idx+4:])
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<name> in <expr>'")
		}
		if !isIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	return nil, fmt.Errorf("@for-each must use 'as' or 'in'")
}

func parseCaptureScope(token string) (restfile.CaptureScope, bool, bool) {
	lowered := strings.ToLower(strings.TrimSpace(token))
	secret := false
	if strings.HasSuffix(lowered, "-secret") {
		secret = true
		lowered = strings.TrimSuffix(lowered, "-secret")
	}
	switch lowered {
	case "request":
		return restfile.CaptureScopeRequest, secret, true
	case "file":
		return restfile.CaptureScopeFile, secret, true
	case "global":
		return restfile.CaptureScopeGlobal, secret, true
	default:
		return 0, false, false
	}
}

func parseAuthSpec(rest string) *restfile.AuthSpec {
	fields := splitAuthFields(rest)
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
		for key, value := range parseKeyValuePairs(fields[1:]) {
			params[key] = value
		}
		if params["token_url"] == "" && params["cache_key"] == "" {
			return nil
		}
		if params["grant"] == "" {
			params["grant"] = "client_credentials"
		}
		if params["client_auth"] == "" {
			params["client_auth"] = "basic"
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
	trimmed := strings.TrimSpace(rest)
	spec := &restfile.ProfileSpec{}

	if trimmed == "" {
		spec.Count = 10
		return spec
	}

	fields := splitAuthFields(trimmed)
	params := parseKeyValuePairs(fields)

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
		if dur, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && dur >= 0 {
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
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return spec
	}

	fields := splitAuthFields(trimmed)
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		switch lower {
		case "off", "disable", "disabled", "false":
			spec.Enabled = false
			continue
		case "on", "enable", "enabled", "true":
			spec.Enabled = true
			continue
		}

		if parts := strings.SplitN(value, "<=", 2); len(parts) == 2 {
			name := normalizeTracePhaseName(parts[0])
			dur := parseDuration(parts[1])
			if dur <= 0 {
				continue
			}
			if name == "total" {
				spec.Budgets.Total = dur
				continue
			}
			if spec.Budgets.Phases == nil {
				spec.Budgets.Phases = make(map[string]time.Duration)
			}
			spec.Budgets.Phases[name] = dur
			continue
		}

		if idx := strings.Index(value, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(value[:idx]))
			val := strings.TrimSpace(value[idx+1:])
			switch key {
			case "enabled":
				if b, ok := parseBool(val); ok {
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
					continue
				}
				name := normalizeTracePhaseName(key)
				if name == "total" {
					spec.Budgets.Total = dur
					continue
				}
				if spec.Budgets.Phases == nil {
					spec.Budgets.Phases = make(map[string]time.Duration)
				}
				spec.Budgets.Phases[name] = dur
			}
		}
	}

	if len(spec.Budgets.Phases) == 0 {
		spec.Budgets.Phases = nil
	}
	return spec
}

func parseCompareDirective(rest string) (*restfile.CompareSpec, error) {
	fields := splitAuthFields(rest)
	envs := make([]string, 0, len(fields))
	seen := make(map[string]struct{})
	var baseline string

	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		if idx := strings.Index(value, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(value[:idx]))
			val := strings.TrimSpace(value[idx+1:])
			switch key {
			case "base", "baseline", "primary", "ref":
				if val == "" {
					return nil, fmt.Errorf("@compare baseline cannot be empty")
				}
				baseline = val
			default:
				return nil, fmt.Errorf("@compare unsupported option %q", key)
			}
			continue
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
	}, nil
}

func parseDuration(value string) time.Duration {
	dur, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return dur
}

func normalizeTracePhaseName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dns", "lookup", "name":
		return "dns"
	case "connect", "dial":
		return "connect"
	case "tls", "handshake":
		return "tls"
	case "headers", "request_headers", "req_headers", "header":
		return "request_headers"
	case "body", "request_body", "req_body":
		return "request_body"
	case "ttfb", "first_byte", "wait":
		return "ttfb"
	case "transfer", "download":
		return "transfer"
	case "total", "overall":
		return "total"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

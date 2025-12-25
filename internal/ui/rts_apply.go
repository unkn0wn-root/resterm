package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type applyPatch struct {
	method     *string
	url        *string
	headers    map[string][]string
	headerDels map[string]struct{}
	query      map[string]*string
	body       *string
	vars       map[string]string
}

func (m *Model) parseApplyPatch(ctx context.Context, pos rts.Pos, v rts.Value) (applyPatch, error) {
	if v.K != rts.VDict {
		return applyPatch{}, applyErr("", "expects dict")
	}
	var p applyPatch
	for key, val := range v.M {
		field := applyKey(key)
		switch field {
		case "method":
			s, err := m.applyScalar(ctx, pos, val, "method")
			if err != nil {
				return applyPatch{}, err
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return applyPatch{}, applyErr("method", "expects non-empty value")
			}
			p.method = &s
		case "url":
			s, err := m.applyScalar(ctx, pos, val, "url")
			if err != nil {
				return applyPatch{}, err
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return applyPatch{}, applyErr("url", "expects non-empty value")
			}
			p.url = &s
		case "headers":
			set, del, err := m.parseApplyHeaders(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.headers = set
			p.headerDels = del
		case "query":
			q, err := m.parseApplyQuery(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.query = q
		case "body":
			s, err := m.rtsValueString(ctx, pos, val)
			if err != nil {
				return applyPatch{}, applyErr("body", err.Error())
			}
			p.body = &s
		case "vars":
			vars, err := m.parseApplyVars(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.vars = vars
		default:
			if field == "" {
				return applyPatch{}, applyErr("", "empty field")
			}
			return applyPatch{}, applyErr("", fmt.Sprintf("unknown field %q", field))
		}
	}
	return p, nil
}

func (m *Model) parseApplyHeaders(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string][]string, map[string]struct{}, error) {
	if v.K != rts.VDict {
		return nil, nil, applyErr("headers", "expects dict")
	}
	set := make(map[string][]string)
	del := make(map[string]struct{})
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, nil, applyErr("headers", "expects non-empty header name")
		}
		switch val.K {
		case rts.VNull:
			delete(set, name)
			del[name] = struct{}{}
		case rts.VList:
			values, err := m.applyList(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = values
			delete(del, name)
		default:
			s, err := m.applyScalar(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = []string{s}
			delete(del, name)
		}
	}
	if len(set) == 0 {
		set = nil
	}
	if len(del) == 0 {
		del = nil
	}
	return set, del, nil
}

func (m *Model) parseApplyQuery(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]*string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("query", "expects dict")
	}
	out := make(map[string]*string)
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, applyErr("query", "expects non-empty key")
		}
		if val.K == rts.VNull {
			out[name] = nil
			continue
		}
		s, err := m.applyScalar(ctx, pos, val, "query."+name)
		if err != nil {
			return nil, err
		}
		valCopy := s
		out[name] = &valCopy
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (m *Model) parseApplyVars(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("vars", "expects dict")
	}
	out := make(map[string]string)
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, applyErr("vars", "expects non-empty name")
		}
		s, err := m.applyScalar(ctx, pos, val, "vars."+name)
		if err != nil {
			return nil, err
		}
		out[name] = s
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (m *Model) applyScalar(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
	field string,
) (string, error) {
	switch v.K {
	case rts.VStr, rts.VNum, rts.VBool:
		s, err := m.rtsValueString(ctx, pos, v)
		if err != nil {
			return "", applyErr(field, err.Error())
		}
		return s, nil
	default:
		return "", applyErr(field, "expects string/number/bool")
	}
}

func (m *Model) applyList(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
	field string,
) ([]string, error) {
	if v.K != rts.VList {
		return nil, applyErr(field, "expects list")
	}
	if len(v.L) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(v.L))
	for _, item := range v.L {
		s, err := m.applyScalar(ctx, pos, item, field)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func applyPatchToRequest(req *restfile.Request, vars map[string]string, p applyPatch) error {
	if req == nil {
		return nil
	}
	applyPatchMethod(req, p.method)
	applyPatchURL(req, p.url)
	if err := applyPatchQuery(req, p.query); err != nil {
		return err
	}
	applyPatchHeaders(req, p.headers, p.headerDels)
	applyPatchBody(req, p.body)
	applyPatchVars(req, vars, p.vars)
	return nil
}

func applyPatchMethod(req *restfile.Request, val *string) {
	if val == nil || req == nil {
		return
	}
	req.Method = strings.ToUpper(strings.TrimSpace(*val))
}

func applyPatchURL(req *restfile.Request, val *string) {
	if val == nil || req == nil {
		return
	}
	req.URL = strings.TrimSpace(*val)
}

func applyPatchQuery(req *restfile.Request, q map[string]*string) error {
	if req == nil || len(q) == 0 {
		return nil
	}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return nil
	}
	if hasTpl(raw) {
		applyPatchQueryLoose(req, q)
		return nil
	}
	return applyPatchQueryURL(req, q)
}

func applyPatchQueryURL(req *restfile.Request, q map[string]*string) error {
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Errorf("invalid url after @apply: %w", err)
	}
	vals := parsed.Query()
	applyQueryPatch(vals, q)
	parsed.RawQuery = vals.Encode()
	req.URL = parsed.String()
	return nil
}

func applyPatchQueryLoose(req *restfile.Request, q map[string]*string) {
	base, qs, frag := splitURL(req.URL)
	vals, enc := parseQuery(qs)
	applyQueryPatch(vals, q)
	qs = encodeQuery(vals, enc)
	req.URL = joinURL(base, qs, frag)
}

func applyQueryPatch(vals url.Values, q map[string]*string) {
	for key, val := range q {
		if val == nil {
			vals.Del(key)
		} else {
			vals.Set(key, *val)
		}
	}
}

func hasTpl(raw string) bool {
	return strings.Contains(raw, "{{")
}

func splitURL(raw string) (string, string, string) {
	base := raw
	frag := ""
	qs := ""
	if idx := strings.Index(base, "#"); idx >= 0 {
		frag = base[idx+1:]
		base = base[:idx]
	}
	if idx := strings.Index(base, "?"); idx >= 0 {
		qs = base[idx+1:]
		base = base[:idx]
	}
	return base, qs, frag
}

func parseQuery(qs string) (url.Values, bool) {
	if qs == "" {
		return url.Values{}, true
	}
	if hasTpl(qs) {
		return parseQueryLoose(qs), false
	}
	vals, err := url.ParseQuery(qs)
	if err == nil {
		return vals, true
	}
	return parseQueryLoose(qs), false
}

func parseQueryLoose(qs string) url.Values {
	vals := url.Values{}
	for _, part := range strings.Split(qs, "&") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		key := kv[0]
		if key == "" {
			continue
		}
		val := ""
		if len(kv) == 2 {
			val = kv[1]
		}
		vals.Add(key, val)
	}
	return vals
}

func encodeQuery(vals url.Values, enc bool) string {
	if len(vals) == 0 {
		return ""
	}
	if enc {
		return vals.Encode()
	}
	return encodeQueryLoose(vals)
}

func encodeQueryLoose(vals url.Values) string {
	if len(vals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(vals))
	for key, list := range vals {
		if len(list) == 0 {
			parts = append(parts, key)
			continue
		}
		for _, val := range list {
			if val == "" {
				parts = append(parts, key+"=")
			} else {
				parts = append(parts, key+"="+val)
			}
		}
	}
	return strings.Join(parts, "&")
}

func joinURL(base, qs, frag string) string {
	out := base
	if qs != "" {
		out += "?" + qs
	}
	if frag != "" {
		out += "#" + frag
	}
	return out
}

func applyPatchHeaders(req *restfile.Request, set map[string][]string, del map[string]struct{}) {
	if req == nil || (len(set) == 0 && len(del) == 0) {
		return
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	for name := range del {
		req.Headers.Del(name)
	}
	for name, values := range set {
		req.Headers.Del(name)
		for _, value := range values {
			req.Headers.Add(name, value)
		}
	}
}

func applyPatchBody(req *restfile.Request, val *string) {
	if req == nil || val == nil {
		return
	}
	req.Body.FilePath = ""
	req.Body.Text = *val
	req.Body.GraphQL = nil
}

func applyPatchVars(req *restfile.Request, vars map[string]string, in map[string]string) {
	if req == nil || len(in) == 0 {
		return
	}
	setRequestVars(req, in)
	if vars == nil {
		return
	}
	for key, val := range in {
		vars[key] = val
	}
}

func setRequestVars(req *restfile.Request, vars map[string]string) {
	if req == nil || len(vars) == 0 {
		return
	}
	existing := make(map[string]int)
	for i, v := range req.Variables {
		existing[strings.ToLower(v.Name)] = i
	}
	for name, value := range vars {
		key := strings.ToLower(name)
		if idx, ok := existing[key]; ok {
			req.Variables[idx].Value = value
		} else {
			req.Variables = append(req.Variables, restfile.Variable{
				Name:  name,
				Value: value,
				Scope: restfile.ScopeRequest,
			})
		}
	}
}

func applyKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func applyErr(field, msg string) error {
	if field == "" {
		return fmt.Errorf("@apply %s", msg)
	}
	return fmt.Errorf("@apply %s: %s", field, msg)
}

func (m *Model) runRTSApply(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	vars map[string]string,
	extraVals map[string]rts.Value,
) error {
	if req == nil || len(req.Metadata.Applies) == 0 {
		return nil
	}
	for idx, spec := range req.Metadata.Applies {
		if err := ctx.Err(); err != nil {
			return err
		}
		expr := strings.TrimSpace(spec.Expression)
		if expr == "" {
			continue
		}
		pos := m.rtsPosForLine(doc, req, spec.Line)
		if spec.Col > 0 {
			pos.Col = spec.Col
		}
		site := fmt.Sprintf("@apply %d", idx+1)
		val, err := m.rtsEvalValue(ctx, doc, req, envName, base, expr, site, pos, vars, extraVals)
		if err != nil {
			return err
		}
		patch, err := m.parseApplyPatch(ctx, pos, val)
		if err != nil {
			return err
		}
		if err := applyPatchToRequest(req, vars, patch); err != nil {
			return err
		}
	}
	return nil
}

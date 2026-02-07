package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
)

type applyPatch struct {
	method     *string
	url        *string
	headers    map[string][]string
	headerDels map[string]struct{}
	query      map[string]*string
	body       *string
	auth       *restfile.AuthSpec
	authSet    bool
	settings   map[string]*string
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
		case "auth":
			if val.K == rts.VNull {
				p.auth = nil
				p.authSet = true
				break
			}
			a, err := m.parseApplyAuth(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.auth = a
			p.authSet = true
		case "settings":
			s, err := m.parseApplySettings(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.settings = s
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

func (m *Model) parseApplyAuth(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (*restfile.AuthSpec, error) {
	if v.K != rts.VDict {
		return nil, applyErr("auth", "expects dict")
	}
	var typ string
	pm := make(map[string]string)
	for k, val := range v.M {
		key := applyKey(k)
		if key == "" {
			return nil, applyErr("auth", "expects non-empty key")
		}
		s, err := m.applyScalar(ctx, pos, val, "auth."+key)
		if err != nil {
			return nil, err
		}
		if key == "type" {
			typ = strings.ToLower(strings.TrimSpace(s))
			continue
		}
		pm[key] = s
	}
	if strings.TrimSpace(typ) == "" {
		return nil, applyErr("auth", "requires type")
	}
	if len(pm) == 0 {
		pm = nil
	}
	return &restfile.AuthSpec{Type: typ, Params: pm}, nil
}

func (m *Model) parseApplySettings(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]*string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("settings", "expects dict")
	}
	out := make(map[string]*string)
	for k, val := range v.M {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			return nil, applyErr("settings", "expects non-empty key")
		}
		if val.K == rts.VNull {
			out[key] = nil
			continue
		}
		s, err := m.applyScalar(ctx, pos, val, "settings."+key)
		if err != nil {
			return nil, err
		}
		cp := s
		out[key] = &cp
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
	applyPatchAuth(req, p.auth, p.authSet)
	applyPatchSettings(req, p.settings)
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
	updated, err := urltpl.PatchQuery(raw, q)
	if err != nil {
		return fmt.Errorf("invalid url after @apply: %w", err)
	}
	req.URL = updated
	return nil
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

func applyPatchAuth(req *restfile.Request, a *restfile.AuthSpec, set bool) {
	if req == nil || !set {
		return
	}
	if a == nil {
		req.Metadata.Auth = nil
		return
	}
	cp := *a
	if len(a.Params) > 0 {
		pm := make(map[string]string, len(a.Params))
		for k, v := range a.Params {
			pm[k] = v
		}
		cp.Params = pm
	}
	req.Metadata.Auth = &cp
}

func applyPatchSettings(req *restfile.Request, in map[string]*string) {
	if req == nil || len(in) == 0 {
		return
	}
	if req.Settings == nil {
		req.Settings = make(map[string]string, len(in))
	}
	for k, v := range in {
		if v == nil {
			delete(req.Settings, k)
			continue
		}
		req.Settings[k] = *v
	}
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

type applyExpr struct {
	ex string
	ps rts.Pos
	st string
}

func (m *Model) applyExprs(
	doc *restfile.Document,
	req *restfile.Request,
	sp restfile.ApplySpec,
	idx int,
) ([]applyExpr, error) {
	if len(sp.Uses) > 0 {
		out := make([]applyExpr, 0, len(sp.Uses))
		for _, n := range sp.Uses {
			pf, ok := m.findPatchProfile(doc, n)
			if !ok {
				return nil, fmt.Errorf("@apply use=%q not found", n)
			}
			ex := strings.TrimSpace(pf.Expression)
			if ex == "" {
				return nil, fmt.Errorf("@apply use=%q has an empty expression", n)
			}
			ps := m.rtsPosForLine(doc, req, pf.Line)
			if pf.Col > 0 {
				ps.Col = pf.Col
			}
			out = append(out, applyExpr{
				ex: ex,
				ps: ps,
				st: fmt.Sprintf("@apply %d use=%s", idx+1, n),
			})
		}
		return out, nil
	}

	ex := strings.TrimSpace(sp.Expression)
	if ex == "" {
		return nil, nil
	}
	ps := m.rtsPosForLine(doc, req, sp.Line)
	if sp.Col > 0 {
		ps.Col = sp.Col
	}
	return []applyExpr{{
		ex: ex,
		ps: ps,
		st: fmt.Sprintf("@apply %d", idx+1),
	}}, nil
}

func (m *Model) findPatchProfile(doc *restfile.Document, n string) (*restfile.PatchProfile, bool) {
	fs := docPatchProfiles(doc)
	gs := append([]restfile.PatchProfile(nil), fs...)
	if m.patchGlobals != nil {
		gs = append(gs, m.patchGlobals.all()...)
	}
	sf := func(p restfile.PatchProfile) restfile.PatchScope { return p.Scope }
	nf := func(p restfile.PatchProfile) string { return p.Name }
	return restfile.ResolveNamedScoped(
		fs,
		gs,
		n,
		restfile.PatchScopeFile,
		restfile.PatchScopeGlobal,
		sf,
		nf,
	)
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
		es, err := m.applyExprs(doc, req, spec, idx)
		if err != nil {
			return err
		}
		for _, e := range es {
			if err := ctx.Err(); err != nil {
				return err
			}
			val, err := m.rtsEvalValue(
				ctx,
				doc,
				req,
				envName,
				base,
				e.ex,
				e.st,
				e.ps,
				vars,
				extraVals,
			)
			if err != nil {
				return err
			}
			patch, err := m.parseApplyPatch(ctx, e.ps, val)
			if err != nil {
				return err
			}
			if err := applyPatchToRequest(req, vars, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

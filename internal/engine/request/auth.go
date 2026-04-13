package request

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func lookupDefaultAuthProfile(
	xs []restfile.AuthProfile,
	scope restfile.AuthScope,
) (*restfile.AuthProfile, bool) {
	for i := len(xs) - 1; i >= 0; i-- {
		pf := &xs[i]
		if pf.Scope != scope {
			continue
		}
		if strings.TrimSpace(pf.Name) != "" {
			continue
		}
		return pf, true
	}
	return nil, false
}

func (e *Engine) resolveInheritedAuth(doc *restfile.Document, req *restfile.Request) {
	if req == nil || req.Metadata.Auth != nil || req.Metadata.AuthDisabled {
		return
	}
	if pf, ok := lookupDefaultAuthProfile(docAuthProfiles(doc), restfile.AuthScopeFile); ok {
		req.Metadata.Auth = restfile.CloneAuthSpec(&pf.Spec)
		return
	}
	if pf, ok := lookupDefaultAuthProfile(docAuthProfiles(doc), restfile.AuthScopeGlobal); ok {
		req.Metadata.Auth = restfile.CloneAuthSpec(&pf.Spec)
	}
}

func commandAuthSecrets(res authcmd.Result) []string {
	tok := strings.TrimSpace(res.Token)
	val := strings.TrimSpace(res.Value)
	switch {
	case tok == "" && val == "":
		return nil
	case tok == val:
		return []string{tok}
	case tok == "":
		return []string{val}
	case val == "":
		return []string{tok}
	default:
		return []string{tok, val}
	}
}

func injectedAuthSecrets(
	auth *restfile.AuthSpec,
	before *restfile.Request,
	after *restfile.Request,
) []string {
	if auth == nil || after == nil {
		return nil
	}
	hdr := "Authorization"
	if strings.EqualFold(strings.TrimSpace(auth.Type), "oauth2") {
		if name := strings.TrimSpace(auth.Params["header"]); name != "" {
			hdr = name
		}
	}
	beforeVal := headerValue(reqHeaders(before), hdr)
	afterVal := headerValue(reqHeaders(after), hdr)
	if strings.TrimSpace(afterVal) == "" || afterVal == beforeVal {
		return nil
	}
	out := []string{afterVal}
	if strings.EqualFold(hdr, "authorization") {
		_, tok, ok := strings.Cut(afterVal, " ")
		if ok {
			tok = strings.TrimSpace(tok)
			if tok != "" {
				out = append(out, tok)
			}
		}
	}
	return out
}

func reqHeaders(req *restfile.Request) http.Header {
	if req == nil {
		return nil
	}
	return req.Headers
}

func headerValue(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	return strings.TrimSpace(h.Get(name))
}

func (e *Engine) ensureCommandAuth(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
	timeout time.Duration,
) (authcmd.Result, error) {
	if req == nil || req.Metadata.Auth == nil {
		return authcmd.Result{}, nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "command") {
		return authcmd.Result{}, nil
	}
	prep, err := e.prepareCommandAuth(doc, req.Metadata.Auth, res, env, timeout)
	if err != nil {
		return authcmd.Result{}, err
	}
	hdr := prep.HeaderName()
	if req.Headers != nil && req.Headers.Get(hdr) != "" {
		return authcmd.Result{}, nil
	}

	ac := e.rt.AuthCmd()
	if ac == nil {
		return authcmd.Result{}, errdef.New(
			errdef.CodeHTTP,
			"command auth support is not initialised",
		)
	}
	out, err := ac.ResolvePrepared(ctx, prep)
	if err != nil {
		return authcmd.Result{}, errdef.Wrap(errdef.CodeHTTP, err, "resolve command auth")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(out.Header) != "" {
		return authcmd.Result{}, nil
	}
	req.Headers.Set(out.Header, out.Value)
	return out, nil
}

func (e *Engine) prepareCommandAuth(
	doc *restfile.Document,
	auth *restfile.AuthSpec,
	res *vars.Resolver,
	env string,
	timeout time.Duration,
) (authcmd.Prepared, error) {
	ac := e.rt.AuthCmd()
	if ac == nil {
		return authcmd.Prepared{}, errdef.New(
			errdef.CodeHTTP,
			"command auth support is not initialised",
		)
	}
	cfg, err := e.buildCommandAuthConfig(doc, auth, res, timeout)
	if err != nil {
		return authcmd.Prepared{}, err
	}
	return ac.Prepare(e.envName(env), cfg)
}

func (e *Engine) ensureOAuth(
	ctx context.Context,
	req *restfile.Request,
	res *vars.Resolver,
	opts httpclient.Options,
	env string,
	timeout time.Duration,
) error {
	if req == nil || req.Metadata.Auth == nil {
		return nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "oauth2") {
		return nil
	}
	oa := e.rt.OAuth()
	if oa == nil {
		return errdef.New(errdef.CodeHTTP, "oauth support is not initialised")
	}
	cfg, err := e.buildOAuthConfig(req.Metadata.Auth, res)
	if err != nil {
		return err
	}
	env = e.envName(env)
	cfg = oa.MergeCachedConfig(env, cfg)
	if cfg.TokenURL == "" {
		return errdef.New(
			errdef.CodeHTTP,
			"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
		)
	}
	grant := strings.ToLower(strings.TrimSpace(cfg.GrantType))
	hdr := cfg.Header
	if strings.TrimSpace(hdr) == "" {
		hdr = "Authorization"
	}
	if req.Headers != nil && req.Headers.Get(hdr) != "" {
		return nil
	}
	if grant == "authorization_code" && !oa.CanHeadless(env, cfg) {
		return errdef.New(
			errdef.CodeHTTP,
			"headless oauth authorization_code requires a cached or refreshable token; seed it outside CI or use a non-interactive grant",
		)
	}

	tmo := timeout
	if grant == "authorization_code" && tmo < 2*time.Minute {
		tmo = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, tmo)
	defer cancel()

	tok, err := oa.Token(ctx, env, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(hdr) != "" {
		return nil
	}

	val := tok.AccessToken
	if strings.EqualFold(hdr, "authorization") {
		typ := strings.TrimSpace(tok.TokenType)
		if typ == "" {
			typ = "Bearer"
		}
		val = strings.TrimSpace(typ) + " " + tok.AccessToken
	}
	req.Headers.Set(hdr, val)
	return nil
}

func (e *Engine) buildCommandAuthConfig(
	doc *restfile.Document,
	auth *restfile.AuthSpec,
	res *vars.Resolver,
	timeout time.Duration,
) (authcmd.Config, error) {
	cfg := authcmd.Config{}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing command auth spec")
	}
	pm := make(map[string]string, len(auth.Params))
	for k, raw := range auth.Params {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		if res != nil && k != "argv" {
			out, err := res.ExpandTemplates(val)
			if err != nil {
				return cfg, errdef.Wrap(errdef.CodeHTTP, err, "expand command auth param %s", k)
			}
			val = strings.TrimSpace(out)
		}
		pm[k] = val
	}

	dir := ""
	if path := e.filePath(doc); path != "" {
		dir = filepath.Dir(path)
	}
	out, err := authcmd.Parse(pm, dir)
	if err != nil {
		return out, err
	}
	if res != nil {
		for i, arg := range out.Argv {
			val, err := res.ExpandTemplates(arg)
			if err != nil {
				return out, errdef.Wrap(errdef.CodeHTTP, err, "expand command auth argv[%d]", i)
			}
			out.Argv[i] = val
		}
	}
	return out.WithBaseTimeout(timeout), nil
}

func (e *Engine) buildOAuthConfig(
	auth *restfile.AuthSpec,
	res *vars.Resolver,
) (oauth.Config, error) {
	cfg := oauth.Config{Extra: make(map[string]string)}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing oauth spec")
	}
	expand := func(key string) (string, error) {
		val := auth.Params[key]
		if strings.TrimSpace(val) == "" {
			return "", nil
		}
		if res == nil {
			return strings.TrimSpace(val), nil
		}
		out, err := res.ExpandTemplates(val)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand oauth param %s", key)
		}
		return strings.TrimSpace(out), nil
	}

	var err error
	if cfg.TokenURL, err = expand("token_url"); err != nil {
		return cfg, err
	}
	if cfg.AuthURL, err = expand("auth_url"); err != nil {
		return cfg, err
	}
	if cfg.RedirectURL, err = expand("redirect_uri"); err != nil {
		return cfg, err
	}
	if cfg.ClientID, err = expand("client_id"); err != nil {
		return cfg, err
	}
	if cfg.ClientSecret, err = expand("client_secret"); err != nil {
		return cfg, err
	}
	if cfg.Scope, err = expand("scope"); err != nil {
		return cfg, err
	}
	if cfg.Audience, err = expand("audience"); err != nil {
		return cfg, err
	}
	if cfg.Resource, err = expand("resource"); err != nil {
		return cfg, err
	}
	if cfg.Username, err = expand("username"); err != nil {
		return cfg, err
	}
	if cfg.Password, err = expand("password"); err != nil {
		return cfg, err
	}
	if cfg.ClientAuth, err = expand("client_auth"); err != nil {
		return cfg, err
	}
	if cfg.GrantType, err = expand("grant"); err != nil {
		return cfg, err
	}
	if cfg.Header, err = expand("header"); err != nil {
		return cfg, err
	}
	if cfg.CacheKey, err = expand("cache_key"); err != nil {
		return cfg, err
	}
	if cfg.CodeVerifier, err = expand("code_verifier"); err != nil {
		return cfg, err
	}
	if cfg.CodeMethod, err = expand("code_challenge_method"); err != nil {
		return cfg, err
	}
	if cfg.State, err = expand("state"); err != nil {
		return cfg, err
	}

	known := map[string]struct{}{
		"token_url":             {},
		"auth_url":              {},
		"redirect_uri":          {},
		"client_id":             {},
		"client_secret":         {},
		"scope":                 {},
		"audience":              {},
		"resource":              {},
		"username":              {},
		"password":              {},
		"client_auth":           {},
		"grant":                 {},
		"header":                {},
		"cache_key":             {},
		"code_verifier":         {},
		"code_challenge_method": {},
		"state":                 {},
	}
	for k, raw := range auth.Params {
		if _, ok := known[strings.ToLower(k)]; ok {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		val, err := expand(k)
		if err != nil {
			return cfg, err
		}
		if val != "" {
			cfg.Extra[strings.ToLower(strings.ReplaceAll(k, "-", "_"))] = val
		}
	}
	if len(cfg.Extra) == 0 {
		cfg.Extra = nil
	}
	return cfg, nil
}

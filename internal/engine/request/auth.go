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

const (
	authTypeCommand = "command"
	authTypeOAuth2  = "oauth2"

	authParamArgv = "argv"

	errCommandAuthNotInitialized = "command auth support is not initialised"
	errOAuthNotInitialized       = "oauth support is not initialised"
	errMissingCommandAuthSpec    = "missing command auth spec"
	errMissingOAuthSpec          = "missing oauth spec"
	errOAuthTokenURLRequired     = "@auth oauth2 requires token_url (include it once per cache_key to seed the cache)"
	errOAuthHeadlessSeedRequired = "headless oauth authorization_code requires a cached or refreshable token; seed it outside CI or use a non-interactive grant"

	minOAuthAuthorizationCodeTimeout = 2 * time.Minute
)

type oauthConfigField struct {
	key string
	set func(*oauth.Config, string)
}

var oauthConfigFields = []oauthConfigField{
	{key: "token_url", set: func(cfg *oauth.Config, value string) { cfg.TokenURL = value }},
	{key: "auth_url", set: func(cfg *oauth.Config, value string) { cfg.AuthURL = value }},
	{key: "redirect_uri", set: func(cfg *oauth.Config, value string) { cfg.RedirectURL = value }},
	{key: "client_id", set: func(cfg *oauth.Config, value string) { cfg.ClientID = value }},
	{key: "client_secret", set: func(cfg *oauth.Config, value string) { cfg.ClientSecret = value }},
	{key: "scope", set: func(cfg *oauth.Config, value string) { cfg.Scope = value }},
	{key: "audience", set: func(cfg *oauth.Config, value string) { cfg.Audience = value }},
	{key: "resource", set: func(cfg *oauth.Config, value string) { cfg.Resource = value }},
	{key: "username", set: func(cfg *oauth.Config, value string) { cfg.Username = value }},
	{key: "password", set: func(cfg *oauth.Config, value string) { cfg.Password = value }},
	{key: "client_auth", set: func(cfg *oauth.Config, value string) { cfg.ClientAuth = value }},
	{key: "grant", set: func(cfg *oauth.Config, value string) { cfg.GrantType = value }},
	{key: "header", set: func(cfg *oauth.Config, value string) { cfg.Header = value }},
	{key: "cache_key", set: func(cfg *oauth.Config, value string) { cfg.CacheKey = value }},
	{key: "code_verifier", set: func(cfg *oauth.Config, value string) { cfg.CodeVerifier = value }},
	{key: "code_challenge_method", set: func(cfg *oauth.Config, value string) { cfg.CodeMethod = value }},
	{key: "state", set: func(cfg *oauth.Config, value string) { cfg.State = value }},
}

func (e *Engine) resolveInheritedAuth(doc *restfile.Document, req *restfile.Request) {
	if req == nil || requestAuth(req) != nil || req.Metadata.AuthDisabled {
		return
	}
	if pf, ok := e.registryIndex().DefaultAuth(doc); ok {
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
	hdr := injectedAuthHeader(auth)
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

func requestAuth(req *restfile.Request) *restfile.AuthSpec {
	if req == nil {
		return nil
	}
	return req.Metadata.Auth
}

func requestAuthOfType(req *restfile.Request, kind string) *restfile.AuthSpec {
	auth := requestAuth(req)
	if authKind(auth) != kind {
		return nil
	}
	return auth
}

func authKind(auth *restfile.AuthSpec) string {
	if auth == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(auth.Type))
}

func injectedAuthHeader(auth *restfile.AuthSpec) string {
	if authKind(auth) != authTypeOAuth2 {
		return oauth.DefaultHeader
	}
	if name := strings.TrimSpace(auth.Params["header"]); name != "" {
		return name
	}
	return oauth.DefaultHeader
}

func headerPresent(h http.Header, name string) bool {
	return h != nil && h.Get(name) != ""
}

func requestHeaderPresent(req *restfile.Request, name string) bool {
	return headerPresent(reqHeaders(req), name)
}

func ensureReqHeaders(req *restfile.Request) http.Header {
	if req == nil {
		return nil
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	return req.Headers
}

func setRequestHeaderIfMissing(req *restfile.Request, name, value string) bool {
	headers := ensureReqHeaders(req)
	if headers == nil || headerPresent(headers, name) {
		return false
	}
	headers.Set(name, value)
	return true
}

func headerValue(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	return strings.TrimSpace(h.Get(name))
}

func (e *Engine) authCmdManager() (*authcmd.Manager, error) {
	if e == nil || e.rt == nil {
		return nil, errdef.New(errdef.CodeHTTP, errCommandAuthNotInitialized)
	}
	ac := e.rt.AuthCmd()
	if ac == nil {
		return nil, errdef.New(errdef.CodeHTTP, errCommandAuthNotInitialized)
	}
	return ac, nil
}

func (e *Engine) oauthManager() (*oauth.Manager, error) {
	if e == nil || e.rt == nil {
		return nil, errdef.New(errdef.CodeHTTP, errOAuthNotInitialized)
	}
	oa := e.rt.OAuth()
	if oa == nil {
		return nil, errdef.New(errdef.CodeHTTP, errOAuthNotInitialized)
	}
	return oa, nil
}

func (e *Engine) ensureCommandAuth(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
	timeout time.Duration,
) (authcmd.Result, error) {
	auth := requestAuthOfType(req, authTypeCommand)
	if auth == nil {
		return authcmd.Result{}, nil
	}
	prep, err := e.prepareCommandAuth(doc, auth, res, env, timeout)
	if err != nil {
		return authcmd.Result{}, err
	}
	hdr := prep.HeaderName()
	if requestHeaderPresent(req, hdr) {
		return authcmd.Result{}, nil
	}

	ac, err := e.authCmdManager()
	if err != nil {
		return authcmd.Result{}, err
	}
	out, err := ac.ResolvePrepared(ctx, prep)
	if err != nil {
		return authcmd.Result{}, errdef.Wrap(errdef.CodeHTTP, err, "resolve command auth")
	}
	if !setRequestHeaderIfMissing(req, out.Header, out.Value) {
		return authcmd.Result{}, nil
	}
	return out, nil
}

func (e *Engine) prepareCommandAuth(
	doc *restfile.Document,
	auth *restfile.AuthSpec,
	res *vars.Resolver,
	env string,
	timeout time.Duration,
) (authcmd.Prepared, error) {
	ac, err := e.authCmdManager()
	if err != nil {
		return authcmd.Prepared{}, err
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
	auth := requestAuthOfType(req, authTypeOAuth2)
	if auth == nil {
		return nil
	}
	oa, err := e.oauthManager()
	if err != nil {
		return err
	}
	cfg, err := e.buildOAuthConfig(auth, res)
	if err != nil {
		return err
	}
	env = e.envName(env)
	cfg = oa.MergeCachedConfig(env, cfg)
	if cfg.TokenURL == "" {
		return errdef.New(errdef.CodeHTTP, errOAuthTokenURLRequired)
	}
	hdr := cfg.Header
	if requestHeaderPresent(req, hdr) {
		return nil
	}
	if e.oauthNeedsHeadlessSeed(oa, env, cfg) {
		return errdef.New(errdef.CodeHTTP, errOAuthHeadlessSeedRequired)
	}

	tmo := oauthTimeout(cfg.GrantType, timeout)
	ctx, cancel := context.WithTimeout(ctx, tmo)
	defer cancel()

	tok, err := oa.Token(ctx, env, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	setRequestHeaderIfMissing(req, hdr, oauthHeaderValue(hdr, tok))
	return nil
}

func (e *Engine) allowInteractiveOAuth() bool {
	return e != nil && e.cfg.AllowInteractiveOAuth
}

func (e *Engine) oauthNeedsHeadlessSeed(oa *oauth.Manager, env string, cfg oauth.Config) bool {
	if cfg.GrantType != oauth.GrantAuthorizationCode {
		return false
	}
	if e.allowInteractiveOAuth() || oa == nil {
		return false
	}
	return !oa.CanHeadless(env, cfg)
}

func (e *Engine) buildCommandAuthConfig(
	doc *restfile.Document,
	auth *restfile.AuthSpec,
	res *vars.Resolver,
	timeout time.Duration,
) (authcmd.Config, error) {
	cfg := authcmd.Config{}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, errMissingCommandAuthSpec)
	}
	pm, err := commandAuthParams(auth, res)
	if err != nil {
		return cfg, err
	}

	dir := ""
	if path := e.filePath(doc); path != "" {
		dir = filepath.Dir(path)
	}
	out, err := authcmd.Parse(pm, dir)
	if err != nil {
		return out, err
	}
	if err := expandCommandAuthArgv(out.Argv, res); err != nil {
		return out, err
	}
	return out.WithBaseTimeout(timeout), nil
}

func (e *Engine) buildOAuthConfig(
	auth *restfile.AuthSpec,
	res *vars.Resolver,
) (oauth.Config, error) {
	cfg := oauth.Config{}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, errMissingOAuthSpec)
	}
	for _, field := range oauthConfigFields {
		value, err := expandAuthParam(res, authTypeOAuth2, field.key, auth.Params[field.key])
		if err != nil {
			return cfg, err
		}
		field.set(&cfg, value)
	}
	extra, err := oauthExtraParams(auth, res)
	if err != nil {
		return cfg, err
	}
	cfg.Extra = extra
	return cfg.Normalized(), nil
}

func commandAuthParams(auth *restfile.AuthSpec, res *vars.Resolver) (map[string]string, error) {
	if auth == nil || len(auth.Params) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(auth.Params))
	for rawKey, rawValue := range auth.Params {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if key == "" {
			continue
		}

		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}
		if key != authParamArgv {
			var err error
			value, err = expandAuthParam(res, authTypeCommand, key, value)
			if err != nil {
				return nil, err
			}
		}
		out[key] = value
	}
	return out, nil
}

func expandCommandAuthArgv(argv []string, res *vars.Resolver) error {
	if res == nil {
		return nil
	}
	for i, arg := range argv {
		value, err := res.ExpandTemplates(arg)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "expand command auth argv[%d]", i)
		}
		argv[i] = value
	}
	return nil
}

func expandAuthParam(res *vars.Resolver, scope, key, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if res == nil {
		return strings.TrimSpace(raw), nil
	}
	value, err := res.ExpandTemplates(raw)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "expand %s param %s", scope, key)
	}
	return strings.TrimSpace(value), nil
}

func oauthExtraParams(auth *restfile.AuthSpec, res *vars.Resolver) (map[string]string, error) {
	if auth == nil || len(auth.Params) == 0 {
		return nil, nil
	}
	out := make(map[string]string)
	for rawKey, rawValue := range auth.Params {
		if isKnownOAuthParam(strings.ToLower(rawKey)) || strings.TrimSpace(rawValue) == "" {
			continue
		}
		value, err := expandAuthParam(res, authTypeOAuth2, rawKey, rawValue)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		out[strings.ToLower(strings.ReplaceAll(rawKey, "-", "_"))] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func isKnownOAuthParam(key string) bool {
	for _, field := range oauthConfigFields {
		if field.key == key {
			return true
		}
	}
	return false
}

func oauthTimeout(grant string, timeout time.Duration) time.Duration {
	if grant == oauth.GrantAuthorizationCode && timeout < minOAuthAuthorizationCodeTimeout {
		return minOAuthAuthorizationCodeTimeout
	}
	return timeout
}

func oauthHeaderValue(header string, tok oauth.Token) string {
	if !strings.EqualFold(header, oauth.DefaultHeader) {
		return tok.AccessToken
	}
	typ := strings.TrimSpace(tok.TokenType)
	if typ == "" {
		typ = "Bearer"
	}
	return strings.TrimSpace(typ) + " " + tok.AccessToken
}

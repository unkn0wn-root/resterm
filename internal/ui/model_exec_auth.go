package ui

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type explainAuthPreviewResult struct {
	status       xplain.StageStatus
	summary      string
	notes        []string
	extraSecrets []string
}

func explainAuthSecretValues(auth *restfile.AuthSpec, resolver *vars.Resolver) []string {
	if auth == nil || len(auth.Params) == 0 {
		return nil
	}

	expand := func(key string) string {
		value := strings.TrimSpace(auth.Params[key])
		if value == "" {
			return ""
		}
		if resolver == nil {
			return value
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return value
		}
		return strings.TrimSpace(expanded)
	}

	values := make(map[string]struct{})
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		values[value] = struct{}{}
	}

	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "basic":
		add(expand("password"))
	case "bearer":
		add(expand("token"))
	case "apikey", "api-key", "header":
		add(expand("value"))
	case "oauth2":
		for _, key := range []string{"client_secret", "password", "refresh_token", "access_token"} {
			add(expand(key))
		}
	}

	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func explainInjectedAuthSecrets(
	auth *restfile.AuthSpec,
	before *restfile.Request,
	after *restfile.Request,
) []string {
	if auth == nil || after == nil {
		return nil
	}
	header := "Authorization"
	if strings.EqualFold(strings.TrimSpace(auth.Type), "oauth2") {
		if name := strings.TrimSpace(auth.Params["header"]); name != "" {
			header = name
		}
	}
	beforeValue := headerValue(reqHeaders(before), header)
	afterValue := headerValue(reqHeaders(after), header)
	if strings.TrimSpace(afterValue) == "" || afterValue == beforeValue {
		return nil
	}

	values := []string{afterValue}
	if strings.EqualFold(header, "authorization") {
		_, token, ok := strings.Cut(afterValue, " ")
		if ok {
			token = strings.TrimSpace(token)
			if token != "" {
				values = append(values, token)
			}
		}
	}
	return values
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

func lookupDefaultAuthProfile(
	xs []restfile.AuthProfile,
	scope restfile.AuthScope,
) (*restfile.AuthProfile, bool) {
	for i := len(xs) - 1; i >= 0; i-- {
		profile := &xs[i]
		if profile.Scope != scope {
			continue
		}
		if strings.TrimSpace(profile.Name) != "" {
			continue
		}
		return profile, true
	}
	return nil, false
}

func (m *Model) resolveInheritedAuth(doc *restfile.Document, req *restfile.Request) {
	if req == nil || req.Metadata.Auth != nil || req.Metadata.AuthDisabled {
		return
	}

	if profile, ok := lookupDefaultAuthProfile(docAuthProfiles(doc), restfile.AuthScopeFile); ok {
		req.Metadata.Auth = restfile.CloneAuthSpec(&profile.Spec)
		return
	}
	if profile, ok := lookupDefaultAuthProfile(docAuthProfiles(doc), restfile.AuthScopeGlobal); ok {
		req.Metadata.Auth = restfile.CloneAuthSpec(&profile.Spec)
		return
	}
	if m.authGlobals == nil {
		return
	}
	if profile, ok := lookupDefaultAuthProfile(m.authGlobals.all(), restfile.AuthScopeGlobal); ok {
		req.Metadata.Auth = restfile.CloneAuthSpec(&profile.Spec)
	}
}

func (m *Model) prepareExplainAuthPreview(
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (explainAuthPreviewResult, error) {
	if req == nil || req.Metadata.Auth == nil {
		return explainAuthPreviewResult{}, nil
	}

	auth := req.Metadata.Auth
	kind := strings.ToLower(strings.TrimSpace(auth.Type))
	switch kind {
	case "", "basic", "bearer", "apikey", "api-key", "header":
		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: explainSummaryAuthPrepared,
			notes:   []string{"auth headers/query are applied during HTTP request build"},
		}, nil
	case "command":
		prep, err := m.prepareCommandAuth(auth, resolver, envName, 0)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}
		header := prep.HeaderName()
		if req.Headers != nil && req.Headers.Get(header) != "" {
			return explainAuthPreviewResult{
				status:  xplain.StageOK,
				summary: explainSummaryAuthPrepared,
				notes:   []string{"auth header already set on request"},
			}, nil
		}

		ac := m.authCmdMgr()
		if ac == nil {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"command auth support is not initialised",
			)
		}
		res, ok, err := ac.CachedPrepared(prep)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}
		if !ok {
			return explainAuthPreviewResult{
				status:  xplain.StageSkipped,
				summary: explainSummaryCommandAuthExecutionSkipped,
				notes: []string{
					"Command auth execution is skipped in explain preview",
					fmt.Sprintf("%s is omitted without a cached command auth result", header),
				},
			}, nil
		}

		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		req.Headers.Set(res.Header, res.Value)

		return explainAuthPreviewResult{
			status:       xplain.StageOK,
			summary:      explainSummaryAuthPrepared,
			notes:        []string{"used cached command auth result for explain preview"},
			extraSecrets: commandAuthSecrets(res),
		}, nil
	case "oauth2":
		oa := m.oauthMgr()
		if oa == nil {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"oauth support is not initialised",
			)
		}

		cfg, err := m.buildOAuthConfig(auth, resolver)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}

		envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
		cfg = oa.MergeCachedConfig(envKey, cfg)
		if cfg.TokenURL == "" {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
			)
		}

		header := strings.TrimSpace(cfg.Header)
		if header == "" {
			header = "Authorization"
		}
		if req.Headers != nil && req.Headers.Get(header) != "" {
			return explainAuthPreviewResult{
				status:  xplain.StageOK,
				summary: explainSummaryAuthPrepared,
				notes:   []string{"auth header already set on request"},
			}, nil
		}

		token, ok := oa.CachedToken(envKey, cfg)
		if !ok {
			return explainAuthPreviewResult{
				status:  xplain.StageSkipped,
				summary: explainSummaryOAuthTokenFetchSkipped,
				notes: []string{
					"OAuth token acquisition is skipped in explain preview",
					fmt.Sprintf("%s is omitted without a cached token", header),
				},
			}, nil
		}

		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		value := token.AccessToken
		if strings.EqualFold(header, "authorization") {
			typeValue := strings.TrimSpace(token.TokenType)
			if typeValue == "" {
				typeValue = "Bearer"
			}
			value = strings.TrimSpace(typeValue) + " " + token.AccessToken
		}
		req.Headers.Set(header, value)

		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: explainSummaryAuthPrepared,
			notes:   []string{"used cached OAuth token for explain preview"},
			extraSecrets: []string{
				token.AccessToken,
				value,
			},
		}, nil
	default:
		return explainAuthPreviewResult{
			status:  xplain.StageSkipped,
			summary: explainSummaryAuthTypeNotApplied,
			notes:   []string{fmt.Sprintf("unsupported auth type %q is not applied", auth.Type)},
		}, nil
	}
}

func (m *Model) ensureCommandAuth(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
	timeout time.Duration,
) (authcmd.Result, error) {
	if req == nil || req.Metadata.Auth == nil {
		return authcmd.Result{}, nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "command") {
		return authcmd.Result{}, nil
	}
	prep, err := m.prepareCommandAuth(req.Metadata.Auth, resolver, envName, timeout)
	if err != nil {
		return authcmd.Result{}, err
	}
	header := prep.HeaderName()
	if req.Headers != nil && req.Headers.Get(header) != "" {
		return authcmd.Result{}, nil
	}

	ac := m.authCmdMgr()
	if ac == nil {
		return authcmd.Result{}, errdef.New(
			errdef.CodeHTTP,
			"command auth support is not initialised",
		)
	}
	res, err := ac.ResolvePrepared(ctx, prep)
	if err != nil {
		return authcmd.Result{}, errdef.Wrap(errdef.CodeHTTP, err, "resolve command auth")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(res.Header) != "" {
		return authcmd.Result{}, nil
	}

	req.Headers.Set(res.Header, res.Value)
	return res, nil
}

func (m *Model) prepareCommandAuth(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	envName string,
	timeout time.Duration,
) (authcmd.Prepared, error) {
	ac := m.authCmdMgr()
	if ac == nil {
		return authcmd.Prepared{}, errdef.New(
			errdef.CodeHTTP,
			"command auth support is not initialised",
		)
	}

	cfg, err := m.buildCommandAuthConfig(auth, resolver, timeout)
	if err != nil {
		return authcmd.Prepared{}, err
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	return ac.Prepare(envKey, cfg)
}

func (m *Model) ensureOAuth(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts httpclient.Options,
	envName string,
	timeout time.Duration,
) error {
	if req == nil || req.Metadata.Auth == nil {
		return nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "oauth2") {
		return nil
	}
	oa := m.oauthMgr()
	if oa == nil {
		return errdef.New(errdef.CodeHTTP, "oauth support is not initialised")
	}

	cfg, err := m.buildOAuthConfig(req.Metadata.Auth, resolver)
	if err != nil {
		return err
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	cfg = oa.MergeCachedConfig(envKey, cfg)
	if cfg.TokenURL == "" {
		return errdef.New(
			errdef.CodeHTTP,
			"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
		)
	}

	grant := strings.ToLower(strings.TrimSpace(cfg.GrantType))
	header := cfg.Header
	if strings.TrimSpace(header) == "" {
		header = "Authorization"
	}
	if req.Headers != nil && req.Headers.Get(header) != "" {
		return nil
	}
	tokenTimeout := timeout
	if grant == "authorization_code" && tokenTimeout < 2*time.Minute {
		tokenTimeout = 2 * time.Minute
		m.setStatusMessage(
			statusMsg{
				level: statusInfo,
				text:  "Open browser to complete OAuth (auth code/PKCE). Press send again to cancel.",
			},
		)
	}

	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)

	defer cancel()

	token, err := oa.Token(ctx, envKey, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(header) != "" {
		return nil
	}

	value := token.AccessToken
	if strings.EqualFold(header, "authorization") {
		typeValue := strings.TrimSpace(token.TokenType)
		if typeValue == "" {
			typeValue = "Bearer"
		}
		value = strings.TrimSpace(typeValue) + " " + token.AccessToken
	}

	req.Headers.Set(header, value)
	return nil
}

func (m *Model) buildCommandAuthConfig(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	timeout time.Duration,
) (authcmd.Config, error) {
	cfg := authcmd.Config{}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing command auth spec")
	}

	params := make(map[string]string, len(auth.Params))
	for key, raw := range auth.Params {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if resolver != nil && key != "argv" {
			expanded, err := resolver.ExpandTemplates(value)
			if err != nil {
				return cfg, errdef.Wrap(errdef.CodeHTTP, err, "expand command auth param %s", key)
			}
			value = strings.TrimSpace(expanded)
		}
		params[key] = value
	}

	dir := ""
	if m.currentFile != "" {
		dir = filepath.Dir(m.currentFile)
	}
	cfg, err := authcmd.Parse(params, dir)
	if err != nil {
		return cfg, err
	}
	if resolver != nil {
		for i, arg := range cfg.Argv {
			expanded, err := resolver.ExpandTemplates(arg)
			if err != nil {
				return cfg, errdef.Wrap(errdef.CodeHTTP, err, "expand command auth argv[%d]", i)
			}
			cfg.Argv[i] = expanded
		}
	}
	return cfg.WithBaseTimeout(timeout), nil
}

func (m *Model) buildOAuthConfig(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
) (oauth.Config, error) {
	cfg := oauth.Config{Extra: make(map[string]string)}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing oauth spec")
	}

	expand := func(key string) (string, error) {
		value := auth.Params[key]
		if strings.TrimSpace(value) == "" {
			return "", nil
		}
		if resolver == nil {
			return strings.TrimSpace(value), nil
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand oauth param %s", key)
		}
		return strings.TrimSpace(expanded), nil
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
	for key, raw := range auth.Params {
		if _, ok := known[strings.ToLower(key)]; ok {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := expand(key)
		if err != nil {
			return cfg, err
		}
		if value != "" {
			cfg.Extra[strings.ToLower(strings.ReplaceAll(key, "-", "_"))] = value
		}
	}
	if len(cfg.Extra) == 0 {
		cfg.Extra = nil
	}
	return cfg, nil
}

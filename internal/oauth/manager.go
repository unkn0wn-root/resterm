package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Config struct {
	TokenURL     string
	AuthURL      string
	RedirectURL  string
	ClientID     string
	ClientSecret string
	Scope        string
	Audience     string
	Resource     string
	Username     string
	Password     string
	ClientAuth   string
	GrantType    string
	Header       string
	CacheKey     string
	Code         string
	CodeVerifier string
	CodeMethod   string
	State        string
	Extra        map[string]string
}

type Token struct {
	AccessToken  string
	TokenType    string
	RefreshToken string
	Expiry       time.Time
	Raw          map[string]any
}

type Manager struct {
	client *httpclient.Client

	mu       sync.Mutex
	cache    map[string]*cacheEntry
	inflight map[string]*call
	do       func(context.Context, *restfile.Request, httpclient.Options) (*httpclient.Response, error)
}

type cacheEntry struct {
	token Token
	cfg   Config
}

type call struct {
	done  chan struct{}
	token Token
	err   error
}

type tokenResponse struct {
	AccessToken  string          `json:"access_token"`
	TokenType    string          `json:"token_type"`
	ExpiresIn    json.Number     `json:"expires_in"`
	RefreshToken string          `json:"refresh_token"`
	Raw          json.RawMessage `json:"-"`
}

const expirySlack = 30 * time.Second

func NewManager(client *httpclient.Client) *Manager {
	if client == nil {
		client = httpclient.NewClient(nil)
	}

	mgr := &Manager{
		client:   client,
		cache:    make(map[string]*cacheEntry),
		inflight: make(map[string]*call),
	}
	mgr.do = func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
		return mgr.client.Execute(ctx, req, nil, opts)
	}
	return mgr
}

// Deduplicates concurrent token requests for the same config.
// If another goroutine is already fetching, we wait on their done channel
// instead of hitting the auth server twice.
func (m *Manager) Token(
	ctx context.Context,
	env string,
	cfg Config,
	opts httpclient.Options,
) (Token, error) {
	key := m.cacheKey(env, cfg)

	if token, ok := m.cachedToken(key); ok && token.valid() {
		return token, nil
	}

	m.mu.Lock()
	if call, ok := m.inflight[key]; ok {
		done := call.done
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Token{}, ctx.Err()
		case <-done:
			if call.err != nil {
				return Token{}, call.err
			}
			return call.token, nil
		}
	}
	call := &call{done: make(chan struct{})}
	m.inflight[key] = call
	m.mu.Unlock()

	token, err := m.obtainToken(ctx, key, cfg, opts)
	call.token = token
	call.err = err
	close(call.done)

	m.mu.Lock()
	delete(m.inflight, key)
	m.mu.Unlock()

	if err != nil {
		return Token{}, err
	}
	return token, nil
}

func (m *Manager) obtainToken(
	ctx context.Context,
	key string,
	cfg Config,
	opts httpclient.Options,
) (Token, error) {
	if token, ok := m.cachedToken(key); ok && token.valid() {
		return token, nil
	}

	entry := m.cacheEntry(key)
	if entry != nil && entry.token.RefreshToken != "" {
		if refreshed, err := m.refreshToken(
			ctx,
			entry.cfg,
			entry.token.RefreshToken,
			opts,
		); err == nil {
			m.storeToken(key, cfg, refreshed)
			return refreshed, nil
		}
	}

	if strings.EqualFold(strings.TrimSpace(cfg.GrantType), "authorization_code") {
		return m.requestAuthCodeToken(ctx, key, cfg, opts)
	}

	fetched, err := m.requestToken(ctx, cfg, opts)
	if err != nil {
		return Token{}, err
	}

	m.storeToken(key, cfg, fetched)
	return fetched, nil
}

func (m *Manager) SetRequestFunc(
	fn func(context.Context, *restfile.Request, httpclient.Options) (*httpclient.Response, error),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		m.do = func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			return m.client.Execute(ctx, req, nil, opts)
		}
		return
	}
	m.do = fn
}

func (m *Manager) cachedToken(key string) (Token, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cache[key]
	if !ok {
		return Token{}, false
	}
	return entry.token, true
}

func (m *Manager) cacheEntry(key string) *cacheEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cache[key]
}

func (m *Manager) storeToken(key string, cfg Config, token Token) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = make(map[string]*cacheEntry)
	}
	m.cache[key] = &cacheEntry{token: token, cfg: cfg}
}

// MergeCachedConfig fills empty fields in cfg from any cached config that shares the cache key.
// This allows follow-up requests to omit repeated parameters (auth_url, token_url, etc.) as long
// as the initial request stored a config under the same cache_key.
func (m *Manager) MergeCachedConfig(env string, cfg Config) Config {
	if strings.TrimSpace(cfg.CacheKey) == "" {
		return cfg
	}

	key := m.cacheKey(env, cfg)
	entry := m.cacheEntry(key)
	if entry == nil {
		return cfg
	}

	base := entry.cfg
	merged := cfg

	merged.TokenURL = inheritIfEmpty(merged.TokenURL, base.TokenURL)
	merged.AuthURL = inheritIfEmpty(merged.AuthURL, base.AuthURL)
	merged.RedirectURL = inheritIfEmpty(merged.RedirectURL, base.RedirectURL)
	merged.ClientID = inheritIfEmpty(merged.ClientID, base.ClientID)
	merged.ClientSecret = inheritIfEmpty(merged.ClientSecret, base.ClientSecret)
	merged.Scope = inheritIfEmpty(merged.Scope, base.Scope)
	merged.Audience = inheritIfEmpty(merged.Audience, base.Audience)
	merged.Resource = inheritIfEmpty(merged.Resource, base.Resource)
	merged.Username = inheritIfEmpty(merged.Username, base.Username)
	merged.Password = inheritIfEmpty(merged.Password, base.Password)
	merged.ClientAuth = inheritIfEmpty(merged.ClientAuth, base.ClientAuth)
	merged.GrantType = inheritIfEmpty(merged.GrantType, base.GrantType)
	merged.Header = inheritIfEmpty(merged.Header, base.Header)
	merged.CodeVerifier = inheritIfEmpty(merged.CodeVerifier, base.CodeVerifier)
	merged.CodeMethod = inheritIfEmpty(merged.CodeMethod, base.CodeMethod)
	merged.State = inheritIfEmpty(merged.State, base.State)

	merged.Extra = mergeExtras(base.Extra, cfg.Extra)
	return merged
}

func inheritIfEmpty(current, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return fallback
}

func mergeExtras(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		if strings.TrimSpace(v) == "" {
			continue
		}
		merged[k] = v
	}
	return merged
}

func (m *Manager) cacheKey(env string, cfg Config) string {
	if strings.TrimSpace(cfg.CacheKey) != "" {
		return strings.TrimSpace(cfg.CacheKey)
	}

	parts := []string{
		strings.ToLower(strings.TrimSpace(env)),
		strings.TrimSpace(cfg.TokenURL),
		strings.TrimSpace(cfg.AuthURL),
		strings.TrimSpace(cfg.RedirectURL),
		strings.TrimSpace(cfg.ClientID),
		strings.TrimSpace(cfg.Scope),
		strings.TrimSpace(cfg.Audience),
		strings.TrimSpace(cfg.Resource),
		strings.ToLower(strings.TrimSpace(cfg.GrantType)),
		strings.ToLower(strings.TrimSpace(cfg.CodeMethod)),
		strings.TrimSpace(cfg.CodeVerifier),
		strings.TrimSpace(cfg.Username),
		strings.ToLower(strings.TrimSpace(cfg.ClientAuth)),
	}
	if len(cfg.Extra) > 0 {
		keys := make([]string, 0, len(cfg.Extra))
		for k := range cfg.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, k+"="+cfg.Extra[k])
		}
	}
	return strings.Join(parts, "|")
}

func (m *Manager) requestToken(
	ctx context.Context,
	cfg Config,
	opts httpclient.Options,
) (Token, error) {
	grant := strings.ToLower(strings.TrimSpace(cfg.GrantType))
	if grant == "" {
		grant = "client_credentials"
	}

	form := url.Values{}
	form.Set("grant_type", grant)
	if cfg.Scope != "" {
		form.Set("scope", cfg.Scope)
	}
	if cfg.Audience != "" {
		form.Set("audience", cfg.Audience)
	}
	if cfg.Resource != "" {
		form.Set("resource", cfg.Resource)
	}
	for k, v := range cfg.Extra {
		if k != "" && v != "" {
			form.Set(k, v)
		}
	}

	authMode := resolveClientAuth(grant, strings.TrimSpace(cfg.ClientAuth), cfg)

	switch grant {
	case "client_credentials":
		if authMode.useBody {
			form.Set("client_id", cfg.ClientID)
			form.Set("client_secret", cfg.ClientSecret)
		}
	case "password":
		form.Set("username", cfg.Username)
		form.Set("password", cfg.Password)
		if authMode.useBody {
			form.Set("client_id", cfg.ClientID)
			form.Set("client_secret", cfg.ClientSecret)
		}
	case "authorization_code":
		if cfg.Code == "" {
			return Token{}, errdef.New(errdef.CodeHTTP, "missing authorization code")
		}
		if strings.TrimSpace(cfg.RedirectURL) == "" {
			return Token{}, errdef.New(errdef.CodeHTTP, "authorization_code requires redirect_uri")
		}
		form.Set("code", cfg.Code)
		form.Set("redirect_uri", cfg.RedirectURL)
		if cfg.CodeVerifier != "" {
			form.Set("code_verifier", cfg.CodeVerifier)
		}
		if authMode.useBody || cfg.ClientSecret == "" {
			if cfg.ClientID != "" {
				form.Set("client_id", cfg.ClientID)
			}
			if cfg.ClientSecret != "" && authMode.useBody {
				form.Set("client_secret", cfg.ClientSecret)
			}
		}
	default:
		return Token{}, errdef.New(errdef.CodeHTTP, "unsupported oauth2 grant type: %s", grant)
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Accept", "application/json")
	if authMode.useHeader && cfg.ClientID != "" {
		credentials := cfg.ClientID + ":" + cfg.ClientSecret
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		headers.Set("Authorization", "Basic "+encoded)
	}

	req := &restfile.Request{
		Method:  "POST",
		URL:     cfg.TokenURL,
		Headers: headers,
		Body: restfile.BodySource{
			Text: form.Encode(),
		},
	}

	resp, err := m.do(ctx, req, opts)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Token{}, errdef.New(errdef.CodeHTTP, "oauth token request failed: %s", resp.Status)
	}

	token, err := parseTokenResponse(resp.Body)
	if err != nil {
		return Token{}, err
	}
	return token, nil
}

type clientAuthMode struct {
	useHeader bool
	useBody   bool
}

func resolveClientAuth(grant, clientAuthRaw string, cfg Config) clientAuthMode {
	mode := strings.ToLower(clientAuthRaw)
	if mode == "" {
		mode = "basic"
	}

	useHeader := mode == "basic"
	if useHeader && cfg.ClientSecret == "" &&
		(clientAuthRaw == "" || grant == "authorization_code") {
		useHeader = false
		mode = "body"
	}

	return clientAuthMode{
		useHeader: useHeader,
		useBody:   mode == "body",
	}
}

func (m *Manager) refreshToken(
	ctx context.Context,
	cfg Config,
	refresh string,
	opts httpclient.Options,
) (Token, error) {
	if refresh == "" {
		return Token{}, errdef.New(errdef.CodeHTTP, "missing refresh token")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	if cfg.Scope != "" {
		form.Set("scope", cfg.Scope)
	}
	if cfg.Audience != "" {
		form.Set("audience", cfg.Audience)
	}
	if cfg.Resource != "" {
		form.Set("resource", cfg.Resource)
	}
	for k, v := range cfg.Extra {
		if k != "" && v != "" {
			form.Set(k, v)
		}
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Accept", "application/json")
	clientAuth := strings.ToLower(strings.TrimSpace(cfg.ClientAuth))
	if clientAuth == "basic" && cfg.ClientID != "" {
		credentials := cfg.ClientID + ":" + cfg.ClientSecret
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		headers.Set("Authorization", "Basic "+encoded)
	} else {
		form.Set("client_id", cfg.ClientID)
		form.Set("client_secret", cfg.ClientSecret)
	}

	req := &restfile.Request{
		Method:  "POST",
		URL:     cfg.TokenURL,
		Headers: headers,
		Body:    restfile.BodySource{Text: form.Encode()},
	}

	resp, err := m.do(ctx, req, opts)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Token{}, errdef.New(errdef.CodeHTTP, "oauth token refresh failed: %s", resp.Status)
	}

	token, err := parseTokenResponse(resp.Body)
	if err != nil {
		return Token{}, err
	}
	return token, nil
}

func parseTokenResponse(body []byte) (Token, error) {
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// this covers that some legacy providers can return application/x-www-form-urlencoded
		// or unless an explicit Accept: application/json is sent. Fall back to decoding
		// the form-encoded body so these responses still work.
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return Token{}, errdef.Wrap(errdef.CodeHTTP, err, "decode oauth token response")
		}
		resp.AccessToken = values.Get("access_token")
		resp.TokenType = values.Get("token_type")
		resp.RefreshToken = values.Get("refresh_token")
		if expires := values.Get("expires_in"); expires != "" {
			resp.ExpiresIn = json.Number(expires)
		}
		return buildToken(resp, buildRawMap(values))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err == nil {
		return buildToken(resp, raw)
	}
	return buildToken(resp, nil)
}

func buildToken(resp tokenResponse, raw map[string]any) (Token, error) {
	if resp.AccessToken == "" {
		return Token{}, errdef.New(errdef.CodeHTTP, "oauth token response missing access_token")
	}
	if resp.TokenType == "" {
		resp.TokenType = "Bearer"
	}

	expiry := time.Time{}
	if resp.ExpiresIn != "" {
		if seconds, err := resp.ExpiresIn.Int64(); err == nil && seconds > 0 {
			expiry = time.Now().Add(time.Duration(seconds) * time.Second)
		}
	}

	token := Token{
		AccessToken:  resp.AccessToken,
		TokenType:    resp.TokenType,
		RefreshToken: resp.RefreshToken,
		Expiry:       expiry,
	}
	token.Raw = raw
	return token, nil
}

func buildRawMap(values url.Values) map[string]any {
	if len(values) == 0 {
		return nil
	}
	raw := make(map[string]any, len(values))
	for k, v := range values {
		if len(v) == 1 {
			raw[k] = v[0]
			continue
		}
		raw[k] = v
	}
	return raw
}

// Treats tokens expiring in the next 30 seconds as already expired
// to avoid racing with the actual expiration.
func (t Token) valid() bool {
	if t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return time.Now().Add(expirySlack).Before(t.Expiry)
}

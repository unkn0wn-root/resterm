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

// NewManager creates an OAuth manager with optional custom HTTP client.
func NewManager(client *httpclient.Client) *Manager {
	if client == nil {
		client = httpclient.NewClient(nil)
	}
	mgr := &Manager{client: client, cache: make(map[string]*cacheEntry), inflight: make(map[string]*call)}
	mgr.do = func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
		return mgr.client.Execute(ctx, req, nil, opts)
	}
	return mgr
}

// Token returns a cached or freshly fetched token, deduplicating concurrent requests.
func (m *Manager) Token(ctx context.Context, env string, cfg Config, opts httpclient.Options) (Token, error) {
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

// obtainToken tries cached tokens, refresh tokens, and finally fetches a new token.
func (m *Manager) obtainToken(ctx context.Context, key string, cfg Config, opts httpclient.Options) (Token, error) {
	if token, ok := m.cachedToken(key); ok && token.valid() {
		return token, nil
	}

	entry := m.cacheEntry(key)
	if entry != nil && entry.token.RefreshToken != "" {
		if refreshed, err := m.refreshToken(ctx, entry.cfg, entry.token.RefreshToken, opts); err == nil {
			m.storeToken(key, cfg, refreshed)
			return refreshed, nil
		}
	}

	fetched, err := m.requestToken(ctx, cfg, opts)
	if err != nil {
		return Token{}, err
	}

	m.storeToken(key, cfg, fetched)
	return fetched, nil
}

// SetRequestFunc overrides the HTTP execution function, enabling tests or custom transports.
func (m *Manager) SetRequestFunc(fn func(context.Context, *restfile.Request, httpclient.Options) (*httpclient.Response, error)) {
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

// cachedToken returns a token from cache without validating expiry.
func (m *Manager) cachedToken(key string) (Token, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cache[key]
	if !ok {
		return Token{}, false
	}
	return entry.token, true
}

// cacheEntry exposes the raw cache entry for refresh operations.
func (m *Manager) cacheEntry(key string) *cacheEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cache[key]
}

// storeToken updates the cache entry for a given key.
func (m *Manager) storeToken(key string, cfg Config, token Token) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = make(map[string]*cacheEntry)
	}
	m.cache[key] = &cacheEntry{token: token, cfg: cfg}
}

// cacheKey derives a stable cache key from the environment and config fields.
func (m *Manager) cacheKey(env string, cfg Config) string {
	if strings.TrimSpace(cfg.CacheKey) != "" {
		return strings.TrimSpace(cfg.CacheKey)
	}

	parts := []string{
		strings.ToLower(strings.TrimSpace(env)),
		strings.TrimSpace(cfg.TokenURL),
		strings.TrimSpace(cfg.ClientID),
		strings.TrimSpace(cfg.Scope),
		strings.TrimSpace(cfg.Audience),
		strings.TrimSpace(cfg.Resource),
		strings.ToLower(strings.TrimSpace(cfg.GrantType)),
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

// requestToken performs the grant-specific OAuth token request.
func (m *Manager) requestToken(ctx context.Context, cfg Config, opts httpclient.Options) (Token, error) {
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

	clientAuth := strings.ToLower(strings.TrimSpace(cfg.ClientAuth))
	if clientAuth == "" {
		clientAuth = "basic"
	}

	switch grant {
	case "client_credentials":
		if clientAuth == "body" {
			form.Set("client_id", cfg.ClientID)
			form.Set("client_secret", cfg.ClientSecret)
		}
	case "password":
		form.Set("username", cfg.Username)
		form.Set("password", cfg.Password)
		if clientAuth == "body" {
			form.Set("client_id", cfg.ClientID)
			form.Set("client_secret", cfg.ClientSecret)
		}
	default:
		return Token{}, errdef.New(errdef.CodeHTTP, "unsupported oauth2 grant type: %s", grant)
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientAuth == "basic" && cfg.ClientID != "" {
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

// refreshToken executes the refresh_token flow to rotate an access token.
func (m *Manager) refreshToken(ctx context.Context, cfg Config, refresh string, opts httpclient.Options) (Token, error) {
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

// parseTokenResponse decodes the JSON payload and populates expiry metadata.
func parseTokenResponse(body []byte) (Token, error) {
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Token{}, errdef.Wrap(errdef.CodeHTTP, err, "decode oauth token response")
	}

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

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err == nil {
		token.Raw = raw
	}

	return token, nil
}

// valid reports whether the token has a non-empty access token and has not expired.
func (t Token) valid() bool {
	if t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return time.Now().Add(expirySlack).Before(t.Expiry)
}

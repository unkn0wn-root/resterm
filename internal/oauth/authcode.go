package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

const (
	defaultCallbackPath = "/oauth/callback"
	verifierBytes       = 64
	stateBytes          = 24
)

var launchBrowser = openBrowser

func (m *Manager) requestAuthCodeToken(
	ctx context.Context,
	key string,
	cfg Config,
	opts httpclient.Options,
) (Token, error) {
	authURL := strings.TrimSpace(cfg.AuthURL)
	if authURL == "" {
		return Token{}, errdef.New(errdef.CodeHTTP, "authorization_code requires auth_url")
	}

	ver, err := pickVerifier(cfg.CodeVerifier)
	if err != nil {
		return Token{}, err
	}

	method := pickMethod(cfg.CodeMethod)
	challenge, err := buildChallenge(ver, method)
	if err != nil {
		return Token{}, err
	}

	state, err := pickState(cfg.State)
	if err != nil {
		return Token{}, err
	}

	redirect, ln, err := prepareRedirect(cfg.RedirectURL)
	if err != nil {
		return Token{}, err
	}
	defer func() {
		_ = ln.Close()
	}()

	srv := newCodeServer(redirect, state)
	srv.serve(ln)
	defer srv.shutdown(context.Background())

	link, err := buildAuthURL(authURL, redirect.String(), cfg, state, challenge, method)
	if err != nil {
		return Token{}, err
	}

	if err = launchBrowser(link); err != nil {
		fmt.Fprintf(os.Stderr, "Open this URL to complete OAuth: %s\n", link)
	}

	code, err := srv.wait(ctx)
	if err != nil {
		return Token{}, err
	}

	cfg.Code = code
	cfg.CodeVerifier = ver
	cfg.RedirectURL = redirect.String()

	token, err := m.requestToken(ctx, cfg, opts)
	if err != nil {
		return Token{}, err
	}

	m.storeToken(key, cfg, token)
	return token, nil
}

func pickVerifier(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v != "" {
		if len(v) < 43 || len(v) > 128 {
			return "", errdef.New(
				errdef.CodeHTTP,
				"code_verifier must be between 43 and 128 characters",
			)
		}
		return v, nil
	}
	return randString(verifierBytes)
}

func pickState(raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw), nil
	}
	return randString(stateBytes)
}

func pickMethod(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "plain":
		return "plain"
	default:
		return "s256"
	}
}

func buildChallenge(verifier, method string) (string, error) {
	switch strings.ToLower(method) {
	case "plain":
		return verifier, nil
	case "s256":
		sum := sha256Sum([]byte(verifier))
		return base64.RawURLEncoding.EncodeToString(sum), nil
	default:
		return "", errdef.New(errdef.CodeHTTP, "unsupported code_challenge_method: %s", method)
	}
}

func buildAuthURL(
	base, redirect string,
	cfg Config,
	state, challenge, method string,
) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "parse auth_url")
	}

	q := u.Query()
	q.Set("response_type", "code")
	if cfg.ClientID != "" {
		q.Set("client_id", cfg.ClientID)
	}
	q.Set("redirect_uri", redirect)
	if cfg.Scope != "" {
		q.Set("scope", cfg.Scope)
	}
	if cfg.Audience != "" {
		q.Set("audience", cfg.Audience)
	}
	if cfg.Resource != "" {
		q.Set("resource", cfg.Resource)
	}
	if state != "" {
		q.Set("state", state)
	}
	if challenge != "" {
		q.Set("code_challenge", challenge)
		methodVal := "plain"
		if strings.EqualFold(method, "s256") {
			methodVal = "S256"
		}
		q.Set("code_challenge_method", methodVal)
	}
	for k, v := range cfg.Extra {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func prepareRedirect(raw string) (*url.URL, net.Listener, error) {
	var (
		host  = "127.0.0.1"
		path  = defaultCallbackPath
		query string
	)

	raw = strings.TrimSpace(raw)
	if raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, nil, errdef.Wrap(errdef.CodeHTTP, err, "parse redirect_uri")
		}
		if u.Scheme != "" && u.Scheme != "http" {
			return nil, nil, errdef.New(errdef.CodeHTTP, "redirect_uri must use http")
		}
		if u.Path != "" {
			path = u.Path
		}
		if u.Host != "" {
			host = u.Host
		}
		if u.RawQuery != "" {
			query = u.RawQuery
		}
	}

	h, p := splitHostPort(host)
	if h == "" {
		h = "127.0.0.1"
	}
	if !isLoopback(h) {
		return nil, nil, errdef.New(errdef.CodeHTTP, "redirect_uri host must be loopback")
	}
	if p == "" {
		p = "0"
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(h, p))
	if err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeHTTP, err, "listen for oauth redirect")
	}

	addr := ln.Addr().(*net.TCPAddr)
	targetHost := net.JoinHostPort(h, strconv.Itoa(addr.Port))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u := &url.URL{
		Scheme:   "http",
		Host:     targetHost,
		Path:     path,
		RawQuery: query,
	}
	return u, ln, nil
}

func splitHostPort(input string) (string, string) {
	if input == "" {
		return "", ""
	}
	if strings.Contains(input, ":") {
		host, port, err := net.SplitHostPort(input)
		if err == nil {
			return host, port
		}
	}
	return input, ""
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

type codeServer struct {
	path   string
	state  string
	codeCh chan string
	errCh  chan error
	srv    *http.Server
	once   sync.Once
}

func newCodeServer(redirect *url.URL, state string) *codeServer {
	path := redirect.Path
	if path == "" {
		path = defaultCallbackPath
	}
	return &codeServer{
		path:   path,
		state:  state,
		codeCh: make(chan string, 1),
		errCh:  make(chan error, 1),
	}
}

func (s *codeServer) serve(ln net.Listener) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", s.handle)
	handler.HandleFunc(s.path, s.handle)

	s.srv = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errCh <- err
		}
	}()
}

func (s *codeServer) shutdown(ctx context.Context) {
	s.once.Do(func() {
		if s.srv != nil {
			_ = s.srv.Shutdown(ctx)
		}
	})
}

func (s *codeServer) wait(ctx context.Context) (string, error) {
	defer s.shutdown(context.Background())
	select {
	case code := <-s.codeCh:
		return code, nil
	case err := <-s.errCh:
		return "", errdef.Wrap(errdef.CodeHTTP, err, "oauth callback server")
	case <-ctx.Done():
		return "", errdef.Wrap(errdef.CodeHTTP, ctx.Err(), "waiting for oauth authorization")
	}
}

func (s *codeServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != s.path {
		http.NotFound(w, r)
		return
	}

	q := r.URL.Query()
	if errText := strings.TrimSpace(q.Get("error")); errText != "" {
		http.Error(w, "authorization failed", http.StatusBadRequest)
		s.errCh <- errdef.New(errdef.CodeHTTP, "authorization failed: %s", errText)
		return
	}

	code := strings.TrimSpace(q.Get("code"))
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		s.errCh <- errdef.New(errdef.CodeHTTP, "authorization response missing code")
		return
	}
	gotState := strings.TrimSpace(q.Get("state"))
	if strings.TrimSpace(s.state) != "" && strings.TrimSpace(s.state) != gotState {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		s.errCh <- errdef.New(errdef.CodeHTTP, "state mismatch")
		return
	}

	select {
	case s.codeCh <- code:
	default:
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(
		[]byte(
			"<html><body><p>Authentication complete. You can close this window.</p></body></html>",
		),
	)
	go s.shutdown(context.Background())
}

func openBrowser(link string) error {
	cmd := browserCommand(link)
	if cmd == nil {
		return errdef.New(errdef.CodeHTTP, "unsupported platform for browser launch")
	}
	return cmd.Start()
}

func browserCommand(link string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", link)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		return exec.Command("xdg-open", link)
	}
}

func randString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "generate random string")
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sha256Sum(input []byte) []byte {
	hash := sha256.New()
	hash.Write(input)
	return hash.Sum(nil)
}

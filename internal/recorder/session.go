package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Session struct {
	cfg    Config
	policy policy
	filter filter
	store  *store

	addr      string
	srv       *http.Server
	transport *http.Transport
	proxy     *httputil.ReverseProxy

	done      chan struct{}
	closeOnce sync.Once

	errMu sync.Mutex
	err   error
}

// Start opens the listener before returning. Cancellation allows active
// requests up to ShutdownTimeout to finish.
func Start(ctx context.Context, cfg Config) (*Session, error) {
	cfg, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := newFilter(cfg)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("recorder listen: %w", err)
	}
	upstream, _ := url.Parse(cfg.Upstream)
	if selfTarget(ln.Addr(), upstream) {
		_ = ln.Close()
		return nil, errors.New("recorder upstream points to its own listener")
	}

	s := &Session{
		cfg:       cfg,
		policy:    newPolicy(cfg),
		filter:    f,
		store:     newStore(cfg),
		addr:      ln.Addr().String(),
		transport: newTransport(),
		done:      make(chan struct{}),
	}
	s.proxy = s.newProxy(upstream)
	s.srv = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxMetadataBytes,
	}

	go s.serve(ln)
	go s.watch(ctx)
	return s, nil
}

// Ignore proxy settings to keep traffic on the chosen upstream.
// Disable automatic decompression so forwarding preserves the encoding.
func newTransport() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DisableCompression = true
	tr.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive}).DialContext
	tr.TLSHandshakeTimeout = tlsHandshakeTimeout
	return tr
}

func (s *Session) newProxy(upstream *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: s.transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.Out.URL.RawQuery = r.In.URL.RawQuery
			r.Out.URL.ForceQuery = r.In.URL.ForceQuery
			r.SetXForwarded()
		},
		ModifyResponse: s.captureResponse,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "recorder: upstream request failed", http.StatusBadGateway)
		},
		// Transport errors may expose credentials in URLs or headers.
		ErrorLog: log.New(io.Discard, "", 0),
	}
}

func (s *Session) serve(ln net.Listener) {
	err := s.srv.Serve(ln)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.fail(errors.New("recorder listener failed"))
	}
	s.closeAfter(ShutdownTimeout)
}

func (s *Session) watch(ctx context.Context) {
	select {
	case <-ctx.Done():
		s.closeAfter(ShutdownTimeout)
	case <-s.done:
	}
}

func (s *Session) closeAfter(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = s.Close(ctx)
}

func selfTarget(addr net.Addr, u *url.URL) bool {
	listen, ok := addr.(*net.TCPAddr)
	if !ok {
		return false
	}

	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	if port != strconv.Itoa(listen.Port) {
		return false
	}

	ip := net.ParseIP(u.Hostname())
	if strings.EqualFold(u.Hostname(), "localhost") {
		ip = net.IPv4(127, 0, 0, 1)
	}
	return ip != nil && (listen.IP.Equal(ip) || (listen.IP.IsUnspecified() && ip.IsLoopback()))
}

func (s *Session) Addr() string     { return s.addr }
func (s *Session) Upstream() string { return s.cfg.Upstream }
func (s *Session) Stats() Stats     { return s.store.stats() }

// Updates signals changes. One notification may cover several requests.
func (s *Session) Updates() <-chan struct{} { return s.store.updates }

// Done closes after captured requests finish, including during forced shutdown.
func (s *Session) Done() <-chan struct{} { return s.done }

// Snapshot returns independent copies of entries after the given ID.
func (s *Session) Snapshot(after uint64) []Entry { return s.store.snapshot(after) }

// Summaries omits headers and bodies to avoid copying them on each UI refresh.
func (s *Session) Summaries() []Entry { return s.store.summaries() }

func (s *Session) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *Session) fail(err error) {
	s.errMu.Lock()
	s.err = errors.Join(s.err, err)
	s.errMu.Unlock()
}

// Close is safe to call concurrently. All callers wait for the same result.
func (s *Session) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.store.stop()
		err := s.srv.Shutdown(ctx)
		if err != nil {
			_ = s.srv.Close()
		}
		s.store.drain()
		s.transport.CloseIdleConnections()
		s.fail(err)
		s.store.signal()
		close(s.done)
	})
	return s.Err()
}

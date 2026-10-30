package serve

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// timeout is under the five seconds net/http lets a new connection sit before
// http.Server.Shutdown treats it as idle.
const timeout = 2 * time.Second

var hello = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") })

type fixture struct {
	srv    *Server
	addr   string
	client *http.Client
	tls    *tls.Config // client side, nil for plain HTTP
}

func newFixture(t *testing.T, srv *http.Server, secure bool) *fixture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{addr: ln.Addr().String()}
	tr := &http.Transport{DisableKeepAlives: true}
	if secure {
		ts := httptest.NewTLSServer(nil)
		srv.TLSConfig = &tls.Config{Certificates: ts.TLS.Certificates}
		f.tls = ts.Client().Transport.(*http.Transport).TLSClientConfig
		tr.TLSClientConfig = f.tls
		ts.Close()
		f.srv = StartTLS(srv, ln)
	} else {
		f.srv = Start(srv, ln)
	}
	f.client = &http.Client{Transport: tr}
	// Close does not wait for a handler that a failed test left blocked.
	t.Cleanup(func() { _ = srv.Close() })
	return f
}

func (f *fixture) get() (string, error) {
	scheme := "http://"
	if f.tls != nil {
		scheme = "https://"
	}
	res, err := f.client.Get(scheme + f.addr + "/")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	return string(body), err
}

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func assertClosed(t *testing.T, c net.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("read on the connection without a request = %v, want EOF", err)
	}
}

func TestShutdownClosesConnectionsWithoutARequest(t *testing.T) {
	tests := []struct {
		name      string
		secure    bool
		handshake bool
	}{
		{name: "http"},
		{name: "tls before handshake", secure: true},
		{name: "tls after handshake", secure: true, handshake: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, &http.Server{Handler: hello}, tt.secure)
			var idle net.Conn
			var err error
			if tt.handshake {
				idle, err = tls.Dial("tcp", f.addr, f.tls)
			} else {
				idle, err = net.Dial("tcp", f.addr)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = idle.Close() }()
			// Accept is FIFO, so once a later connection is served, idle was accepted.
			if body, err := f.get(); err != nil || body != "hello" {
				t.Fatalf("get = %q, %v", body, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := f.srv.Shutdown(ctx); err != nil || ctx.Err() != nil {
				t.Fatalf("Shutdown = %v, ctx = %v, want both nil", err, ctx.Err())
			}
			if err := f.srv.Err(); err != nil {
				t.Fatalf("Err = %v after Shutdown, want nil", err)
			}
			assertClosed(t, idle)
		})
	}
}

func TestShutdownLetsARequestInFlightFinish(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	f := newFixture(t, &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "done")
	})}, false)
	idle := dial(t, f.addr)
	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		body, err := f.get()
		got <- result{body, err}
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.srv.Shutdown(ctx) }()
	// Hold the request until Shutdown has closed idle.
	assertClosed(t, idle)
	close(release)
	if r := <-got; r.err != nil || r.body != "done" {
		t.Fatalf("in-flight request = %q, %v, want it served", r.body, r.err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("Shutdown = %v", err)
	}
}

// Serve calls ConnContext in its accept loop before it reports the connection
// as new. Holding it until Shutdown has closed the listener means the
// connection is recorded only after Shutdown began.
func TestShutdownClosesAConnectionAcceptedAsItBegins(t *testing.T) {
	entered, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f := newFixture(t, &http.Server{
		Handler: hello,
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			once.Do(func() {
				close(entered)
				<-proceed
			})
			return ctx
		},
	}, false)
	late := dial(t, f.addr)
	select {
	case <-entered:
	case <-time.After(timeout):
		t.Fatal("Serve did not accept the connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.srv.Shutdown(ctx) }()
	// The listener is closed once a dial fails.
	for start := time.Now(); ; time.Sleep(time.Millisecond) {
		c, err := net.Dial("tcp", f.addr)
		if err != nil {
			break
		}
		_ = c.Close()
		if time.Since(start) > timeout {
			close(proceed) // the cleanup's Close waits for Serve to return
			t.Fatal("listener still accepts after Shutdown began")
		}
	}
	close(proceed)
	if err := <-stopped; err != nil || ctx.Err() != nil {
		t.Fatalf("Shutdown = %v, ctx = %v, want both nil", err, ctx.Err())
	}
	assertClosed(t, late)
}

func TestShutdownCutsOffARequestThatOutlivesCtx(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	f := newFixture(t, &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	})}, false)
	failed := make(chan error, 1)
	go func() {
		_, err := f.get()
		failed <- err
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := f.srv.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown = %v, want %v", err, context.DeadlineExceeded)
	}
	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("request that outlived ctx was served")
		}
	case <-time.After(timeout):
		t.Fatal("request that outlived ctx still runs")
	}
}

func TestStartCallsTheConnStateHook(t *testing.T) {
	states := make(chan http.ConnState, 8)
	f := newFixture(t, &http.Server{
		Handler:   hello,
		ConnState: func(_ net.Conn, state http.ConnState) { states <- state },
	}, false)
	if body, err := f.get(); err != nil || body != "hello" {
		t.Fatalf("get = %q, %v", body, err)
	}
	for _, want := range []http.ConnState{http.StateNew, http.StateActive, http.StateClosed} {
		select {
		case got := <-states:
			if got != want {
				t.Fatalf("ConnState = %v, want %v", got, want)
			}
		case <-time.After(timeout):
			t.Fatalf("ConnState hook did not see %v", want)
		}
	}
}

func TestErrReportsWhyServeReturned(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	s := Start(&http.Server{Handler: hello}, ln)
	<-s.Done()
	if err := s.Err(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Err = %v, want %v", err, net.ErrClosed)
	}
}

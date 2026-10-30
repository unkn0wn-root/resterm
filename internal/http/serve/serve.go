package serve

import (
	"context"
	"errors"
	"maps"
	"net"
	"net/http"
	"slices"
	"sync"
)

// Server runs an http.Server on a listener.
type Server struct {
	srv  *http.Server
	done chan struct{}
	err  error

	mu    sync.Mutex
	fresh map[net.Conn]struct{}
}

// Start calls srv.Serve(ln) in a new goroutine. It wraps srv.ConnState and
// still calls a hook already set there.
func Start(srv *http.Server, ln net.Listener) *Server {
	return start(srv, func() error { return srv.Serve(ln) })
}

// StartTLS is like Start but calls srv.ServeTLS with the certificates in
// srv.TLSConfig.
func StartTLS(srv *http.Server, ln net.Listener) *Server {
	return start(srv, func() error { return srv.ServeTLS(ln, "", "") })
}

func start(srv *http.Server, serve func() error) *Server {
	s := &Server{srv: srv, done: make(chan struct{}), fresh: make(map[net.Conn]struct{})}
	hook := srv.ConnState
	srv.ConnState = func(c net.Conn, state http.ConnState) {
		if hook != nil {
			hook(c, state)
		}
		s.mu.Lock()
		if state == http.StateNew {
			s.fresh[c] = struct{}{}
		} else {
			delete(s.fresh, c)
		}
		s.mu.Unlock()
	}
	go func() {
		if err := serve(); !errors.Is(err, http.ErrServerClosed) {
			s.err = err
		}
		close(s.done)
	}()
	return s
}

// Done is closed when Serve returns.
func (s *Server) Done() <-chan struct{} { return s.done }

// Err reports why Serve returned, or nil if Shutdown stopped it.
// Read it only after Done is closed.
func (s *Server) Err() error { return s.err }

// Shutdown is like http.Server.Shutdown but closes connections that have not
// sent a request instead of waiting for them. On error it closes the server.
func (s *Server) Shutdown(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() { errc <- s.srv.Shutdown(ctx) }()
	// Serve reports every new connection before it returns, so the set is complete.
	<-s.done
	s.mu.Lock()
	fresh := slices.Collect(maps.Keys(s.fresh))
	s.mu.Unlock()
	for _, c := range fresh {
		_ = c.Close()
	}
	if err := <-errc; err != nil {
		_ = s.srv.Close()
		return err
	}
	return nil
}

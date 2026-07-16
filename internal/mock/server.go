package mock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	opts    Options
	handler atomic.Pointer[Handler]
	srv     *http.Server
	addr    string
	done    chan struct{}
	err     error

	logs  ring
	calls atomic.Uint64
}

type requestEventKey struct{}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func Start(addr string, handler *Handler, opts Options) (*Server, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = DefaultAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	n := opts.Logs
	if n <= 0 {
		n = DefaultLogs
	}
	s := &Server{
		opts: opts,
		addr: ln.Addr().String(),
		done: make(chan struct{}),
		logs: ring{limit: n, events: make([]Event, 0, n)},
	}
	s.handler.Store(handler)
	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		err := s.srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.err = err
		close(s.done)
	}()
	return s, nil
}

func (s *Server) Reload(handler *Handler) {
	s.handler.Store(handler)
	s.RecordReload(nil)
}

func (s *Server) RecordReload(err error) {
	event := Event{Time: time.Now(), Reload: true}
	if err != nil {
		event.Error = err.Error()
	}
	s.record(event)
}

func (s *Server) Close(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		_ = s.srv.Close()
		return err
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	event := &Event{Time: start, Method: r.Method, Target: r.URL.RequestURI()}
	r = r.WithContext(context.WithValue(r.Context(), requestEventKey{}, event))
	sw := &statusWriter{ResponseWriter: w}
	defer func() {
		event.Status = sw.status
		event.Duration = time.Since(start)
		s.record(*event)
	}()

	handler := s.handler.Load()
	if s.handleCORS(sw, r, handler) {
		return
	}
	handler.ServeHTTP(sw, r)
}

func requestEvent(r *http.Request) *Event {
	event, _ := r.Context().Value(requestEventKey{}).(*Event)
	return event
}

func (s *Server) record(event Event) {
	if !event.Reload {
		s.calls.Add(1)
	}
	s.logs.add(event)
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(event)
	}
}

func (s *Server) Logs() []Event {
	return s.logs.list()
}

func (s *Server) ClearLogs() {
	s.logs.clear()
}

func (s *Server) Done() <-chan struct{} { return s.done }

func (s *Server) Err() error {
	return s.err
}

func (s *Server) Addr() string { return s.addr }

func (s *Server) Stats() Stats {
	handler := s.handler.Load()
	return Stats{
		Addr:      s.Addr(),
		Routes:    handler.Routes(),
		Scenarios: handler.Scenarios(),
		Calls:     s.calls.Load(),
	}
}

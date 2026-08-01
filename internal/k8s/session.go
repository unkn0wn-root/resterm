package k8s

import (
	"context"
	"errors"
	"sync"
	"time"
)

const closeWaitWindow = 3 * time.Second

type session struct {
	localAddr string
	stopCh    chan struct{}
	doneCh    chan struct{}

	mu        sync.RWMutex
	err       error
	diag      *diagCollector
	ended     bool
	closeOnce sync.Once
	finished  sync.Once
}

func newSession(stopCh chan struct{}) *session {
	return &session{
		stopCh: stopCh,
		doneCh: make(chan struct{}),
	}
}

func (s *session) Alive() bool {
	select {
	case <-s.doneCh:
		return false
	default:
		return true
	}
}

func (s *session) localAddress() (string, error) {
	if s.localAddr == "" {
		return "", errors.New("k8s: local forward address unavailable")
	}
	return s.localAddr, nil
}

func (s *session) finish(err error) {
	s.mu.Lock()
	s.err = err
	s.ended = true
	diag := s.diag
	s.mu.Unlock()

	s.finished.Do(func() {
		close(s.doneCh)
	})
	if diag != nil {
		diag.close()
	}
}

func (s *session) Close() error {
	s.closeOnce.Do(func() {
		close(s.stopCh)
	})

	select {
	case <-s.doneCh:
		return nil
	case <-time.After(closeWaitWindow):
		return errors.New("k8s: timeout closing port-forward")
	}
}

func (s *session) errValue() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *session) setDiag(collector *diagCollector) {
	if collector == nil {
		return
	}

	closeCollector := false
	s.mu.Lock()
	if s.ended {
		closeCollector = true
	} else {
		s.diag = collector
	}
	s.mu.Unlock()

	if closeCollector {
		collector.close()
	}
}

func (s *session) bindRequestDiag(ctx context.Context) {
	s.mu.RLock()
	diag := s.diag
	s.mu.RUnlock()

	bindRequestDiag(ctx, diag)
}

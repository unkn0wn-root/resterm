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

	mu       sync.RWMutex
	err      error
	diag     *diagCollector
	ended    bool
	closed   sync.Once
	finished sync.Once
}

func newSession(stopCh chan struct{}) *session {
	return &session{
		stopCh: stopCh,
		doneCh: make(chan struct{}),
	}
}

func (s *session) alive() bool {
	if s == nil || s.doneCh == nil {
		return false
	}
	select {
	case <-s.doneCh:
		return false
	default:
		return true
	}
}

func (s *session) localAddress() (string, error) {
	if s == nil || s.localAddr == "" {
		return "", errors.New("k8s: local forward address unavailable")
	}
	return s.localAddr, nil
}

func (s *session) finish(err error) {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.err = err
	s.ended = true
	diag := s.diag
	s.mu.Unlock()

	s.finished.Do(func() {
		if s.doneCh != nil {
			close(s.doneCh)
		}
	})
	if diag != nil {
		diag.close()
	}
}

func (s *session) close() error {
	if s == nil {
		return nil
	}

	s.closed.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})

	var errs []error
	if s.doneCh != nil {
		select {
		case <-s.doneCh:
		case <-time.After(closeWaitWindow):
			errs = append(errs, errors.New("k8s: timeout closing port-forward"))
		}
	}
	return errors.Join(errs...)
}

func (s *session) errValue() error {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *session) setDiag(collector *diagCollector) {
	if s == nil || collector == nil {
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
	if s == nil {
		return
	}

	s.mu.RLock()
	diag := s.diag
	s.mu.RUnlock()

	bindRequestDiag(ctx, diag)
}

package ssh

import (
	"errors"
	"net"
	"sync"
	"time"
)

var errSessionClosed = errors.New("ssh: session closed")

type session struct {
	cli Client

	stopCh chan struct{}
	doneCh chan struct{}

	mu     sync.RWMutex
	err    error
	closed sync.Once
}

func newSession(cli Client, keepAlive time.Duration) *session {
	s := &session{
		cli:    cli,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	if keepAlive > 0 {
		go s.keepAlive(keepAlive)
	}
	return s
}

func (s *session) dial(network, addr string) (net.Conn, error) {
	if s == nil || !s.alive() {
		return nil, errSessionClosed
	}
	if s.cli == nil {
		return nil, errSessionClosed
	}
	return s.cli.Dial(network, addr)
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

func (s *session) close() error {
	s.shutdown(nil)
	return s.errValue()
}

func (s *session) fail(err error) {
	s.shutdown(err)
}

func (s *session) shutdown(reason error) {
	if s == nil {
		return
	}

	s.closed.Do(func() {
		close(s.stopCh)
		var closeErr error
		if s.cli != nil {
			closeErr = s.cli.Close()
		}
		s.finish(joinCloseErr(reason, closeErr))
	})
}

func (s *session) finish(err error) {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()

	close(s.doneCh)
}

func (s *session) errValue() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *session) keepAlive(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			_, _, err := s.cli.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				s.fail(err)
				return
			}
		}
	}
}

package request

import "context"

// An interactive stream starts out owned by the request that opened it and ends
// up owned by the console instead. Cancelling the request and handing the
// session over race with each other, so exactly one of them has to win.
type sessionLifetime struct {
	ctx    context.Context
	cancel context.CancelFunc
	stop   func() bool
}

func newSessionLifetime(parent context.Context) *sessionLifetime {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	return &sessionLifetime{
		ctx:    ctx,
		cancel: cancel,
		stop:   context.AfterFunc(parent, cancel),
	}
}

// detach hands the session to its new owner and reports whether that won. A
// false return means the request was canceled first, and ctx is done before
// detach returns so the caller can drop the session without further checks.
func (l *sessionLifetime) detach() bool {
	if l.stop() {
		return true
	}
	// Losing to stop only means the forwarding callback has started, not that it
	// has finished, so cancel here too instead of waiting on it.
	l.cancel()
	return false
}

// close ends the session on the paths that never hand it over.
func (l *sessionLifetime) close() {
	l.stop()
	l.cancel()
}

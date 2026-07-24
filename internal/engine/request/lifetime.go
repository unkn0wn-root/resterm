package request

import "context"

// The session follows parent cancellation until detach is called. Detaching lets
// an interactive stream outlive the command that opened it. The caller still
// owns cancel and uses it when the session ends.
func sessionContext(parent context.Context) (context.Context, context.CancelFunc, func()) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	stop := context.AfterFunc(parent, cancel)
	return ctx, cancel, func() { stop() }
}

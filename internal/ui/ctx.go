package ui

import "context"

func ctxDone(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

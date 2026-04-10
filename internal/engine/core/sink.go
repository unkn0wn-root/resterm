package core

import "context"

type Sink interface {
	OnEvt(context.Context, Evt) error
}

type SinkFunc func(context.Context, Evt) error

func (fn SinkFunc) OnEvt(ctx context.Context, e Evt) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, e)
}

type NopSink struct{}

func (NopSink) OnEvt(context.Context, Evt) error { return nil }

var Discard Sink = NopSink{}

func Emit(ctx context.Context, s Sink, e Evt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || e == nil {
		return nil
	}
	return s.OnEvt(ctx, e)
}

func EmitAll(ctx context.Context, s Sink, es ...Evt) error {
	for _, e := range es {
		if err := Emit(ctx, s, e); err != nil {
			return err
		}
	}
	return nil
}

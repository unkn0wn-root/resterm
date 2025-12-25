package rts

import "time"

type Stream struct {
	Kind    string
	Summary map[string]any
	Events  []map[string]any
}

type streamObj struct {
	s *Stream
}

func newStreamObj(s *Stream) *streamObj {
	return &streamObj{s: s}
}

func (o *streamObj) TypeName() string { return "stream" }

func (o *streamObj) GetMember(name string) (Value, bool) {
	switch name {
	case "enabled":
		return NativeNamed("stream.enabled", o.enabledFn), true
	case "kind":
		return NativeNamed("stream.kind", o.kindFn), true
	case "summary":
		return NativeNamed("stream.summary", o.summaryFn), true
	case "events":
		return NativeNamed("stream.events", o.eventsFn), true
	}
	return Null(), false
}

func (o *streamObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), rtErr(nil, Pos{}, "no member call: %s", name)
}

func (o *streamObj) Index(key Value) (Value, error) {
	return Null(), nil
}

func (o *streamObj) enabledFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "stream.enabled() expects 0 args")
	}
	return Bool(o.s != nil), nil
}

func (o *streamObj) kindFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "stream.kind() expects 0 args")
	}
	if o.s == nil {
		return Str(""), nil
	}
	return Str(o.s.Kind), nil
}

func (o *streamObj) summaryFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "stream.summary() expects 0 args")
	}
	if o.s == nil || len(o.s.Summary) == 0 {
		return Dict(nil), nil
	}
	return toVal(ctx, pos, o.s.Summary)
}

func (o *streamObj) eventsFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "stream.events() expects 0 args")
	}
	if o.s == nil || len(o.s.Events) == 0 {
		return List(nil), nil
	}
	ev := make([]any, 0, len(o.s.Events))
	for _, it := range o.s.Events {
		ev = append(ev, it)
	}
	return toVal(ctx, pos, ev)
}

func toVal(ctx *Ctx, pos Pos, v any) (Value, error) {
	switch t := v.(type) {
	case nil:
		return Null(), nil
	case bool:
		return Bool(t), nil
	case string:
		if ctx != nil && ctx.Lim.MaxStr > 0 && len(t) > ctx.Lim.MaxStr {
			return Null(), rtErr(ctx, pos, "string too long")
		}
		return Str(t), nil
	case float64:
		return Num(t), nil
	case float32:
		return Num(float64(t)), nil
	case int:
		return Num(float64(t)), nil
	case int64:
		return Num(float64(t)), nil
	case int32:
		return Num(float64(t)), nil
	case int16:
		return Num(float64(t)), nil
	case int8:
		return Num(float64(t)), nil
	case uint:
		return Num(float64(t)), nil
	case uint64:
		return Num(float64(t)), nil
	case uint32:
		return Num(float64(t)), nil
	case uint16:
		return Num(float64(t)), nil
	case uint8:
		return Num(float64(t)), nil
	case time.Duration:
		return Num(float64(t) / float64(time.Millisecond)), nil
	case []any:
		return toList(ctx, pos, t)
	case []map[string]any:
		return toList(ctx, pos, toAnySlice(t))
	case map[string]any:
		return toMap(ctx, pos, t)
	default:
		return Null(), rtErr(ctx, pos, "unsupported value")
	}
}

func toList(ctx *Ctx, pos Pos, src []any) (Value, error) {
	if ctx != nil && ctx.Lim.MaxList > 0 && len(src) > ctx.Lim.MaxList {
		return Null(), rtErr(ctx, pos, "list too large")
	}
	out := make([]Value, 0, len(src))
	for _, it := range src {
		v, err := toVal(ctx, pos, it)
		if err != nil {
			return Null(), err
		}
		out = append(out, v)
	}
	return List(out), nil
}

func toMap(ctx *Ctx, pos Pos, src map[string]any) (Value, error) {
	if ctx != nil && ctx.Lim.MaxDict > 0 && len(src) > ctx.Lim.MaxDict {
		return Null(), rtErr(ctx, pos, "dict too large")
	}
	out := make(map[string]Value, len(src))
	for k, it := range src {
		v, err := toVal(ctx, pos, it)
		if err != nil {
			return Null(), err
		}
		out[k] = v
	}
	return Dict(out), nil
}

func toAnySlice[T any](src []T) []any {
	if len(src) == 0 {
		return nil
	}
	out := make([]any, len(src))
	for i, it := range src {
		out[i] = it
	}
	return out
}

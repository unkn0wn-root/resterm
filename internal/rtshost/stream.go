package rtshost

import (
	"maps"
	"slices"
	"time"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
)

type streamObj struct {
	stream *Stream
}

func (*streamObj) TypeName() string { return "stream" }

func (o *streamObj) Member(_ *rts.Ctx, _ rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "enabled":
		return native.Fn0("stream.enabled", "stream.enabled()", o.enabled).Value(), true, nil
	case "kind":
		return native.Fn0("stream.kind", "stream.kind()", o.kind).Value(), true, nil
	case "summary":
		return native.Fn0("stream.summary", "stream.summary()", o.summary).Value(), true, nil
	case "events":
		return native.Fn0("stream.events", "stream.events()", o.events).Value(), true, nil
	default:
		return rts.Null(), false, nil
	}
}

func (*streamObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *streamObj) enabled(native.Call) (rts.Value, error) {
	return rts.Bool(o.stream != nil), nil
}

func (o *streamObj) kind(call native.Call) (rts.Value, error) {
	if o.stream == nil {
		return rts.Str(""), nil
	}
	return native.StringValue(call.Ctx, call.Pos, o.stream.Kind)
}

func (o *streamObj) summary(call native.Call) (rts.Value, error) {
	if o.stream == nil || len(o.stream.Summary) == 0 {
		return rts.Dict(map[string]rts.Value{}), nil
	}
	return hostMap(call, o.stream.Summary)
}

func (o *streamObj) events(call native.Call) (rts.Value, error) {
	if o.stream == nil || len(o.stream.Events) == 0 {
		return rts.List(nil), nil
	}
	if err := rts.CheckList(call.Ctx, call.Pos, len(o.stream.Events)); err != nil {
		return rts.Null(), err
	}
	out := make([]rts.Value, len(o.stream.Events))
	for i, event := range o.stream.Events {
		v, err := hostMap(call, event)
		if err != nil {
			return rts.Null(), err
		}
		out[i] = v
	}
	return rts.List(out), nil
}

func hostValue(call native.Call, value any) (rts.Value, error) {
	switch value := value.(type) {
	case time.Duration:
		return rts.Num(float64(value) / float64(time.Millisecond)), nil
	case int:
		return hostInt(call, int64(value))
	case int64:
		return hostInt(call, value)
	case int32:
		return rts.Num(float64(value)), nil
	case int16:
		return rts.Num(float64(value)), nil
	case int8:
		return rts.Num(float64(value)), nil
	case uint:
		return hostUint(call, uint64(value))
	case uint64:
		return hostUint(call, value)
	case uint32:
		return rts.Num(float64(value)), nil
	case uint16:
		return rts.Num(float64(value)), nil
	case uint8:
		return rts.Num(float64(value)), nil
	case float32:
		return rts.Num(float64(value)), nil
	case []map[string]any:
		if err := rts.CheckList(call.Ctx, call.Pos, len(value)); err != nil {
			return rts.Null(), err
		}
		out := make([]rts.Value, len(value))
		for i, item := range value {
			v, err := hostMap(call, item)
			if err != nil {
				return rts.Null(), err
			}
			out[i] = v
		}
		return rts.List(out), nil
	default:
		return rts.FromIface(call.Ctx, call.Pos, value)
	}
}

const maxExactInteger = 1 << 53

func hostInt(call native.Call, value int64) (rts.Value, error) {
	if value < -maxExactInteger || value > maxExactInteger {
		return rts.Null(), call.Errorf("integer cannot be represented exactly")
	}
	return rts.Num(float64(value)), nil
}

func hostUint(call native.Call, value uint64) (rts.Value, error) {
	if value > maxExactInteger {
		return rts.Null(), call.Errorf("integer cannot be represented exactly")
	}
	return rts.Num(float64(value)), nil
}

func hostMap(call native.Call, values map[string]any) (rts.Value, error) {
	if err := rts.CheckDict(call.Ctx, call.Pos, len(values)); err != nil {
		return rts.Null(), err
	}
	out := make(map[string]rts.Value, len(values))
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if err := rts.CheckStr(call.Ctx, call.Pos, name); err != nil {
			return rts.Null(), err
		}
		v, err := hostValue(call, values[name])
		if err != nil {
			return rts.Null(), err
		}
		out[name] = v
	}
	return rts.Dict(out), nil
}

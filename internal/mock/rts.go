package mock

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

// RTSValue exposes request-journal inspection as the RTS mock object.
func RTSValue(inspector Inspector) rts.Value {
	return rts.Obj(&rtsInspector{inspector: inspector})
}

type rtsInspector struct {
	inspector Inspector
}

func (o *rtsInspector) TypeName() string { return "mock" }

func (o *rtsInspector) GetMember(name string) (rts.Value, bool) {
	switch name {
	case "count":
		return rts.NativeNamed("mock.count", o.count), true
	case "received":
		return rts.NativeNamed("mock.received", o.received), true
	default:
		return rts.Null(), false
	}
}

func (o *rtsInspector) Index(rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *rtsInspector) count(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	count, err := o.requestCount(ctx, pos, args, "mock.count(pattern)")
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(float64(count)), nil
}

func (o *rtsInspector) received(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	count, err := o.requestCount(ctx, pos, args, "mock.received(pattern)")
	if err != nil {
		return rts.Null(), err
	}
	return rts.Bool(count > 0), nil
}

func (o *rtsInspector) requestCount(
	ctx *rts.Ctx,
	pos rts.Pos,
	args []rts.Value,
	sig string,
) (uint64, error) {
	parsed := rts.NewArgs(ctx, pos, args, sig)
	if err := parsed.Count(1); err != nil {
		return 0, err
	}
	pattern, err := requestPatternFromValue(parsed.Arg(0))
	if err != nil {
		return 0, rts.Errf(ctx, pos, "%s: %v", sig, err)
	}
	count, err := o.inspector.Count(ctx.GoCtx(), pattern)
	if err != nil {
		return 0, rts.Errf(ctx, pos, "%s: %v", sig, err)
	}
	return count, nil
}

// requestPatternFromValue decodes a script dict through the pattern's JSON
// schema, so scripts, the control API, and .http files accept the same shape.
// Count validates the decoded pattern when it compiles it.
func requestPatternFromValue(value rts.Value) (RequestPattern, error) {
	if value.K != rts.VDict {
		return RequestPattern{}, errors.New("pattern must be a dict")
	}
	data, err := rts.ToIfaceStrict(value)
	if err != nil {
		return RequestPattern{}, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return RequestPattern{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var pattern RequestPattern
	if err := decoder.Decode(&pattern); err != nil {
		return RequestPattern{}, errors.New(patternDecodeReason(err))
	}
	return pattern, nil
}

// patternDecodeReason strips the Go type noise json decode errors carry, which
// script authors cannot act on.
func patternDecodeReason(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = "pattern"
		}
		return "pattern field " + field + " has an invalid value"
	}
	return err.Error()
}

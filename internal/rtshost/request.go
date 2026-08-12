package rtshost

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/queryparams"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
)

type stateKey struct{}

type evalState struct {
	rt Runtime
}

func withRuntime(ctx context.Context, rt Runtime) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stateKey{}, &evalState{rt: rt})
}

func runtimeState(ctx *rts.Ctx) *evalState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.GoCtx().Value(stateKey{}).(*evalState)
	return state
}

// requestObj is stateless. Its functions read the active Runtime from the
// evaluation context, so requestFns can be shared between evaluations.
type requestObj struct{}

var requestFns = map[string]rts.Value{
	"header": native.Fn1("request.header", "request.header(name)", headerName,
		func(call native.Call, name httpheader.Name) (rts.Value, error) {
			req := requestFrom(call.Ctx)
			if req == nil {
				return rts.Str(""), nil
			}
			vals := req.Headers[name.Key()]
			if len(vals) == 0 {
				return rts.Str(""), nil
			}
			return native.StringValue(call.Ctx, call.Pos, vals[0])
		},
	).Value(),
	"setMethod": mut1("request.setMethod", "request.setMethod(method)",
		native.String, RequestMutator.SetMethod).Value(),
	"setURL": mut1("request.setURL", "request.setURL(url)",
		native.String, RequestMutator.SetURL).Value(),
	"setHeader": mut2("request.setHeader", "request.setHeader(name, value)",
		headerName, native.String, RequestMutator.SetHeader).Value(),
	"addHeader": mut2("request.addHeader", "request.addHeader(name, value)",
		headerName, native.String, RequestMutator.AddHeader).Value(),
	"removeHeader": mut1("request.removeHeader", "request.removeHeader(name)",
		headerName, RequestMutator.DelHeader).Value(),
	"setQueryParam": mut2("request.setQueryParam", "request.setQueryParam(name, value)",
		native.String, native.String, RequestMutator.SetQuery).Value(),
	"setBody": mut1("request.setBody", "request.setBody(body)",
		native.String, RequestMutator.SetBody).Value(),
}

func mut1[A any](
	name, sig string,
	a native.Decoder[A],
	set func(RequestMutator, A),
) native.Def {
	return native.Fn1(name, sig, a, func(call native.Call, av A) (rts.Value, error) {
		mut, err := requestMutator(call)
		if err != nil {
			return rts.Null(), err
		}
		set(mut, av)
		return rts.Null(), nil
	})
}

func mut2[A, B any](
	name, sig string,
	a native.Decoder[A], b native.Decoder[B],
	set func(RequestMutator, A, B),
) native.Def {
	return native.Fn2(name, sig, a, b, func(call native.Call, av A, bv B) (rts.Value, error) {
		mut, err := requestMutator(call)
		if err != nil {
			return rts.Null(), err
		}
		set(mut, av, bv)
		return rts.Null(), nil
	})
}

func (*requestObj) TypeName() string { return "request" }

func (*requestObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	req := requestFrom(ctx)
	switch name {
	case "method":
		if req == nil {
			return rts.Str(""), true, nil
		}
		v, err := native.StringValue(ctx, pos, req.Method)
		return v, true, err
	case "url":
		if req == nil {
			return rts.Str(""), true, nil
		}
		v, err := native.StringValue(ctx, pos, req.URL)
		return v, true, err
	case "headers":
		v, err := native.StringValuesDict(ctx, pos, headerValues(req))
		return v, true, err
	case "query":
		vals, err := queryValues(req)
		if err != nil {
			return rts.Null(), true, rts.Errf(ctx, pos, "request query: %v", err)
		}
		v, err := native.StringValuesDict(ctx, pos, vals)
		return v, true, err
	}
	v, ok := requestFns[name]
	if !ok {
		return rts.Null(), false, nil
	}
	return v, true, nil
}

func (*requestObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func requestFrom(ctx *rts.Ctx) *Request {
	state := runtimeState(ctx)
	if state == nil {
		return nil
	}
	return state.rt.Request
}

func requestMutator(call native.Call) (RequestMutator, error) {
	state := runtimeState(call.Ctx)
	if state == nil || state.rt.Mutator == nil {
		return nil, call.Errorf("request is read-only")
	}
	return state.rt.Mutator, nil
}

func headerValues(req *Request) map[string][]string {
	if req == nil {
		return nil
	}
	return req.Headers
}

func queryValues(req *Request) (queryparams.Values, error) {
	if req == nil {
		return queryparams.Values{}, nil
	}
	if req.Query != nil {
		return req.Query, nil
	}
	return targetQuery(req.URL)
}

func targetQuery(raw string) (queryparams.Values, error) {
	q, err := urltpl.ParseTargetQuery(raw)
	if err != nil {
		return nil, err
	}
	return queryparams.Clone(q), nil
}

func headerName(call native.Call, v rts.Value) (httpheader.Name, error) {
	name, err := native.String(call, v)
	if err != nil {
		return httpheader.Name{}, err
	}
	n, err := httpheader.Parse(name)
	if err != nil {
		return httpheader.Name{}, call.Errorf("%s expects an HTTP header name, got %q", call.Sig, name)
	}
	return n, nil
}

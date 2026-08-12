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

type requestObj struct{}

func (*requestObj) TypeName() string { return "request" }

func (o *requestObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
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
	case "header":
		return o.headerDef().Value(), true, nil
	case "query":
		vals, err := queryValues(req)
		if err != nil {
			return rts.Null(), true, rts.Errf(ctx, pos, "request query: %v", err)
		}
		v, err := native.StringValuesDict(ctx, pos, vals)
		return v, true, err
	case "setMethod":
		return o.setMethodDef().Value(), true, nil
	case "setURL":
		return o.setURLDef().Value(), true, nil
	case "setHeader":
		return o.setHeaderDef().Value(), true, nil
	case "addHeader":
		return o.addHeaderDef().Value(), true, nil
	case "removeHeader":
		return o.removeHeaderDef().Value(), true, nil
	case "setQueryParam":
		return o.setQueryDef().Value(), true, nil
	case "setBody":
		return o.setBodyDef().Value(), true, nil
	default:
		return rts.Null(), false, nil
	}
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
	if state == nil || state.rt.RequestMut == nil {
		return nil, call.Errorf("request is read-only")
	}
	return state.rt.RequestMut, nil
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

func (o *requestObj) headerDef() native.Def {
	const sig = "request.header(name)"
	return native.Fn1("request.header", sig, headerName,
		func(call native.Call, name httpheader.Name) (rts.Value, error) {
			req := requestFrom(call.Ctx)
			if req == nil || len(req.Headers[name.Key()]) == 0 {
				return rts.Str(""), nil
			}
			return native.StringValue(call.Ctx, call.Pos, req.Headers[name.Key()][0])
		},
	)
}

func (o *requestObj) setMethodDef() native.Def {
	const sig = "request.setMethod(method)"
	return native.Fn1("request.setMethod", sig, native.String,
		func(call native.Call, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetMethod(value)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) setURLDef() native.Def {
	const sig = "request.setURL(url)"
	return native.Fn1("request.setURL", sig, native.String,
		func(call native.Call, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetURL(value)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) setHeaderDef() native.Def {
	const sig = "request.setHeader(name, value)"
	return native.Fn2("request.setHeader", sig, headerName, native.String,
		func(call native.Call, name httpheader.Name, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetHeader(name, value)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) addHeaderDef() native.Def {
	const sig = "request.addHeader(name, value)"
	return native.Fn2("request.addHeader", sig, headerName, native.String,
		func(call native.Call, name httpheader.Name, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.AddHeader(name, value)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) removeHeaderDef() native.Def {
	const sig = "request.removeHeader(name)"
	return native.Fn1("request.removeHeader", sig, headerName,
		func(call native.Call, name httpheader.Name) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.DelHeader(name)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) setQueryDef() native.Def {
	const sig = "request.setQueryParam(name, value)"
	return native.Fn2("request.setQueryParam", sig, native.String, native.String,
		func(call native.Call, name, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetQuery(name, value)
			return rts.Null(), nil
		},
	)
}

func (o *requestObj) setBodyDef() native.Def {
	const sig = "request.setBody(body)"
	return native.Fn1("request.setBody", sig, native.String,
		func(call native.Call, value string) (rts.Value, error) {
			mut, err := requestMutator(call)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetBody(value)
			return rts.Null(), nil
		},
	)
}

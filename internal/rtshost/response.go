package rtshost

import (
	"encoding/json"

	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/jsonpath"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
)

const unboundResponse = "response is not available until the request has run; use last for the previous response"

type responseObj struct {
	name    string
	resp    *Response
	unbound string
	json    any
	jsonErr error
	parsed  bool
}

func newResponseObj(name string, resp *Response) *responseObj {
	return &responseObj{name: name, resp: resp}
}

func newUnboundResponseObj(name string) *responseObj {
	return &responseObj{name: name, unbound: unboundResponse}
}

func (o *responseObj) TypeName() string { return o.name }

func (o *responseObj) Unbound() string { return o.unbound }

func (o *responseObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "status", "statusCode":
		if o.resp == nil {
			return rts.Num(0), true, nil
		}
		return rts.Num(float64(o.resp.Code)), true, nil
	case "statusText":
		if o.resp == nil {
			return rts.Str(""), true, nil
		}
		v, err := native.StringValue(ctx, pos, o.resp.Status)
		return v, true, err
	case "url":
		if o.resp == nil {
			return rts.Str(""), true, nil
		}
		v, err := native.StringValue(ctx, pos, o.resp.URL)
		return v, true, err
	case "headers":
		if o.resp == nil {
			return rts.Dict(map[string]rts.Value{}), true, nil
		}
		v, err := native.StringValuesDict(ctx, pos, o.resp.Headers)
		return v, true, err
	case "header":
		return o.headerDef().Value(), true, nil
	case "text":
		return o.textDef().Value(), true, nil
	case "json":
		return o.jsonDef().Value(), true, nil
	default:
		return rts.Null(), false, nil
	}
}

func (*responseObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *responseObj) headerDef() native.Def {
	sig := o.name + ".header(name)"
	return native.Fn1(o.name+".header", sig, headerName,
		func(call native.Call, name httpheader.Name) (rts.Value, error) {
			if o.resp == nil {
				return rts.Str(""), nil
			}
			vals := o.resp.Headers[name.Key()]
			if len(vals) == 0 {
				return rts.Str(""), nil
			}
			return native.StringValue(call.Ctx, call.Pos, vals[0])
		},
	)
}

func (o *responseObj) textDef() native.Def {
	sig := o.name + ".text()"
	return native.Fn0(o.name+".text", sig,
		func(call native.Call) (rts.Value, error) {
			if o.resp == nil {
				return rts.Str(""), nil
			}
			return native.StringValue(call.Ctx, call.Pos, string(o.resp.Body))
		},
	)
}

func (o *responseObj) jsonDef() native.Def {
	sig := o.name + ".json([path])"
	return native.FnOptional(o.name+".json", sig, native.String,
		func(call native.Call, path native.Optional[string]) (rts.Value, error) {
			if o.resp == nil {
				return rts.Null(), nil
			}
			if !o.parsed {
				o.jsonErr = json.Unmarshal(o.resp.Body, &o.json)
				o.parsed = true
			}
			if o.jsonErr != nil {
				return rts.Null(), call.Errorf("invalid json")
			}
			value := o.json
			if path.Set {
				var ok bool
				value, ok = jsonpath.Get(value, path.Value)
				if !ok {
					return rts.Null(), nil
				}
			}
			return rts.FromIface(call.Ctx, call.Pos, value)
		},
	)
}

func assertionOverrides(resp *Response) rts.Locals {
	o := newResponseObj("response", resp)
	code := 0
	status := ""
	if resp != nil {
		code = resp.Code
		status = resp.Status
	}
	return rts.NewLocals(map[string]rts.Value{
		"status":     rts.Num(float64(code)),
		"statusCode": rts.Num(float64(code)),
		"statusText": rts.Str(status),
		"header":     o.headerDef().Value(),
		"text":       o.textDef().Value(),
	})
}

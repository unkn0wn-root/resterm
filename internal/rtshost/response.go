package rtshost

import (
	"encoding/json"

	"github.com/unkn0wn-root/resterm/internal/http/header"
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
	members map[string]rts.Value
}

func newResponseObj(name string, resp *Response) *responseObj {
	o := &responseObj{name: name, resp: resp}
	o.members = map[string]rts.Value{
		"header": o.headerDef().Value(),
		"text":   o.textDef().Value(),
		"json":   o.jsonDef().Value(),
	}
	return o
}

func newUnboundResponseObj(name string) *responseObj {
	o := newResponseObj(name, nil)
	o.unbound = unboundResponse
	return o
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
	}
	v, ok := o.members[name]
	if !ok {
		return rts.Null(), false, nil
	}
	return v, true, nil
}

func (*responseObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *responseObj) headerDef() native.Def {
	sig := o.name + ".header(name)"
	return native.Fn1(o.name+".header", sig, headerName,
		func(call native.Call, name header.Name) (rts.Value, error) {
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

// assertionOverrides binds the shorthand an assertion reads. Every name is
// bound before the assertion runs, so statusText carries the host status as
// given: a name is one stored value with no hook on reading it, and checking
// the string limit here would fail assertions that never mention statusText.
// response.statusText applies the limit at the point it is read.
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
		"header":     o.members["header"],
		"text":       o.members["text"],
	})
}

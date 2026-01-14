package rts

import (
	"encoding/json"
	"fmt"
	"strings"
)

type mapObj struct {
	name string
	m    map[string]string
	s    ms
}

func newMapObj(name string, src map[string]string) *mapObj {
	return &mapObj{name: name, m: lowerMap(src), s: newMS(name)}
}

func (o *mapObj) TypeName() string { return o.name }

func (o *mapObj) GetMember(name string) (Value, bool) {
	switch name {
	case "get":
		return NativeNamed(o.name+".get", o.getFn), true
	case "has":
		return NativeNamed(o.name+".has", o.hasFn), true
	case "require":
		return NativeNamed(o.name+".require", o.requireFn), true
	}

	return mapMember(o.m, name)
}

func (o *mapObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no member call: %s", name)
}

func (o *mapObj) Index(key Value) (Value, error) {
	return mapIndex(o.m, key)
}

func (o *mapObj) getFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapGet(ctx, pos, args, o.s.g, o.m)
}

func (o *mapObj) hasFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapHas(ctx, pos, args, o.s.h, o.m)
}

func (o *mapObj) requireFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapRequire(ctx, pos, args, o.s.r, o.name, o.m)
}

type Resp struct {
	Status string
	Code   int
	H      map[string][]string
	Body   []byte
	URL    string
}

type respObj struct {
	name  string
	r     *Resp
	h     map[string]string
	jv    any
	jerr  error
	jdone bool
}

func newRespObj(name string, r *Resp) *respObj {
	if strings.TrimSpace(name) == "" {
		name = "last"
	}

	o := &respObj{name: name, r: r}
	if r == nil {
		return o
	}

	o.h = make(map[string]string)
	for k, v := range r.H {
		if len(v) == 0 {
			continue
		}
		o.h[strings.ToLower(k)] = v[0]
	}
	return o
}

func (o *respObj) TypeName() string { return o.name }

func (o *respObj) GetMember(name string) (Value, bool) {
	switch name {
	case "status":
		if o.r == nil {
			return Num(0), true
		}
		return Num(float64(o.r.Code)), true
	case "statusCode":
		if o.r == nil {
			return Num(0), true
		}
		return Num(float64(o.r.Code)), true
	case "statusText":
		if o.r == nil {
			return Str(""), true
		}
		return Str(o.r.Status), true
	case "url":
		if o.r == nil {
			return Str(""), true
		}
		return Str(o.r.URL), true
	case "headers":
		m := make(map[string]Value, len(o.h))
		for k, v := range o.h {
			m[k] = Str(v)
		}
		return Dict(m), true
	case "header":
		return NativeNamed(o.name+".header", o.headerFn), true
	case "text":
		return NativeNamed(o.name+".text", o.textFn), true
	case "json":
		return NativeNamed(o.name+".json", o.jsonFn), true
	}
	return Null(), false
}

func (o *respObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no member call: %s", name)
}

func (o *respObj) Index(key Value) (Value, error) {
	return Null(), nil
}

func (o *respObj) headerFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "%s.header(name) expects 1 arg", o.name)
	}

	if o.r == nil {
		return Null(), nil
	}

	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}

	v, ok := o.h[strings.ToLower(k)]
	if !ok {
		return Str(""), nil
	}
	return Str(v), nil
}

func (o *respObj) textFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "%s.text() expects 0 args", o.name)
	}

	if o.r == nil {
		return Str(""), nil
	}

	s := string(o.r.Body)
	if ctx != nil && ctx.Lim.MaxStr > 0 && len(s) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "text too long")
	}
	return Str(s), nil
}

func (o *respObj) jsonFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) > 1 {
		return Null(), rtErr(ctx, pos, "%s.json(path) expects 0 or 1 arg", o.name)
	}

	if o.r == nil {
		return Null(), nil
	}

	if !o.jdone {
		var raw any
		o.jerr = json.Unmarshal(o.r.Body, &raw)
		if o.jerr == nil {
			o.jv = raw
		}
		o.jdone = true
	}

	if o.jerr != nil {
		return Null(), rtErr(ctx, pos, "invalid json")
	}

	path := ""
	if len(args) == 1 {
		p, err := toStr(ctx, pos, args[0])
		if err != nil {
			return Null(), err
		}
		path = p
	}

	val, ok := jsonPathGet(o.jv, path)
	if !ok {
		return Null(), nil
	}
	return fromIface(ctx, pos, val)
}

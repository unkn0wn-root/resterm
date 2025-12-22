package rts

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

type Req struct {
	Method string
	URL    string
	H      map[string][]string
	Q      map[string][]string
}

type ReqMut interface {
	SetMethod(value string)
	SetURL(value string)
	SetHeader(name, value string)
	AddHeader(name, value string)
	DelHeader(name string)
	SetQuery(name, value string)
	SetBody(value string)
}

type requestObj struct {
	name string
	req  atomic.Value
	mu   sync.RWMutex
	mut  ReqMut
}

func newRequestObj(name string) *requestObj {
	if strings.TrimSpace(name) == "" {
		name = "request"
	}
	o := &requestObj{name: name}
	var zero *Req
	o.req.Store(zero)
	return o
}

func (o *requestObj) set(r *Req) {
	var val *Req
	if r != nil {
		val = r
	}
	o.req.Store(val)
}

func (o *requestObj) setMut(m ReqMut) {
	o.mu.Lock()
	o.mut = m
	o.mu.Unlock()
}

func (o *requestObj) mutator(ctx *Ctx, pos Pos) (ReqMut, error) {
	o.mu.RLock()
	m := o.mut
	o.mu.RUnlock()
	if m == nil {
		return nil, rtErr(ctx, pos, "request is read-only")
	}
	return m, nil
}

func (o *requestObj) get() *Req {
	if o == nil {
		return nil
	}
	if v, ok := o.req.Load().(*Req); ok {
		return v
	}
	return nil
}

func (o *requestObj) TypeName() string { return o.name }

func (o *requestObj) GetMember(name string) (Value, bool) {
	switch name {
	case "method":
		return Str(reqMethod(o.get())), true
	case "url":
		return Str(reqURL(o.get())), true
	case "headers":
		return Dict(reqHeaders(o.get())), true
	case "header":
		return NativeNamed(o.name+".header", o.headerFn), true
	case "query":
		return Dict(reqQuery(o.get())), true
	case "setMethod":
		return NativeNamed(o.name+".setMethod", o.setMethodFn), true
	case "setURL":
		return NativeNamed(o.name+".setURL", o.setURLFn), true
	case "setHeader":
		return NativeNamed(o.name+".setHeader", o.setHeaderFn), true
	case "addHeader":
		return NativeNamed(o.name+".addHeader", o.addHeaderFn), true
	case "removeHeader":
		return NativeNamed(o.name+".removeHeader", o.removeHeaderFn), true
	case "setQueryParam":
		return NativeNamed(o.name+".setQueryParam", o.setQueryFn), true
	case "setBody":
		return NativeNamed(o.name+".setBody", o.setBodyFn), true
	}
	return Null(), false
}

func (o *requestObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no member call: %s", name)
}

func (o *requestObj) Index(key Value) (Value, error) {
	return Null(), nil
}

func (o *requestObj) headerFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "%s.header(name) expects 1 arg", o.name)
	}
	name, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	h := reqHeadersRaw(o.get())
	if len(h) == 0 {
		return Str(""), nil
	}
	key := lowerKey(name)
	vals, ok := h[key]
	if !ok || len(vals) == 0 {
		return Str(""), nil
	}
	return Str(vals[0]), nil
}

func (o *requestObj) setMethodFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".setMethod(method)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[0], o.name+".setMethod(method)")
	if err != nil {
		return Null(), err
	}
	m.SetMethod(val)
	return Null(), nil
}

func (o *requestObj) setURLFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".setURL(url)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[0], o.name+".setURL(url)")
	if err != nil {
		return Null(), err
	}
	m.SetURL(val)
	return Null(), nil
}

func (o *requestObj) setHeaderFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, o.name+".setHeader(name, value)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".setHeader(name, value)")
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[1], o.name+".setHeader(name, value)")
	if err != nil {
		return Null(), err
	}
	m.SetHeader(name, val)
	return Null(), nil
}

func (o *requestObj) addHeaderFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, o.name+".addHeader(name, value)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".addHeader(name, value)")
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[1], o.name+".addHeader(name, value)")
	if err != nil {
		return Null(), err
	}
	m.AddHeader(name, val)
	return Null(), nil
}

func (o *requestObj) removeHeaderFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".removeHeader(name)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".removeHeader(name)")
	if err != nil {
		return Null(), err
	}
	m.DelHeader(name)
	return Null(), nil
}

func (o *requestObj) setQueryFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, o.name+".setQueryParam(name, value)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".setQueryParam(name, value)")
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[1], o.name+".setQueryParam(name, value)")
	if err != nil {
		return Null(), err
	}
	m.SetQuery(name, val)
	return Null(), nil
}

func (o *requestObj) setBodyFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".setBody(body)"); err != nil {
		return Null(), err
	}
	m, err := o.mutator(ctx, pos)
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[0], o.name+".setBody(body)")
	if err != nil {
		return Null(), err
	}
	m.SetBody(val)
	return Null(), nil
}

func reqMethod(r *Req) string {
	if r == nil {
		return ""
	}
	return r.Method
}

func reqURL(r *Req) string {
	if r == nil {
		return ""
	}
	return r.URL
}

func reqHeadersRaw(r *Req) map[string][]string {
	if r == nil || len(r.H) == 0 {
		return nil
	}
	return r.H
}

func reqHeaders(r *Req) map[string]Value {
	if r == nil || len(r.H) == 0 {
		return map[string]Value{}
	}
	out := make(map[string]Value, len(r.H))
	for k, v := range r.H {
		if len(v) == 0 {
			out[k] = Str("")
			continue
		}
		out[k] = Str(v[0])
	}
	return out
}

func reqQuery(r *Req) map[string]Value {
	if r == nil {
		return map[string]Value{}
	}
	q := r.Q
	if len(q) == 0 && r.URL != "" {
		q = parseReqQuery(r.URL)
	}
	if len(q) == 0 {
		return map[string]Value{}
	}
	out := make(map[string]Value, len(q))
	for k, v := range q {
		if len(v) == 0 {
			out[k] = Str("")
			continue
		}
		if len(v) == 1 {
			out[k] = Str(v[0])
			continue
		}
		items := make([]Value, 0, len(v))
		for _, it := range v {
			items = append(items, Str(it))
		}
		out[k] = List(items)
	}
	return out
}

func parseReqQuery(raw string) map[string][]string {
	idx := strings.Index(raw, "?")
	if idx < 0 {
		return nil
	}
	q := raw[idx+1:]
	if cut := strings.Index(q, "#"); cut >= 0 {
		q = q[:cut]
	}
	if strings.TrimSpace(q) == "" {
		return nil
	}
	vals, err := url.ParseQuery(q)
	if err != nil || len(vals) == 0 {
		return nil
	}
	out := make(map[string][]string, len(vals))
	for k, v := range vals {
		if len(v) == 0 {
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

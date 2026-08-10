package rts

// overlayAsserts binds the @assert shorthands for the response under test.
// They sit above the locals so an @for-each variable named status does not hide
// the status the assertion is written against.
func (p pre) overlayAsserts(r *Resp) {
	o := newRespObj("response", r)
	code := 0
	status := ""
	if r != nil {
		code = r.Code
		status = r.Status
	}
	p.values["status"] = Num(float64(code))
	p.values["statusCode"] = Num(float64(code))
	p.values["statusText"] = Str(status)
	p.values["header"] = NativeNamed("header", o.headerFn)
	p.values["text"] = NativeNamed("text", o.textFn)
}

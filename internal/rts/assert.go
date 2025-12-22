package rts

func AssertExtra(r *Resp) map[string]Value {
	o := newRespObj("response", r)
	code := 0
	status := ""
	if r != nil {
		code = r.Code
		status = r.Status
	}
	return map[string]Value{
		"status":     Num(float64(code)),
		"statusCode": Num(float64(code)),
		"statusText": Str(status),
		"header":     NativeNamed("header", o.headerFn),
		"text":       NativeNamed("text", o.textFn),
	}
}

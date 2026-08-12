package stdlib

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

func testCtx() *rts.Ctx {
	return rts.NewCtx(context.Background(), rts.Limits{
		MaxStr: 1024, MaxList: 1024, MaxDict: 1024,
	})
}

func TestDictHelpersUseExactKeys(t *testing.T) {
	ctx := testCtx()
	const d = `{" a ": "spaced", "a": "plain", "A": "upper", "": "blank"}`
	tests := map[string]string{
		`rts.dict.get(` + d + `, " a ")`:      "spaced",
		`rts.dict.get(` + d + `, "a")`:        "plain",
		`rts.dict.get(` + d + `, "A")`:        "upper",
		`rts.dict.get(` + d + `, "")`:         "blank",
		`rts.dict.set({}, " a ", "x")[" a "]`: "x",
	}
	for src, want := range tests {
		v := evalExprCtx(t, ctx, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}
}

func TestHeaderHelpersUseCardinalityShape(t *testing.T) {
	ctx := testCtx()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "get singleton", src: `headers.get({"X-Token": "a"}, "x-token")`, want: "a"},
		{name: "get multiple", src: `headers.get({"X-Token": ["a", "b"]}, "x-token")`, want: "a"},
		{name: "normalized singleton", src: `headers.normalize({"X-Token": "a"})["x-token"]`, want: "a"},
		{name: "singleton list collapses", src: `headers.normalize({"X-Token": ["a"]})["x-token"]`, want: "a"},
		{name: "set singleton", src: `headers.set({}, "X-Token", "a")["x-token"]`, want: "a"},
		{name: "normalized key", src: `rts.dict.keys(headers.normalize({"X-Token": "a"}))[0]`, want: "x-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := evalExprCtx(t, ctx, tt.src)
			if v.K != rts.VStr || v.S != tt.want {
				t.Fatalf("%s = %+v, want %q", tt.src, v, tt.want)
			}
		})
	}

	v := evalExprCtx(t, ctx, `headers.normalize({"X-Token": ["a", "b"]})["x-token"]`)
	if v.K != rts.VList || len(v.L) != 2 {
		t.Fatalf("multiple header value = %+v, want list<string> with two values", v)
	}

	v = evalExprCtx(t, ctx, `headers.normalize({"Empty": []}).empty`)
	if v.K != rts.VList || len(v.L) != 0 {
		t.Fatalf("empty header value = %+v, want empty list", v)
	}

	v = evalExprCtx(t, ctx, `len(headers.set({"X-Token": "a"}, "x-token", "b"))`)
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("headers.set retained equivalent names: %+v", v)
	}

	v = evalExprCtx(t, ctx, `len(headers.merge({"X-Token": "a"}, {"x-token": null}))`)
	if v.K != rts.VNum || v.N != 0 {
		t.Fatalf("headers.merge did not remove null patch: %+v", v)
	}
}

func TestHeaderHelpersRejectMalformedBlocksDeterministically(t *testing.T) {
	ctx := testCtx()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "invalid name", src: `headers.normalize({"X Token": "a"})`, want: `"X Token" is not an HTTP header name`},
		{name: "equivalent names", src: `headers.normalize({"X-Token": "a", "x-token": "b"})`, want: `"X-Token" and "x-token" are the same HTTP header`},
		{name: "number value", src: `headers.normalize({"X-Token": 1})`, want: "expects string or list<string>"},
		{name: "null value", src: `headers.normalize({"X-Token": null})`, want: "expects string or list<string>"},
		{name: "mixed list", src: `headers.normalize({"X-Token": ["a", 1]})`, want: "expects list<string>"},
		{name: "untrimmed name", src: `headers.get({}, " X-Token ")`, want: "expects an HTTP header name"},
		{name: "invalid set value", src: `headers.set({}, "X-Token", true)`, want: "expects string or list<string>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 8 {
				err := evalErr(t, ctx, tt.src)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("%s: error = %v, want %q", tt.src, err, tt.want)
				}
			}
		})
	}
}

func TestQueryHelpersUseCardinalityShapeAndExplicitParsing(t *testing.T) {
	ctx := testCtx()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "raw key whitespace", src: `query.parse(" a=1")[" a"]`, want: "1"},
		{name: "raw value whitespace", src: `query.parse("a=1 ").a`, want: "1 "},
		{name: "multiple URL values", src: `query.fromURL("https://x.test/p?a=1&a=2").a[1]`, want: "2"},
		{name: "raw query is not URL", src: `query.parse("https://x.test/p?a=1")["https://x.test/p?a"]`, want: "1"},
		{name: "encode singleton", src: `query.encode({" a ": "1"})`, want: "+a+=1"},
		{name: "encode parsed query", src: `query.encode(query.parse("?=1&a=2"))`, want: "=1&a=2"},
		{name: "singleton list collapses", src: `query.parse(query.encode({a: ["1"]})).a`, want: "1"},
		{name: "merge singleton", src: `query.fromURL(query.merge("https://x.test", {a: "1"})).a`, want: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := evalExprCtx(t, ctx, tt.src)
			if v.K != rts.VStr || v.S != tt.want {
				t.Fatalf("%s = %+v, want %q", tt.src, v, tt.want)
			}
		})
	}

	v := evalExprCtx(t, ctx, `query.fromURL("https://x.test/p?a=1&a=2").a`)
	if v.K != rts.VList || len(v.L) != 2 {
		t.Fatalf("multiple query value = %+v, want list<string> with two values", v)
	}

	v = evalExprCtx(t, ctx, `query.encode({a: []})`)
	if v.K != rts.VStr || v.S != "" {
		t.Fatalf("empty query value encoded as %v, want empty query", v)
	}
}

func TestQueryHelpersRejectValuesOutsideCardinalityShape(t *testing.T) {
	ctx := testCtx()
	for _, src := range []string{
		`query.encode({a: null})`,
		`query.encode({a: true})`,
		`query.encode({a: 1})`,
		`query.encode({a: [1]})`,
		`query.merge("https://x.test", {a: true})`,
	} {
		if err := evalErr(t, ctx, src); err == nil {
			t.Fatalf("%s: expected a type error", src)
		}
	}
}

func evalErr(t *testing.T, ctx *rts.Ctx, src string) error {
	t.Helper()
	mod, err := rts.ParseModule("test", []byte("export let __v = "+src))
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	_, err = rts.Exec(ctx, mod, New())
	return err
}

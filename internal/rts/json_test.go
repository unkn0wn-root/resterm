package rts

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

var (
	jsonNums = []float64{
		0, math.Copysign(0, -1), 1, -1.5, 0.1 + 0.2, 1e-6, 1e-7, -1e-7, 1e20, 1e21, -1e21, 1 << 53,
		9007199254740993, 123456789012345678901234567890, math.MaxFloat64, math.SmallestNonzeroFloat64,
	}
	jsonStrs = []string{
		"", "a", "id", "<b>&amp;</b>", `say "hi"`, `back\slash`, "a/b", "\x00\x01\x1f\x7f", "\b\f\n\r\t",
		"\u2028\u2029", "\xff", "a\xc3(", "é", "😀", "\ufffd",
	}
)

func randValue(r *rand.Rand, depth int) Value {
	switch n := r.IntN(8); {
	case depth > 0 && n == 0:
		l := make([]Value, r.IntN(6))
		for i := range l {
			l[i] = randValue(r, depth-1)
		}
		return List(l)
	case depth > 0 && n == 1:
		m := map[string]Value{}
		for range r.IntN(6) {
			m[jsonStrs[r.IntN(len(jsonStrs))]] = randValue(r, depth-1)
		}
		return Dict(m)
	case depth > 0 && n == 2:
		if r.IntN(2) == 0 {
			return newResult(true, randValue(r, depth-1), nil)
		}
		return newResult(false, Null(), errors.New(jsonStrs[r.IntN(len(jsonStrs))]))
	case n == 3:
		return Bool(r.IntN(2) == 0)
	case n == 4:
		return Num(jsonNums[r.IntN(len(jsonNums))])
	case n == 5:
		return Str(jsonStrs[r.IntN(len(jsonStrs))])
	case n == 6:
		return []Value{{K: VFunc}, {K: VNative}, Obj(nil)}[r.IntN(3)]
	}
	return Null()
}

func TestEncodeJSONMatchesEncodingJSON(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		v := randValue(r, 5)
		html, err := json.Marshal(ToIface(v))
		if err != nil {
			t.Fatal(err)
		}
		var plain bytes.Buffer
		enc := json.NewEncoder(&plain)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(ToIface(v)); err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct {
			escape  bool
			compact string
		}{
			{true, string(html)},
			{false, strings.TrimSuffix(plain.String(), "\n")},
		} {
			for _, indent := range []string{"", "\t", "  ", "ab", strings.Repeat(" ", 32)} {
				want := tt.compact
				if indent != "" {
					var b bytes.Buffer
					if err := json.Indent(&b, []byte(tt.compact), "", indent); err != nil {
						t.Fatal(err)
					}
					want = b.String()
				}
				o := JSONOptions{EscapeHTML: tt.escape, Indent: indent}
				got, err := EncodeJSON(v, o)
				if err != nil || got != want {
					t.Fatalf("EncodeJSON(html %t, indent %q) = %s, %v\nwant %s", tt.escape, indent, got, err, want)
				}
				o.Max = 1 + r.IntN(len(want)+1)
				got, err = EncodeJSON(v, o)
				cut := len(want) > o.Max
				if err != nil || !cut && got != want || cut && (len(got) <= o.Max || !strings.HasPrefix(want, got)) {
					t.Fatalf("EncodeJSON(max %d) = %s, %v\nwant a prefix past the limit of %s", o.Max, got, err, want)
				}
			}
		}
	}
}

func TestEncodeJSONRejectsNaNAndInfinity(t *testing.T) {
	for _, n := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, v := range []Value{
			Num(n),
			List([]Value{Num(1), Dict(map[string]Value{"a": Num(n)})}),
			newResult(true, List([]Value{Num(n)}), nil),
		} {
			if s, err := EncodeJSON(v, JSONOptions{}); s != "" || !errors.Is(err, ErrNotJSON) {
				t.Fatalf("EncodeJSON(%v) = %q, %v, want ErrNotJSON", n, s, err)
			}
		}
	}
}

func TestEncodeJSONCheckSkipsObjectForms(t *testing.T) {
	errFn := errors.New("function")
	o := JSONOptions{EscapeHTML: true, Check: func(v Value) error {
		if v.K == VFunc {
			return errFn
		}
		return nil
	}}
	fn := Value{K: VFunc}
	if _, err := EncodeJSON(Dict(map[string]Value{"a": List([]Value{Num(1), fn})}), o); !errors.Is(err, errFn) {
		t.Fatalf("EncodeJSON error = %v, want the check error", err)
	}
	r := newResult(true, List([]Value{fn}), nil)
	want, err := json.Marshal(ToIface(r))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := EncodeJSON(r, o); err != nil || got != string(want) {
		t.Fatalf("EncodeJSON(result) = %s, %v, want %s", got, err, want)
	}
}

package history

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const secret = "later-cap-77aa-bbcc"

var timeType = reflect.TypeFor[time.Time]()

func fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem())
	case reflect.Struct:
		if v.Type() == timeType {
			return
		}
		for i := range v.NumField() {
			fill(v.Field(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fill(v.Index(0))
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		fill(k)
		e := reflect.New(v.Type().Elem()).Elem()
		fill(e)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.String:
		v.SetString(`tok "` + secret + `" \/`)
	}
}

func walk(v reflect.Value, path string, f func(path, s string)) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			walk(v.Elem(), path, f)
		}
	case reflect.Struct:
		if v.Type() == timeType {
			return
		}
		for i := range v.NumField() {
			walk(v.Field(i), path+"."+v.Type().Field(i).Name, f)
		}
	case reflect.Slice:
		for i := range v.Len() {
			walk(v.Index(i), path+"[]", f)
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			walk(it.Key(), path+"{key}", f)
			walk(it.Value(), path+"{}", f)
		}
	case reflect.String:
		f(path, v.String())
	}
}

func TestEntryMaskedMasksEveryField(t *testing.T) {
	for typ, n := range map[reflect.Type]int{
		reflect.TypeFor[Entry]():          18,
		reflect.TypeFor[CompareEntry]():   3,
		reflect.TypeFor[CompareResult]():  9,
		reflect.TypeFor[ProfileResults](): 16,
		reflect.TypeFor[ProfileFailure](): 5,
		reflect.TypeFor[TraceSummary]():   8,
		reflect.TypeFor[TracePhase]():     5,
		reflect.TypeFor[TracePhaseMeta](): 3,
		reflect.TypeFor[TraceDetails]():   2,
		reflect.TypeFor[TraceConn]():      13,
		reflect.TypeFor[TraceTLS]():       7,
		reflect.TypeFor[TraceCert]():      6,
		reflect.TypeFor[TraceBudget]():    3,
		reflect.TypeFor[TraceBreach]():    4,
	} {
		if got := typ.NumField(); got != n {
			t.Errorf("%v has %d fields, want %d. Mask the new field in masked or list it in kept", typ, got, n)
		}
	}

	kept := map[string]bool{}
	for _, p := range []string{
		".ID", ".Environment", ".EnvironmentSelection{key}", ".EnvironmentSelection{}", ".FilePath", ".Method",
		".Compare.Baseline", ".Compare.Group", ".Compare.Results[].Environment", ".Compare.Results[].Profile",
		".Compare.Results[].EnvironmentSelection{key}", ".Compare.Results[].EnvironmentSelection{}",
		".ProfileResults.Status", ".Trace.Phases[].Kind", ".Trace.Details.Connection.Network",
		".Trace.Details.Connection.Protocol", ".Trace.Details.TLS.Version", ".Trace.Details.TLS.Cipher",
		".Trace.Details.TLS.ALPN", ".Trace.Breaches[].Kind", ".Trace.Budgets.Phases{key}",
	} {
		kept[p] = true
	}

	var in Entry
	fill(reflect.ValueOf(&in).Elem())
	out, hit := in.masked(redact.New(secret))
	if !hit {
		t.Error("masked reported no change")
	}
	seen := map[string]bool{}
	walk(reflect.ValueOf(out), "", func(p, s string) {
		seen[p] = true
		if has := strings.Contains(s, secret); has != kept[p] {
			t.Errorf("%s = %q, kept %v", p, s, kept[p])
		}
	})
	for p := range kept {
		if !seen[p] {
			t.Errorf("kept field %s is not in Entry", p)
		}
	}

	var want Entry
	fill(reflect.ValueOf(&want).Elem())
	if !reflect.DeepEqual(in, want) {
		t.Error("masked changed its input")
	}
}

func TestEntryMaskedKeepsPlain(t *testing.T) {
	in := Entry{
		ID:          "1",
		Method:      "GET",
		URL:         "https://api.test/v1?q=plain%2Fvalue",
		Status:      "200 OK",
		BodySnippet: `{"a":"plain%2Fvalue","n":1}`,
		RequestText: "GET https://api.test/v1?q=plain%2Fvalue\nAccept: */*\n",
		Tags:        []string{"smoke"},
		ProfileResults: &ProfileResults{
			Status:   "pass",
			Error:    "timeout",
			Failures: []ProfileFailure{{Reason: "500 Internal Server Error"}},
		},
		Trace: &TraceSummary{
			Error:  "dial tcp: refused",
			Phases: []TracePhase{{Kind: "dns", Meta: TracePhaseMeta{Addr: "api.test"}}},
			Details: &TraceDetails{
				Connection: &TraceConn{RemoteAddr: "10.0.0.1:443", ResolvedAddrs: []string{"10.0.0.1"}},
				TLS: &TraceTLS{
					ServerName:   "api.test",
					Certificates: []TraceCert{{Subject: "api.test", SANs: []string{"api.test"}}},
				},
			},
		},
		Compare: &CompareEntry{Results: []CompareResult{{Status: "200 OK", BodySnippet: "plain", Error: "e"}}},
	}
	for name, sec := range map[string]redact.Set{"zero Set": {}, "no hit": redact.New(secret)} {
		out, hit := in.masked(sec)
		if hit || !reflect.DeepEqual(out, in) {
			t.Errorf("%s: masked = %+v, %v, want the input unchanged", name, out, hit)
		}
	}
}

func TestEntryMaskedForms(t *testing.T) {
	const s = "tok/later-cap-77aa"
	sec := redact.New(s)
	for name, f := range map[string]string{
		"json":   `tok\/later-cap-77aa`,
		"pct":    "tok%2flater-cap-77aa",
		"PCT":    "tok%2Flater-cap-77aa",
		"html":   "tok&#47;later-cap-77aa",
		"base64": base64.StdEncoding.EncodeToString([]byte("bin-" + s)),
	} {
		in := Entry{
			URL:         "https://api.test/x?k=" + f,
			Status:      "401 denied " + f,
			RequestText: "POST https://api.test/x\n\n{\"t\": \"" + f + "\"}",
			BodySnippet: `{"v": "` + f + `"}`,
		}
		out, hit := in.masked(sec)
		if !hit {
			t.Errorf("%s: masked reported no change", name)
		}
		for field, v := range map[string]string{
			"URL":         out.URL,
			"Status":      out.Status,
			"RequestText": out.RequestText,
			"BodySnippet": out.BodySnippet,
		} {
			if strings.Contains(v, f) || !strings.Contains(v, redact.Mask) {
				t.Errorf("%s: %s = %q, want %q masked", name, field, v, f)
			}
		}
	}
	plain := Entry{URL: "https://api.test/x?k=plain%2Fvalue"}
	if out, _ := plain.masked(sec); out.URL != plain.URL {
		t.Errorf("URL = %q, want %q", out.URL, plain.URL)
	}
}

func TestEntryMaskedSnippetCut(t *testing.T) {
	check := func(name, snip, s string) {
		t.Helper()
		if len(snip) > maxSnippet || !utf8.ValidString(snip) || strings.Contains(snip, s) {
			t.Errorf("%s: snippet of %d bytes, valid %v, tail %q", name, len(snip), utf8.ValidString(snip),
				snip[max(0, len(snip)-24):])
		}
		for k := 1; k < len(s); k++ {
			if strings.HasSuffix(snip, s[:k]) {
				t.Errorf("%s: snippet ends in %q, a prefix of the secret", name, s[:k])
			}
		}
	}
	sec := redact.New(secret)
	for off := maxSnippet - 10; off <= maxSnippet; off++ {
		text := strings.Repeat("x", off) + secret + strings.Repeat("y", 50)
		out, _ := Entry{BodySnippet: text}.masked(sec)
		check(fmt.Sprintf("text at %d", off), out.BodySnippet, secret)
	}
	js := `{"a":"` + strings.Repeat("x", 1995-6) + secret + `yy"}`
	out, _ := Entry{BodySnippet: js}.masked(sec)
	check("json at 1995", out.BodySnippet, secret)
	for _, s := range []string{"Q", "QZ"} {
		for _, off := range []int{maxSnippet - 1, maxSnippet} {
			text := strings.Repeat("x", off-1) + " " + s + " " + strings.Repeat("y", 50)
			out, _ := Entry{BodySnippet: text}.masked(redact.New(s))
			check(fmt.Sprintf("RST-441 short secret %s as a word at %d", s, off), out.BodySnippet, s)
		}
	}

	for _, tt := range []struct {
		name string
		in   Entry
		want func(Entry) string
		n    int
	}{
		{
			name: "short",
			in:   Entry{BodySnippet: "short"},
			want: func(e Entry) string { return e.BodySnippet },
			n:    len("short"),
		},
		{
			name: "exactly maxSnippet",
			in:   Entry{BodySnippet: strings.Repeat("x", maxSnippet)},
			want: func(e Entry) string { return e.BodySnippet },
			n:    maxSnippet,
		},
		{
			name: "rune at the cut",
			in:   Entry{BodySnippet: strings.Repeat("x", maxSnippet-1) + "é" + strings.Repeat("y", 10)},
			want: func(e Entry) string { return e.BodySnippet },
			n:    maxSnippet - 1,
		},
		{
			name: "no rune start near the cut",
			in:   Entry{BodySnippet: "ab" + strings.Repeat("\x80", maxSnippet+100)},
			want: func(e Entry) string { return e.BodySnippet },
			n:    maxSnippet,
		},
		{
			name: "workflow report",
			in:   Entry{Method: restfile.HistoryMethodWorkflow, BodySnippet: strings.Repeat("x", 5000)},
			want: func(e Entry) string { return e.BodySnippet },
			n:    5000,
		},
		{
			name: "compare row",
			in:   Entry{Compare: &CompareEntry{Results: []CompareResult{{BodySnippet: strings.Repeat("x", 5000)}}}},
			want: func(e Entry) string { return e.Compare.Results[0].BodySnippet },
			n:    maxSnippet,
		},
	} {
		out, _ := tt.in.masked(sec)
		if got, raw := tt.want(out), tt.want(tt.in); len(got) != tt.n || got != raw[:tt.n] {
			t.Errorf("%s: snippet of %d bytes, want the first %d", tt.name, len(got), tt.n)
		}
	}
}

func BenchmarkEntryMasked(b *testing.B) {
	sec := redact.New(secret, "other-secret-0001", "third-secret-00002")
	for _, size := range []int{10 << 10, 1 << 20} {
		e := Entry{
			ID:          "1",
			Method:      "GET",
			URL:         "https://api.example.test/items?page=1",
			Status:      "200 OK",
			BodySnippet: jsonBody(size),
			RequestText: "GET https://api.example.test/items?page=1\nAccept: application/json\n",
			Tags:        []string{"smoke"},
		}
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			b.SetBytes(int64(size))
			for b.Loop() {
				e.masked(sec)
			}
		})
	}
}

func jsonBody(n int) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; b.Len() < n-200; i++ {
		fmt.Fprintf(&b, `{"id":%d,"name":"user %d ü","email":"u%d@example.test","note":"line\nnext <b> & \"q\""},`,
			i, i, i)
	}
	b.WriteString(`{"id":-1}]}`)
	return b.String() + strings.Repeat(" ", max(0, n-b.Len()))
}

package redact

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

var hard = []string{
	`q"x1`, `b\x2`, `t3\`, "n\nx4", `l<x5>`, `a&x6`, `p+x7`, `s x8`, `c%x9`, `sl/x10`, `e=x11`,
	"üñï12", "u" + string(rune(0x2028)) + "x13", "t\tx14", "c" + string(rune(1)) + "x15", "😀x16", `m(k)!*'x17`,
}

func TestNewKeepsEachSecretOnce(t *testing.T) {
	tests := []struct {
		secs []string
		want []string
	}{
		{nil, nil},
		{[]string{""}, nil},
		{[]string{"tok", "tok"}, []string{"tok"}},
		{[]string{"a b"}, []string{"a b"}},
		{[]string{"ü b"}, []string{"ü b", "\xfc b"}},
		{[]string{"bcd", "abc"}, []string{"abc", "bcd"}},
	}
	for _, tt := range tests {
		if got := New(tt.secs...).needles; !slices.Equal(got, tt.want) {
			t.Errorf("New(%q) needles = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestTextForms(t *testing.T) {
	tests := []struct {
		sec  string
		in   string
		want string
	}{
		{"sl/x10", "http://h/status?k=sl%2fx10", "http://h/status?k=•••"},
		{"sl/x10", "http://h/redir2?to=sl%252fx10", "http://h/redir2?to=•••"},
		{"sl/x10", "http://h/redir2?to=sl%252Fx10&n=1", "http://h/redir2?to=•••&n=1"},
		{"a/b9", `denied {"t":"a\/b9"}`, `denied {"t":"•••"}`},
		{"0000%00", "& 0%30&#48;0%25&#48;0 0", "& ••• 0"},
		{"a/b9", "ws://h/deny?k=%7B%22t%22%3A%22a%5C%2Fb9%22%7D", "ws://h/deny?k=%7B%22t%22%3A%22•••%22%7D"},
		{"üñï12", "{\"t\":\"\\u00fc\\u00f1\\u00ef12\"}", `{"t":"•••"}`},
		{"üñï12", "{\"t\":\"\\u00FC\\u00F1\\u00EF12\"}", `{"t":"•••"}`},
		{"üñï12", "k=%C3%BC%C3%B1%C3%AF12", "k=•••"},
		{"üñï12", "k=%c3%bc%c3%b1%c3%af12", "k=•••"},
		{"üñï12", "<b>&uuml;&ntilde;&iuml;12</b>", "<b>•••</b>"},
		{"a&x6", "<p>bad a&amp;x6</p>", "<p>bad •••</p>"},
		{"l<x5>", "<p>bad l&lt;x5&#62;</p>", "<p>bad •••</p>"},
		{"l<x5>", "x &#x6c;&#x3C;x5&#X3e; y", "x ••• y"},
		{"l<x5>", "{\"t\":\"l\\u003cx5\\u003e\"}", `{"t":"•••"}`},
		{`q"x1`, `field "q\"x1" bad`, `field "•••" bad`},
		{`q"x1`, "{\"t\":\"q\\u0022x1\"}", `{"t":"•••"}`},
		{"c\x01x15", `Get "https://h/?k=c\x01x15": refused`, `Get "https://h/?k=•••": refused`},
		{"c\x01x15", "{\"t\":\"c\\u0001x15\"}", `{"t":"•••"}`},
		{"t\tx14", `"t\tx14"`, `"•••"`},
		{"😀x16", "{\"t\":\"\\ud83d\\ude00x16\"}", `{"t":"•••"}`},
		{"😀x16", "\"\\U0001F600x16\"", `"•••"`},
		{"😀x16", "<i>&#128512;x16</i>", "<i>•••</i>"},
		{"s x8", "k=s+x8&z=1", "k=•••&z=1"},
		{"s x8", "k=s%20x8", "k=•••"},
		{"p+x7", "k=p%2Bx7", "k=•••"},
		{"p+x7", "k=p%2bx7", "k=•••"},
		{"p+x/7", "https://h/" + url.PathEscape("p+x/7") + "?n=1", "https://h/•••?n=1"},
		{"m(k)!*'x17", "k=m%28k%29%21%2A%27x17", "k=•••"},
		{"it's (a) b*!", "https://h/x?k=it's%20(a)%20b*!&y=1", "https://h/x?k=•••&y=1"},
		{"s3 cr+t/v=l;x", "https://h/x?k=s3+cr%2Bt%2Fv%3Dl%3Bx&y=1", "https://h/x?k=•••&y=1"},
		{"s3 cr+t/v=l;x", "https://h/hooks/s3%20cr+t/v=l;x", "https://h/hooks/•••"},
		{"s3 cr+t/v=l;x", `Get "https://h/x?k=s3+cr%2Bt%2Fv%3Dl%3Bx": refused`, `Get "https://h/x?k=•••": refused`},
		{`b\x2`, `{"t":"b\\x2"}`, `{"t":"•••"}`},
		{`b\x2`, `b\x2`, "•••"},
		{`t3\`, `{"t":"t3\\"}`, `{"t":"•••"}`},
		{"n\nx4", `{"t":"n\nx4"}`, `{"t":"•••"}`},
		{"u" + string(rune(0x2028)) + "x13", "{\"t\":\"u\\u2028x13\"}", `{"t":"•••"}`},
		{`pa%2Fss"w0rd`, `401 denied {"t": "pa%2Fss\"w0rd"}`, `401 denied {"t": "•••"}`},
		{`pa%2Fss"w0rd`, "<p>Token: pa%2Fss&quot;w0rd</p>", "<p>Token: •••</p>"},
		{"a%41&b", "<p>a%41&amp;b</p>", "<p>•••</p>"},
		{"\x00%00", `"\x00%00"`, `"•••"`},
		{`x<\`, `{"t":"x\u0026lt;\\"}`, `{"t":"•••"}`},
		{`"0\`, `"\u0026#34;0\\"`, `"•••"`},
		{"😀x16", "k=%uD83D%uDE00x16", "k=•••"},
		{"€uro", "k=%u20ACuro&n=1", "k=•••&n=1"},
		{"€uro", "k=%u20acuro", "k=•••"},
		{"üñï12", "k=%FC%F1%EF12", "k=•••"},
		{"üñï12", "k=%fc%f1%ef12&n=1", "k=•••&n=1"},
		{"ü€x", "k=%FC%u20ACx", "k=•••"},
		{"s ü", "k=s+%FC&n=1", "k=•••&n=1"},
		{"üñï12", "<p>\xfc\xf1\xef12</p>", "<p>•••</p>"},
		{"ab\xffcd", "k=ab%FFcd", "k=•••"},
	}
	for _, tt := range tests {
		if got := New(tt.sec).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.sec, got, tt.want)
		}
	}
}

func TestTextKeepsOtherText(t *testing.T) {
	const in = `GET https://h/x?k=plain%2Fvalue "q\"x" &amp; ü YmluLXNlbmQ= s3cr3t-`
	tests := []struct {
		name string
		set  Set
		in   string
	}{
		{"zero Set", Set{}, in},
		{"empty secret", New(""), in},
		{"no secret", New("secret"), in},
		{"escaped other value", New("plain/value2"), in},
		{"base64 without the secret", New("binsec77"), "YmluLXNlbmQ="},
		{"short word", New("binsec77"), "abcd"},
		{"other query value", New("s3 cr+t/v=l;x"), "https://h/x?k=other"},
		{"words", New("s3cr3tpw77"), "plain log line with words and numbers 12345\nnext line"},
		{"query", New("s3cr3tpw77"), "GET https://api.example.test/v1/users/42?key=abc%2Fdef"},
		{"path", New("s3cr3tpw77"), "/usr/local/bin/some-long-path-name_here"},
		{"other base64", New("s3cr3tpw77"), "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:other-pw"))},
		{"blank line", New("s3cr3t-pw-77"), "czNjcjN0\n\nLXB3LTc3"},
		{"indented blank line", New("s3cr3t-pw-77"), "czNjcjN0\r\n    \r\n    LXB3LTc3"},
		{"two carriage returns", New("s3cr3t-pw-77"), "czNjcjN0\r\rLXB3LTc3"},
		{"spaces without a line break", New("s3cr3t-pw-77"), "czNjcjN0    LXB3LTc3"},
	}
	for _, tt := range tests {
		if got := tt.set.Text(tt.in); got != tt.in {
			t.Errorf("%s: Text(%q) = %q, want it unchanged", tt.name, tt.in, got)
		}
	}
}

func TestTextOverlap(t *testing.T) {
	tests := []struct {
		secs []string
		in   string
		want string
	}{
		{[]string{"xab", "abcdef"}, "1xabcdef2", "1•••2"},
		{[]string{"abcdef", "xab"}, "1xabcdef2", "1•••2"},
		{[]string{"aa"}, "aaaa", "•••"},
		{[]string{"ab", "cd"}, "xabcdy", "x•••y"},
		{[]string{"ab"}, "ab-ab", "•••-•••"},
	}
	for _, tt := range tests {
		if got := New(tt.secs...).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.secs, got, tt.want)
		}
	}
}

func TestTextDepth(t *testing.T) {
	v := "a/b9"
	for depth := 1; depth <= 4; depth++ {
		v = url.QueryEscape(v)
		if got := New("a/b9").Text("k=" + v + "&n=1"); got != "k=•••&n=1" {
			t.Errorf("depth %d: Text(%q) = %q", depth, v, got)
		}
	}
}

func TestTextDepthBoundsNesting(t *testing.T) {
	s := New("secret")
	nest := func(n int) float64 {
		in := "%" + strings.Repeat("25", n) + "41+"
		return testing.AllocsPerRun(1, func() { s.Text(in) })
	}
	if small, big := nest(10), nest(1000); big > 2*small {
		t.Errorf("Text allocates %v times for 10 nested escapes and %v for 1000, want one bound", small, big)
	}
}

func TestTextBase64(t *testing.T) {
	const sec = "binsec77"
	raw := []byte("bin-send-" + sec + "?>")
	for _, in := range []string{
		base64.StdEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw),
		base64.URLEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
		base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(raw))),
		base64.StdEncoding.EncodeToString([]byte("k=" + url.QueryEscape("bin/"+sec))),
		base64.RawStdEncoding.EncodeToString([]byte(sec)),
		base64.StdEncoding.EncodeToString([]byte(sec)),
	} {
		if got := New(sec).Text(in); got != Mask {
			t.Errorf("Text(%q) = %q, want %q", in, got, Mask)
		}
	}
}

func TestTextBase64InText(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString
	pem := b64([]byte(strings.Repeat("x", 53) + "s3cr3tpw77" + strings.Repeat("y", 30)))
	tests := []struct {
		sec  string
		in   string
		want string
	}{
		{"binsec77", "not base64 YmluLXNlbmQtYmluc2VjNzc=", "not base64 •••"},
		{
			"s3cr3t-b64",
			`{"data":"data:application/octet-stream;base64,` + b64([]byte("\x00\x01s3cr3t-b64\xff")) + `"}`,
			`{"data":"data:application/octet-stream;base64,•••"}`,
		},
		{
			"s3cr3tpw77",
			`"Authorization": "Basic ` + b64([]byte("alice:s3cr3tpw77")) + `"`,
			`"Authorization": "Basic •••"`,
		},
		{
			"s3cr3tpw77",
			"Bearer eyJhbGciOiJIUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(
				[]byte(`{"pw":"s3cr3tpw77"}`),
			) + ".c2ln",
			"Bearer eyJhbGciOiJIUzI1NiJ9.•••.c2ln",
		},
		{
			"s3cr3tpw77",
			"Content-Transfer-Encoding: base64\r\n\r\n" + pem[:76] + "\r\n" + pem[76:] + "\r\n",
			"Content-Transfer-Encoding: base64\r\n\r\n•••\r\n",
		},
		{"s3cr3tpw77", "https://h/download/" + b64([]byte("\xff\xfealice:s3cr3tpw77")), "https:•••"},
		{
			"s3cr3tpw77",
			"https://h/dl/" + base64.RawURLEncoding.EncodeToString([]byte("\xff\xfes3cr3tpw77")),
			"https:•••",
		},
		{"pw?>s3c?>r3t", "GET /x?auth=" + url.QueryEscape(b64([]byte("pw?>s3c?>r3t"))) + "&n=1", "GET /x?auth=•••&n=1"},
		{"pw?>s3c?>r3t", `{"d":"` + strings.ReplaceAll(b64([]byte("pw?>s3c?>r3t")), "/", `\/`) + `"}`, `{"d":"•••"}`},
		{"s3cr3t-pw-77", "ab\nc" + b64([]byte("s3cr3t-pw-77")), "•••"},
		{"s3cr3t-pw-77", "a\nbc" + b64([]byte("s3cr3t-pw-77")), "•••"},
		{"s3cr3t-pw-77", strings.Repeat(" ", 13) + "cz\nNjcjN0LXB3LTc3", strings.Repeat(" ", 13) + "•••"},
		{"s3cr3t-pw-77", strings.Repeat(" ", 10) + "cz\n    NjcjN0LXB3LTc3", strings.Repeat(" ", 10) + "•••"},
		{"s3cr3t-pw-77", strings.Repeat(" ", 12) + "cz\n\t\tNjcjN0LXB3LTc3", strings.Repeat(" ", 12) + "•••"},
		{"s3cr3t-pw-77", "czNjcjN0\rLXB3LTc3", "•••"},
		{"s3cr3t-pw-77", "czNjcjN0\n\tLXB3LTc3", "•••"},
		{"s3cr3t-pw-77", "<key>\r\n    czNjcjN0\r\n    LXB3LTc3\r\n</key>", "<key>\r\n    •••\r\n</key>"},
		{"s3cr3t-pw-77", "<key>\n    czNjcjN0\n    LXB3LTc3\n</key>", "<key>\n    •••\n</key>"},
		{"s3cr3t-pw-77", "cert: |\n  czNjcjN0\n  LXB3LTc3\n", "cert: |\n  •••\n"},
		{
			"s3cr3t-pw-77",
			"xml=" + url.QueryEscape("<key>\n    czNjcjN0\n    LXB3LTc3\n</key>") + "&n=1",
			"xml=%3Ckey%3E%0A++++•••%0A%3C%2Fkey%3E&n=1",
		},
		{
			"pw?>s3c?>r3t",
			"https://h/" + url.PathEscape("<k>\n    cHc/PnMz\n    Yz8+cjN0\n</k>"),
			"https://h/%3Ck%3E%0A%20%20%20%20•••%0A%3C%2Fk%3E",
		},
	}
	for _, tt := range tests {
		if got := New(tt.sec).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.sec, got, tt.want)
		}
	}
}

func TestTextSkipsShortBase64(t *testing.T) {
	s := New("binsec77")
	if n := testing.AllocsPerRun(10, func() { s.Text("abcdefghij") }); n != 0 {
		t.Errorf("Text of a run too short to hold the secret allocates %v times, want 0", n)
	}
}

func TestTextHTMLJSONURL(t *testing.T) {
	inner := strings.ReplaceAll(url.QueryEscape("a/b9"), "%", "\\u0025")
	page := "<pre>{&#34;next&#34;:&#34;/cb?k=" + strings.ReplaceAll(inner, `\`, "&#92;") + "&amp;n=1&#34;}</pre>"
	want := "<pre>{&#34;next&#34;:&#34;/cb?k=•••&amp;n=1&#34;}</pre>"
	if got := New("a/b9").Text(page); got != want {
		t.Fatalf("Text(%q) = %q, want %q", page, got, want)
	}
}

func TestURLQueryEscapeAll(t *testing.T) {
	for _, sec := range hard {
		for _, in := range []string{
			url.QueryEscape(sec),
			url.PathEscape(sec),
			(&url.URL{Path: sec}).EscapedPath(),
			strconv.Quote(sec),
			strconv.QuoteToASCII(sec),
		} {
			got := New(sec).Text("x=" + in + ";")
			if got != "x="+Mask+";" && got != `x="`+Mask+`";` {
				t.Errorf("%q written as %q: %q", sec, in, got)
			}
		}
	}
}

func escapeMix(sec string, modes []byte) string {
	var b strings.Builder
	pct := func(s string) {
		for _, c := range []byte(s) {
			fmt.Fprintf(&b, "%%%02x", c)
		}
	}
	for i, n := 0, 0; i < len(sec); n++ {
		r, size := utf8.DecodeRuneInString(sec[i:])
		chunk := sec[i : i+size]
		i += size
		mode := byte(n)
		if len(modes) > 0 {
			mode = modes[n%len(modes)]
		}
		if r == utf8.RuneError && size == 1 {
			mode = mode%2 + 1
		}
		switch mode % 8 {
		case 0:
			if strings.ContainsAny(chunk, `\%&`) {
				pct(chunk)
				continue
			}
			b.WriteString(chunk)
		case 1:
			pct(chunk)
		case 2:
			for _, c := range []byte(chunk) {
				fmt.Fprintf(&b, "\\x%02X", c)
			}
		case 3:
			if r > 0xffff {
				hi, lo := utf16.EncodeRune(r)
				fmt.Fprintf(&b, "\\u%04x\\u%04X", hi, lo)
				continue
			}
			fmt.Fprintf(&b, "\\u%04x", r)
		case 4:
			fmt.Fprintf(&b, "\\U%08x", r)
		case 5, 6:
			ref := fmt.Sprintf("&#%d;", r)
			if mode%8 == 6 {
				ref = fmt.Sprintf("&#x%X;", r)
			}
			if html.UnescapeString(ref) != chunk {
				pct(chunk)
				continue
			}
			b.WriteString(ref)
		case 7:
			if j := strings.IndexByte(shortBytes, chunk[0]); size == 1 && j >= 0 {
				b.WriteString(`\` + shortNames[j:j+1])
				continue
			}
			pct(chunk)
		}
	}
	return b.String()
}

func sharesMaskByte(sec string) bool {
	for i := range len(Mask) {
		if strings.IndexByte(sec, Mask[i]) >= 0 {
			return true
		}
	}
	return false
}

func FuzzText(f *testing.F) {
	for i, sec := range hard {
		f.Add(sec, "GET https://h/?k=", "&p=1", []byte{byte(i), 1, 2, 3, 4, 5, 6, 7, 0})
	}
	f.Fuzz(func(t *testing.T, sec, prefix, suffix string, modes []byte) {
		if sec == "" || sharesMaskByte(sec) {
			t.Skip()
		}
		s := New(sec)
		enc := escapeMix(sec, modes)
		if got := s.Text(enc); got != Mask {
			t.Fatalf("Text(%q) = %q, want %q for the secret %q", enc, got, Mask, sec)
		}
		in := prefix + " " + enc + " " + suffix
		if got := s.Text(in); strings.Contains(got, " "+enc+" ") {
			t.Fatalf("Text(%q) = %q keeps the secret %q", in, got, sec)
		}

		v := prefix + suffix
		if strings.ContainsAny(v, `\%+&`) ||
			slices.ContainsFunc(s.needles, func(n string) bool { return strings.Contains(v, n) }) {
			return
		}
		short := len(slices.MinFunc(s.needles, func(a, b string) int { return len(a) - len(b) }))
		for _, run := range strings.FieldsFunc(v, func(r rune) bool { return !strings.ContainsRune(base64Chars+"\r\n", r) }) {
			if base64.RawStdEncoding.DecodedLen(len(run)) >= short {
				return
			}
		}
		if got := s.Text(v); got != v {
			t.Fatalf("Text(%q) = %q, want it unchanged for the secret %q", v, got, sec)
		}
	})
}

var stdEncoders = []func(string) string{
	func(s string) string { return string(goJSON(s, true)) },
	func(s string) string { return string(goJSON(s, false)) },
	func(s string) string { return strings.ReplaceAll(string(goJSON(s, false)), "/", `\/`) },
	html.EscapeString,
	strconv.Quote,
	url.QueryEscape,
	url.PathEscape,
	func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
	func(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) },
	func(s string) string { return base64.RawStdEncoding.EncodeToString([]byte(s)) },
	func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) },
	func(s string) string {
		var lines []string
		for c := range slices.Chunk([]byte(base64.StdEncoding.EncodeToString([]byte(s))), 3) {
			lines = append(lines, string(c))
		}
		return strings.Join(lines, "\n    ")
	},
}

func FuzzTextEncoders(f *testing.F) {
	for i, sec := range hard {
		f.Add(sec, byte(i), byte(i+1), "GET https://h/?k=", "&p=1")
	}
	f.Fuzz(func(t *testing.T, sec string, first, second byte, prefix, suffix string) {
		if sec == "" || sharesMaskByte(sec) || !utf8.ValidString(sec) {
			t.Skip()
		}
		enc := stdEncoders[int(first)%len(stdEncoders)](sec)
		if i := int(second) % (len(stdEncoders) + 1); i < len(stdEncoders) {
			enc = stdEncoders[i](enc)
		}
		in := prefix + " " + enc + " " + suffix
		if got := New(sec).Text(in); strings.Contains(got, enc) {
			t.Fatalf("Text(%q) = %q keeps %q, the secret %q", in, got, enc, sec)
		}
	})
}

package redact

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"runtime"
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

const localeUnit = `c.defineLocale("en-xx",{` +
	`months:"January_February_March_April_May_June_July_August_September_October_November_December".split("_"),` +
	`weekdaysShort:"Sun_Mon_Tue_Wed_Thu_Fri_Sat".split("_"),` +
	`longDateFormat:{LT:"HH:mm",L:"DD/MM/YYYY",LLL:"D MMMM YYYY HH:mm"},` +
	`calendar:{sameDay:"[Today at] LT",lastDay:"[Yesterday at] LT"},` +
	`relativeTime:{future:"in %s",past:"%s ago",s:"a few seconds",ss:"%d seconds",m:"a minute",mm:"%d minutes",` +
	`h:"an hour",hh:"%d hours",dd:"%d days",MM:"%d months",yy:"%d years"},` +
	`dayOfMonthOrdinalParse:/\d{1,2}(st|nd|rd|th)/,` +
	`ordinal:function(e){var a=e%10;return e+(1==~~(e%100/10)?"th":1==a?"st":2==a?"nd":3==a?"rd":"th")}}),`

func TestNewKeepsEachSecretOnce(t *testing.T) {
	tail := strings.Repeat("a", 14)
	tests := []struct {
		secs []string
		want []needle
	}{
		{nil, nil},
		{[]string{""}, nil},
		{[]string{"tok", "tok"}, []needle{{"tok", 3}}},
		{[]string{"a b"}, []needle{{"a b", 3}}},
		{[]string{"ü b"}, []needle{{"\xfc b", 3}, {"ü b", 3}}},
		{[]string{"bcd", "abc"}, []needle{{"abc", 3}, {"bcd", 3}}},
		{[]string{"©" + tail, "Â©" + tail}, []needle{{"\xa9" + tail, 15}, {"©" + tail, 16}, {"Â©" + tail, 16}}},
	}
	for _, tt := range tests {
		if got := New(tt.secs...).needles; !slices.Equal(got, tt.want) {
			t.Errorf("New(%q) needles = %#v, want %#v", tt.secs, got, tt.want)
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
		{"\u0080ÿ1", "k=%80%FF1", "k=•••"},
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
	tail := strings.Repeat("a", 14)
	tests := []struct {
		secs []string
		in   string
		want string
	}{
		{[]string{"xab", "abcdef"}, "1 xab-abcdef 2", "1 •••-••• 2"},
		{[]string{"abcdef", "xab"}, "1xabcdef2", "1xabcdef2"},
		{[]string{"x-ab", "ab-cd"}, "1 x-ab-cd 2", "1 ••• 2"},
		{[]string{"aa"}, "aaaa", "aaaa"},
		{[]string{"ab", "cd"}, "xabcdy", "xabcdy"},
		{[]string{"ab", "cd"}, "x ab cd y", "x ••• ••• y"},
		{[]string{"ab"}, "ab-ab", "•••-•••"},
		{[]string{"©" + tail, "Â©" + tail}, "x©" + tail + "y", "x•••y"},
		{[]string{"©" + tail}, "x©" + tail + "y", "x©" + tail + "y"},
		{[]string{"abc", "x" + tail}, "YWJj", "•••"},
	}
	for _, tt := range tests {
		if got := New(tt.secs...).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.secs, got, tt.want)
		}
	}
}

func TestTextWords(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString
	tests := []struct {
		sec  string
		in   string
		want string
	}{
		{"test", "latest test failed", "latest ••• failed"},
		{"test", "?pw=test&x=1", "?pw=•••&x=1"},
		{"test", "test", "•••"},
		{"test", "attest tests test_ _test test9 9test", "attest tests test_ _test test9 9test"},
		{"1234", "pin1234 pin 1234", "pin1234 pin •••"},
		{"a", "a", "•••"},
		{"a", "ab a ba,a", "ab ••• ba,•••"},
		{"ab", "ab", "•••"},
		{"s3cr3t-pw-77-x1", "xs3cr3t-pw-77-x1y", "xs3cr3t-pw-77-x1y"},
		{"s3cr3t-pw-77-x1", "x s3cr3t-pw-77-x1 y", "x ••• y"},
		{"s3cr3t-pw-77-x16", "xs3cr3t-pw-77-x16y", "x•••y"},
		{"s3cr3t-pw-77-x17!", "xs3cr3t-pw-77-x17!y", "x•••y"},
		{"üñï12", "xüñï12y", "xüñï12y"},
		{"ü1234", "xü1234", "x•••"},
		{"üüüüüüüü1", "xüüüüüüüü1y", "xüüüüüüüü1y"},
		{"ü1234567890ñ1234", "xü1234567890ñ1234y", "x•••y"},
		{"p/ss!", "xp/ss!", "xp/ss!"},
		{"p/ss!", "x p/ss!y", "x •••y"},
		{"!!", "a!!b", "a•••b"},
		{"-tok", "x-tok", "x•••"},
		{"-tok", "-toky", "-toky"},
		{"test", "日本test", "日本•••"},
		{"latest", "latestä", "•••ä"},
		{"t", "tést", "•••ést"},
		{"test", "\xfftest\xfe", "\xff•••\xfe"},
		{"test", `\ntest`, `\n•••`},
		{"test", "%2Ftest", "%2F•••"},
		{"test", "%252Ftest", "%252F•••"},
		{"test", "&#47;test", "&#47;•••"},
		{"test", "%41test", "%41test"},
		{"test", "%2541test", "%2541test"},
		{"test", `\x41test`, `\x41test`},
		{"test", "test%41", "•••%41"},
		{"st", "t%C3%A9st", "t%C3%A9•••"},
		{"test", "k=a+test+b", "k=a+•••+b"},
		{"test", b64([]byte("latest")), b64([]byte("latest"))},
		{"test", b64([]byte("a test")), "•••"},
		{"test", "Basic+" + b64([]byte("test")), "•••"},
		{"l<x5>", `\U0001F600` + b64([]byte("l<x5>")), `\U0001F600•••`},
		{`q"x1`, "%uD83D%uDE00\n" + b64([]byte(`q"x1`)), "%uD83D%uDE00\n•••"},
		{"l<x5>", "\\U0001F600\r\n\t" + b64([]byte("l<x5>")), "\\U0001F600\r\n\t•••"},
		{"a日本日本日本日本日本日本日本", `\U0001F600` + b64([]byte("a日本日本日本日本日本日本日本")), `\U0001F600•••`},
		{"abc", "YWJjY+", "•••+"},
	}
	for _, tt := range tests {
		if got := New(tt.sec).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.sec, got, tt.want)
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
	pem := b64([]byte(strings.Repeat("x", 52) + " s3cr3tpw77 " + strings.Repeat("y", 29)))
	tests := []struct {
		sec  string
		in   string
		want string
	}{
		{"binsec77", "not base64 YmluLXNlbmQtYmluc2VjNzc=", "not base64 •••"},
		{"s3cr3t-pw-77-x18a?", "k=" + url.QueryEscape(b64([]byte("s3cr3t-pw-77-x18a?"))) + "&n=1", "k=•••&n=1"},
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

func TestTextClusterEdges(t *testing.T) {
	const sec = "s3cr3tpw77"
	b64 := base64.StdEncoding.EncodeToString
	pct := func(v string) string { return fmt.Sprintf("%%%02X", v[0]) + v[1:] }
	wrap := b64([]byte(strings.Repeat("x", 119) + "-" + sec))
	pad := b64([]byte(strings.Repeat("x", 119) + "-" + sec + "-" + strings.Repeat("y", 107)))
	ref := "&#" + strings.Repeat("0", 28) + "38;"
	tests := []struct {
		secs []string
		in   string
		want string
	}{
		{[]string{"zz", ".abc/"}, "z.abc%2F", "z•••"},
		{[]string{"zz", "abcdefghij/"}, "abcdefghij%2F tail", "••• tail"},
		{[]string{"AA"}, "%41A", "•••"},
		{[]string{"AA"}, "%41%41A", "%41%41A"},
		{[]string{"b"}, "a%62", "a%62"},
		{[]string{"b"}, ".%62", ".•••"},
		{[]string{"ab.c/d"}, "%41" + strings.Repeat("y", 129) + "-ab.c%2Fd", "%41" + strings.Repeat("y", 129) + "-•••"},
		{
			[]string{"zz", ".abc/"},
			"%41" + strings.Repeat("y", 131) + ".abc%2F",
			"%41" + strings.Repeat("y", 131) + "•••",
		},
		{[]string{sec}, pct(wrap[:168]) + "\n" + wrap[168:], "•••"},
		{[]string{sec}, pct(wrap[:168]) + "\r\n" + wrap[168:], "•••"},
		{[]string{sec}, pct(wrap[:168]) + "\n    " + wrap[168:], "•••"},
		{[]string{sec}, pct(wrap[:168]) + "\r " + wrap[168:], "•••"},
		{[]string{sec}, pad[:166] + pct(pad[166:]) + " x", "••• x"},
		{[]string{"&s3cr.."}, ref + strings.Repeat(ref[1:], 3) + "s3cr.. end", "••• end"},
		{[]string{"k9"}, "az" + ref + strings.Repeat(ref[1:], 2) + ref[1:30] + "10; k", "•••"},
		{[]string{"x"}, "e" + ref + strings.Repeat(ref[1:], 2) + ref[1:30] + "10; A", "•••"},
		{[]string{"x"}, "%65" + ref + strings.Repeat(ref[1:], 2) + ref[1:30] + "10; A", "•••"},
		{
			[]string{`t3\`},
			"\n  t&#x33;\\u005c&#92;%5c\"t3\\\\\"\\u00\\x74&#51;\\x5CdDNc,+.%FC",
			"\n  •••\"•••\"\\u00\\x74&#51;\\•••,+.%FC",
		},
		{[]string{"s x8"}, "d=" + b64([]byte("k=s+x8")), "d=•••"},
		{[]string{"s x8"}, "d=" + b64([]byte("\xff\xfek=s+x8")), "d=•••"},
		{[]string{"s x8"}, "d=" + b64([]byte("k=s+x8\xff\xfe")), "d=•••"},
		{[]string{"s x8"}, "<a href=\"/x?k=s+x8\">caf\xe9</a>", "<a href=\"/x?k=•••\">caf\xe9</a>"},
	}
	for _, tt := range tests {
		if got := New(tt.secs...).Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) with %q = %q, want %q", tt.in, tt.secs, got, tt.want)
		}
	}
}

func TestTextBudget(t *testing.T) {
	const unit = `%2525&amp;amp;\\\\+%5C `
	dense := strings.Repeat(unit, 1<<11)
	b64 := base64.StdEncoding.EncodeToString
	run := b64([]byte(strings.Repeat(unit, 256))) + " "
	page := `<a href="/q?u=a%2520b&amp;c=1">x</a> <script>var s="{\"a\":\"b\\\"c+d\"}"</script> `
	page = strings.Repeat(page+strings.Repeat("plain log line with words\n", 20), 40)
	loc := html.EscapeString(string(goJSON(strings.Repeat(localeUnit, 64<<10/len(localeUnit)), true)))
	near := `%25252541` + strings.Repeat("YWJj", 8) + `+&amp; plain log words here and more of them. `
	near = strings.Repeat(near, 16<<10/len(near))
	tests := []struct {
		sec  string
		in   string
		want string
	}{
		{"a%41&b", "plain text here " + dense + "a%41&amp;b " + dense, "plain text •••"},
		{
			"s3cr3tpw77",
			"plain " + b64([]byte(dense)) + " " + b64([]byte("s3cr3tpw77")) + " plain",
			"plain ••• ••• plain",
		},
		{"s3cr3tpw77", strings.Repeat(run, 4) + "plain", strings.Repeat(run, 3) + "••• plain"},
		{"a%41&b", page + "a%41&amp;b", page + Mask},
		{"s3cr3tpw77", loc + " k=s3cr3tpw77", loc + " k=" + Mask},
		{"s3cr3t-pw-77-x16", near + "k=s3cr3t-pw-77-x16", near + "k=" + Mask},
		{"s3cr3tpw77", near + "k=s3cr3tpw77", Mask},
	}
	for _, tt := range tests {
		if got := New(tt.sec).Text(tt.in); got != tt.want {
			t.Errorf("Text of %d bytes with %q = %d bytes ending %q, want %d bytes",
				len(tt.in), tt.sec, len(got), got[max(0, len(got)-40):], len(tt.want))
		}
	}
}

func TestTextSnippetAllocs(t *testing.T) {
	s := New("s3cr3tpw77")
	for _, snip := range snippets {
		in := withSnippet(snip)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		s.Text(in)
		runtime.ReadMemStats(&after)
		if n := after.TotalAlloc - before.TotalAlloc; n > uint64(len(in)) {
			t.Errorf("Text of %d bytes with %q allocates %d bytes, want at most %d", len(in), snip, n, len(in))
		}
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
			slices.ContainsFunc(s.needles, func(n needle) bool { return strings.Contains(v, n.v) }) {
			return
		}
		short := len(slices.MinFunc(s.needles, func(a, b needle) int { return len(a.v) - len(b.v) }).v)
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
		in := prefix + ". " + enc + " ." + suffix
		if got := New(sec).Text(in); strings.Contains(got, " "+enc+" ") {
			t.Fatalf("Text(%q) = %q keeps %q, the secret %q", in, got, enc, sec)
		}
	})
}

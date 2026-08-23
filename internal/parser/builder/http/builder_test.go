package http

import (
	"errors"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/http/version"
)

func TestParseRequestLine(t *testing.T) {
	cases := []struct {
		line   string
		method string
		url    string
		ver    version.HTTP
	}{
		{line: "GET http://example.com", method: "GET", url: "http://example.com"},
		{line: "CONNECT proxy.example.com:443", method: "CONNECT", url: "proxy.example.com:443"},
		{line: "WS ws://example.com/s", method: "GET", url: "ws://example.com/s"},
		{line: "GET {{base}}/two", method: "GET", url: "{{base}}/two"},
		{line: `"https://example.com/q"`, method: "GET", url: "https://example.com/q"},
		{line: `'https://example.com/q'`, method: "GET", url: "https://example.com/q"},
		{line: `https://example.com/q'`, method: "GET", url: `https://example.com/q'`},
		{line: `https://example.com/q"`, method: "GET", url: `https://example.com/q"`},
		{line: `"https://example.com/q" HTTP/1.1`, method: "GET", url: "https://example.com/q", ver: version.V11},
		{line: "ws://example.com/s b", method: "GET", url: "ws://example.com/s b"},
		{line: "http://example.com/a http/foo", method: "GET", url: "http://example.com/a http/foo"},
		{line: "http://example.com HTTP/1.1", method: "GET", url: "http://example.com", ver: version.V11},
		{line: "GET http://example.com HTTP/2", method: "GET", url: "http://example.com", ver: version.V2},
	}
	for _, tc := range cases {
		ml, ok, err := ParseRequestLine(tc.line)
		if err != nil {
			t.Fatalf("ParseRequestLine(%q): %v", tc.line, err)
		}
		if !ok {
			t.Fatalf("ParseRequestLine(%q) recognized no request", tc.line)
		}
		if ml.Method != tc.method || ml.URL != tc.url || ml.Version != tc.ver {
			t.Fatalf("ParseRequestLine(%q) = %s %q %v, want %s %q %v",
				tc.line, ml.Method, ml.URL, ml.Version, tc.method, tc.url, tc.ver)
		}
	}
}

func TestParseRequestLineIgnoresNonRequests(t *testing.T) {
	for _, line := range []string{"example.com", "we dropped HTTP/1.0", "{{base}}/two"} {
		ml, ok, err := ParseRequestLine(line)
		if err != nil {
			t.Fatalf("ParseRequestLine(%q): %v", line, err)
		}
		if ok {
			t.Fatalf("ParseRequestLine(%q) built %s %q", line, ml.Method, ml.URL)
		}
	}
}

func TestParseRequestLineRejectsUnsupportedVersion(t *testing.T) {
	for _, line := range []string{
		"GET {{base}}/two HTTP/1.0",
		"http://example.com HTTP/3",
	} {
		_, ok, err := ParseRequestLine(line)
		var unsupported *version.UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("ParseRequestLine(%q) error = %v, want *version.UnsupportedError", line, err)
		}
		if ok {
			t.Fatalf("ParseRequestLine(%q) recognized a request", line)
		}
	}
}

func TestParseWebSocketURLLine(t *testing.T) {
	for _, tc := range []struct {
		line string
		url  string
		ver  version.HTTP
	}{
		{line: "wss://example.com/chat HTTP/2", url: "wss://example.com/chat", ver: version.V2},
		{line: `"wss://example.com/chat"`, url: "wss://example.com/chat"},
		{line: `wss://example.com/chat'`, url: `wss://example.com/chat'`},
	} {
		ml, ok, err := ParseWebSocketURLLine(tc.line)
		if err != nil || !ok || ml.URL != tc.url || ml.Version != tc.ver {
			t.Fatalf("ParseWebSocketURLLine(%q) = %+v, %v, %v", tc.line, ml, ok, err)
		}
	}
	if _, ok, err := ParseWebSocketURLLine("http://example.com"); ok || err != nil {
		t.Fatalf("ParseWebSocketURLLine(http) = ok %v, err %v", ok, err)
	}
}

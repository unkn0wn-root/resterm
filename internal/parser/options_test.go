package parser

import (
	"maps"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Typos used to vanish without a word. Warning and not error, so the rest of
// the directive still applies.
func TestUnknownOptionsWarnWithoutDiscardingTheDirective(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "ssh",
			src:  "### r\n# @ssh host=jump userr=bob\nGET http://x\n",
			want: `unknown @ssh option "userr"`,
		},
		{
			name: "k8s",
			src:  "### r\n# @k8s target=pod/api port=8080 namespac=dev\nGET http://x\n",
			want: `unknown @k8s option "namespac"`,
		},
		{
			name: "sse",
			src:  "### r\n# @sse max-event=5\nGET http://x\n",
			want: `unknown @sse option "max-event"`,
		},
		{
			name: "websocket",
			src:  "### r\n# @websocket compresion=true\nWS wss://x\n",
			want: `unknown @websocket option "compresion"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("t.http", []byte(tt.src))
			if len(doc.Errors) != 0 {
				t.Fatalf("expected no errors, got %v", doc.Errors)
			}
			if !hasParseMessage(doc.Warnings, tt.want) {
				t.Fatalf("expected warning %q, got %v", tt.want, doc.Warnings)
			}
			if len(doc.Requests) != 1 {
				t.Fatalf("request was discarded, got %d requests", len(doc.Requests))
			}
		})
	}
}

func TestOptionAliasesConflictWithoutBeingUnknown(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "ssh known hosts",
			src:  "### r\n# @ssh host=h known-hosts=a known_hosts=b\nGET http://x\n",
			want: `@ssh options "known-hosts", "known_hosts" are the same option`,
		},
		{
			name: "ssh strict",
			src:  "### r\n# @ssh host=h strict_hostkey=yes strict-hostkey=no\nGET http://x\n",
			want: `@ssh options "strict-hostkey", "strict_hostkey" are the same option`,
		},
		{
			name: "ssh strict with an empty spelling",
			src:  "### r\n# @ssh host=h strict_hostkey= strict-hostkey=false\nGET http://x\n",
			want: `@ssh options "strict-hostkey", "strict_hostkey" are the same option`,
		},
		{
			name: "k8s local port",
			src:  "### r\n# @k8s target=pod/api port=1 local_port=2 local-port=3\nGET http://x\n",
			want: `@k8s options "local-port", "local_port" are the same option`,
		},
		{
			name: "sse idle",
			src:  "### r\n# @sse idle=1s idle-timeout=2s\nGET http://x\n",
			want: `@sse options "idle", "idle-timeout" are the same option`,
		},
		{
			name: "every spelling of the group is named",
			src:  "### r\n# @k8s target=pod/api port=1 local_port=2 local-port=3 localport=4\nGET http://x\n",
			want: `@k8s options "local-port", "local_port", "localport" are the same option`,
		},
		{
			name: "workflow step run and using",
			src:  "# @workflow f\n# @step one using=A run=B\n",
			want: `@step options "run", "using" are the same option`,
		},
		{
			name: "workflow branch run and using",
			src:  "# @workflow f\n# @if last.status == 200 run=A using=B\n# @endif\n",
			want: `@if options "run", "using" are the same option`,
		},
		{
			name: "workflow on-failure",
			src:  "# @workflow f on-failure=stop onfailure=continue\n# @step one using=A\n",
			want: `@workflow options "on-failure", "onfailure" are the same option`,
		},
		{
			name: "k8s target kind",
			src:  "### r\n# @k8s service=api svc=api-canary port=8080\nGET http://x\n",
			want: `@k8s options "service", "svc" are the same option`,
		},
		{
			name: "script lang and language",
			src:  "### r\nGET http://x\n# @script test lang=js language=rts\n> tests.assert(true)\n",
			want: `@script options "lang", "language" are the same option`,
		},
		{
			name: "rts lang and language",
			src:  "### r\n# @rts pre-request lang=rts language=rts\nGET http://x\n",
			want: `@rts options "lang", "language" are the same option`,
		},
		{
			name: "compare baseline",
			src:  "### r\n# @compare dev stage base=dev baseline=stage\nGET http://x\n",
			want: `@compare options "base", "baseline" are the same option`,
		},
		{
			name: "one spelling is not a conflict",
			src:  "### r\n# @k8s target=pod/api port=1 kube-context=c\nGET http://x\n",
		},
		{
			name: "a spelling without a value is not a conflict",
			src:  "### r\n# @ssh host=h known-hosts=a known_hosts=\nGET http://x\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("t.http", []byte(tt.src))
			if hasParseMessage(doc.Warnings, "unknown") {
				t.Fatalf("alias reported as unknown: %v", doc.Warnings)
			}
			if tt.want == "" {
				if len(doc.Errors) != 0 {
					t.Fatalf("expected no errors, got %v", doc.Errors)
				}
				return
			}
			if !hasParseMessage(doc.Errors, tt.want) {
				t.Fatalf("errors = %v, want %q", doc.Errors, tt.want)
			}
		})
	}
}

// yes/no/on/off work everywhere else, so they have to work here too.
func TestWebSocketCompressionAcceptsEveryBooleanSpelling(t *testing.T) {
	on := []string{"true", "yes", "on", "1", "t"}
	off := []string{"false", "no", "off", "0", "f"}

	check := func(t *testing.T, value string, want bool) {
		t.Helper()
		src := "### r\n# @websocket compression=" + value + "\nWS wss://x\n"
		doc := Parse("t.http", []byte(src))
		if len(doc.Errors) != 0 {
			t.Fatalf("compression=%s: %v", value, doc.Errors)
		}
		req := firstRequest(t, doc)
		if req.WebSocket == nil {
			t.Fatalf("compression=%s: no websocket request", value)
		}
		got, ok := req.WebSocket.Options.Compression.Get()
		if !ok {
			t.Fatalf("compression=%s: option not recorded", value)
		}
		if got != want {
			t.Fatalf("compression=%s = %t, want %t", value, got, want)
		}
	}

	for _, value := range on {
		check(t, value, true)
	}
	for _, value := range off {
		check(t, value, false)
	}

	doc := Parse("t.http", []byte("### r\n# @websocket compression=maybe\nWS wss://x\n"))
	if !hasParseMessage(doc.Errors, "expected true or false") {
		t.Fatalf("expected an error for a non-boolean, got %v", doc.Errors)
	}
}

func firstRequest(t *testing.T, doc *restfile.Document) *restfile.Request {
	t.Helper()
	if len(doc.Requests) == 0 {
		t.Fatal("no requests parsed")
	}
	return doc.Requests[0]
}

// @setting and @settings have to agree on a key written without a value. Only
// the parser can tell a flag apart from a value that was written empty.
func TestBareSettingKeyIsAFlag(t *testing.T) {
	tests := []struct {
		name string
		src  string
		key  string
		want string
	}{
		{
			name: "setting flag",
			src:  "### r\nGET http://x\n# @setting insecure\n",
			key:  "insecure",
			want: "true",
		},
		{
			name: "settings flag",
			src:  "### r\nGET http://x\n# @settings insecure\n",
			key:  "insecure",
			want: "true",
		},
		{
			name: "setting value still wins",
			src:  "### r\nGET http://x\n# @setting insecure false\n",
			key:  "insecure",
			want: "false",
		},
		{
			name: "flag on an unknown key",
			src:  "### r\nGET http://x\n# @setting feature.flag\n",
			key:  "feature.flag",
			want: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("t.http", []byte(tt.src))
			if len(doc.Errors) != 0 {
				t.Fatalf("expected no errors, got %v", doc.Errors)
			}
			req := firstRequest(t, doc)
			if got := req.Settings[tt.key]; got != tt.want {
				t.Fatalf("settings[%q] = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// Writing @setting with an equals sign is the @settings spelling, and it used
// to store a key with the equals sign in it that nothing would ever look up.
func TestSettingAcceptsTheEqualsSpelling(t *testing.T) {
	tests := []struct {
		name string
		src  string
		key  string
		want string
	}{
		{
			name: "boolean",
			src:  "### r\nGET http://x\n# @setting insecure=false\n",
			key:  "insecure",
			want: "false",
		},
		{
			name: "duration",
			src:  "### r\nGET http://x\n# @setting timeout=5s\n",
			key:  "timeout",
			want: "5s",
		},
		{
			name: "written empty stays empty",
			src:  "### r\nGET http://x\n# @setting insecure=\n",
			key:  "insecure",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("t.http", []byte(tt.src))
			if len(doc.Errors) != 0 {
				t.Fatalf("expected no errors, got %v", doc.Errors)
			}
			req := firstRequest(t, doc)
			if got, ok := req.Settings[tt.key]; !ok || got != tt.want {
				t.Fatalf("settings[%q] = %q (present %t), want %q", tt.key, got, ok, tt.want)
			}
		})
	}

	t.Run("file level", func(t *testing.T) {
		doc := Parse("t.http", []byte("# @setting insecure=false\n\n### r\nGET http://x\n"))
		if len(doc.Errors) != 0 {
			t.Fatalf("expected no errors, got %v", doc.Errors)
		}
		if got, ok := doc.Settings["insecure"]; !ok || got != "false" {
			t.Fatalf("file settings[insecure] = %q (present %t), want %q", got, ok, "false")
		}
	})
}

// @settings accepts arbitrary keys, so quoted options need an explicit warning.
func TestQuotedSettingWarns(t *testing.T) {
	doc := Parse("/ws/api.http", []byte("# @settings \"timeout=1s\" retries=2\nGET https://example.com\n"))
	want := `@settings option "timeout=1s" is quoted. Write it as timeout=1s`
	if len(doc.Errors) != 0 || len(doc.Warnings) != 1 || doc.Warnings[0].Message != want {
		t.Fatalf("errors = %v, warnings = %v, want one warning %q", doc.Errors, doc.Warnings, want)
	}
	if want := map[string]string{"retries": "2"}; !maps.Equal(doc.Settings, want) {
		t.Fatalf("settings = %v, want %v", doc.Settings, want)
	}
}

// A value written as empty is not a flag. These are the spellings that still
// reach the appliers with nothing in them, which is what they report as missing.
func TestWrittenEmptySettingValueStaysEmpty(t *testing.T) {
	tests := []struct {
		name string
		src  string
		key  string
	}{
		{
			name: "settings trailing equals",
			src:  "### r\nGET http://x\n# @settings insecure=\n",
			key:  "insecure",
		},
		{
			name: "bare timeout directive",
			src:  "### r\nGET http://x\n# @timeout\n",
			key:  "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("t.http", []byte(tt.src))
			req := firstRequest(t, doc)
			if got, ok := req.Settings[tt.key]; !ok || got != "" {
				t.Fatalf("settings[%q] = %q (present %t), want an empty value", tt.key, got, ok)
			}
		})
	}
}

func TestOptionWrittenWithLessOrEqualIsAnError(t *testing.T) {
	src := "# @settings timeout<=1s timeout=2s\n" +
		"# @ssh global jump host=h port<=22\n" +
		"# @k8s global k pod=p port=80 persist<=false\n" +
		"# @settings retries <= 3\n\n" +
		"### r\n# @sse timeout<=1s\nGET https://example.com\n"
	doc := Parse("/ws/api.http", []byte(src))
	lines := strings.Split(src, "\n")
	want := []struct {
		line int
		key  string
		msg  string
	}{
		{1, "timeout", `@settings option "timeout" takes = instead of <=. Write it as timeout=1s`},
		{2, "port", `@ssh option "port" takes = instead of <=. Write it as port=22`},
		{3, "persist", `@k8s option "persist" takes = instead of <=. Write it as persist=false`},
		{4, "retries", `@settings option "retries" has spaces around =. Write it as key=value`},
		{7, "timeout", `@sse option "timeout" takes = instead of <=. Write it as timeout=1s`},
	}
	if len(doc.Errors) != len(want) || len(doc.Warnings) != 0 {
		t.Fatalf("errors = %v, warnings = %v, want %d errors", doc.Errors, doc.Warnings, len(want))
	}
	for i, w := range want {
		got := doc.Errors[i]
		col := strings.Index(lines[w.line-1], w.key) + 1
		if got.Message != w.msg || got.Span.Start.Line != w.line || got.Span.Start.Col != col {
			t.Fatalf("error %d = %q at %d:%d, want %q at %d:%d",
				i, got.Message, got.Span.Start.Line, got.Span.Start.Col, w.msg, w.line, col)
		}
	}
	if want := map[string]string{"timeout": "2s"}; !maps.Equal(doc.Settings, want) {
		t.Fatalf("settings = %v, want %v", doc.Settings, want)
	}
	if len(doc.SSH) != 0 {
		t.Fatalf("ssh = %+v, want the profile rejected", doc.SSH)
	}
	if len(doc.K8s) != 1 || !doc.K8s[0].Invalid || doc.K8s[0].Error != want[2].msg {
		t.Fatalf("k8s = %+v, want the profile kept as invalid", doc.K8s)
	}
}

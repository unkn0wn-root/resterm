package directive

import (
	"errors"
	"maps"
	"testing"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr string
	}{
		{
			name:  "mixed",
			input: `enabled path="hello resterm" json={"id":1, "name":"test"}`,
			want: map[string]string{
				"enabled": "true",
				"path":    "hello resterm",
				"json":    `{"id":1, "name":"test"}`,
			},
		},
		{name: "bare key is true", input: "persist", want: map[string]string{"persist": "true"}},
		{name: "key is lowercased", input: "Persist=NO", want: map[string]string{"persist": "NO"}},
		{name: "empty value is kept", input: "key=", want: map[string]string{"key": ""}},
		{name: "value without key is dropped", input: "=orphan", want: map[string]string{}},
		{
			name:    "a repeat is reported",
			input:   "a=1 a=2",
			want:    map[string]string{"a": "2"},
			wantErr: `@mock option "a" is repeated`,
		},
		{
			name:    "repeats are named once each, in order",
			input:   "b=1 a=1 b=2 a=2 b=3",
			want:    map[string]string{"a": "2", "b": "3"},
			wantErr: `@mock options "a", "b" are repeated`,
		},
		{
			name:    "casing does not hide a repeat",
			input:   "Path=one path=two",
			want:    map[string]string{"path": "two"},
			wantErr: `@mock option "path" is repeated`,
		},
		{
			name:    "flag repeated after a value",
			input:   "persist=false persist",
			want:    map[string]string{"persist": "true"},
			wantErr: `@mock option "persist" is repeated`,
		},
		{
			name:  "a repeat inside an open bracket is not one",
			input: `json={"a":1 json=2`,
			want:  map[string]string{"json": `{"a":1 json=2`},
		},
		{name: "empty input", input: "   ", want: map[string]string{}},
		{
			name:  "quoted value keeps backslashes",
			input: `kubeconfig="C:\Users\me\.kube\config"`,
			want:  map[string]string{"kubeconfig": `C:\Users\me\.kube\config`},
		},
		{
			name:  "unquoted backslash escapes the next rune",
			input: `path=hello\ resterm`,
			want:  map[string]string{"path": "hello resterm"},
		},
		{
			name:  "escaped quote inside a quoted value",
			input: `desc="say \"hi\""`,
			want:  map[string]string{"desc": `say "hi"`},
		},
		{
			name:  "unterminated quote runs to the end",
			input: `desc="abc def`,
			want:  map[string]string{"desc": "abc def"},
		},
		{
			name:  "unbalanced bracket swallows the rest",
			input: `json={"a":1 next=2`,
			want:  map[string]string{"json": `{"a":1 next=2`},
		},
		{
			name:  "trailing backslash stays",
			input: `path=trailing\`,
			want:  map[string]string{"path": `trailing\`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseOptions(Mock, tt.input)
			if !maps.Equal(got.vals, tt.want) {
				t.Fatalf("ParseOptions(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("ParseOptions(%q) err = %v, want nil", tt.input, err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("ParseOptions(%q) err = nil, want %q", tt.input, tt.wantErr)
			case tt.wantErr != "" && err.Error() != tt.wantErr:
				t.Fatalf("ParseOptions(%q) err = %q, want %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

// A bare key is true for ParseOptions but is dropped here.
func TestOptionFields(t *testing.T) {
	t.Parallel()

	got, err := OptionFields(Auth, []string{"a=1", "bare", "", "=orphan", " B = 2 "})
	want := map[string]string{"a": "1", "b": "2"}
	if !maps.Equal(got.vals, want) {
		t.Fatalf("OptionFields() = %#v, want %#v", got, want)
	}
	if err != nil {
		t.Fatalf("OptionFields() err = %v, want nil", err)
	}

	got, err = OptionFields(Auth, []string{"a=1", "bare", "bare", "a=2"})
	if want := (map[string]string{"a": "2"}); !maps.Equal(got.vals, want) {
		t.Fatalf("OptionFields() = %#v, want %#v", got, want)
	}
	if err == nil || err.Error() != `@auth option "a" is repeated` {
		t.Fatalf(`OptionFields() err = %v, want @auth option "a" is repeated`, err)
	}
}

func TestOptionsGetAndFirst(t *testing.T) {
	t.Parallel()

	opts := testOptions(map[string]string{"host": " jump ", "empty": "", "port": "22"})

	if got := opts.Get("host"); got != "jump" {
		t.Fatalf("Get(host) = %q, want %q", got, "jump")
	}
	if got := opts.Get("missing"); got != "" {
		t.Fatalf("Get(missing) = %q, want empty", got)
	}
	// A key present with an empty value is skipped so the next alias wins.
	if got, ok := opts.First("empty", "port"); !ok || got != "22" {
		t.Fatalf("First(empty, port) = (%q, %t), want (%q, true)", got, ok, "22")
	}
	if _, ok := opts.First("missing"); ok {
		t.Fatal("First(missing) ok = true, want false")
	}
}

func TestOptionsPopKey(t *testing.T) {
	t.Parallel()

	opts := testOptions(map[string]string{"empty": "", "port": " 22 ", "local-port": "9"})
	key, val, ok := opts.PopKey("empty", "port")
	if !ok || key != "port" || val != "22" {
		t.Fatalf("PopKey() = (%q, %q, %t), want (port, 22, true)", key, val, ok)
	}
	// Both spellings go away so leftover validation does not flag the loser.
	if opts.Len() != 1 {
		t.Fatalf("PopKey() left %#v, want only local-port", opts.vals)
	}
	if _, _, ok := opts.PopKey("missing"); ok {
		t.Fatal("PopKey(missing) ok = true, want false")
	}
}

func TestOptionsPop(t *testing.T) {
	t.Parallel()

	opts := testOptions(map[string]string{"name": " step ", "using": "req", "run": "other"})

	if got := opts.Pop("name"); got != "step" {
		t.Fatalf("Pop(name) = %q, want %q", got, "step")
	}
	if opts.Has("name") {
		t.Fatal("Pop(name) left the key behind")
	}
	if got := opts.Pop("missing"); got != "" {
		t.Fatalf("Pop(missing) = %q, want empty", got)
	}

	// Both aliases go away so leftover validation does not flag the loser.
	if got, _ := opts.PopAny("using", "run"); got != "req" {
		t.Fatalf("PopAny() = %q, want %q", got, "req")
	}
	if opts.Len() != 0 {
		t.Fatalf("PopAny() left %#v", opts.vals)
	}
}

func TestOptionsZeroValue(t *testing.T) {
	t.Parallel()

	var opts Options
	if opts.Len() != 0 || opts.Has("x") || opts.Get("x") != "" {
		t.Fatal("zero Options is not empty")
	}
	if got := opts.Pop("x"); got != "" {
		t.Fatalf("Pop() on nil = %q, want empty", got)
	}
	if got, _ := opts.PopAny("x", "y"); got != "" {
		t.Fatalf("PopAny() on nil = %q, want empty", got)
	}
}

func TestOptionsPopBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts map[string]string
		want bool
		ok   bool
		bad  string
	}{
		{name: "absent", opts: map[string]string{}},
		{name: "true", opts: map[string]string{"persist": "true"}, want: true, ok: true},
		{name: "off", opts: map[string]string{"persist": "off"}, ok: true},
		{name: "bare key", opts: map[string]string{"persist": "true"}, want: true, ok: true},
		{name: "empty value", opts: map[string]string{"persist": ""}, want: true, ok: true},
		{name: "blank value", opts: map[string]string{"persist": "  "}, want: true, ok: true},
		{
			name: "typo reads as true but is reported",
			opts: map[string]string{"persist": "flase"},
			want: true,
			ok:   true,
			bad:  "flase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, bad := testOptions(tt.opts).PopBool("persist")
			if got != tt.want || ok != tt.ok || bad != tt.bad {
				t.Fatalf("PopBool() = (%t, %t, %q), want (%t, %t, %q)",
					got, ok, bad, tt.want, tt.ok, tt.bad)
			}
		})
	}
}

func TestUnknownOptions(t *testing.T) {
	t.Parallel()

	if err := UnknownOption(SSH); err != nil {
		t.Fatalf("UnknownOption() with no keys = %v, want nil", err)
	}
	if err := newOptions(0).Unknown(SSH); err != nil {
		t.Fatalf("empty Unknown() = %v, want nil", err)
	}
	if err := (Options{}).Unknown(SSH); err != nil {
		t.Fatalf("zero Unknown() = %v, want nil", err)
	}

	one := UnknownOption(SSH, "userr")
	if got, want := one.Error(), `unknown @ssh option "userr"`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	// Keys come back sorted so the message does not depend on map order.
	many := testOptions(map[string]string{"zeta": "1", "alpha": "2"}).Unknown(K8s)
	if got, want := many.Error(), `unknown @k8s options "alpha", "zeta"`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	var target *UnknownOptionsError
	if !errors.As(many, &target) {
		t.Fatal("errors.As did not match UnknownOptionsError")
	}
	if target.Directive != K8s {
		t.Fatalf("Directive = %q, want %q", target.Directive, K8s)
	}
}

func TestPopBoolConflictsOnEveryWrittenSpelling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts map[string]string
		want string
	}{
		{
			name: "empty spelling beside an explicit value",
			opts: map[string]string{"strict_hostkey": "", "strict-hostkey": "false"},
			want: `@ssh options "strict-hostkey", "strict_hostkey" are the same option`,
		},
		{
			name: "both spellings empty",
			opts: map[string]string{"strict_hostkey": "", "strict-hostkey": ""},
			want: `@ssh options "strict-hostkey", "strict_hostkey" are the same option`,
		},
		{
			name: "both spellings valued",
			opts: map[string]string{"strict_hostkey": "yes", "strict-hostkey": "no"},
			want: `@ssh options "strict-hostkey", "strict_hostkey" are the same option`,
		},
		{
			name: "one spelling on its own",
			opts: map[string]string{"strict-hostkey": "false"},
		},
		{
			name: "one spelling written empty",
			opts: map[string]string{"strict_hostkey": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := testOptions(tt.opts)
			opts.PopBool("strict_hostkey", "strict-hostkey", "strict_host_key")

			err := opts.Conflicts(SSH)
			switch {
			case tt.want == "":
				if err != nil {
					t.Fatalf("Conflicts() = %v, want nil", err)
				}
			case err == nil:
				t.Fatalf("Conflicts() = nil, want %q", tt.want)
			case err.Error() != tt.want:
				t.Fatalf("Conflicts() = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestPopAnySkipsEmptySpellingWithoutConflict(t *testing.T) {
	t.Parallel()

	opts := testOptions(map[string]string{"known_hosts": "", "known-hosts": "/tmp/kh"})
	got, ok := opts.PopAny("known_hosts", "known-hosts")
	if !ok || got != "/tmp/kh" {
		t.Fatalf("PopAny() = (%q, %t), want the spelling that carries a value", got, ok)
	}
	if err := opts.Conflicts(SSH); err != nil {
		t.Fatalf("Conflicts() = %v, want nil", err)
	}
}

// Popping every spelling a directive knows leaves only the typos behind.
func TestUnknownAfterPopping(t *testing.T) {
	t.Parallel()

	opts, _ := ParseOptions(SSH, "host=jump known-hosts=a known_hosts=b agent=yes userr=bob")
	opts.PopAny("host")
	opts.PopAny("known_hosts", "known-hosts")
	opts.PopBool("agent")

	err := opts.Unknown(SSH)
	if err == nil {
		t.Fatal("Unknown() = nil, want the leftover key")
	}
	if got, want := err.Error(), `unknown @ssh option "userr"`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestParseProfileHeader(t *testing.T) {
	t.Parallel()

	head, ok, err := ParseProfileHeader(SSH, `file office timeout=5s host="hello resterm" persist`)
	if !ok {
		t.Fatal("ParseProfileHeader() ok = false")
	}
	if err != nil {
		t.Fatalf("ParseProfileHeader() err = %v, want nil", err)
	}
	if head.Scope != ScopeFile || head.Name != "office" {
		t.Fatalf("ParseProfileHeader() = (%v, %q), want (file, %q)", head.Scope, head.Name, "office")
	}
	want := map[string]string{
		"timeout": "5s",
		"host":    "hello resterm",
		"persist": "true",
	}
	if !maps.Equal(head.Options.vals, want) {
		t.Fatalf("options = %#v, want %#v", head.Options, want)
	}

	// A leading option means the name was omitted.
	head, ok, _ = ParseProfileHeader(SSH, `host=jump`)
	if !ok || head.Scope != ScopeRequest || head.Name != "" {
		t.Fatalf("ParseProfileHeader() = (%v, %q, %t), want (request, empty, true)",
			head.Scope, head.Name, ok)
	}

	if _, ok, _ := ParseProfileHeader(SSH, "   "); ok {
		t.Fatal("ParseProfileHeader(blank) ok = true, want false")
	}

	_, _, err = ParseProfileHeader(SSH, `file host host=jump`)
	if err != nil {
		t.Fatalf("ParseProfileHeader() err = %v, want nil", err)
	}
	_, _, err = ParseProfileHeader(SSH, `file office host=a host=b`)
	if err == nil || err.Error() != `@ssh option "host" is repeated` {
		t.Fatalf(`ParseProfileHeader() err = %v, want @ssh option "host" is repeated`, err)
	}
}

func TestParseNameValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		key   string
		value string
	}{
		{name: "space", input: "token value", key: "token", value: "value"},
		{name: "equals", input: "token = value", key: "token", value: "value"},
		{name: "colon", input: "token: value", key: "token", value: "value"},
		{name: "no separator", input: "token", key: "token"},
		{name: "colon without value", input: "token:", key: "token"},
		{name: "dotted name", input: "a.b:c", key: "a.b", value: "c"},
		{name: "value keeps later separators", input: "tok = = v", key: "tok", value: "= v"},
		{name: "invalid rune in name", input: "X-Invalid/Name: v"},
		{name: "empty"},
		{name: "blank", input: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, val := ParseNameValue(tt.input)
			if key != tt.key || val != tt.value {
				t.Fatalf("ParseNameValue(%q) = (%q, %q), want (%q, %q)",
					tt.input, key, val, tt.key, tt.value)
			}
		})
	}
}

func TestIsOption(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"host=jump":            true,
		"local-port=1":         true,
		"bare":                 false,
		"=orphan":              false,
		"has space=1":          false,
		"expect.status":        false,
		"last.statusCode==200": false,
		"a!=b":                 false,
		"a>=b":                 false,
	}
	for input, want := range tests {
		if got := isOption(input); got != want {
			t.Fatalf("isOption(%q) = %t, want %t", input, got, want)
		}
	}
}

func TestFieldSpans(t *testing.T) {
	t.Parallel()

	input := `"file edge" host=jump json={"a":"b c"} last==200 timeout="5 s" path=a\ b persist`
	want := []struct {
		field string
		key   string
	}{
		{field: `"file edge"`},
		{field: "host=jump", key: "host"},
		{field: `json={"a":"b c"}`, key: "json"},
		{field: "last==200"},
		{field: `timeout="5 s"`, key: "timeout"},
		{field: `path=a\ b`, key: "path"},
		{field: "persist"},
	}

	spans := FieldSpans(input)
	if len(spans) != len(want) {
		t.Fatalf("FieldSpans(%q) returned %d spans, want %d", input, len(spans), len(want))
	}
	for i, w := range want {
		f := spans[i]
		if got := input[f.Start:f.End]; got != w.field {
			t.Fatalf("span %d = %q, want %q", i, got, w.field)
		}
		if w.key == "" {
			if f.Eq >= 0 {
				t.Fatalf("span %d (%q) has Eq %d, want positional", i, w.field, f.Eq)
			}
			continue
		}
		if f.Eq < 0 || input[f.Start:f.Eq] != w.key {
			t.Fatalf("span %d (%q) Eq = %d, want key %q", i, w.field, f.Eq, w.key)
		}
	}
}

// The lexer already decoded the value. A second strip used to take a layer off
// anything that was itself a quoted string.
func TestParseOptionsKeepsDecodedValues(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		`fail="\"hi\""`: `"hi"`,
		`fail="=err"`:   "=err",
		`fail="a=b"`:    "a=b",
		"fail=\"a\tb\"": "a\tb",
		`fail=plain`:    "plain",
	}
	for in, want := range tests {
		opts, _ := ParseOptions(Step, in)
		if got := opts.Get("fail"); got != want {
			t.Fatalf("ParseOptions(%q)[fail] = %q, want %q", in, got, want)
		}
	}
}

func TestOptionsOpen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  rune
	}{
		{name: "empty"},
		{name: "closed object", input: `json={"a":1}`},
		{name: "closed array", input: `json=[1,2]`},
		{name: "open object", input: `json={`, want: '}'},
		{name: "open partway", input: `json={"a": {"gt": 1},`, want: '}'},
		{name: "open array", input: `json=[1,`, want: ']'},
		{name: "closed after an open one", input: `json={ headers={"X":"y"}`, want: '}'},
		{name: "open after a closed one", input: `query={"a":"b"} json={`, want: '}'},
		{name: "open behind a closed one", input: `json={ } headers={"X":"y"}`},
		{name: "bracket inside a string", input: `json={"a":"{"}`},
		{name: "escaped quote inside a string", input: `json={"a":"\""}`},
		{name: "unclosed quoted value", input: `regex="^v`, want: '"'},
		{name: "closed quoted value", input: `regex="^v[0-9]+$"`},
		{name: "stray closer", input: `json=}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OptionsOpen(tt.input); got != tt.want {
				t.Fatalf("OptionsOpen(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOptionsScannerMatchesOptionsOpenAtEveryChunkBoundary(t *testing.T) {
	inputs := []string{
		`json={"a":{"b":[1, 2]}} regex="^v[0-9]+$"`,
		`json={"a":"\\\"}"}`,
		`regex="first\nsecond"`,
		"one=true\u2003json={\"a\": 1}",
		`latency=random(1s, 2s)`,
		`json={ headers={"X":"y"}`,
		`regex="unfinished\\`,
		`json={"a": 1}	"`,
		"\"\xe2\x80",
	}

	for _, input := range inputs {
		for cut := range len(input) + 1 {
			var scan OptionsScanner
			scan.Feed(input[:cut])
			scan.Feed(input[cut:])
			if got, want := scan.Closer(), OptionsOpen(input); got != want {
				t.Fatalf("chunks [%q, %q] closed with %q, whole read gives %q", input[:cut], input[cut:], got, want)
			}
		}
	}
}

// Options only ever fill through put, so tests see what a directive would store.
func testOptions(vals map[string]string) Options {
	opts := newOptions(len(vals))
	for key, val := range vals {
		opts.put(key, val)
	}
	return opts
}

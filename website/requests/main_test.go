package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/unkn0wn-root/resterm/internal/redact"
)

const aPath = "_examples/a.http"

func TestBuild(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  map[string][]entry
	}{
		{
			name:  "named request at its first line",
			files: map[string]string{aPath: "### A\n# @name getUser\nGET /u"},
			want:  map[string][]entry{aPath: {{2, "GET", "getUser"}}},
		},
		{
			name:  "unnamed request by target",
			files: map[string]string{aPath: "###\nGET https://x/y"},
			want:  map[string][]entry{aPath: {{2, "GET", "https://x/y"}}},
		},
		{
			name:  "separator text is not a name",
			files: map[string]string{aPath: "### Status Check\nGET /s"},
			want:  map[string][]entry{aPath: {{2, "GET", "/s"}}},
		},
		{
			name:  "start line skips plain comments",
			files: map[string]string{aPath: "### A\n# note\n# @k8s use=x\nGET http://h/"},
			want:  map[string][]entry{aPath: {{3, "GET", "http://h/"}}},
		},
		{
			name:  "block without a request is skipped",
			files: map[string]string{aPath: "### D\n# @auth file bearer t\n\n### R\nGET /a"},
			want:  map[string][]entry{aPath: {{5, "GET", "/a"}}},
		},
		{
			name:  "mocks are skipped",
			files: map[string]string{aPath: "### Reply\n# @mock method=GET path=/p\nHTTP/1.1 200 OK\n\n{}"},
			want:  map[string][]entry{aPath: {}},
		},
		{
			name:  "workflows come after requests",
			files: map[string]string{aPath: "# @workflow flow\n# @step S using=a\n\n### A\n# @name a\nGET /a"},
			want:  map[string][]entry{aPath: {{5, "GET", "a"}, {1, "WF", "flow"}}},
		},
		{
			name: "only request files, no dotfiles",
			files: map[string]string{
				aPath:                     "GET /a\n",
				"_examples/b.rts":         "fn f() {}\n",
				"_examples/env.json":      "{}\n",
				"_examples/.#a.http":      "GET /lock\n",
				"_examples/.cache/c.http": "GET /c\n",
				"_examples/sub/c.rest":    "GET /c\n",
			},
			want: map[string][]entry{aPath: {{1, "GET", "/a"}}, "_examples/sub/c.rest": {{1, "GET", "/c"}}},
		},
		{
			name: "declared secrets are masked",
			files: map[string]string{
				aPath: "@file-secret api.key k3y-7f2a9c\n\n### A\n# @name ping k3y-7f2a9c\nGET /u",
			},
			want: map[string][]entry{aPath: {{4, "GET", "ping " + redact.Mask}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for p, src := range tt.files {
				fsys[p] = &fstest.MapFile{Data: []byte(src)}
			}
			want := map[string]file{}
			for p, reqs := range tt.want {
				sum := sha256.Sum256([]byte(tt.files[p]))
				want[p] = file{SHA256: hex.EncodeToString(sum[:]), Requests: reqs}
			}

			js, err := build(fsys)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]file
			if err := json.Unmarshal(js, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("build() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestBuildOutput(t *testing.T) {
	const src = "GET https://x/y?a=1&b=<c>\n"
	js, err := build(fstest.MapFS{aPath: {Data: []byte(src)}})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{
  "_examples/a.http": {
    "sha256": "%x",
    "requests": [
      {
        "line": 1,
        "method": "GET",
        "name": "https://x/y?a=1&b=<c>"
      }
    ]
  }
}
`, sha256.Sum256([]byte(src)))
	if string(js) != want {
		t.Errorf("build() =\n%s\nwant\n%s", js, want)
	}
}

func TestRequestsJSONIsFresh(t *testing.T) {
	want, err := build(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../src/content/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("website/src/content/requests.json is stale. Run make website-requests.")
	}
}

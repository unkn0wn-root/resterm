package request

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRuntimeWriteClearsEnvRef(t *testing.T) {
	tests := []struct {
		name  string
		write func(req *restfile.Request, value string)
	}{
		{
			name: "script and @apply",
			write: func(req *restfile.Request, value string) {
				var set vars.NameMap[string]
				set.Set("token", value)
				prerequest.SetRequestVars(req, set)
			},
		},
		{
			name: "capture",
			write: func(req *restfile.Request, value string) {
				upsertVariable(&req.Variables, directive.ScopeRequest, "token", value, false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, value := range []string{"patched", "env:RESTERM_RUNTIME_WRITE_OTHER"} {
				req := &restfile.Request{Variables: []restfile.Variable{{
					Name:     "token",
					Value:    "env:RESTERM_RUNTIME_WRITE",
					Scope:    directive.ScopeRequest,
					Authored: true,
				}}}
				tt.write(req, value)

				got := req.Variables[0]
				if got.Value != value {
					t.Fatalf("Value = %q, want %q", got.Value, value)
				}
				if got.Authored {
					t.Fatal("the write kept the declaration flag, so it could still be read as a reference")
				}
			}
		})
	}
}

func TestApplyOverwritesEnvBackedRequestVar(t *testing.T) {
	t.Setenv("RESTERM_APPLY_ORIGINAL", "ORIGINAL")
	t.Setenv("RESTERM_APPLY_OTHER", "OTHER")

	for _, tt := range []struct {
		name  string
		patch string
		want  string
	}{
		{name: "ordinary value", patch: `"patched"`, want: "patched"},
		{
			name:  "value that looks like a reference",
			patch: `"env:RESTERM_APPLY_OTHER"`,
			want:  "env:RESTERM_APPLY_OTHER",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc, req := parseDoc(t, `### one
# @name one
# @request token env:RESTERM_APPLY_ORIGINAL
# @apply {vars: {token: `+tt.patch+`}}
GET http://example.test
X-Token: {{token}}
`)
			req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
				`request.setHeader("X-Vars", vars.require("token"))`,
			))

			sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
			for _, name := range []string{"X-Token", "X-Vars"} {
				if got := sent.wire.Header.Get(name); got != tt.want {
					t.Fatalf("%s = %q, want %q", name, got, tt.want)
				}
			}
		})
	}
}

func TestScriptOverwritesEnvBackedRequestVar(t *testing.T) {
	t.Setenv("RESTERM_SCRIPT_ORIGINAL", "ORIGINAL")

	doc, req := parseDoc(t, `### one
# @name one
# @request token env:RESTERM_SCRIPT_ORIGINAL
GET http://example.test
X-Token: {{token}}
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(`vars.set("token", "patched")`))

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Token"); got != "patched" {
		t.Fatalf("X-Token = %q, want the script write to win", got)
	}
}

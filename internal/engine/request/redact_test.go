package request

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestRedactResult(t *testing.T) {
	res := engine.RequestResult{
		Response:  &httpx.Response{Status: "200 " + leakedSecret, Body: []byte(`{"t":"` + leakedSecret + `"}`)},
		Err:       sourceOnlySecretErr(leakedSecret),
		ScriptErr: errors.New("script saw " + leakedSecret),
		Tests:     []scriptapi.TestResult{{Name: "has " + leakedSecret, Message: "got " + leakedSecret}},
		Stream: &scripts.StreamInfo{
			Kind:    "websocket",
			Summary: map[string]any{"closeReason": "bye " + leakedSecret, "closeCode": 1000},
			Err:     errors.New("closed: " + leakedSecret),
		},
		Warnings:   []string{"Script sends {{" + leakedSecret + "}} in header X as written."},
		SkipReason: `@when evaluated to false: vars.token == "` + leakedSecret + `"`,
	}
	dump := func(r engine.RequestResult) string {
		return fmt.Sprintf("%v %v %v %v %q %v %v %s", r.Tests, *r.Stream, r.Stream.Summary, r.Warnings, r.SkipReason,
			r.Err, r.Response.Status, r.Response.Body)
	}
	before := dump(res)
	plain := redactResult(res)
	if plain.Err != res.Err || plain.ScriptErr != res.ScriptErr || dump(plain) != before {
		t.Errorf("redactResult without secrets changed the result:\n%s\n%s", before, dump(plain))
	}

	res.RuntimeSecrets = []string{leakedSecret}
	out := redactResult(res)
	res.RuntimeSecrets = nil
	if dump(res) != before {
		t.Fatalf("redactResult changed its input:\n%s\n%s", before, dump(res))
	}
	shown := map[string]string{
		"Err":              errText(out.Err),
		"Err report":       errRender(out.Err),
		"ScriptErr":        errText(out.ScriptErr),
		"ScriptErr report": errRender(out.ScriptErr),
		"stream error":     errText(out.Stream.Err),
		"tests":            fmt.Sprint(out.Tests),
		"warnings":         strings.Join(out.Warnings, "\n"),
		"skip reason":      out.SkipReason,
	}
	for field, text := range shown {
		if strings.Contains(text, leakedSecret) || field != "Err" && !strings.Contains(text, "•••") {
			t.Errorf("%s = %q, want the secret masked", field, text)
		}
	}
	if !errors.Is(out.Stream.Err, res.Stream.Err) || !errors.Is(out.ScriptErr, res.ScriptErr) {
		t.Errorf("masked errors %v and %v do not wrap the originals", out.Stream.Err, out.ScriptErr)
	}
	if out.Response != res.Response || string(out.Response.Body) != `{"t":"`+leakedSecret+`"}` {
		t.Errorf("response = %+v, want it raw", out.Response)
	}
	if got := out.Stream.Summary["closeReason"]; got != "bye "+leakedSecret {
		t.Errorf("stream summary closeReason = %v, want it raw", got)
	}
}

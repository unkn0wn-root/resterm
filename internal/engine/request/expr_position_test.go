package request

import (
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

// A failing expression keeps its script report and points at the expression
// inside the body, past the comment line the parser drops.
func TestExecuteReportsExpressionErrorAtItsPlaceholder(t *testing.T) {
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			t.Fatal("transport must not be built")
			return nil, nil
		}),
	)
	eng := New(engcfg.Config{Client: client, SourceDiagnostics: true}, nil)
	src := "### Users\n" +
		"POST https://example.com/users\n" +
		"\n" +
		"{\n" +
		"# note\n" +
		"  \"id\": \"{{= nosuch.value }}\"\n" +
		"}\n"
	doc := parser.Parse("requests.http", []byte(src))

	res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected an expression error")
	}

	got := diag.Render(res.Err)
	for _, want := range []string{
		"error[script]: ",
		"--> requests.http:6:14\n",
		"6 |   \"id\": \"{{= nosuch.value }}\"\n",
		"| " + strings.Repeat(" ", 13) + "^",
		"at requests.http:6:14 in {{= nosuch.value }}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
}

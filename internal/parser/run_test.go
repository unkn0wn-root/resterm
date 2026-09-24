package parser

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestParseRunVarsBelongToTheirOwner(t *testing.T) {
	src := `# @workflow order
# @run var suffix = {{$fake.word}}
# @step Create using=Create
# @run var region: eu

### Create
# @name Create
# @run var id {{$uuid}}
# @run var suffix = {{suffix}}-x
POST https://example.com/orders/{{id}}
`
	doc := Parse("order.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	if len(doc.Workflows) != 1 || len(doc.Requests) != 1 {
		t.Fatalf("expected 1 workflow and 1 request, got %d and %d", len(doc.Workflows), len(doc.Requests))
	}

	wf := []restfile.RunVar{
		{Name: "suffix", Value: "{{$fake.word}}", Line: 2, Col: 21},
		{Name: "region", Value: "eu", Line: 4, Col: 20},
	}
	if got := doc.Workflows[0].RunVars; !reflect.DeepEqual(got, wf) {
		t.Fatalf("workflow run vars = %+v, want %+v", got, wf)
	}
	req := []restfile.RunVar{
		{Name: "id", Value: "{{$uuid}}", Line: 8, Col: 15},
		{Name: "suffix", Value: "{{suffix}}-x", Line: 9, Col: 21},
	}
	if got := doc.Requests[0].RunVars; !reflect.DeepEqual(got, req) {
		t.Fatalf("request run vars = %+v, want %+v", got, req)
	}
	if len(doc.Requests[0].Variables) != 0 {
		t.Fatalf("@run var leaked into request variables: %+v", doc.Requests[0].Variables)
	}
}

func TestParseRunVarErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		msg  string
	}{
		{"no subcommand", "# @run\nGET https://example.com\n", "@run requires 'var <name> = <value>'"},
		{"unknown subcommand", "# @run secret x = 1\nGET https://example.com\n", `@run subcommand "secret" is unknown`},
		{"missing name", "# @run var\nGET https://example.com\n", "@run var name missing"},
		{"invalid name", "# @run var a$b = 1\nGET https://example.com\n", `@run var name "a$b" is invalid`},
		{"env ref", "# @run var t = env:TOKEN\nGET https://example.com\n", "@run var cannot read env: references"},
		{
			"request duplicate",
			"# @run var id = 1\n# @run var ID = 2\nGET https://example.com\n",
			`@run var "ID" is already declared`,
		},
		{
			"workflow duplicate",
			"# @workflow w\n# @run var id = 1\n# @step S using=S\n# @run var Id = 2\n\n### S\n# @name S\nGET https://example.com\n",
			`@run var "Id" is already declared`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse("run.http", []byte(tc.src))
			if !hasParseMessage(doc.Errors, tc.msg) {
				t.Fatalf("errors = %v, want one containing %q", doc.Errors, tc.msg)
			}
		})
	}
}

func TestParseRunVarDuplicateKeepsFirst(t *testing.T) {
	doc := Parse("run.http", []byte("# @run var id = 1\n# @run var id = 2\nGET https://example.com\n"))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	got := doc.Requests[0].RunVars
	if len(got) != 1 || got[0].Value != "1" {
		t.Fatalf("run vars = %+v, want only the first declaration", got)
	}
}

func TestParseRunVarSameNameInWorkflowAndRequest(t *testing.T) {
	src := `# @workflow w
# @run var id = 1
# @step S using=S

### S
# @name S
# @run var id = 2
GET https://example.com
`
	doc := Parse("run.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	if len(doc.Workflows[0].RunVars) != 1 || len(doc.Requests[0].RunVars) != 1 {
		t.Fatalf("expected one declaration on each owner")
	}
}

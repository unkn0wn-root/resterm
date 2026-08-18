package parser

import (
	"strings"
	"testing"
)

func TestParseMarksDeclarationsAuthored(t *testing.T) {
	src := `# @const c_tok env:CONST_TOKEN
# @global g_tok env:GLOBAL_TOKEN
# @file f_tok env:FILE_TOKEN
# @file-secret f_sec env:FILE_SECRET
# @file plain literal-value
# @file nested env:{{picked}}

### one
# @name one
# @request r_tok env:REQUEST_TOKEN
GET http://example.test
`
	doc := Parse("sample.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %#v, want none", doc.Errors)
	}

	if len(doc.Constants) != 1 || !doc.Constants[0].Authored {
		t.Fatalf("constants = %#v, want the constant marked authored", doc.Constants)
	}
	if len(doc.Globals) != 1 || !doc.Globals[0].Authored {
		t.Fatalf("globals = %#v, want the global marked authored", doc.Globals)
	}
	if got := len(doc.Variables); got != 4 {
		t.Fatalf("file variables = %d, want 4", got)
	}
	for _, v := range doc.Variables {
		if !v.Authored {
			t.Fatalf("file variable %q is not marked authored", v.Name)
		}
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.Requests))
	}
	rv := doc.Requests[0].Variables
	if len(rv) != 1 || !rv[0].Authored {
		t.Fatalf("request variables = %#v, want the variable marked authored", rv)
	}
}

func TestParseReportsEnvRefWithNoName(t *testing.T) {
	src := `# @file f_tok env:
# @const c_tok env:
# @global g_tok env:
# @file fine env:REAL_NAME

### one
# @name one
# @request r_tok env:
GET http://example.test
`
	doc := Parse("sample.http", []byte(src))
	if len(doc.Errors) != 4 {
		t.Fatalf("errors = %#v, want one per malformed declaration", doc.Errors)
	}
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "env: reference is missing a variable name") {
			t.Fatalf("error %d: %q, want the malformed reference named", e.Line, e.Message)
		}
		if e.Line == 0 {
			t.Fatalf("error %q has no line", e.Message)
		}
	}
}

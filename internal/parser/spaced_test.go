package parser

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestSpacedSettingsAreNotSet(t *testing.T) {
	src := "# @settings http-insecure = false timeout=5s\n\n" +
		"### r\n# @settings http-insecure =false\n# @settings followredirects= false\nGET https://example.com\n"
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 3 {
		t.Fatalf("errors = %v, want one per @settings line", doc.Errors)
	}
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "has spaces around =. Write it as key=value") {
			t.Fatalf("error = %q", e.Message)
		}
	}
	if want := map[string]string{"timeout": "5s"}; !maps.Equal(doc.Settings, want) {
		t.Fatalf("file settings = %v, want %v", doc.Settings, want)
	}
	if got := doc.Requests[0].Settings; len(got) != 0 {
		t.Fatalf("request settings = %v, want none", got)
	}
}

func TestSpacedOptionsNeverSetTheirKey(t *testing.T) {
	src := "### ws\n# @websocket compression = false\nGET ws://example.com/a\n\n" +
		"### ssh\n# @ssh host=h agent = false\nGET https://example.com/b\n\n" +
		"### ssh after\n# @ssh host=h agent= false\nGET https://example.com/c\n\n" +
		"### retry\n# @retry count=4 =x\nGET https://example.com/d\n"
	doc := Parse("/ws/api.http", []byte(src))

	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", doc.Warnings)
	}
	var lines []int
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "has spaces around =. Write it as key=value") {
			t.Fatalf("error = %q", e.Message)
		}
		lines = append(lines, e.Span.Start.Line)
	}
	if want := []int{2, 6, 10, 14}; !slices.Equal(lines, want) {
		t.Fatalf("error lines = %v, want %v", lines, want)
	}

	if ws := doc.Requests[0].WebSocket; ws == nil || ws.Options.Compression.Set {
		t.Fatalf("websocket = %+v, want compression unset", ws)
	}
	for _, i := range []int{1, 2} {
		if ssh := doc.Requests[i].SSH; ssh == nil || ssh.Inline == nil || ssh.Inline.Agent.Set {
			t.Fatalf("request %d ssh = %+v, want agent unset", i, ssh)
		}
	}
	if retry := doc.Requests[3].Metadata.Retry; retry == nil || retry.Count != 4 {
		t.Fatalf("retry = %+v, want it kept", retry)
	}
}

func TestSpacedSharedProfilesAreRejected(t *testing.T) {
	src := "# @ssh global jump host=h persist = false\n# @k8s global k pod=p port=80 persist = false\n"
	doc := Parse("/ws/defs.http", []byte(src))
	if len(doc.Errors) != 2 {
		t.Fatalf("errors = %v, want one per line", doc.Errors)
	}
	if len(doc.SSH) != 0 {
		t.Fatalf("ssh = %+v, want the profile rejected", doc.SSH)
	}
	if len(doc.K8s) != 1 || !doc.K8s[0].Invalid ||
		!strings.Contains(doc.K8s[0].Error, `"persist" has spaces around =`) {
		t.Fatalf("k8s = %+v, want the profile kept as invalid with the spacing error", doc.K8s)
	}
}

func TestSpacedWorkflowOptions(t *testing.T) {
	src := "# @workflow w region = eu\n" +
		"# @step a using=A vars.request.name = Ada custom=kept\n" +
		"# @if last.status == 200 run=A\n" +
		"# @elif last.status = 404 run=A\n" +
		"# @else fail=nope note = x\n" +
		"# @switch last.status\n" +
		"# @case 500 run=A =x\n" +
		"# @default fail =nope\n\n" +
		"### A\n# @name A\nGET https://example.com\n"
	doc := Parse("/ws/flow.http", []byte(src))

	var lines []int
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "has spaces around =. Write it as key=value") {
			t.Fatalf("error = %q", e.Message)
		}
		lines = append(lines, e.Span.Start.Line)
	}
	if want := []int{1, 2, 5, 7, 8}; !slices.Equal(lines, want) {
		t.Fatalf("error lines = %v, want %v", lines, want)
	}

	wf := doc.Workflows[0]
	if len(wf.Options) != 0 {
		t.Fatalf("workflow options = %v, want none", wf.Options)
	}
	steps := wf.Steps
	if len(steps) != 3 || steps[0].Using != "A" || !maps.Equal(steps[0].Options, map[string]string{"custom": "kept"}) {
		t.Fatalf("steps = %+v, want the step kept with only its unknown option", steps)
	}
	if ifs := steps[1].If; ifs == nil || len(ifs.Elifs) != 1 || ifs.Else == nil || ifs.Else.Fail != "nope" {
		t.Fatalf("if = %+v, want @elif and @else kept", ifs)
	}
	if sw := steps[2].Switch; sw == nil || len(sw.Cases) != 1 || sw.Default == nil {
		t.Fatalf("switch = %+v, want @case and @default kept", sw)
	}
}

func TestSpacedStepKeepsItsPlace(t *testing.T) {
	src := "# @workflow w\n# @when true\n# @step a using = A\n# @step b using=A\n\n" +
		"### A\n# @name A\nGET https://example.com\n"
	doc := Parse("/ws/flow.http", []byte(src))
	if len(doc.Errors) != 1 ||
		doc.Errors[0].Message != `@step option "using" has spaces around =. Write it as key=value` {
		t.Fatalf("errors = %v, want only the spaced using", doc.Errors)
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 2 || steps[0].Name != "a" || steps[0].When == nil || steps[1].When != nil {
		t.Fatalf("steps = %+v, want a kept with the @when and b without it", steps)
	}
}

func TestWorkflowErrorsAreReportedOneByOne(t *testing.T) {
	src := "# @workflow w\n# @step a using=A note = x expect.statuscode=abc\n\n" +
		"### A\n# @name A\nGET https://example.com\n"
	doc := Parse("/ws/flow.http", []byte(src))
	if len(doc.Errors) != 2 {
		t.Fatalf("errors = %v, want the spaced note and the bad status code apart", doc.Errors)
	}
	for _, e := range doc.Errors {
		if strings.Contains(e.Message, "\n") {
			t.Fatalf("error = %q, want one problem per diagnostic", e.Message)
		}
	}
}

func TestSpacedOptionsReadQuotesFromSource(t *testing.T) {
	src := "# @workflow w\n" +
		"# @step a using=A vars.request.foo= \"a=b\"\n" +
		"# @step b using=A note=\"\" flag\n\n" +
		"### A\n# @name A\n# @ssh host=h known_hosts= \"x=y\"\nGET https://example.com\n"
	doc := Parse("/ws/flow.http", []byte(src))

	var lines []int
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "has spaces around =. Write it as key=value") {
			t.Fatalf("error = %q", e.Message)
		}
		lines = append(lines, e.Span.Start.Line)
	}
	if want := []int{2, 7}; !slices.Equal(lines, want) {
		t.Fatalf("error lines = %v, want %v", lines, want)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", doc.Warnings)
	}
	steps := doc.Workflows[0].Steps
	if _, ok := steps[0].Vars["vars.request.foo"]; ok || steps[0].Options["a"] != "" {
		t.Fatalf("step a = %+v, want no var and no a option", steps[0])
	}
	if steps[1].Options["flag"] != "true" {
		t.Fatalf("step b options = %v, want flag kept after an explicit empty value", steps[1].Options)
	}
}

func TestSpacedOptionFieldsReadQuotesFromSource(t *testing.T) {
	src := "### a\n# @auth oauth2 token_url=https://id.example.com/t client_id=a audience= \"api=v2\"\nGET https://example.com/a\n\n" +
		"### b\n# @auth oauth2 token_url=https://id.example.com/t client_id=a client_secret= \"abc=def\"\nGET https://example.com/b\n\n" +
		"### c\n# @compare dev uat base= \"a=b\"\nGET https://example.com/c\n\n" +
		"### d\n# @script pre-request lang=\"\" js\n> {% x %}\nGET https://example.com/d\n\n" +
		"### e\n# @rts lang=\"\" pre-request\n> {% x %}\nGET https://example.com/e\n"
	doc := Parse("/ws/api.http", []byte(src))

	var lines []int
	for _, e := range doc.Errors {
		if !strings.Contains(e.Message, "has spaces around =. Write it as key=value") {
			t.Fatalf("error = %q", e.Message)
		}
		lines = append(lines, e.Span.Start.Line)
	}
	if want := []int{2, 6, 10}; !slices.Equal(lines, want) {
		t.Fatalf("error lines = %v, want %v", lines, want)
	}
	for i := range 3 {
		if a := doc.Requests[i].Metadata; a.Auth != nil && a.Auth.Rejected == "" {
			t.Fatalf("request %d auth = %+v, want it rejected", i, a.Auth)
		}
	}
}

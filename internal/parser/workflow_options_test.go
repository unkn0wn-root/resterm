package parser

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// The branch expression and the options after it used to be rebuilt by joining
// lexed tokens with spaces, which threw the quotes away and let the second half
// of a quoted value parse as extra options.
func TestParseWorkflowBranchKeepsQuotedValues(t *testing.T) {
	src := `# @workflow demo
# @if response.body.name == "John Doe" run=First
# @elif response.body.name == "Jane Roe" fail=mismatch
# @else fail="nothing matched"
# @switch response.body.role
# @case "site admin" fail="not allowed here"
# @default run=First

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(doc.Workflows))
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 2 {
		t.Fatalf("expected an @if and a @switch step, got %d", len(steps))
	}

	branch := steps[0].If
	if branch == nil {
		t.Fatal("expected an @if step")
	}
	if want := `response.body.name == "John Doe"`; branch.Then.Cond != want {
		t.Fatalf("@if condition = %q, want %q", branch.Then.Cond, want)
	}
	if branch.Then.Run != "First" {
		t.Fatalf("@if run = %q, want %q", branch.Then.Run, "First")
	}
	if len(branch.Elifs) != 1 {
		t.Fatalf("expected 1 @elif, got %d", len(branch.Elifs))
	}
	if want := `response.body.name == "Jane Roe"`; branch.Elifs[0].Cond != want {
		t.Fatalf("@elif condition = %q, want %q", branch.Elifs[0].Cond, want)
	}
	if branch.Else == nil {
		t.Fatal("expected an @else")
	}
	if want := "nothing matched"; branch.Else.Fail != want {
		t.Fatalf("@else fail = %q, want %q", branch.Else.Fail, want)
	}

	sw := steps[1].Switch
	if sw == nil {
		t.Fatal("expected a @switch step")
	}
	if len(sw.Cases) != 1 {
		t.Fatalf("expected 1 @case, got %d", len(sw.Cases))
	}
	if want := "site admin"; sw.Cases[0].Expr != `"`+want+`"` {
		t.Fatalf("@case expression = %q, want %q", sw.Cases[0].Expr, `"`+want+`"`)
	}
	if want := "not allowed here"; sw.Cases[0].Fail != want {
		t.Fatalf("@case fail = %q, want %q", sw.Cases[0].Fail, want)
	}
}

// An option key is a name followed by a single equals sign. A comparison that
// happens to contain one belongs to the expression.
func TestParseWorkflowBranchKeepsTightComparison(t *testing.T) {
	src := `# @workflow demo
# @if last.statusCode==200 run=First

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 1 || steps[0].Kind != restfile.WorkflowStepKindIf {
		t.Fatalf("expected a single @if step, got %+v", steps)
	}
	if want := "last.statusCode==200"; steps[0].If.Then.Cond != want {
		t.Fatalf("@if condition = %q, want %q", steps[0].If.Then.Cond, want)
	}
	if steps[0].If.Then.Run != "First" {
		t.Fatalf("@if run = %q, want %q", steps[0].If.Then.Run, "First")
	}
}

// A quoted alias may hold spaces. A bare one still ends at the first space,
// because a second bare word is indistinguishable from a bare option.
func TestParseWorkflowStepAliasGrammar(t *testing.T) {
	src := `# @workflow demo
# @step "Create Account" using=First
# @step Delete dry-run using=First
# @step using=First name="Read Account"
# @step "a=b" using=First
# @step Path\Name using=First

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	want := []struct {
		name string
		opts map[string]string
	}{
		{name: "Create Account"},
		{name: "Delete", opts: map[string]string{"dry-run": "true"}},
		{name: "Read Account"},
		{name: "a=b"},
		{name: `Path\Name`},
	}
	for i, step := range steps {
		if step.Name != want[i].name {
			t.Fatalf("step %d name = %q, want %q", i, step.Name, want[i].name)
		}
		if step.Using != "First" {
			t.Fatalf("step %d using = %q, want %q", i, step.Using, "First")
		}
		if len(step.Options) != len(want[i].opts) {
			t.Fatalf("step %d options = %v, want %v", i, step.Options, want[i].opts)
		}
		for k, v := range want[i].opts {
			if step.Options[k] != v {
				t.Fatalf("step %d option %q = %q, want %q", i, k, step.Options[k], v)
			}
		}
	}
}

func TestParseWorkflowStepWithoutName(t *testing.T) {
	src := `# @workflow demo
# @step using=First

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	step := doc.Workflows[0].Steps[0]
	if step.Name != "" || step.Using != "First" {
		t.Fatalf("step = %+v, want no name and using=First", step)
	}
}

// A quoted value and a comparison decode to the same token, so classifying an
// option after decoding got all of these wrong.
func TestParseWorkflowBranchClassifiesOptionsFromSource(t *testing.T) {
	src := `# @workflow demo
# @if true fail="=foo"
# @elif response.text() == "a=b" run=First
# @else fail=none
# @switch role
# @case "a=b" run=First
# @default fail=none

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 2 {
		t.Fatalf("expected an @if and a @switch step, got %d", len(steps))
	}

	branch := steps[0].If
	if branch.Then.Fail != "=foo" {
		t.Fatalf("@if fail = %q, want %q", branch.Then.Fail, "=foo")
	}
	if want := `response.text() == "a=b"`; branch.Elifs[0].Cond != want {
		t.Fatalf("@elif condition = %q, want %q", branch.Elifs[0].Cond, want)
	}
	if branch.Elifs[0].Run != "First" {
		t.Fatalf("@elif run = %q, want %q", branch.Elifs[0].Run, "First")
	}
	if want := `"a=b"`; steps[1].Switch.Cases[0].Expr != want {
		t.Fatalf("@case expression = %q, want %q", steps[1].Switch.Cases[0].Expr, want)
	}
}

// An escaped quote does not end the string it sits in. Splitting spans with a
// lexer that ignored the escape used to close the quote early and read the tail
// of the string as options.
func TestParseWorkflowBranchKeepsEscapedQuotes(t *testing.T) {
	src := `# @workflow demo
# @if response.body.msg == "say \" fail=x" run=First

### First
# @name First
GET https://example.com/first
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", doc.Errors)
	}
	branch := doc.Workflows[0].Steps[0].If
	if want := `response.body.msg == "say \" fail=x"`; branch.Then.Cond != want {
		t.Fatalf("@if condition = %q, want %q", branch.Then.Cond, want)
	}
	if branch.Then.Run != "First" {
		t.Fatalf("@if run = %q, want %q", branch.Then.Run, "First")
	}
	if branch.Then.Fail != "" {
		t.Fatalf("@if fail = %q, want it empty", branch.Then.Fail)
	}
}

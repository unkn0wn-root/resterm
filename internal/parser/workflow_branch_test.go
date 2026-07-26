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

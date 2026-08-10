package restwriter

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// directiveWriter emits the "# @name arg key=value" lines directives share.
type directiveWriter struct{ b *strings.Builder }

// head opens a directive line and leaves it open for options.
func (w directiveWriter) head(name directive.Name, arg string) {
	w.b.WriteString(name.Comment())
	if arg != "" {
		w.b.WriteString(" ")
		w.b.WriteString(commentLines(arg))
	}
}

func (w directiveWriter) option(key, value string) {
	w.b.WriteString(" ")
	w.b.WriteString(key)
	w.b.WriteString("=")
	w.b.WriteString(directive.Quote(value))
}

func (w directiveWriter) end() {
	w.b.WriteString("\n")
}

func (w directiveWriter) line(name directive.Name, arg string) {
	w.head(name, arg)
	w.end()
}

// argOf maps a declaration to its directive and the text after the name. None
// of them re-check fields the parser already rejected empty or untrimmed.
type argOf[T any] func(T) (directive.Name, string)

func writeEach[T any](w directiveWriter, specs []T, arg argOf[T]) {
	for _, spec := range specs {
		w.line(arg(spec))
	}
}

func writeOne[T any](w directiveWriter, spec *T, arg argOf[T]) {
	if spec != nil {
		w.line(arg(*spec))
	}
}

// The parser padded each continuation to its source column, so the marker takes
// the place of one padding byte and the rest of the indent survives.
func commentLines(arg string) string {
	if !strings.Contains(arg, "\n") {
		return arg
	}
	lines := strings.Split(arg, "\n")
	for i, ln := range lines[1:] {
		lines[i+1] = "#" + strings.TrimPrefix(ln, " ")
	}
	return strings.Join(lines, "\n")
}

// The parser accepts only file and global scope, and Scope.String answers
// "request" for anything else, which it would then reject on the way back in.
func renderPatches(w directiveWriter, patches []restfile.PatchProfile) error {
	for _, p := range patches {
		switch p.Scope {
		case directive.ScopeFile, directive.ScopeGlobal:
			w.line(patchArg(p))
		default:
			return fmt.Errorf("writer: @patch %q scope must be file or global", p.Name)
		}
	}
	return nil
}

func patchArg(p restfile.PatchProfile) (directive.Name, string) {
	return directive.Patch, fmt.Sprintf("%s %s %s", p.Scope, p.Name, p.Expression)
}

// A leading use= is the profile form, so profiles and an expression never mix.
func applyArg(a restfile.ApplySpec) (directive.Name, string) {
	if len(a.Uses) == 0 {
		return directive.Apply, a.Expression
	}
	uses := make([]string, len(a.Uses))
	for i, name := range a.Uses {
		uses[i] = "use=" + name
	}
	return directive.Apply, strings.Join(uses, ", ")
}

func conditionArg(c restfile.ConditionSpec) (directive.Name, string) {
	if c.Negate {
		return directive.SkipIf, c.Expression
	}
	return directive.When, c.Expression
}

// The "as" form is written because the expression may itself contain "in".
func forEachArg(f restfile.ForEachSpec) (directive.Name, string) {
	return directive.ForEach, fmt.Sprintf("%s as %s", f.Expression, f.Var)
}

func stepForEachArg(f restfile.WorkflowForEach) (directive.Name, string) {
	return forEachArg(restfile.ForEachSpec{Expression: f.Expr, Var: f.Var})
}

func captureArg(c restfile.CaptureSpec) (directive.Name, string) {
	return directive.Capture, fmt.Sprintf("%s %s %s", captureScopeToken(c), c.Name, c.Expression)
}

func assertArg(a restfile.AssertSpec) (directive.Name, string) {
	if msg := assertMessage(a.Message); msg != "" {
		return directive.Assert, fmt.Sprintf("%s => %s", a.Expression, msg)
	}
	return directive.Assert, a.Expression
}

// The parser trims the message and then takes one layer of quotes off it, so a
// message either step would alter has to be quoted to survive both.
func assertMessage(msg string) string {
	if msg == "" {
		return ""
	}
	if directive.TrimQuotes(msg) != msg || strings.TrimSpace(msg) != msg {
		return `"` + msg + `"`
	}
	return msg
}

func captureScopeToken(c restfile.CaptureSpec) string {
	scope := "request"
	switch c.Scope {
	case directive.ScopeFile:
		scope = "file"
	case directive.ScopeGlobal:
		scope = "global"
	}
	if c.Secret {
		scope += "-secret"
	}
	return scope
}

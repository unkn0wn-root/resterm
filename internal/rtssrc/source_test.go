package rtssrc

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestLoadInlineSourceMapsLinesAndColumns(t *testing.T) {
	doc := &restfile.Document{
		Path: "sample.http",
		Raw:  []byte("### Sample\n# @rts pre-request\n> first()\n\n  > second()\nGET https://example.com\n"),
	}
	block := restfile.ScriptBlock{
		Body:       "first()\nsecond()",
		SourcePath: "sample.http",
		Lines: []restfile.ScriptLine{
			{Line: 3, Col: 3},
			{Line: 5, Col: 5},
		},
	}

	src, err := Load(doc, block, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if src.Text != "  first()\n\n    second()" {
		t.Fatalf("unexpected source text: %q", src.Text)
	}
	if src.Path != "sample.http" || src.Pos.Line != 3 || src.Pos.Col != 1 {
		t.Fatalf("unexpected source position: %+v path=%q", src.Pos, src.Path)
	}
	if string(src.Raw) != string(doc.Raw) {
		t.Fatalf("expected raw document source")
	}
}

func TestAnnotateAddsSourceToDiagnostic(t *testing.T) {
	err := &rts.RuntimeError{
		Pos: rts.Pos{Path: "sample.http", Line: 3, Col: 3},
		Msg: "boom",
	}
	src := Source{
		Path: "sample.http",
		Raw:  []byte("### Sample\n# @rts pre-request\n> boom()\n"),
	}

	out := diag.Render(Annotate(err, src))
	if !strings.Contains(out, "   3 | > boom()") {
		t.Fatalf("expected source line in rendered diagnostic:\n%s", out)
	}
}

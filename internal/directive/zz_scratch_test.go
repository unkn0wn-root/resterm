package directive

import "testing"

func TestScratchCutOptionsEscapedQuote(t *testing.T) {
	in := `response.body.msg == "say \" fail=x" run=Y`
	head, opts := CutOptions(in)
	t.Logf("head=%q opts=%v", head, opts)
	if want := `response.body.msg == "say \" fail=x"`; head != want {
		t.Errorf("head = %q, want %q", head, want)
	}
	if opts["run"] != "Y" {
		t.Errorf("opts = %v, want run=Y", opts)
	}
	if _, ok := opts["fail"]; ok {
		t.Errorf("fail leaked out of the quoted string: %v", opts)
	}
}

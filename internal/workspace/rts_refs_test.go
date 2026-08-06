package workspace

import (
	"slices"
	"testing"
)

func TestRTSRefsWalksTaggedSwitch(t *testing.T) {
	src := `
fn pick() {
  switch json.file("./tag.json").kind {
  case json.file("./a.json").id, json.file("./b.json").id:
    return json.file("./matched.json")
  case "other":
    return json.file("./unmatched.json")
  default:
    return json.file("./default.json")
  }
}
`
	got := jsonFileModuleRefs("mod.rts", src)
	want := []string{
		"./tag.json",
		"./a.json",
		"./b.json",
		"./matched.json",
		"./unmatched.json",
		"./default.json",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("refs: got %v, want %v", got, want)
	}
}

func TestRTSRefsWalksTaglessSwitch(t *testing.T) {
	src := `
switch {
case rts.json.file("./flag.json").on:
  let a = json.file("./on.json")
default:
  let b = json.file("./off.json")
}
`
	got := jsonFileModuleRefs("mod.rts", src)
	want := []string{"./flag.json", "./on.json", "./off.json"}
	if !slices.Equal(got, want) {
		t.Fatalf("refs: got %v, want %v", got, want)
	}
}

package navigator

import "testing"

func TestWordsFromTextHandlesUnicode(t *testing.T) {
	tokens := wordsFromText("Bonjour-titre 你好/请求 München")
	want := []string{"bonjour-titre", "你好", "请求", "münchen"}
	for _, expected := range want {
		found := false
		for _, token := range tokens {
			if token == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected token %q in %v", expected, tokens)
		}
	}
}

func TestSelectByID(t *testing.T) {
	nodes := []*Node[int]{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}
	m := New(nodes)
	m.SetViewportHeight(1)

	if !m.SelectByID("b") {
		t.Fatalf("expected select to succeed")
	}
	if sel := m.Selected(); sel == nil || sel.ID != "b" {
		t.Fatalf("expected selection id b, got %#v", sel)
	}
	visible := m.VisibleRows()
	if len(visible) != 1 || visible[0].Node == nil || visible[0].Node.ID != "b" {
		t.Fatalf("expected visible window to include selected node b, got %#v", visible)
	}
}

func TestSelectByIDMissingKeepsSelection(t *testing.T) {
	nodes := []*Node[int]{
		{ID: "root", Title: "root"},
		{ID: "child", Title: "child"},
	}
	m := New(nodes)

	if m.SelectByID("absent") {
		t.Fatalf("expected select to fail for missing id")
	}
	if sel := m.Selected(); sel == nil || sel.ID != "root" {
		t.Fatalf("expected selection to remain at root, got %#v", sel)
	}
}

package navigator

import "testing"

func TestToggleExpandedCollapsesLeaf(t *testing.T) {
	n := &Node[any]{ID: "file:/tmp/empty.http", Kind: KindFile, Expanded: true}
	m := New([]*Node[any]{n})

	m.ToggleExpanded()

	if n.Expanded {
		t.Fatalf("expected leaf node to collapse")
	}
}

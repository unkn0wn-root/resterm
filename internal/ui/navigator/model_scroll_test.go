package navigator

import (
	"fmt"
	"testing"
)

func TestNavigatorTypewriterScroll(t *testing.T) {
	nodes := make([]*Node[any], 0, 6)
	for i := 0; i < 6; i++ {
		nodes = append(nodes, &Node[any]{ID: fmt.Sprintf("n%d", i), Title: "n"})
	}
	m := New(nodes)
	m.SetViewportHeight(3)

	local := func() int { return m.sel - m.offset }

	if local() != 0 {
		t.Fatalf("expected selection at top initially, got local %d offset %d", local(), m.offset)
	}

	m.Move(2)
	if m.offset != 1 || local() != 1 {
		t.Fatalf("expected offset 1 after moving twice, got local %d offset %d", local(), m.offset)
	}

	m.Move(1)
	if m.offset != 2 || local() != 1 {
		t.Fatalf(
			"expected offset to advance with buffer, got local %d offset %d",
			local(),
			m.offset,
		)
	}

	m.Move(1)
	if m.offset != 3 || local() != 1 {
		t.Fatalf("expected offset to pin near end, got %d (local %d)", m.offset, local())
	}
}

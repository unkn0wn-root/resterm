package prompt

type Menu struct {
	items     []Item
	selection int
}

func (m *Menu) Reset(items []Item) {
	m.items = items
	m.selection = -1
}

func (m *Menu) Move(delta int) {
	if len(m.items) == 0 {
		return
	}
	if m.selection < 0 {
		if delta < 0 {
			m.selection = len(m.items) - 1
		} else {
			m.selection = 0
		}
		return
	}

	next := (m.selection + delta) % len(m.items)
	if next < 0 {
		next += len(m.items)
	}
	m.selection = next
}

func (m Menu) Items() []Item { return m.items }

func (m Menu) Selection() int {
	if m.selection < 0 || m.selection >= len(m.items) {
		return -1
	}
	return m.selection
}

func (m Menu) Selected() (Item, bool) {
	at := m.Selection()
	if at < 0 {
		return Item{}, false
	}
	return m.items[at], true
}

func (m Menu) Preferred() (Item, bool) {
	if item, ok := m.Selected(); ok {
		return item, true
	}
	if len(m.items) == 0 {
		return Item{}, false
	}
	return m.items[0], true
}

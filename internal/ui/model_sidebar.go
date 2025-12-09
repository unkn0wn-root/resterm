package ui

func (m *Model) reqCompactMode() bool {
	return m.reqCompact != nil && *m.reqCompact
}

func (m *Model) wfCompactMode() bool {
	return m.wfCompact != nil && *m.wfCompact
}

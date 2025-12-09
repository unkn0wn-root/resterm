package ui

func (m *Model) reqCompactMode() bool {
	return m.reqCompact != nil && *m.reqCompact
}

func (m *Model) setReqCompact(v bool) bool {
	if m.reqCompact == nil {
		m.reqCompact = new(bool)
	}
	if *m.reqCompact == v {
		return false
	}
	*m.reqCompact = v
	return true
}

func (m *Model) wfCompactMode() bool {
	return m.wfCompact != nil && *m.wfCompact
}

func (m *Model) setWfCompact(v bool) bool {
	if m.wfCompact == nil {
		m.wfCompact = new(bool)
	}
	if *m.wfCompact == v {
		return false
	}
	*m.wfCompact = v
	return true
}

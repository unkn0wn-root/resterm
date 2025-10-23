package stream

import (
	"sync"
	"time"
)

type CompletionHook func(summary SessionSummary, events []*Event)

type SessionSummary struct {
	ID        string
	Kind      Kind
	State     State
	Err       error
	StartedAt time.Time
	EndedAt   time.Time
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*managedSession
}

type managedSession struct {
	session *Session
	hooks   []CompletionHook
	summary SessionSummary
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*managedSession)}
}

func (m *Manager) Register(session *Session) SessionSummary {
	if session == nil {
		return SessionSummary{}
	}

	state, err := session.State()
	stats := session.StatsSnapshot()

	summary := SessionSummary{
		ID:        session.ID(),
		Kind:      session.Kind(),
		State:     state,
		Err:       err,
		StartedAt: stats.StartedAt,
	}

	managed := &managedSession{session: session, summary: summary}

	m.mu.Lock()
	m.sessions[session.ID()] = managed
	m.mu.Unlock()

	go m.watch(managed)

	return summary
}

func (m *Manager) Cancel(id string) bool {
	m.mu.RLock()
	managed := m.sessions[id]
	m.mu.RUnlock()
	if managed == nil {
		return false
	}
	managed.session.Cancel()
	return true
}

func (m *Manager) List() []SessionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summaries := make([]SessionSummary, 0, len(m.sessions))
	for _, ms := range m.sessions {
		summaries = append(summaries, ms.currentSummary())
	}
	return summaries
}

func (m *Manager) Get(id string) (SessionSummary, bool) {
	m.mu.RLock()
	managed, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return SessionSummary{}, false
	}
	return managed.currentSummary(), true
}

func (m *Manager) AddCompletionHook(id string, hook CompletionHook) bool {
	if hook == nil {
		return false
	}
	m.mu.Lock()
	managed, ok := m.sessions[id]
	if ok {
		managed.hooks = append(managed.hooks, hook)
	}
	m.mu.Unlock()
	return ok
}

func (m *Manager) Snapshot(id string) ([]*Event, bool) {
	m.mu.RLock()
	managed, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return managed.session.EventsSnapshot(), true
}

func (m *Manager) watch(managed *managedSession) {
	session := managed.session
	if session == nil {
		return
	}
	<-session.Done()

	id := session.ID()
	state, err := session.State()
	stats := session.StatsSnapshot()
	summary := SessionSummary{
		ID:        id,
		Kind:      session.Kind(),
		State:     state,
		Err:       err,
		StartedAt: stats.StartedAt,
		EndedAt:   stats.EndedAt,
	}

	events := session.EventsSnapshot()

	m.mu.Lock()
	hooks := append([]CompletionHook(nil), managed.hooks...)
	managed.summary = summary
	delete(m.sessions, id)
	managed.session = nil
	managed.hooks = nil
	m.mu.Unlock()

	for _, hook := range hooks {
		hook(summary, events)
	}
}

func (ms *managedSession) currentSummary() SessionSummary {
	if ms.session == nil {
		return ms.summary
	}
	state, err := ms.session.State()
	stats := ms.session.StatsSnapshot()
	summary := ms.summary
	summary.ID = ms.session.ID()
	summary.Kind = ms.session.Kind()
	summary.State = state
	summary.Err = err
	summary.StartedAt = stats.StartedAt
	summary.EndedAt = stats.EndedAt
	return summary
}

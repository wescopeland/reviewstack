package reviewer

// SetStatusForTest overrides a reviewer's state. It is intended for tests.
func (m *Manager) SetStatusForTest(id string, state State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return
	}
	st.State = state
}

package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) updateSynthesisPhase() {
	if !m.allReviewsFinished() {
		return
	}
	if m.synthPhase == synthesisRunning || m.synthPhase == synthesisReady {
		return
	}
	m.synthPhase = synthesisDirty
}

func (m *Model) maybeAutoSynthesize() tea.Cmd {
	if !m.cfg.AutoSynthesize {
		return nil
	}
	if m.synthPhase != synthesisDirty {
		return nil
	}
	return m.requestSynthesis(false)
}

func (m *Model) requestSynthesis(force bool) tea.Cmd {
	if m.cfg.OnSynthesize == nil {
		return nil
	}
	if m.synthPhase == synthesisRunning {
		if !force {
			return nil
		}
		m.cancelSynthesis()
	}
	return m.startSynthesisCmd()
}

func (m *Model) startSynthesisCmd() tea.Cmd {
	if m.cfg.OnSynthesize == nil {
		return nil
	}
	m.synthReqID++
	reqID := m.synthReqID
	m.synthPhase = synthesisRunning
	m.errMsg = ""

	ctx, cancel := context.WithCancel(context.Background())
	m.synthCancel = cancel
	onSynth := m.cfg.OnSynthesize

	return func() tea.Msg {
		err := onSynth(ctx)
		return synthDoneMsg{reqID: reqID, err: err}
	}
}

func (m *Model) invalidateSynthesis() {
	if m.synthPhase == synthesisRunning {
		m.cancelSynthesis()
		m.synthReqID++
	}
	m.synthPhase = synthesisNone
}

func (m *Model) cancelSynthesis() {
	if m.synthCancel != nil {
		m.synthCancel()
		m.synthCancel = nil
	}
}

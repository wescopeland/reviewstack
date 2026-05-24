package tui

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

func tick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitEvent(ch <-chan reviewer.Status) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			<-time.After(time.Second)
			return eventIdleMsg(time.Now())
		}
		st, ok := <-ch
		if !ok {
			<-time.After(time.Second)
			return eventIdleMsg(time.Now())
		}
		timer := time.NewTimer(statusCoalesceWindow)
		defer timer.Stop()
		for {
			select {
			case next, ok := <-ch:
				if !ok {
					return statusMsg(st)
				}
				st = next
			case <-timer.C:
				return statusMsg(st)
			}
		}
	}
}

func (m *Model) heartbeat() []tea.Cmd {
	return []tea.Cmd{
		m.ensureEventWait(),
		m.ensureTick(),
		m.ensureSpinner(),
	}
}

func (m *Model) ensureSpinner() tea.Cmd {
	if (m.summarize().running > 0 || m.synthPhase == synthesisRunning) && !m.spinnerActive {
		m.spinnerActive = true
		return m.spinner.Tick
	}
	return nil
}

func (m *Model) ensureTick() tea.Cmd {
	if !m.allReviewsFinished() && !m.tickActive {
		m.tickActive = true
		return tick()
	}
	return nil
}

func (m *Model) ensureEventWait() tea.Cmd {
	if m.cfg.Events != nil && !m.eventWaitActive {
		m.eventWaitActive = true
		return waitEvent(m.cfg.Events)
	}
	return nil
}

func (m *Model) afterStatusChange() tea.Cmd {
	m.updateSynthesisPhase()
	return m.maybeAutoSynthesize()
}

func (m *Model) cycleView(delta int) tea.Cmd {
	next := int(m.viewMode) + delta
	if next < 0 {
		next = 2
	}
	if next > 2 {
		next = 0
	}
	m.viewMode = viewMode(next)
	if m.viewMode == viewDashboard {
		m.viewportKey = ""
		return nil
	}
	return m.queueOutputLoad()
}

func (m *Model) queueOutputLoad() tea.Cmd {
	m.outputReqID++
	reqID := m.outputReqID
	path := m.outputPath()
	m.viewport.SetContent(muted.Render("Loading…"))
	m.viewportKey = ""
	return func() tea.Msg {
		content, key := readOutputFile(path)
		return outputLoadedMsg{reqID: reqID, path: path, key: key, content: content}
	}
}

func (m *Model) maybeRefreshOutput() tea.Cmd {
	if m.viewMode == viewDashboard {
		return nil
	}
	if time.Since(m.lastOutputPoll) < 500*time.Millisecond {
		return nil
	}
	m.lastOutputPoll = time.Now()

	path := m.outputPath()
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := outputCacheKey(path, info.Size(), info.ModTime())
	if key == m.viewportKey {
		return nil
	}
	return m.queueOutputLoad()
}

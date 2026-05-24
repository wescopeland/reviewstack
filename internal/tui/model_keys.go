package tui

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

type (
	tickMsg      time.Time
	eventIdleMsg time.Time
	statusMsg    reviewer.Status
	synthDoneMsg struct {
		reqID int
		err   error
	}
	outputLoadedMsg struct {
		reqID   int
		path    string
		key     string
		content string
	}
)

type keyMap struct {
	Up, Down, Left, Right, Tab, Enter, Rerun, Kill, Synth, Quit, Open, ScrollUp, ScrollDown key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:       key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "prev tab")),
		Right:      key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "next tab")),
		Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open output")),
		Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun")),
		Kill:       key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill")),
		Synth:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "synthesize")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open report")),
		ScrollUp:   key.NewBinding(key.WithKeys("u", "pgup"), key.WithHelp("u", "scroll up")),
		ScrollDown: key.NewBinding(key.WithKeys("d", "pgdown"), key.WithHelp("d", "scroll down")),
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) ([]tea.Cmd, bool) {
	if m.quitting {
		return []tea.Cmd{tea.Quit}, false
	}

	var cmds []tea.Cmd
	navKey := false

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancelSynthesis()
		m.cfg.Manager.KillAll()
		m.quitting = true
		return []tea.Cmd{tea.Quit}, false

	case key.Matches(msg, m.keys.Up):
		if m.viewMode == viewDashboard && m.selected > 0 {
			m.selected--
		} else if m.viewMode != viewDashboard {
			m.viewport.ScrollUp(1)
		}

	case key.Matches(msg, m.keys.Down):
		if m.viewMode == viewDashboard && m.selected < len(m.statuses)-1 {
			m.selected++
		} else if m.viewMode != viewDashboard {
			m.viewport.ScrollDown(1)
		}

	case key.Matches(msg, m.keys.ScrollUp):
		if m.viewMode != viewDashboard {
			m.viewport.ScrollUp(3)
		}

	case key.Matches(msg, m.keys.ScrollDown):
		if m.viewMode != viewDashboard {
			m.viewport.ScrollDown(3)
		}

	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Right):
		navKey = true
		cmds = append(cmds, m.cycleView(1))

	case key.Matches(msg, m.keys.Left):
		navKey = true
		cmds = append(cmds, m.cycleView(-1))

	case key.Matches(msg, m.keys.Enter):
		if m.viewMode == viewDashboard {
			m.viewMode = viewRaw
			cmds = append(cmds, m.queueOutputLoad())
		}

	case key.Matches(msg, m.keys.Rerun):
		id := m.selectedID()
		if id != "" && m.cfg.OnRerun != nil {
			if err := m.cfg.OnRerun(id); err != nil {
				m.errMsg = err.Error()
			} else {
				m.infoMsg = fmt.Sprintf("Rerunning %s…", id)
				m.errMsg = ""
				m.invalidateSynthesis()
				m.refreshStatuses()
				m.reviewsFinishedAt = time.Time{}
				cmds = append(cmds, m.heartbeat()...)
			}
		}

	case key.Matches(msg, m.keys.Kill):
		id := m.selectedID()
		if id != "" && m.cfg.OnKill != nil {
			if err := m.cfg.OnKill(id); err != nil {
				m.errMsg = err.Error()
			} else {
				m.infoMsg = fmt.Sprintf("Killed %s", id)
			}
		}

	case key.Matches(msg, m.keys.Synth):
		cmds = append(cmds, m.requestSynthesis(true))
		cmds = append(cmds, m.ensureSpinner())

	case key.Matches(msg, m.keys.Open):
		if m.synthPhase == synthesisReady || fileExists(m.finalReportPath()) {
			_ = openFile(m.finalReportPath())
		}
	}

	return cmds, navKey
}

func openFile(path string) error {
	return exec.Command("open", path).Run()
}

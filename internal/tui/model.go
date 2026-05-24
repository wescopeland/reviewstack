package tui

import (
	"context"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wescopeland/reviewstack/internal/config"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

type Config struct {
	PR             int
	Subject        string
	Base           string
	RunDir         string
	Reviewers      []config.Reviewer
	Manager        *reviewer.Manager
	Events         <-chan reviewer.Status
	AutoSynthesize bool
	OnRerun        func(id string) error
	OnKill         func(id string) error
	OnSynthesize   func(context.Context) error
}

type synthesisPhase int

const (
	synthesisNone synthesisPhase = iota
	synthesisDirty
	synthesisRunning
	synthesisReady
)

type viewMode int

const (
	viewDashboard viewMode = iota
	viewRaw
	viewLog
)

const statusCoalesceWindow = 75 * time.Millisecond

type summary struct {
	pending, running, done, failed, killed int
}

type Model struct {
	cfg               Config
	statuses          []reviewer.Status
	selected          int
	viewMode          viewMode
	errMsg            string
	infoMsg           string
	startedAt         time.Time
	reviewsFinishedAt time.Time
	quitting          bool
	synthPhase        synthesisPhase
	synthReqID        int
	synthCancel       context.CancelFunc
	width             int
	height            int
	tickActive        bool
	eventWaitActive   bool
	spinner           spinner.Model
	spinnerActive     bool
	viewport          viewport.Model
	viewportKey       string
	outputReqID       int
	lastOutputPoll    time.Time
	keys              keyMap
	tickCount         int
}

func New(cfg Config) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	vp := viewport.New(80, 12)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	m := &Model{
		cfg:       cfg,
		startedAt: time.Now(),
		viewMode:  viewDashboard,
		spinner:   sp,
		viewport:  vp,
		keys:      defaultKeyMap(),
		width:     100,
		height:    28,
	}
	m.refreshStatuses()
	return m
}

func Run(m *Model) error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.heartbeat()...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = max(20, msg.Width-4)
		m.viewport.Height = max(6, m.contentHeight())
		if m.viewMode != viewDashboard {
			cmds = append(cmds, m.maybeRefreshOutput())
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case spinner.TickMsg:
		if m.summarize().running > 0 || m.synthPhase == synthesisRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			m.spinnerActive = false
		}

	case outputLoadedMsg:
		if msg.reqID != m.outputReqID || msg.path != m.outputPath() {
			return m, tea.Batch(cmds...)
		}
		wasAtBottom := m.viewportKey == "" || m.viewport.AtBottom()
		m.viewport.SetContent(msg.content)
		m.viewportKey = msg.key
		if wasAtBottom {
			m.viewport.GotoBottom()
		}

	case tickMsg:
		m.tickActive = false
		m.tickCount++
		m.refreshStatuses()
		cmds = append(cmds, m.afterStatusChange())
		cmds = append(cmds, m.maybeRefreshOutput())
		cmds = append(cmds, m.heartbeat()...)

	case eventIdleMsg:
		m.eventWaitActive = false
		m.refreshStatuses()
		cmds = append(cmds, m.afterStatusChange())
		cmds = append(cmds, m.heartbeat()...)

	case statusMsg:
		m.eventWaitActive = false
		m.refreshStatuses()
		cmds = append(cmds, m.afterStatusChange())
		cmds = append(cmds, m.heartbeat()...)

	case synthDoneMsg:
		if msg.reqID != m.synthReqID {
			return m, tea.Batch(cmds...)
		}
		m.synthCancel = nil
		if msg.err != nil {
			m.synthPhase = synthesisDirty
			m.errMsg = msg.err.Error()
		} else {
			m.synthPhase = synthesisReady
			m.errMsg = ""
		}
		cmds = append(cmds, m.ensureSpinner())

	case tea.KeyMsg:
		keyCmds, navKey := m.handleKey(msg)
		cmds = append(cmds, keyCmds...)
		if navKey {
			return m, tea.Batch(cmds...)
		}
	}

	if m.viewMode != viewDashboard {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) contentHeight() int {
	reserved := 14
	if m.viewMode == viewDashboard {
		reserved += 8
	}
	return max(6, m.height-reserved)
}

func (m *Model) refreshStatuses() {
	order := make([]string, len(m.cfg.Reviewers))
	for i, r := range m.cfg.Reviewers {
		order[i] = r.ID
	}
	byID := map[string]reviewer.Status{}
	for _, st := range m.cfg.Manager.Snapshot() {
		byID[st.ID] = st
	}
	m.statuses = make([]reviewer.Status, 0, len(order))
	for _, id := range order {
		if st, ok := byID[id]; ok {
			m.statuses = append(m.statuses, st)
		}
	}
	if m.selected >= len(m.statuses) {
		m.selected = max(0, len(m.statuses)-1)
	}
	m.updateReviewsFinishedAt()
}

func (m *Model) allReviewsFinished() bool {
	if len(m.statuses) == 0 {
		return false
	}
	sum := m.summarize()
	return sum.pending == 0 && sum.running == 0
}

func (m *Model) updateReviewsFinishedAt() {
	if m.allReviewsFinished() {
		if m.reviewsFinishedAt.IsZero() {
			m.reviewsFinishedAt = time.Now()
		}
		return
	}
	m.reviewsFinishedAt = time.Time{}
}

func (m *Model) elapsedDuration() time.Duration {
	if m.reviewsFinishedAt.IsZero() {
		return time.Since(m.startedAt)
	}
	return m.reviewsFinishedAt.Sub(m.startedAt)
}

func (m *Model) summarize() summary {
	var s summary
	for _, st := range m.statuses {
		switch st.State {
		case reviewer.StatePending:
			s.pending++
		case reviewer.StateRunning:
			s.running++
		case reviewer.StateDone:
			s.done++
		case reviewer.StateFailed:
			s.failed++
		case reviewer.StateKilled:
			s.killed++
		}
	}
	return s
}

func (m *Model) selectedID() string {
	if m.selected < 0 || m.selected >= len(m.statuses) {
		return ""
	}
	return m.statuses[m.selected].ID
}

func (m *Model) selectedStatus() (reviewer.Status, bool) {
	if m.selected < 0 || m.selected >= len(m.statuses) {
		return reviewer.Status{}, false
	}
	return m.statuses[m.selected], true
}

func (m *Model) finalReportPath() string {
	return filepath.Join(m.cfg.RunDir, "final-review.md")
}

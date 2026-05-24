package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	return tea.Batch(
		m.ensureTick(),
		m.ensureEventWait(),
		m.ensureSpinner(),
	)
}

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

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = max(20, msg.Width-4)
		m.viewport.Height = max(6, m.contentHeight())
		if m.viewMode != viewDashboard {
			if cmd := m.maybeRefreshOutput(); cmd != nil {
				return m, cmd
			}
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
		if cmd := m.afterStatusChange(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.maybeRefreshOutput(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureEventWait(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureTick(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureSpinner(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case eventIdleMsg:
		m.eventWaitActive = false
		m.refreshStatuses()
		if cmd := m.afterStatusChange(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureEventWait(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureTick(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case statusMsg:
		m.eventWaitActive = false
		m.refreshStatuses()
		if cmd := m.afterStatusChange(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureEventWait(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureTick(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.ensureSpinner(); cmd != nil {
			cmds = append(cmds, cmd)
		}

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
		if cmd := m.ensureSpinner(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		if m.quitting {
			return m, tea.Quit
		}
		navKey := false
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.cancelSynthesis()
			m.cfg.Manager.KillAll()
			m.quitting = true
			return m, tea.Quit

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
			if cmd := m.cycleView(1); cmd != nil {
				cmds = append(cmds, cmd)
			}

		case key.Matches(msg, m.keys.Left):
			navKey = true
			if cmd := m.cycleView(-1); cmd != nil {
				cmds = append(cmds, cmd)
			}

		case key.Matches(msg, m.keys.Enter):
			if m.viewMode == viewDashboard {
				m.viewMode = viewRaw
				if cmd := m.queueOutputLoad(); cmd != nil {
					cmds = append(cmds, cmd)
				}
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
					if cmd := m.ensureEventWait(); cmd != nil {
						cmds = append(cmds, cmd)
					}
					if cmd := m.ensureTick(); cmd != nil {
						cmds = append(cmds, cmd)
					}
					if cmd := m.ensureSpinner(); cmd != nil {
						cmds = append(cmds, cmd)
					}
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
			if cmd := m.requestSynthesis(true); cmd != nil {
				cmds = append(cmds, cmd)
				if spin := m.ensureSpinner(); spin != nil {
					cmds = append(cmds, spin)
				}
			}

		case key.Matches(msg, m.keys.Open):
			if m.synthPhase == synthesisReady || fileExists(m.finalReportPath()) {
				_ = openFile(m.finalReportPath())
			}
		}
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

func (m *Model) View() string {
	if m.quitting {
		return ""
	}

	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderSummary())
	sections = append(sections, m.renderTabs())

	switch m.viewMode {
	case viewDashboard:
		sections = append(sections, m.renderTable())
		if detail := m.renderDetail(); detail != "" {
			sections = append(sections, detail)
		}
	default:
		sections = append(sections, m.renderOutputPane())
	}

	if banner := m.renderBanner(); banner != "" {
		sections = append(sections, banner)
	}
	if m.errMsg != "" {
		sections = append(sections, errStyle.Render("⚠ "+m.errMsg))
	}
	if m.infoMsg != "" && m.synthPhase != synthesisReady {
		sections = append(sections, infoStyle.Render("→ "+m.infoMsg))
	}
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *Model) renderHeader() string {
	elapsed := formatElapsed(m.elapsedDuration())
	if !m.reviewsFinishedAt.IsZero() {
		elapsed = statusDone.Render(elapsed) + muted.Render(" ✓")
	}
	subject := m.cfg.Subject
	if subject == "" && m.cfg.PR > 0 {
		subject = fmt.Sprintf("PR %d", m.cfg.PR)
	}
	if subject == "" {
		subject = "review"
	}
	refLabel := "base " + m.cfg.Base
	if m.cfg.Subject != "" && strings.HasPrefix(m.cfg.Subject, "uncommitted") {
		refLabel = "working tree"
	}
	if strings.HasPrefix(m.cfg.Subject, "branch") {
		refLabel = "base " + m.cfg.Base
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Top,
		titleStyle.Render("reviewstack"),
		muted.Render("  │  "),
		lipgloss.NewStyle().Bold(true).Render(subject),
		muted.Render("  ·  "),
		muted.Render(refLabel),
		muted.Render("  ·  "),
		muted.Render(elapsed),
	)
	runLine := muted.Render("run  ") + pathStyle.Render(shortPath(m.cfg.RunDir, m.width-8))
	return panelStyle.Width(m.width - 2).Render(title + "\n" + runLine)
}

func (m *Model) renderSummary() string {
	sum := m.summarize()
	total := len(m.statuses)
	if total == 0 {
		return ""
	}

	finished := sum.done + sum.failed + sum.killed
	pct := int(float64(finished) / float64(total) * 100)

	parts := []string{}
	if sum.running > 0 {
		parts = append(parts, statusRunning.Render(fmt.Sprintf("%s %d running", m.spinner.View(), sum.running)))
	}
	if sum.done > 0 {
		parts = append(parts, statusDone.Render(fmt.Sprintf("✓ %d done", sum.done)))
	}
	if sum.failed+sum.killed > 0 {
		parts = append(parts, statusFailed.Render(fmt.Sprintf("✗ %d failed", sum.failed+sum.killed)))
	}
	if sum.pending > 0 {
		parts = append(parts, statusPending.Render(fmt.Sprintf("○ %d pending", sum.pending)))
	}

	line := strings.Join(parts, muted.Render("  ·  "))
	if finished == total && sum.running == 0 && m.synthPhase == synthesisDirty && !m.cfg.AutoSynthesize {
		line += muted.Render("  —  press ") + keyStyle.Render("s") + muted.Render(" to synthesize")
	}
	if finished == total && sum.running == 0 && m.synthPhase == synthesisRunning {
		line += muted.Render("  —  ") + statusRunning.Render("synthesizing final report")
	}

	bar := progressBar(pct, min(24, m.width-30), finished == total && sum.running == 0)
	return lipgloss.JoinHorizontal(lipgloss.Center, line, "  ", bar, muted.Render(fmt.Sprintf(" %d%%", pct)))
}

func (m *Model) renderTabs() string {
	tabs := []struct {
		mode  viewMode
		label string
	}{
		{viewDashboard, "Dashboard"},
		{viewRaw, "Raw output"},
		{viewLog, "Logs"},
	}
	var parts []string
	for _, t := range tabs {
		label := t.label
		if m.viewMode == t.mode {
			parts = append(parts, tabActive.Render(" "+label+" "))
		} else {
			parts = append(parts, tabInactive.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, muted.Render(" "))
}

func (m *Model) renderTable() string {
	now := time.Now()
	w := max(60, m.width-4)

	widths := []int{2, 20, 10, 7, 8, 8, w - 55}
	if widths[6] < 8 {
		widths[6] = 8
	}

	header := tableRow(widths, []string{
		"",
		headerStyle.Render("Reviewer"),
		headerStyle.Render("State"),
		headerStyle.Render("Time"),
		headerStyle.Render("Raw"),
		headerStyle.Render("Log"),
		headerStyle.Render("Last event"),
	}, false)

	var rows []string
	for i, st := range m.statuses {
		selected := i == m.selected

		marker := " "
		if st.State == reviewer.StateRunning && !selected {
			marker = m.spinner.View()
		}

		label := reviewerLabel(st.ID)
		if selected {
			label = reviewerLabelSelected(st.ID)
		}

		rows = append(rows, tableRow(widths, []string{
			marker,
			label,
			stateBadge(st),
			reviewer.FormatDuration(reviewer.Elapsed(st, now)),
			reviewer.FormatBytes(st.RawBytes),
			reviewer.FormatBytes(st.LogBytes),
			muted.Render(displayEvent(st)),
		}, selected))
	}

	body := header + "\n" + muted.Render(strings.Repeat("─", w)) + "\n" + strings.Join(rows, "\n")
	return panelStyle.Render(body)
}

func (m *Model) renderDetail() string {
	st, ok := m.selectedStatus()
	if !ok {
		return ""
	}

	var lines []string
	switch st.State {
	case reviewer.StateFailed, reviewer.StateKilled:
		lines = append(lines, statusFailed.Render("✗ "+st.LastEvent))
		if st.LastError != "" && st.LastError != st.LastEvent {
			lines = append(lines, muted.Render(st.LastError))
		}
		lines = append(lines, hintStyle.Render("Press r to rerun this reviewer"))
	case reviewer.StateRunning:
		lines = append(lines, statusRunning.Render(m.spinner.View()+" "+st.LastEvent))
	case reviewer.StateDone:
		size := reviewer.FormatBytes(st.RawBytes)
		if st.RawBytes == 0 {
			lines = append(lines, statusDone.Render("✓ done")+muted.Render(" — no output captured"))
			if st.LogBytes > 0 {
				lines = append(lines, hintStyle.Render("Output may be in logs — press → to view"))
			}
		} else {
			lines = append(lines, statusDone.Render("✓ done")+muted.Render(" — ")+pathStyle.Render(size+" review"))
			lines = append(lines, hintStyle.Render("Press enter to read raw output"))
		}
	case reviewer.StatePending:
		lines = append(lines, statusPending.Render("Waiting to start…"))
	}

	rawPath := filepath.Join(m.cfg.RunDir, "raw", st.ID+".md")
	logPath := filepath.Join(m.cfg.RunDir, "logs", st.ID+".err")
	lines = append(
		lines,
		muted.Render("raw  ")+pathStyle.Render(shortPath(rawPath, m.width-10)),
		muted.Render("log  ")+pathStyle.Render(shortPath(logPath, m.width-10)),
	)

	title := detailTitle.Render("Selected: " + st.ID)
	return detailPanel.Width(min(m.width-2, 90)).Render(title + "\n" + strings.Join(lines, "\n"))
}

func (m *Model) renderOutputPane() string {
	id := m.selectedID()
	if id == "" {
		return panelStyle.Render("No reviewer selected")
	}

	st, _ := m.selectedStatus()
	title := "Raw output"
	if m.viewMode == viewLog {
		title = "Log / stderr"
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		headerStyle.Render(title),
		muted.Render("  ·  "),
		lipgloss.NewStyle().Bold(true).Render(id),
		muted.Render("  ·  "),
		stateBadge(st),
	)

	path := filepath.Join(m.cfg.RunDir, "raw", id+".md")
	if m.viewMode == viewLog {
		path = filepath.Join(m.cfg.RunDir, "logs", id+".err")
	}
	pathLine := muted.Render(shortPath(path, m.width-6))

	content := m.viewport.View()
	if m.viewportKey == "" && strings.Contains(content, "Loading") {
		content = muted.Render("Loading…")
	} else if strings.TrimSpace(stripANSI(content)) == "" {
		content = muted.Render("(no output yet)")
	}

	return panelStyle.Width(m.viewport.Width + 2).Render(
		header + "\n" + pathLine + "\n" + muted.Render(strings.Repeat("─", m.viewport.Width)) + "\n" + content,
	)
}

func (m *Model) renderBanner() string {
	if m.synthPhase == synthesisReady {
		path := m.finalReportPath()
		return bannerStyle.Width(m.width - 2).Render(
			statusDone.Render("✓ Report ready") + "  " + pathStyle.Render(shortPath(path, m.width-20)) +
				muted.Render("  ·  ") + keyStyle.Render("o") + muted.Render(" open"),
		)
	}
	if m.synthPhase == synthesisRunning {
		return synthPromptStyle.Width(m.width - 2).Render(
			statusRunning.Render(m.spinner.View() + " Synthesizing final report…"),
		)
	}
	sum := m.summarize()
	if m.allReviewsFinished() && m.synthPhase == synthesisDirty && !m.cfg.AutoSynthesize {
		msg := hintStyle.Render("All reviewers finished") + muted.Render(" — press ") + keyStyle.Render("s") + muted.Render(" to merge findings into ")
		msg += pathStyle.Render("final-review.md")
		if sum.failed+sum.killed > 0 {
			msg += statusFailed.Render(fmt.Sprintf("  (%d failed — synthesis will note gaps)", sum.failed+sum.killed))
		}
		return synthPromptStyle.Width(m.width - 2).Render(msg)
	}
	return ""
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

func (m *Model) renderFooter() string {
	sum := m.summarize()
	var bindings []string

	switch m.viewMode {
	case viewDashboard:
		bindings = append(bindings, keyStyle.Render("↑↓")+" select")
		bindings = append(bindings, keyStyle.Render("←→")+muted.Render("/")+keyStyle.Render("tab")+" tabs")
		bindings = append(bindings, keyStyle.Render("enter")+" output")
		if sum.running > 0 {
			bindings = append(bindings, keyStyle.Render("k")+" kill")
		}
		if sum.failed+sum.killed > 0 {
			bindings = append(bindings, keyStyle.Render("r")+" rerun")
		}
		if m.synthPhase == synthesisDirty && !m.cfg.AutoSynthesize {
			bindings = append(bindings, keyStyle.Render("s")+" synth ★")
		} else {
			bindings = append(bindings, keyStyle.Render("s")+" synth")
		}
	default:
		bindings = append(bindings, keyStyle.Render("←→")+muted.Render("/")+keyStyle.Render("tab")+" tabs")
		bindings = append(bindings, keyStyle.Render("↑↓")+muted.Render("/")+keyStyle.Render("u/d")+" scroll")
		if sum.failed+sum.killed > 0 {
			bindings = append(bindings, keyStyle.Render("r")+" rerun")
		}
	}

	if m.synthPhase == synthesisReady || fileExists(m.finalReportPath()) {
		bindings = append(bindings, keyStyle.Render("o")+" open")
	}
	bindings = append(bindings, keyStyle.Render("q")+" quit")
	return helpStyle.Render(strings.Join(bindings, muted.Render("  ·  ")))
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

func stateBadge(st reviewer.Status) string {
	switch st.State {
	case reviewer.StateRunning:
		return statusRunning.Render("running")
	case reviewer.StateDone:
		return statusDone.Render("done")
	case reviewer.StateFailed:
		return statusFailed.Render("failed")
	case reviewer.StateKilled:
		return statusFailed.Render("killed")
	default:
		return statusPending.Render("pending")
	}
}

func reviewerLabel(id string) string {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 2 {
		return providerStyle.Render(parts[0]) + muted.Render("-") + parts[1]
	}
	return id
}

func reviewerLabelSelected(id string) string {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 2 {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141")).Render(parts[0]) +
			muted.Render("-") +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(parts[1])
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(id)
}

func tableRow(widths []int, cells []string, selected bool) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = padCell(cell, width, selected)
	}
	return strings.Join(parts, "")
}

func padCell(content string, width int, selected bool) string {
	content = truncateVisible(content, width)
	if pad := width - lipgloss.Width(content); pad > 0 {
		content += strings.Repeat(" ", pad)
	}
	if selected {
		return rowSelected.Render(content)
	}
	return content
}

func progressBar(pct, width int, complete bool) string {
	if width < 4 {
		width = 10
	}
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	color := accent
	if complete {
		color = lipgloss.Color("42")
	}
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}

func displayEvent(st reviewer.Status) string {
	if st.State == reviewer.StateFailed {
		if st.LastError != "" {
			return st.LastError
		}
		return st.LastEvent
	}
	if st.State == reviewer.StateRunning {
		switch st.LastEvent {
		case "", "starting", "running", "pending":
			return "working…"
		}
		return st.LastEvent
	}
	switch st.LastEvent {
	case "", "starting", "running", "pending", "completed", "killed":
		return "—"
	}
	return st.LastEvent
}

func shortPath(path string, maxLen int) string {
	if maxLen < 20 {
		maxLen = 40
	}
	if len(path) <= maxLen {
		return path
	}
	keep := maxLen - 1
	head := keep * 2 / 3
	tail := keep - head
	return path[:head] + "…" + path[len(path)-tail:]
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func truncateVisible(s string, max int) string {
	if max <= 1 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	trimmed := s
	for lipgloss.Width(trimmed) > max-1 {
		trimmed = strings.TrimSpace(trimmed[:len(trimmed)-1])
		if trimmed == "" {
			return "…"
		}
	}
	return trimmed + "…"
}

func stripANSI(s string) string {
	var out strings.Builder
	esc := false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func openFile(path string) error {
	return exec.Command("open", path).Run()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

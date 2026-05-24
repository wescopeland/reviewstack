package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

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

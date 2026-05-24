package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/wescopeland/reviewstack/internal/reviewer"
)

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

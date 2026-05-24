package launcher

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wescopeland/reviewstack/internal/cli"
)

type screen int

const (
	screenMode screen = iota
	screenPRList
	screenPRManual
	screenRereviewPRList
	screenRereviewPRManual
	screenBranch
)

type prsLoadedMsg struct {
	items []PRItem
	err   error
}

type Model struct {
	screen      screen
	selected    int
	prs         []PRItem
	prsLoading  bool
	prsErr      string
	rereview    bool
	baseInput   textinput.Model
	manualInput textinput.Model
	errMsg      string
	done        bool
	cancelled   bool
	result      Result
	width       int
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	subtitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
)

func New(defaultBase string) *Model {
	base := textinput.New()
	base.Placeholder = defaultBase
	base.SetValue(defaultBase)
	base.Prompt = "base ref: "
	base.CharLimit = 120
	base.Width = 40

	manual := textinput.New()
	manual.Placeholder = "4914"
	manual.Prompt = "PR #: "
	manual.CharLimit = 10
	manual.Width = 20

	return &Model{
		screen:      screenMode,
		baseInput:   base,
		manualInput: manual,
		result: Result{
			Base:   defaultBase,
			Target: "HEAD",
		},
	}
}

func Run(defaultBase string) (Result, error) {
	m := New(defaultBase)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	r := final.(*Model)
	if r.cancelled {
		return Result{Cancelled: true}, nil
	}
	return r.result, nil
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case prsLoadedMsg:
		m.prsLoading = false
		if msg.err != nil {
			m.prsErr = msg.err.Error()
		} else {
			m.prs = msg.items
			m.prsErr = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.screen == screenMode || m.screen == screenPRList || m.screen == screenRereviewPRList {
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			}
			m.screen = screenMode
			m.selected = 0
			m.rereview = false
			m.errMsg = ""
			return m, nil

		case "enter":
			return m.handleEnter()

		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < m.maxSelected() {
				m.selected++
			}
		case "n":
			if m.screen == screenPRList || m.screen == screenRereviewPRList {
				if m.screen == screenPRList {
					m.screen = screenPRManual
				} else {
					m.screen = screenRereviewPRManual
				}
				m.manualInput.SetValue("")
				return m, m.manualInput.Focus()
			}
		}
	}

	if m.screen == screenBranch {
		var cmd tea.Cmd
		m.baseInput, cmd = m.baseInput.Update(msg)
		return m, cmd
	}
	if m.screen == screenPRManual || m.screen == screenRereviewPRManual {
		var cmd tea.Cmd
		m.manualInput, cmd = m.manualInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) maxSelected() int {
	switch m.screen {
	case screenMode:
		return 3
	case screenPRList, screenRereviewPRList:
		if len(m.prs) == 0 {
			return 0
		}
		return len(m.prs) - 1
	default:
		return 0
	}
}

func (m *Model) menuWidth() int {
	if m.width >= 72 {
		return 68
	}
	if m.width >= 40 {
		return m.width - 4
	}
	return 52
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMode:
		switch m.selected {
		case 0:
			m.rereview = false
			m.screen = screenPRList
			m.selected = 0
			m.prsLoading = true
			return m, loadPRs
		case 1:
			m.rereview = true
			m.screen = screenRereviewPRList
			m.selected = 0
			m.prsLoading = true
			return m, loadPRs
		case 2:
			m.result = Result{Mode: cli.ModeUncommitted, Uncommitted: true, Base: "HEAD", Target: "working tree"}
			m.done = true
			return m, tea.Quit
		case 3:
			m.screen = screenBranch
			return m, m.baseInput.Focus()
		}

	case screenPRList, screenRereviewPRList:
		if len(m.prs) == 0 {
			if m.screen == screenPRList {
				m.screen = screenPRManual
			} else {
				m.screen = screenRereviewPRManual
			}
			return m, m.manualInput.Focus()
		}
		if m.selected >= len(m.prs) {
			m.selected = len(m.prs) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		pr := m.prs[m.selected]
		base := pr.BaseRef
		if base == "" {
			base = m.result.Base
		}
		m.result = Result{
			Mode:     cli.ModePR,
			PR:       pr.Number,
			Base:     base,
			Target:   pr.HeadRef,
			Rereview: m.rereview,
		}
		if m.result.Target == "" {
			m.result.Target = "HEAD"
		}
		m.done = true
		return m, tea.Quit

	case screenPRManual, screenRereviewPRManual:
		n, err := strconv.Atoi(strings.TrimSpace(m.manualInput.Value()))
		if err != nil || n <= 0 {
			m.errMsg = "enter a valid PR number"
			return m, nil
		}
		m.result = Result{
			Mode:     cli.ModePR,
			PR:       n,
			Base:     m.result.Base,
			Target:   "HEAD",
			Rereview: m.rereview,
		}
		m.done = true
		return m, tea.Quit

	case screenBranch:
		base := strings.TrimSpace(m.baseInput.Value())
		if base == "" {
			m.errMsg = "base ref is required"
			return m, nil
		}
		m.result = Result{Mode: cli.ModeBranch, Base: base, Target: "HEAD"}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func loadPRs() tea.Msg {
	items, err := FetchPRs(15)
	return prsLoadedMsg{items: items, err: err}
}

func (m *Model) View() string {
	if m.done && m.cancelled {
		return ""
	}

	var sections []string
	sections = append(sections, titleStyle.Render("reviewstack"))
	sections = append(sections, subtitleStyle.Render("What are you reviewing?"))
	sections = append(sections, "")

	switch m.screen {
	case screenMode:
		sections = append(sections, m.renderModeMenu())
	case screenPRList, screenRereviewPRList:
		sections = append(sections, m.renderPRList())
	case screenPRManual, screenRereviewPRManual:
		sections = append(sections, m.renderPRManual())
	case screenBranch:
		sections = append(sections, m.renderBranch())
	}

	if m.errMsg != "" {
		sections = append(sections, errStyle.Render(m.errMsg))
	}
	sections = append(sections, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *Model) renderMenuLine(selected bool, primary, secondary string) string {
	const colMarker = 2
	const colPrimary = 14

	marker := " "
	if selected {
		marker = accentStyle.Render("▸")
	}

	textStyle := labelStyle
	if selected {
		textStyle = selectedStyle
	}

	line := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(colMarker).Render(marker),
		lipgloss.NewStyle().Width(colPrimary).Render(textStyle.Render(truncate(primary, colPrimary))),
	)
	if secondary != "" {
		line += muted.Render(" — " + secondary)
	}
	return line
}

func (m *Model) renderModeMenu() string {
	items := []struct{ title, desc string }{
		{"Pull request", "review an open PR"},
		{"Re-review PR", "check your prior feedback against updates"},
		{"Uncommitted", "staged + unstaged local changes"},
		{"Branch diff", "compare against a base ref"},
	}
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = m.renderMenuLine(i == m.selected, item.title, item.desc)
	}
	return lipgloss.NewStyle().Width(m.menuWidth()).Render(strings.Join(lines, "\n"))
}

func (m *Model) renderPRList() string {
	var lines []string
	header := "Select PR"
	if m.rereview {
		header = "Re-review PR"
	}
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, "")

	if m.prsLoading {
		lines = append(lines, muted.Render("Loading open PRs…"))
		return strings.Join(lines, "\n")
	}
	if m.prsErr != "" {
		lines = append(lines, errStyle.Render(m.prsErr))
		lines = append(lines, muted.Render("Press n to enter a PR number manually."))
		return strings.Join(lines, "\n")
	}
	if len(m.prs) == 0 {
		lines = append(lines, muted.Render("No open PRs found."))
		lines = append(lines, muted.Render("Press n to enter a PR number."))
		return strings.Join(lines, "\n")
	}

	if m.selected >= len(m.prs) {
		m.selected = len(m.prs) - 1
	}
	for i, pr := range m.prs {
		title := truncate(pr.Title, 44)
		meta := fmt.Sprintf("#%d  %s→%s", pr.Number, pr.HeadRef, pr.BaseRef)
		lines = append(lines, m.renderMenuLine(i == m.selected, title, meta))
	}
	return lipgloss.NewStyle().Width(m.menuWidth()).Render(strings.Join(lines, "\n"))
}

func (m *Model) renderPRManual() string {
	return headerStyle.Render("Enter PR number") + "\n\n" + m.manualInput.View()
}

func (m *Model) renderBranch() string {
	return headerStyle.Render("Diff against base") + "\n\n" +
		m.baseInput.View() + "\n\n" +
		muted.Render("Compares base...HEAD (committed branch diff, not working tree).")
}

func (m *Model) renderHelp() string {
	switch m.screen {
	case screenMode:
		return helpStyle.Render(keyStyle.Render("↑↓") + " select  " + keyStyle.Render("enter") + " continue  " + keyStyle.Render("q") + " quit")
	case screenPRList, screenRereviewPRList:
		return helpStyle.Render(keyStyle.Render("↑↓") + " select  " + keyStyle.Render("enter") + " review  " + keyStyle.Render("n") + " type number  " + keyStyle.Render("esc") + " back")
	case screenPRManual, screenRereviewPRManual, screenBranch:
		return helpStyle.Render(keyStyle.Render("enter") + " start  " + keyStyle.Render("esc") + " back")
	default:
		return ""
	}
}

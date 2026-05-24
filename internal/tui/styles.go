package tui

import "github.com/charmbracelet/lipgloss"

var (
	accent = lipgloss.Color("39")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	pathStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	providerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Italic(true)

	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusPending = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	detailPanel = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	detailTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))

	rowSelected = lipgloss.NewStyle().Background(lipgloss.Color("234"))

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("252")).
			Underline(true)

	tabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	bannerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("42")).
			Padding(0, 1)

	synthPromptStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("220")).
				Padding(0, 1)
)

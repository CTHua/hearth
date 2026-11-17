package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	// maxMessages caps the message area; a quarter of the height at most.
	maxMessages   = 8
	messageShare  = 4
	cursorMark    = "> "
	noCursor      = "  "
	checkedBox    = "[x] "
	uncheckedBox  = "[ ] "
	notSet        = "not set"
	staleSource   = "-"
	columnGap     = "  "
	tabSeparator  = " "
	disabledLabel = " (unavailable: %s)"
	// chromeLines are the tab bar, the blank line under it, the status line
	// and the help line.
	chromeLines = 4
)

var (
	activeTab   = lipgloss.NewStyle().Bold(true).Reverse(true)
	inactiveTab = lipgloss.NewStyle().Faint(true)
	heading     = lipgloss.NewStyle().Bold(true)
)

// View renders the screen in the alternate buffer.
func (m *Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

// render lays out the screen so no line is wider than the terminal; the
// body scrolls to keep the cursor visible.
func (m *Model) render() string {
	if m.quitting {
		return ""
	}
	messages := m.messages
	area := min(maxMessages, m.height/messageShare)
	if len(messages) > area {
		messages = messages[len(messages)-area:]
	}
	body, cursorLine := m.body()
	room := max(1, m.height-chromeLines-len(messages))
	start := max(0, cursorLine-room+1)
	end := min(len(body), start+room)
	lines := []string{m.tabBar(), ""}
	lines = append(lines, body[start:end]...)
	lines = append(lines, messages...)
	lines = append(lines, m.statusLine(), m.help())
	clip := lipgloss.NewStyle().MaxWidth(max(1, m.width))
	for i, line := range lines {
		lines[i] = clip.Render(line)
	}
	return strings.Join(lines, lineSeparator)
}

func (m *Model) tabBar() string {
	parts := []string{heading.Render(brand.Name)}
	for t := range tabCount {
		label := fmt.Sprintf(" %d %s ", t+1, tabNames[t])
		if t == m.tab {
			parts = append(parts, activeTab.Render(label))
			continue
		}
		parts = append(parts, inactiveTab.Render(label))
	}
	return strings.Join(parts, tabSeparator)
}

func (m *Model) statusLine() string {
	if m.job == nil {
		return ""
	}
	if m.job.canceling {
		return fmt.Sprintf(msgCanceling, m.job.label)
	}
	return fmt.Sprintf(msgRunning, m.job.label)
}

func (m *Model) help() string {
	if m.form != nil {
		return helpLine(formKeys)
	}
	return helpLine(append(slices.Clone(tabKeys[m.tab]), commonKeys...))
}

// body returns the tab's lines and the index of the cursor line.
func (m *Model) body() ([]string, int) {
	if m.form != nil {
		return m.formLines(), 0
	}
	lines := append([]string(nil), m.snap.problems...)
	var rows []string
	switch m.tab {
	case tabRepos:
		lines, rows = m.reposLines(lines)
	case tabProtocol:
		lines, rows = m.protocolLines(lines)
	case tabSkills:
		return m.skillsLines(lines)
	default:
		lines, rows = m.mcpLines(lines)
	}
	cursorAt := len(lines) + m.cursor[m.tab]
	for i, row := range rows {
		lines = append(lines, m.mark(i)+row)
	}
	return lines, cursorAt
}

func (m *Model) mark(row int) string {
	if row == m.cursor[m.tab] {
		return cursorMark
	}
	return noCursor
}

func (m *Model) formLines() []string {
	f := m.form
	lines := []string{heading.Render(f.title)}
	width := 0
	for _, label := range f.labels {
		width = max(width, len(label))
	}
	for i, label := range f.labels {
		marker := noCursor
		if i == f.focus {
			marker = cursorMark
		}
		lines = append(lines, fmt.Sprintf("%s%-*s  %s", marker, width, label, f.view(i)))
	}
	return lines
}

func (m *Model) reposLines(lines []string) ([]string, []string) {
	if len(m.snap.repos) == 0 {
		return append(lines, fmt.Sprintf("no repos in %s; press a to add one", brand.TeamFile)), nil
	}
	cells := [][]string{}
	for _, row := range m.snap.repos {
		cells = append(cells, []string{row.Repo.Name, row.Repo.Branch, string(row.Status.State), row.Status.Detail})
	}
	header, rows := table([]string{"NAME", "BRANCH", "STATUS", "DETAIL"}, cells)
	return append(lines, noCursor+heading.Render(header)), rows
}

func (m *Model) protocolLines(lines []string) ([]string, []string) {
	current := m.snap.home.Personal.Protocol
	if current == "" {
		current = notSet
	}
	lines = append(lines, fmt.Sprintf("protocol: %s (in %s)", current, brand.LocalFile), "")
	detect := "detect from the repo URLs (replaces the current value)"
	if len(m.snap.home.Team.Repos) == 0 {
		detect += fmt.Sprintf(disabledLabel, core.ErrNoRepos.Message)
	}
	return lines, []string{"ssh", "https", detect}
}

func (m *Model) skillsLines(lines []string) ([]string, int) {
	var rows []string
	for _, agent := range config.Agents() {
		rows = append(rows, box(slices.Contains(m.snap.home.Personal.Agents, agent))+agent)
	}
	cells := [][]string{}
	for _, row := range m.snap.skills {
		source := string(row.Source)
		if source == "" {
			source = staleSource
		}
		cells = append(cells, []string{box(row.Enabled) + row.Name, source, string(row.State), row.Detail})
	}
	header, skillRows := table([]string{"    SKILL", "SOURCE", "STATUS", "DETAIL"}, cells)
	lines = append(lines, heading.Render("agents"))
	agentStart := len(lines)
	for i, row := range rows {
		lines = append(lines, m.mark(i)+row)
	}
	lines = append(lines, "", noCursor+heading.Render(header))
	skillStart := len(lines)
	for i, row := range skillRows {
		lines = append(lines, m.mark(agentRows+i)+row)
	}
	if len(skillRows) == 0 {
		lines = append(lines, noCursor+"no skills")
	}
	cursor := m.cursor[tabSkills]
	if cursor < agentRows {
		return lines, agentStart + cursor
	}
	return lines, skillStart + cursor - agentRows
}

func (m *Model) mcpLines(lines []string) ([]string, []string) {
	if len(m.snap.servers) == 0 {
		return append(lines, fmt.Sprintf("no MCP servers in %s; add one with the CLI", brand.TeamFile)), nil
	}
	cells := [][]string{}
	for _, row := range m.snap.servers {
		transport := row.Transport
		if row.Stale {
			transport = staleSource
		}
		cells = append(cells, []string{box(row.Enabled) + row.Name, transport, strings.Join(row.Warnings(), "; ")})
	}
	header, rows := table([]string{"    SERVER", "TRANSPORT", "WARNINGS"}, cells)
	return append(lines, noCursor+heading.Render(header)), rows
}

func box(on bool) string {
	if on {
		return checkedBox
	}
	return uncheckedBox
}

// table pads every column but the last to its widest cell.
func table(header []string, rows [][]string) (string, []string) {
	widths := make([]int, len(header))
	for _, row := range append([][]string{header}, rows...) {
		for i, cell := range row {
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	format := func(row []string) string {
		var b strings.Builder
		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)
				break
			}
			b.WriteString(cell + strings.Repeat(" ", widths[i]-lipgloss.Width(cell)) + columnGap)
		}
		return b.String()
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, format(row))
	}
	return format(header), lines
}

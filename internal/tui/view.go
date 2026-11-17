package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	// minWidth and minHeight are the smallest terminal the layout is drawn
	// for; below it only a hint is shown.
	minWidth  = 80
	minHeight = 24
	// maxMessages caps the message area; a quarter of the height at most.
	maxMessages  = 8
	messageShare = 4
	gutter       = " "
	selectedBar  = "▌"
	noBar        = " "
	checkedBox   = "[✓]"
	uncheckedBox = "[ ]"
	builtinBox   = "[■]"
	radioOn      = "(•)"
	radioOff     = "( )"
	columnGap    = "  "
	hintGap      = "  "
	listSep      = ", "
	argSep       = " "
	staleSource  = "-"
	notSet       = "not set"
	tabGap       = "   "
	teamGroup    = "│ team "
	tabUnderline = "━"
	ruleLine     = "─"
	badgeMine    = "MINE"
	badgeTeam    = "TEAM"
	markAdded    = "+"
	markModified = "~"
	noMark       = " "
	shortCommit  = 7
	ellipsis     = "…"
	// formWidth is the widest a form or the key sheet is drawn.
	formWidth = 66
	// formChrome is the border and padding around a form's lines;
	// formBorder is the border's top and bottom lines.
	formChrome = 4
	formBorder = 2
	// keyColumn is the width of the key in the key sheet.
	keyColumn      = 10
	keySheetColumn = 34
	disabledLabel  = " (unavailable: %s)"
	// topLines are the tab bar, its underline and the blank line under it;
	// footerLines are the rule, the toast line and the help line.
	topLines    = 3
	footerLines = 3
)

// View renders the screen in the alternate buffer.
func (m *Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

// render lays out the screen so no line is wider than the terminal; the
// body scrolls to keep the cursor visible. A form or the key sheet is drawn
// over the screen, which loses its colors and dims.
func (m *Model) render() string {
	if m.quitting {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmall()
	}
	lines := m.screen()
	switch {
	case m.form != nil:
		return m.overlay(lines, m.formBox())
	case m.showKeys:
		return m.overlay(lines, m.keysBox())
	}
	return strings.Join(lines, lineSeparator)
}

func (m *Model) screen() []string {
	top := append(m.tabBar(), "")
	if m.tab != tabMine {
		if pending := m.pendingLine(); pending != "" {
			top = append(top, pending, "")
		}
	}
	footer := m.footer()
	body, cursorLine := m.body()
	room := max(1, m.height-len(top)-len(footer))
	start := max(0, cursorLine-room+1)
	end := min(len(body), start+room)
	lines := append(top, body[start:end]...)
	for len(lines) < m.height-len(footer) {
		lines = append(lines, "")
	}
	lines = append(lines, footer...)
	clip := lipgloss.NewStyle().MaxWidth(m.width)
	for i, line := range lines {
		lines[i] = clip.Render(line)
	}
	return lines
}

func (m *Model) tooSmall() string {
	lines := []string{
		bold.Render(fmt.Sprintf("%s needs at least %d×%d", brand.Name, minWidth, minHeight)),
		"",
		muted.Render("this terminal is ") + warnText.Render(fmt.Sprintf("%d×%d", m.width, m.height)),
		muted.Render("make the window larger, or ") + accentBold.Render(keys.Quit.Help().Key) + muted.Render(" to quit"),
	}
	width := max(1, m.width)
	height := max(1, m.height)
	placed := strings.Split(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, strings.Join(lines, lineSeparator)), lineSeparator)
	placed = placed[:min(len(placed), height)]
	clip := lipgloss.NewStyle().MaxWidth(width)
	for i, line := range placed {
		placed[i] = clip.Render(line)
	}
	return strings.Join(placed, lineSeparator)
}

// overlay centers box over the screen, whose colors are dropped and which
// is drawn dim.
func (m *Model) overlay(lines []string, box string) string {
	background := dim.Render(plain(strings.Join(lines, lineSeparator)))
	x := max(0, (m.width-lipgloss.Width(box))/2)
	y := max(0, (m.height-lipgloss.Height(box))/2)
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(background), lipgloss.NewLayer(box).X(x).Y(y).Z(1)))
	return canvas.Render()
}

// tabBar is the tab names with the team group marked, the file the tab
// edits on the right, and an underline that marks the current tab.
func (m *Model) tabBar() []string {
	top := gutter + accentBold.Render(brand.Name) + tabGap
	under := strings.Repeat(" ", lipgloss.Width(top))
	for t := range tabCount {
		if t == tabRepos {
			top += dim.Render(teamGroup)
			under += dim.Render(strings.Repeat(ruleLine, lipgloss.Width(teamGroup)))
		}
		name := tabNames[t]
		if t == m.tab {
			top += bold.Render(name)
			under += accent.Render(strings.Repeat(tabUnderline, lipgloss.Width(name)))
		} else {
			top += muted.Render(name)
			under += dim.Render(strings.Repeat(ruleLine, lipgloss.Width(name)))
		}
		top += tabGap
		under += dim.Render(strings.Repeat(ruleLine, len(tabGap)))
	}
	badge := mineBadge.Render(fmt.Sprintf(" %s · %s ", badgeMine, brand.LocalFile))
	if m.tab != tabMine {
		badge = teamBadge.Render(fmt.Sprintf(" %s · %s ", badgeTeam, brand.TeamFile))
	}
	under += dim.Render(strings.Repeat(ruleLine, max(0, m.width-lipgloss.Width(under))))
	return []string{spread(top, badge+gutter, m.width), under}
}

// pendingLine tells, on the team tabs, that the team file differs from
// HEAD and has to go through a PR; empty when it does not differ.
func (m *Model) pendingLine() string {
	changes := m.snap.changes
	if len(changes) == 0 {
		return ""
	}
	noun := "changes"
	if len(changes) == 1 {
		noun = "change"
	}
	var removed []string
	for _, change := range changes {
		if change.Kind == core.Removed {
			removed = append(removed, change.Name)
		}
	}
	line := gutter + warnText.Render(symbolWarn+" ") + fmt.Sprintf("%s: %d %s since HEAD", brand.TeamFile, len(changes), noun)
	if len(removed) > 0 {
		line += muted.Render(" · removed: ") + errText.Render(strings.Join(removed, listSep))
	}
	return line + muted.Render(" · commit and open a PR")
}

func (m *Model) footer() []string {
	var lines []string
	toast := m.statusLine()
	switch {
	case len(m.messages) == 1:
		toast = m.messageLine(m.messages[0])
	case len(m.messages) > 1:
		messages := m.messages
		area := min(maxMessages, m.height/messageShare)
		if len(messages) > area {
			messages = messages[len(messages)-area:]
		}
		for _, message := range messages {
			lines = append(lines, gutter+m.messageLine(message))
		}
	}
	rule := gutter + dim.Render(strings.Repeat(ruleLine, max(0, m.width-2*len(gutter))))
	help := spread(gutter+hints(tabKeys[m.tab]), hints(commonKeys)+gutter, m.width)
	return append(lines, rule, gutter+toast, help)
}

func (m *Model) messageLine(line string) string {
	if m.pendingRemove != "" {
		return warnText.Render(symbolWarn+" ") + line
	}
	return messageStyle(line).Render(line)
}

func (m *Model) statusLine() string {
	if m.job == nil {
		return ""
	}
	if m.job.canceling {
		return fmt.Sprintf(msgCanceling, m.job.label)
	}
	return accent.Render(fmt.Sprintf(msgRunning, m.job.label))
}

// hints renders key bindings as "key desc" pairs.
func hints(bindings []key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		parts = append(parts, accentBold.Render(help.Key)+" "+muted.Render(help.Desc))
	}
	return strings.Join(parts, hintGap)
}

// spread puts left and right on one line of the given width.
func spread(left string, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	return left + strings.Repeat(" ", max(1, gap)) + right
}

// rows collects a tab's lines and where the cursor row landed.
type rows struct {
	lines      []string
	cursor     int
	cursorLine int
	next       int
	width      int
}

func (b *rows) text(line string) {
	b.lines = append(b.lines, line)
}

// item adds a row the cursor can stop on.
func (b *rows) item(cells []cell, widths []int) {
	selected := b.next == b.cursor
	if selected {
		b.cursorLine = len(b.lines)
	}
	b.lines = append(b.lines, row(cells, widths, selected, b.width))
	b.next++
}

func (b *rows) header(titles []string, widths []int) {
	cells := make([]cell, 0, len(titles))
	for _, title := range titles {
		cells = append(cells, cell{title, colorMuted})
	}
	b.text(row(cells, widths, false, b.width))
}

// row pads every cell but the last to its column; a selected row has the
// bar and a highlight up to the screen's edge.
func row(cells []cell, widths []int, selected bool, width int) string {
	base := lipgloss.NewStyle()
	bar := noBar
	if selected {
		base = base.Background(colorSelected)
		bar = base.Foreground(colorAccent).Render(selectedBar)
	}
	var b strings.Builder
	b.WriteString(gutter + bar + base.Render(" "))
	used := len(gutter) + lipgloss.Width(selectedBar) + 1
	for i, c := range cells {
		text := c.text
		if i < len(cells)-1 {
			text += strings.Repeat(" ", max(0, widths[i]-lipgloss.Width(text))) + columnGap
		}
		style := base
		if c.color != nil {
			style = style.Foreground(c.color)
		}
		b.WriteString(style.Render(text))
		used += lipgloss.Width(text)
	}
	if selected && used < width {
		b.WriteString(base.Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
}

// columnWidths is each column's widest cell over the header and all rows.
func columnWidths(header []string, sets ...[][]cell) []int {
	widths := make([]int, len(header))
	for i, title := range header {
		widths[i] = lipgloss.Width(title)
	}
	for _, set := range sets {
		for _, cells := range set {
			for i, c := range cells {
				if i < len(widths) {
					widths[i] = max(widths[i], lipgloss.Width(c.text))
				}
			}
		}
	}
	return widths
}

// body returns the tab's lines and the index of the cursor line.
func (m *Model) body() ([]string, int) {
	b := &rows{cursor: m.cursor[m.tab], width: m.width}
	for _, problem := range m.snap.problems {
		b.text(gutter + messageStyle(problem).Render(problem))
	}
	switch m.tab {
	case tabMine:
		m.mineLines(b)
	case tabRepos:
		m.reposLines(b)
	case tabSkills:
		m.skillsLines(b)
	default:
		m.mcpLines(b)
	}
	return b.lines, b.cursorLine
}

func box(on bool) cell {
	if on {
		return cell{checkedBox, colorOK}
	}
	return cell{uncheckedBox, colorMuted}
}

func radio(on bool, label string) []cell {
	if on {
		return []cell{{radioOn, colorAccent}, {label, nil}}
	}
	return []cell{{radioOff, colorMuted}, {label, colorMuted}}
}

func (m *Model) mineLines(b *rows) {
	skillHeader := []string{"", "SKILL", "SOURCE", "STATUS", "DETAIL"}
	serverHeader := []string{"", "MCP SERVER", "TRANSPORT", "STATUS", "DETAIL"}
	skillCells := m.skillCells()
	serverCells := m.serverCells()
	// One set of widths keeps the two tables' columns aligned; the server
	// header is the wider of the two.
	widths := columnWidths(serverHeader, skillCells, serverCells)
	personal := m.snap.home.Personal
	protocol := personal.Protocol
	if protocol == "" {
		protocol = notSet
	}
	b.header([]string{"", fmt.Sprintf("PROTOCOL (%s: %s)", brand.LocalFile, protocol)}, widths)
	detect := "detect from the repo URLs"
	if len(m.snap.home.Team.Repos) == 0 {
		detect += fmt.Sprintf(disabledLabel, core.ErrNoRepos.Message)
	}
	b.item(radio(personal.Protocol == giturl.SSH, giturl.SSH), widths)
	b.item(radio(personal.Protocol == giturl.HTTPS, giturl.HTTPS), widths)
	b.item(radio(false, detect), widths)
	b.text("")
	b.header([]string{"", "AGENTS"}, widths)
	for _, agent := range config.Agents() {
		b.item([]cell{box(slices.Contains(personal.Agents, agent)), {agent, nil}}, widths)
	}
	b.text("")
	b.header(skillHeader, widths)
	for _, cells := range skillCells {
		b.item(cells, widths)
	}
	if len(skillCells) == 0 {
		b.text(gutter + muted.Render("   no skills"))
	}
	b.text("")
	b.header(serverHeader, widths)
	for _, cells := range serverCells {
		b.item(cells, widths)
	}
	if len(serverCells) == 0 {
		b.text(gutter + muted.Render(fmt.Sprintf("   no MCP servers in %s; add one on the %s tab", brand.TeamFile, tabNames[tabMCP])))
	}
}

func (m *Model) skillCells() [][]cell {
	var cells [][]cell
	for _, row := range m.snap.skills {
		check := box(row.Enabled)
		if row.Source == skill.Builtin {
			check = cell{builtinBox, colorDim}
		}
		source := string(row.Source)
		if source == "" {
			source = staleSource
		}
		cells = append(cells, []cell{check, {row.Name, nil}, {source, colorMuted}, statusCell(string(row.State)), {row.Detail, colorMuted}})
	}
	return cells
}

func (m *Model) serverCells() [][]cell {
	var cells [][]cell
	for _, row := range m.snap.servers {
		state, detail := serverState(row)
		check := box(row.Enabled)
		transport := row.Transport
		if row.Stale {
			check = cell{checkedBox, colorWarn}
			transport = staleSource
		}
		cells = append(cells, []cell{check, {row.Name, nil}, {transport, colorMuted}, statusCell(state), {detail, colorMuted}})
	}
	return cells
}

// serverState is an MCP row's state word and the reason for it.
func serverState(row mcp.Row) (string, string) {
	warnings := strings.Join(row.Warnings(), "; ")
	switch {
	case row.Stale:
		return stateStale, fmt.Sprintf("no such server in %s; space drops the choice", brand.TeamFile)
	case !row.Enabled:
		return stateOff, warnings
	case row.MissingCommand != "":
		return stateCommandMissing, warnings
	case len(row.MissingEnv) > 0:
		return stateEnvMissing, warnings
	}
	return stateOK, ""
}

// changeCell marks a team-file entry that differs from HEAD.
func (m *Model) changeCell(table core.TeamTable, name string) cell {
	for _, change := range m.snap.changes {
		if change.Table != table || change.Name != name {
			continue
		}
		switch change.Kind {
		case core.Added:
			return cell{markAdded, colorOK}
		case core.Modified:
			return cell{markModified, colorWarn}
		}
	}
	return cell{noMark, nil}
}

func (m *Model) reposLines(b *rows) {
	if len(m.snap.repos) == 0 {
		b.text(gutter + muted.Render(fmt.Sprintf("no repos in %s; press a to add one", brand.TeamFile)))
		return
	}
	header := []string{"", "NAME", "BRANCH", "STATUS", "DETAIL"}
	var cells [][]cell
	for _, row := range m.snap.repos {
		cells = append(cells, []cell{m.changeCell(core.TableRepos, row.Repo.Name), {row.Repo.Name, nil}, {row.Repo.Branch, colorMuted}, statusCell(string(row.Status.State)), {row.Status.Detail, colorMuted}})
	}
	m.teamTable(b, header, cells)
}

func (m *Model) skillsLines(b *rows) {
	entries := m.snap.home.Team.Skills
	if len(entries) == 0 {
		b.text(gutter + muted.Render(fmt.Sprintf("no external skills in %s; press a to add one", brand.TeamFile)))
	}
	header := []string{"", "NAME", "PIN", "SUBDIR", "URL"}
	var cells [][]cell
	for _, entry := range entries {
		cells = append(cells, []cell{m.changeCell(core.TableSkills, entry.Name), {entry.Name, nil}, {shortSHA(entry.Commit), colorMuted}, {entry.Subdir, colorMuted}, {entry.URL, colorMuted}})
	}
	if len(cells) > 0 {
		m.teamTable(b, header, cells)
	}
	b.text("")
	b.text(gutter + dim.Render(fmt.Sprintf("external skills only; team skills are folders under %s/ and are edited as files; on or off for yourself is on %s", skill.TeamDir, tabNames[tabMine])))
}

func (m *Model) mcpLines(b *rows) {
	servers := m.snap.home.Team.MCP
	if len(servers) == 0 {
		b.text(gutter + muted.Render(fmt.Sprintf("no MCP servers in %s; press a to add one", brand.TeamFile)))
	}
	header := []string{"", "NAME", "TRANSPORT", "COMMAND / URL", "ENV"}
	var cells [][]cell
	for _, server := range servers {
		target := server.URL
		if server.Transport == config.TransportStdio {
			target = strings.Join(append([]string{server.Command}, server.Args...), argSep)
		}
		cells = append(cells, []cell{m.changeCell(core.TableMCP, server.Name), {server.Name, nil}, {server.Transport, colorMuted}, {target, colorMuted}, {strings.Join(server.EnvNames(), listSep), colorMuted}})
	}
	if len(cells) > 0 {
		m.teamTable(b, header, cells)
	}
	b.text("")
	b.text(gutter + dim.Render(fmt.Sprintf("secrets are stored as variable names only; on or off for yourself is on %s", tabNames[tabMine])))
}

func (m *Model) teamTable(b *rows, header []string, cells [][]cell) {
	widths := columnWidths(header, cells)
	b.header(header, widths)
	for _, row := range cells {
		b.item(row, widths)
	}
}

func shortSHA(commit string) string {
	if len(commit) > shortCommit {
		return commit[:shortCommit]
	}
	return commit
}

// formBox is the form as a bordered box: the focused label in the accent
// color, and each blank field showing what it defaults to.
func (m *Model) formBox() string {
	f := m.form
	inner := min(formWidth, m.width) - formChrome
	labelWidth := 0
	for _, label := range f.labels {
		labelWidth = max(labelWidth, lipgloss.Width(label))
	}
	fieldWidth := max(1, inner-labelWidth-len(columnGap))
	hintValues := f.hints(f.texts())
	field := lipgloss.NewStyle().Background(colorSelected)
	wrap := lipgloss.NewStyle().Width(inner)
	rest := []string{dim.Render(strings.Repeat(ruleLine, inner)), wrap.Foreground(colorMuted).Render(f.note), "", hints(formKeys)}
	var problems []string
	for _, problem := range f.problems {
		problems = append(problems, strings.Split(wrap.Foreground(colorErr).Render(problem), lineSeparator)...)
	}
	// Errors get only the rows the title, fields, note and key hints leave
	// (each followed by one blank line); the rest is cut with an ellipsis.
	room := m.height - (2 + len(f.labels) + 1 + lipgloss.Height(strings.Join(rest, lineSeparator)) + formBorder + 1)
	if len(problems) > room {
		problems = append(problems[:max(0, room-1)], errText.Render(ellipsis))
	}
	tail := problems
	if len(problems) > 0 {
		tail = append(tail, "")
	}
	tail = append(tail, rest...)
	// Fields get a blank line between them when the box still fits the
	// terminal, so long errors do not push the key hints off screen.
	spacer := []string{""}
	if 2+2*len(f.labels)+lipgloss.Height(strings.Join(tail, lineSeparator))+formBorder > m.height {
		spacer = nil
	}
	lines := []string{accentBold.Render(f.title), ""}
	for i, label := range f.labels {
		labelStyle := lipgloss.NewStyle()
		cursor := ""
		if i == f.focus {
			labelStyle = accentBold
			cursor = fieldCursor
		}
		value := string(f.values[i])
		text := fitTail(value, fieldWidth-len(cursor)) + cursor
		style := field
		if value == "" {
			text = fitHead(cursor+hintValues[i], fieldWidth)
			style = field.Foreground(colorMuted)
		}
		text += strings.Repeat(" ", max(0, fieldWidth-lipgloss.Width(text)))
		lines = append(lines, labelStyle.Render(label+strings.Repeat(" ", labelWidth-lipgloss.Width(label)))+columnGap+style.Render(text))
		lines = append(lines, spacer...)
	}
	if spacer == nil {
		lines = append(lines, "")
	}
	return framed(append(lines, tail...))
}

// keysBox is the key sheet ? opens.
func (m *Model) keysBox() string {
	columns := make([]string, 0, len(keySheet))
	for _, bindings := range keySheet {
		var lines []string
		for _, binding := range bindings {
			help := binding.Help()
			if help.Key == "" {
				lines = append(lines, dim.Render(strings.ToUpper(help.Desc)))
				continue
			}
			lines = append(lines, accentBold.Render(help.Key+strings.Repeat(" ", max(1, keyColumn-lipgloss.Width(help.Key))))+help.Desc)
		}
		columns = append(columns, lipgloss.NewStyle().Width(keySheetColumn).Render(strings.Join(lines, lineSeparator)))
	}
	lines := []string{accentBold.Render("Keys"), "", lipgloss.JoinHorizontal(lipgloss.Top, columns...), ""}
	lines = append(lines, muted.Render(fmt.Sprintf("%s or %s closes", keys.Help.Help().Key, keys.Cancel.Help().Key)))
	return framed(lines)
}

func framed(lines []string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1).Render(strings.Join(lines, lineSeparator))
}

// fitTail keeps the end of text within width, so the cursor stays in view.
func fitTail(text string, width int) string {
	runes := []rune(text)
	if lipgloss.Width(text) <= width {
		return text
	}
	for len(runes) > 0 && lipgloss.Width(ellipsis+string(runes)) > width {
		runes = runes[1:]
	}
	return ellipsis + string(runes)
}

// fitHead keeps the start of text within width.
func fitHead(text string, width int) string {
	runes := []rune(text)
	if lipgloss.Width(text) <= width {
		return text
	}
	for len(runes) > 0 && lipgloss.Width(string(runes)+ellipsis) > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + ellipsis
}

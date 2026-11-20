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
	"github.com/CTHua/hearth/internal/subagent"
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
	// partialBox is a group title with some of its skills on.
	partialBox = "[-]"
	// groupTitle is a group's row on Mine: its name, then enabled/members.
	groupTitle = "%s (%d/%d)"
	// memberIndent puts a group's skills under its title row.
	memberIndent = "  "
	// groupSize is a group's SUBDIR cell on the Skills tab.
	groupSize    = "group of %d"
	radioOn      = "(•)"
	radioOff     = "( )"
	columnGap    = "  "
	hintGap      = "  "
	listSep      = ", "
	argSep       = " "
	staleSource  = "-"
	notSet       = "not set"
	tabGap       = "   "
	narrowTabGap = "  "
	// minSpread is the fewest spaces between the two sides of a line.
	minSpread     = 1
	mouseOffLabel = "mouse off"
	teamGroup     = "│ team "
	tabUnderline  = "━"
	ruleLine      = "─"
	badgeMine     = "MINE"
	badgeTeam     = "TEAM"
	markAdded     = "+"
	markModified  = "~"
	noMark        = " "
	shortCommit   = 7
	ellipsis      = "…"
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
	// All motion, not just clicks, so what is under the mouse can be marked.
	view.MouseMode = tea.MouseModeAllMotion
	if m.mouseOff {
		view.MouseMode = tea.MouseModeNone
	}
	return view
}

// render lays out the screen so no line is wider than the terminal; the
// body scrolls to keep the cursor visible. A form or the key sheet is drawn
// over the screen, which loses its colors and dims. It records in m.layout
// where it drew what a click can hit.
func (m *Model) render() string {
	m.layout = layout{}
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
	footer, helpZones := m.footer()
	b := m.body()
	room := max(1, m.height-len(top)-len(footer))
	start := max(0, b.cursorLine-room+1)
	end := min(len(b.lines), start+room)
	visible := slices.Clone(b.lines[start:end])
	m.layout.list = area{x: 0, y: len(top), width: m.width, height: room}
	for i, item := range b.items {
		if item.line < start || item.line >= end {
			continue
		}
		y := len(top) + item.line - start
		if m.hovering && m.hover.y == y && i != b.cursor {
			visible[item.line-start] = row(item.cells, item.widths, lookHovered, m.width)
		}
		m.layout.add(zone{area: lineArea(0, y, m.width), row: i})
		// Mine's first column is the checkbox or radio; a click on it is
		// space on that row.
		if m.tab == tabMine {
			m.layout.add(zone{area: lineArea(firstCellX(), y, lipgloss.Width(item.cells[0].text)), row: i, press: bindingPress(keys.Toggle)})
		}
	}
	lines := append(top, visible...)
	for len(lines) < m.height-len(footer) {
		lines = append(lines, "")
	}
	lines = append(lines, footer...)
	helpY := len(lines) - 1
	for _, z := range helpZones {
		z.area.y = helpY
		m.layout.add(z)
	}
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
	m.layout.box = area{x: x, y: y, width: lipgloss.Width(box), height: lipgloss.Height(box)}
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(background), lipgloss.NewLayer(box).X(x).Y(y).Z(1)))
	return canvas.Render()
}

// tabBar is the tab names with the team group marked, the file the tab
// edits on the right, and an underline that marks the current tab. The tab
// under the mouse is underlined.
func (m *Model) tabBar() []string {
	badge := mineBadge.Render(fmt.Sprintf(" %s · %s ", badgeMine, brand.LocalFile))
	if m.tab != tabMine {
		badge = teamBadge.Render(fmt.Sprintf(" %s · %s ", badgeTeam, brand.TeamFile))
	}
	if m.mouseOff {
		badge = warnText.Render(mouseOffLabel) + gutter + badge
	}
	right := badge + gutter
	top, under, zones := m.tabs(tabGap)
	// At 80 columns the Mine badge and mouse off leave no room for the
	// usual gaps; the tabs move closer instead of the badge being cut.
	if lipgloss.Width(top)+lipgloss.Width(right) >= m.width {
		top, under, zones = m.tabs(narrowTabGap)
	}
	for _, z := range zones {
		m.layout.add(z)
	}
	under += dim.Render(strings.Repeat(ruleLine, max(0, m.width-lipgloss.Width(under))))
	return []string{spread(top, right, m.width), under}
}

// tabs draws the tab names gap apart, with the line under them and the
// zone of each name.
func (m *Model) tabs(gap string) (string, string, []zone) {
	top := gutter + accentBold.Render(brand.Name) + gap
	under := strings.Repeat(" ", lipgloss.Width(top))
	zones := make([]zone, 0, tabCount)
	for t := range tabCount {
		if t == tabRepos {
			top += dim.Render(teamGroup)
			under += dim.Render(strings.Repeat(ruleLine, lipgloss.Width(teamGroup)))
		}
		name := tabNames[t]
		place := lineArea(lipgloss.Width(top), 0, lipgloss.Width(name))
		zones = append(zones, zone{area: place, row: noRow, press: tabPress(t)})
		style := muted
		line := dim.Render(strings.Repeat(ruleLine, lipgloss.Width(name)))
		if t == m.tab {
			style = bold
			line = accent.Render(strings.Repeat(tabUnderline, lipgloss.Width(name)))
		}
		if m.hovering && place.contains(m.hover) {
			style = style.Underline(true)
		}
		top += style.Render(name) + gap
		under += line + dim.Render(strings.Repeat(ruleLine, len(gap)))
	}
	return top, under, zones
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

// footer is the message area, the rule, the toast and the help line, with
// the zones of the help line's keys; their y is left for screen to set.
func (m *Model) footer() ([]string, []zone) {
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
	help, zones := m.helpLine()
	return append(lines, rule, gutter+toast, help), zones
}

// helpLine is the tab's keys on the left and the common keys on the right;
// the tab's last keys are dropped when both do not fit.
func (m *Model) helpLine() (string, []zone) {
	left := tabKeys[m.tab]
	right := hints(commonKeys) + gutter
	for len(left) > 0 && lipgloss.Width(gutter+hints(left))+minSpread+lipgloss.Width(right) > m.width {
		left = left[:len(left)-1]
	}
	leftText := gutter + hints(left)
	zones := hintZones(left, len(gutter))
	rightX := lipgloss.Width(leftText) + max(minSpread, m.width-lipgloss.Width(leftText)-lipgloss.Width(right))
	zones = append(zones, hintZones(commonKeys, rightX)...)
	return spread(leftText, right, m.width), zones
}

// hintZones are the zones of hints(bindings) drawn from x on.
func hintZones(bindings []key.Binding, x int) []zone {
	zones := make([]zone, 0, len(bindings))
	for _, binding := range bindings {
		width := lipgloss.Width(hint(binding))
		zones = append(zones, zone{area: lineArea(x, 0, width), row: noRow, press: bindingPress(binding)})
		x += width + len(hintGap)
	}
	return zones
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
		parts = append(parts, hint(binding))
	}
	return strings.Join(parts, hintGap)
}

func hint(binding key.Binding) string {
	help := binding.Help()
	return accentBold.Render(help.Key) + " " + muted.Render(help.Desc)
}

// spread puts left and right on one line of the given width.
func spread(left string, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	return left + strings.Repeat(" ", max(minSpread, gap)) + right
}

// rows collects a tab's lines and where the cursor row landed.
type rows struct {
	lines      []string
	items      []rowItem
	cursor     int
	cursorLine int
	width      int
}

// rowItem is a row the cursor can stop on, kept so it can be drawn again
// under the mouse.
type rowItem struct {
	line   int
	cells  []cell
	widths []int
}

func (b *rows) text(line string) {
	b.lines = append(b.lines, line)
}

// item adds a row the cursor can stop on.
func (b *rows) item(cells []cell, widths []int) {
	look := lookPlain
	if len(b.items) == b.cursor {
		look = lookSelected
		b.cursorLine = len(b.lines)
	}
	b.items = append(b.items, rowItem{line: len(b.lines), cells: cells, widths: widths})
	b.lines = append(b.lines, row(cells, widths, look, b.width))
}

func (b *rows) header(titles []string, widths []int) {
	cells := make([]cell, 0, len(titles))
	for _, title := range titles {
		cells = append(cells, cell{title, colorMuted})
	}
	b.text(row(cells, widths, lookPlain, b.width))
}

// rowLook is how a row is highlighted.
type rowLook int

const (
	lookPlain rowLook = iota
	// lookSelected is the cursor row: the bar and the selected background.
	lookSelected
	// lookHovered is the row under the mouse: a fainter background.
	lookHovered
)

// cellPad is between the bar and the first cell.
const cellPad = " "

// firstCellX is the column a row's first cell starts at.
func firstCellX() int {
	return len(gutter) + lipgloss.Width(selectedBar) + len(cellPad)
}

// row pads every cell but the last to its column and cuts the last one
// with an ellipsis at the screen's edge; a highlighted row is filled up to
// that edge.
func row(cells []cell, widths []int, look rowLook, width int) string {
	base := lipgloss.NewStyle()
	bar := noBar
	switch look {
	case lookSelected:
		base = base.Background(colorSelected)
		bar = base.Foreground(colorAccent).Render(selectedBar)
	case lookHovered:
		base = base.Background(colorHover)
		bar = base.Render(noBar)
	}
	var b strings.Builder
	b.WriteString(gutter + bar + base.Render(cellPad))
	used := firstCellX()
	for i, c := range cells {
		text := c.text
		if i < len(cells)-1 {
			text += strings.Repeat(" ", max(0, widths[i]-lipgloss.Width(text))) + columnGap
		} else {
			text = fitWidth(text, width-used)
		}
		style := base
		if c.color != nil {
			style = style.Foreground(c.color)
		}
		b.WriteString(style.Render(text))
		used += lipgloss.Width(text)
	}
	if look != lookPlain && used < width {
		b.WriteString(base.Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
}

// fitWidth is fitHead that gives nothing when there is no room at all,
// where even the ellipsis would not fit.
func fitWidth(text string, width int) string {
	if width < lipgloss.Width(ellipsis) {
		return ""
	}
	return fitHead(text, width)
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
func (m *Model) body() *rows {
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
	return b
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
	subagentHeader := []string{"", "SUBAGENT", "", "STATUS", "DETAIL"}
	skillCells := m.skillCells()
	serverCells := m.serverCells()
	subagentCells := m.subagentCells()
	// One set of widths keeps the tables' columns aligned; the server
	// header is the widest.
	widths := columnWidths(serverHeader, skillCells, serverCells, subagentCells)
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
	b.text("")
	b.header(subagentHeader, widths)
	for _, cells := range subagentCells {
		b.item(cells, widths)
	}
	if len(subagentCells) == 0 {
		b.text(gutter + muted.Render(fmt.Sprintf("   no subagents; the team adds one as %s/<name>.md", subagent.SourceDir)))
	}
}

func (m *Model) subagentCells() [][]cell {
	var cells [][]cell
	for _, row := range m.snap.subagents {
		check := box(row.Enabled)
		switch {
		case row.Stale:
			check = cell{checkedBox, colorWarn}
		case row.Invalid != "" && !row.Enabled:
			check = cell{uncheckedBox, colorDim}
		}
		cells = append(cells, []cell{check, {row.Name, nil}, {"", nil}, statusCell(string(row.State)), {row.FullDetail(), colorMuted}})
	}
	return cells
}

func (m *Model) skillCells() [][]cell {
	var cells [][]cell
	for _, line := range m.skillLines() {
		if line.skill < 0 {
			on, total := m.groupCounts(line)
			check := box(on == total)
			if on > 0 && on < total {
				check = cell{partialBox, colorWarn}
			}
			title := fmt.Sprintf(groupTitle, line.group, on, total)
			cells = append(cells, []cell{check, {title, nil}, {string(skill.External), colorMuted}, {"", nil}, {"", nil}})
			continue
		}
		row := m.snap.skills[line.skill]
		name := row.Name
		if row.Group != "" {
			name = memberIndent + name
		}
		check := box(row.Enabled)
		if row.Source == skill.Builtin {
			check = cell{builtinBox, colorDim}
		}
		source := string(row.Source)
		if source == "" {
			source = staleSource
		}
		cells = append(cells, []cell{check, {name, nil}, {source, colorMuted}, statusCell(string(row.State)), {row.Detail, colorMuted}})
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
		// A group is one row: d and u act on the whole entry.
		subdir := entry.Subdir
		if entry.IsGroup() {
			subdir = fmt.Sprintf(groupSize, len(entry.Subdirs))
		}
		cells = append(cells, []cell{m.changeCell(core.TableSkills, entry.Name), {entry.Name, nil}, {shortSHA(entry.Commit), colorMuted}, {subdir, colorMuted}, {entry.URL, colorMuted}})
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

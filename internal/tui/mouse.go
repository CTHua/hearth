package tui

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	// The clipboard goes through OSC 52, which a terminal may ignore
	// without telling, so the toast says sent, not copied.
	msgSent          = "sent %q to the clipboard"
	msgSentLines     = "sent %d lines to the clipboard"
	msgNothingToCopy = "nothing to copy: this row has no URL or message"
	msgMouseOff      = "mouse off: drag to select text; m turns it back on"
	msgMouseOn       = "mouse on"
	// noRow marks a zone that does not stand for a row.
	noRow = -1
)

// namedKeys are the keys a click can stand for whose name is not the text
// they type; every other key in keys is one character.
var namedKeys = map[string]tea.KeyPressMsg{
	"tab":   {Code: tea.KeyTab},
	"space": {Code: tea.KeySpace, Text: " "},
	"esc":   {Code: tea.KeyEscape},
}

type point struct {
	x int
	y int
}

// area is a rectangle of screen cells.
type area struct {
	x      int
	y      int
	width  int
	height int
}

func (a area) contains(p point) bool {
	return p.x >= a.x && p.x < a.x+a.width && p.y >= a.y && p.y < a.y+a.height
}

// zone is a place a click can hit: it selects row, when there is one,
// then acts as if press were pressed, when there is one.
type zone struct {
	area  area
	row   int
	press *tea.KeyPressMsg
}

// layout is what the last render drew where; clicks are checked against
// it, so they land on what the user sees.
type layout struct {
	zones []zone
	// list is where the tab's rows are drawn, for the wheel.
	list area
	// box is the form or key sheet over the screen; empty when none is.
	box area
}

func (l *layout) add(z zone) {
	l.zones = append(l.zones, z)
}

// at is the zone at p; later zones lie on top of earlier ones, as a
// checkbox does on its row.
func (l *layout) at(p point) (zone, bool) {
	for i := len(l.zones) - 1; i >= 0; i-- {
		if l.zones[i].area.contains(p) {
			return l.zones[i], true
		}
	}
	return zone{}, false
}

func lineArea(x int, y int, width int) area {
	return area{x: x, y: y, width: width, height: 1}
}

// pressOf is the key press a key's name stands for.
func pressOf(name string) tea.KeyPressMsg {
	if press, found := namedKeys[name]; found {
		return press
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

// bindingPress is the press of binding's first key.
func bindingPress(binding key.Binding) *tea.KeyPressMsg {
	press := pressOf(binding.Keys()[0])
	return &press
}

func tabPress(t tab) *tea.KeyPressMsg {
	press := pressOf(string(rune(firstTabKey + int(t))))
	return &press
}

// mouse handles a click, the wheel and motion. A click does what the key
// it stands for does, so a remove still needs a second d or click.
func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if m.mouseOff {
		return nil
	}
	mouse := msg.Mouse()
	at := point{x: mouse.X, y: mouse.Y}
	switch msg.(type) {
	case tea.MouseMotionMsg:
		m.hover = at
		m.hovering = true
		return nil
	case tea.MouseWheelMsg:
		return m.wheel(at, mouse.Button)
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return nil
		}
		return m.click(at)
	}
	return nil
}

// overlaid reports whether a form or the key sheet is over the screen.
func (m *Model) overlaid() bool {
	return m.form != nil || m.showKeys
}

func (m *Model) wheel(at point, button tea.MouseButton) tea.Cmd {
	// Drawn again so the layout is that of the current state even before
	// the program has asked for a first View.
	m.render()
	if m.overlaid() || !m.layout.list.contains(at) {
		return nil
	}
	switch button {
	case tea.MouseWheelUp:
		m.dropQuestion()
		m.move(-1)
	case tea.MouseWheelDown:
		m.dropQuestion()
		m.move(1)
	}
	return nil
}

// click acts on the zone under the mouse; with a form or the key sheet
// open, a click outside it is esc and one inside does nothing.
func (m *Model) click(at point) tea.Cmd {
	m.render()
	if m.overlaid() {
		if m.layout.box.contains(at) {
			return nil
		}
		return m.key(pressOf(keys.Cancel.Keys()[0]))
	}
	z, found := m.layout.at(at)
	if !found {
		return nil
	}
	if z.row != noRow {
		if z.press == nil {
			m.dropQuestion()
		}
		m.cursor[m.tab] = z.row
	}
	if z.press == nil {
		return nil
	}
	return m.key(*z.press)
}

// toggleMouse gives the mouse to the terminal or takes it back.
func (m *Model) toggleMouse() {
	m.mouseOff = !m.mouseOff
	m.hovering = false
	if m.mouseOff {
		m.messages = []string{msgMouseOff}
		return
	}
	m.messages = []string{msgMouseOn}
}

// copy sends the cursor row's URL, else its message, else the messages on
// screen, to the clipboard.
func (m *Model) copy() tea.Cmd {
	text := m.rowText()
	if text == "" {
		text = strings.Join(m.messages, lineSeparator)
	}
	if text == "" {
		m.messages = []string{msgNothingToCopy}
		return nil
	}
	m.messages = []string{fmt.Sprintf(msgSent, text)}
	if lines := strings.Count(text, lineSeparator) + 1; lines > 1 {
		m.messages = []string{fmt.Sprintf(msgSentLines, lines)}
	}
	return tea.SetClipboard(text)
}

// rowText is the cursor row's URL, or its status message when it has no
// URL; empty when it has neither.
func (m *Model) rowText() string {
	row, ok := m.selected()
	if !ok {
		return ""
	}
	team := m.snap.home.Team
	switch m.tab {
	case tabRepos:
		repo := m.snap.repos[row]
		return cmp.Or(repo.Repo.URL, repo.Status.Detail)
	case tabSkills:
		return team.Skills[row].URL
	case tabMCP:
		return team.MCP[row].URL
	}
	group, index := m.mineRow(row)
	switch group {
	case sectionRepos:
		repo := m.snap.repos[index]
		return cmp.Or(repo.Repo.URL, repo.Status.Detail)
	case sectionSkills:
		line := m.skillLines()[index]
		if line.skill < 0 {
			if entry, found := team.FindSkill(line.group); found {
				return entry.URL
			}
			return ""
		}
		item := m.snap.skills[line.skill]
		entryName := item.Name
		if item.Group != "" {
			entryName = item.Group
		}
		if entry, found := team.FindSkill(entryName); found && item.Source == skill.External {
			return entry.URL
		}
		return item.Detail
	case sectionServers:
		server := m.snap.servers[index]
		if entry, found := team.FindMCP(server.Name); found && entry.Transport == config.TransportHTTP {
			return entry.URL
		}
		_, detail := serverState(server)
		return detail
	case sectionSubagents:
		return m.snap.subagents[index].FullDetail()
	}
	return ""
}

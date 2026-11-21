package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/tui"
)

// hoverSGR is the background of the row under the mouse.
const hoverSGR = "48;2;38;38;38"

// underline starts an underlined cell as lipgloss writes it.
const underline = "\x1b[4;"

// find is where text first shows on the screen, from line from on, as a
// cell column and a line.
func find(t *testing.T, m *tui.Model, from int, text string) (int, int) {
	t.Helper()
	all := lines(m)
	for y := from; y < len(all); y++ {
		if i := strings.Index(all[y], text); i >= 0 {
			return lipgloss.Width(all[y][:i]), y
		}
	}
	t.Fatalf("screen lacks %q:\n%s", text, screen(m))
	return 0, 0
}

// findHint is where text shows on the help line.
func findHint(t *testing.T, m *tui.Model, text string) (int, int) {
	t.Helper()
	return find(t, m, len(lines(m))-1, text)
}

func click(m *tui.Model, x int, y int) tea.Cmd {
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return cmd
}

func wheel(m *tui.Model, y int, button tea.MouseButton) {
	m.Update(tea.MouseWheelMsg{X: 10, Y: y, Button: button})
}

// cursorRow is the screen line with the cursor bar.
func cursorRow(t *testing.T, m *tui.Model) string {
	t.Helper()
	for _, line := range lines(m) {
		if strings.Contains(line, "▌") {
			return line
		}
	}
	t.Fatalf("no cursor row:\n%s", screen(m))
	return ""
}

func TestClickTabsRowsBoxesAndHints(t *testing.T) {
	f := newFake()
	m := newModel(f)
	x, _ := find(t, m, 0, "Skills")
	click(m, x+1, 0)
	expectScreen(t, m, "TEAM · "+brand.TeamFile, "external skills only")
	x, _ = find(t, m, 0, "Mine")
	click(m, x, 0)
	expectScreen(t, m, "MINE · "+brand.LocalFile)

	// A click on a row selects it; one on its checkbox is space.
	x, y := find(t, m, 1, config.AgentCodex)
	click(m, x, y)
	expectCalls(t, f)
	if row := cursorRow(t, m); !strings.Contains(row, config.AgentCodex) {
		t.Errorf("the click selected %q", row)
	}
	x, y = find(t, m, 1, "[✓]")
	click(m, x+1, y)
	expectCalls(t, f, "SetAgents []")
	x, y = find(t, m, 1, "( )")
	click(m, x, y)
	expectCalls(t, f, "SetProtocol ssh")

	press(m, "2")
	x, y = find(t, m, 1, "web")
	click(m, x, y)
	press(m, "d", "d")
	expectCalls(t, f, "RemoveRepo web")

	// A click on a help line key is that key.
	press(m, "1")
	finish(t, m, clickHint(t, m, "s clone + sync"))
	expectCalls(t, f, "Sync force=false")
	clickHint(t, m, "tab tabs")
	expectScreen(t, m, "TEAM · "+brand.TeamFile)
	clickHint(t, m, "? keys")
	expectScreen(t, m, "EVERYWHERE")
}

// clickHint clicks the middle of text on the help line.
func clickHint(t *testing.T, m *tui.Model, text string) tea.Cmd {
	t.Helper()
	x, y := findHint(t, m, text)
	return click(m, x+len(text)/2, y)
}

func TestWheelMovesTheCursorOnly(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2")
	_, y := find(t, m, 1, "app")
	wheel(m, y, tea.MouseWheelDown)
	if row := cursorRow(t, m); !strings.Contains(row, "web") {
		t.Errorf("the wheel moved the cursor to %q", row)
	}
	expectScreen(t, m, "TEAM · "+brand.TeamFile)
	if strings.Contains(screen(m), "Add repo") {
		t.Error("the wheel opened a form")
	}
	wheel(m, y, tea.MouseWheelUp)
	if row := cursorRow(t, m); !strings.Contains(row, "app") {
		t.Errorf("the wheel moved the cursor to %q", row)
	}
	// Off the list, the wheel does nothing.
	wheel(m, 0, tea.MouseWheelDown)
	if row := cursorRow(t, m); !strings.Contains(row, "app") {
		t.Errorf("the wheel over the tab bar moved the cursor to %q", row)
	}
	expectCalls(t, f)
}

func TestHoverMarksTabsAndRows(t *testing.T) {
	m := newModel(newFake())
	press(m, "2")
	x, _ := find(t, m, 0, "Skills")
	m.Update(tea.MouseMotionMsg{X: x + 2, Y: 0})
	// The underline starts where Skills is drawn.
	bar := strings.Split(m.View().Content, "\n")[0]
	start := strings.Index(bar, underline)
	if start < 0 || lipgloss.Width(ansiCodes.ReplaceAllString(bar[:start], "")) != x {
		t.Errorf("Skills under the mouse is not underlined: %q", bar)
	}
	_, y := find(t, m, 1, "web")
	m.Update(tea.MouseMotionMsg{X: 5, Y: y})
	raw := strings.Split(m.View().Content, "\n")
	if strings.Contains(raw[0], underline) {
		t.Errorf("a tab stayed underlined after the mouse left: %q", raw[0])
	}
	if !strings.Contains(raw[y], hoverSGR) || strings.Contains(raw[y], selectedSGR) {
		t.Errorf("the row under the mouse is not faintly marked: %q", raw[y])
	}
	if cursor := lines(m); !strings.Contains(cursorRow(t, m), "app") {
		t.Errorf("hovering moved the cursor:\n%s", strings.Join(cursor, "\n"))
	}
	// The cursor row keeps its own highlight under the mouse.
	_, y = find(t, m, 1, "app")
	m.Update(tea.MouseMotionMsg{X: 5, Y: y})
	raw = strings.Split(m.View().Content, "\n")
	if !strings.Contains(raw[y], selectedSGR) {
		t.Errorf("the cursor row lost its highlight under the mouse: %q", raw[y])
	}
}

func TestClickedRemoveStillAsks(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2")
	clickHint(t, m, "d remove")
	expectCalls(t, f)
	expectScreen(t, m, "press d again to remove app")
	clickHint(t, m, "d remove")
	expectCalls(t, f, "RemoveRepo app")
	clickHint(t, m, "d remove")
	press(m, "d")
	expectCalls(t, f, "RemoveRepo app")
	// A click elsewhere drops the question, as any other key does.
	clickHint(t, m, "d remove")
	x, y := find(t, m, 1, "web")
	click(m, x, y)
	if strings.Contains(screen(m), "press d again") {
		t.Error("the question stayed after a click on a row")
	}
	press(m, "d")
	expectCalls(t, f)
	expectScreen(t, m, "press d again to remove web")
}

func TestClickOutsideClosesFormAndKeySheet(t *testing.T) {
	m := newModel(newFake())
	press(m, "2", "a")
	click(m, 0, 0)
	if text := screen(m); strings.Contains(text, "Add repo") || !strings.Contains(text, "closed the form") {
		t.Errorf("a click outside did not close the form:\n%s", text)
	}
	press(m, "a")
	x, y := find(t, m, 0, "Add repo")
	click(m, x, y)
	expectScreen(t, m, "Add repo")
	press(m, "esc", "?")
	x, y = find(t, m, 0, "EVERYWHERE")
	click(m, x, y)
	expectScreen(t, m, "EVERYWHERE")
	click(m, 0, 0)
	if strings.Contains(screen(m), "EVERYWHERE") {
		t.Error("a click outside did not close the key sheet")
	}
}

// expectClipboard checks that cmd sets the clipboard to want.
func expectClipboard(t *testing.T, cmd tea.Cmd, want string) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("no command; want the clipboard set to %q", want)
	}
	if got := cmd(); got != tea.SetClipboard(want)() {
		t.Errorf("command gave %#v; want the clipboard set to %q", got, want)
	}
}

func TestCopySendsURLOrMessage(t *testing.T) {
	f := newFake()
	f.servers[0].MissingCommand = "npx"
	m := newModel(f)
	appURL := f.home.Team.Repos[0].URL
	expectClipboard(t, press(m, "2", "y"), appURL)
	expectScreen(t, m, `sent "`+appURL+`" to the clipboard`)
	expectClipboard(t, press(m, "3", "y"), skillURL)
	expectClipboard(t, press(m, "4", "j", "y"), f.home.Team.MCP[1].URL)
	// Mine: an external skill's URL, a stdio server's message.
	expectClipboard(t, press(m, "1", "down", "down", "down", "down", "down", "down", "down", "y"), skillURL)
	expectClipboard(t, press(m, "down", "down", "down", "y"), "command npx not found in PATH")

	// A row with neither sends the toast; with no toast there is nothing.
	m = newModel(f)
	if cmd := press(m, "y"); cmd != nil {
		t.Errorf("y with nothing to copy gave %#v", cmd())
	}
	expectScreen(t, m, "nothing to copy")
	press(m, "enter")
	expectClipboard(t, press(m, "y"), "protocol: ssh (saved to "+brand.LocalFile+")")
	expectClipboard(t, press(m, "4", "y"), `sent "protocol: ssh (saved to `+brand.LocalFile+`)" to the clipboard`)
}

func TestMouseKeyTurnsTheMouseOff(t *testing.T) {
	f := newFake()
	m := newModel(f)
	if mode := m.View().MouseMode; mode != tea.MouseModeAllMotion {
		t.Errorf("mouse mode is %v", mode)
	}
	press(m, "m")
	if mode := m.View().MouseMode; mode != tea.MouseModeNone {
		t.Errorf("mouse mode with the mouse off is %v", mode)
	}
	for _, tab := range []string{"1", "2"} {
		press(m, tab)
		bar := lines(m)[0]
		if !strings.Contains(bar, "mouse off") || lipgloss.Width(bar) > 80 {
			t.Errorf("tab %s bar at 80 columns: %q", tab, bar)
		}
		if !strings.Contains(bar, "· ") {
			t.Errorf("tab %s lost its badge: %q", tab, bar)
		}
	}
	x, _ := find(t, m, 0, "Mine")
	click(m, x, 0)
	expectScreen(t, m, "TEAM · "+brand.TeamFile)
	press(m, "m")
	if mode := m.View().MouseMode; mode != tea.MouseModeAllMotion {
		t.Errorf("mouse mode after m again is %v", mode)
	}
	if strings.Contains(lines(m)[0], "mouse off") {
		t.Error("mouse off stayed after m again")
	}
}

func TestKeySheetExplainsTheMouse(t *testing.T) {
	m := newModel(newFake())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "?")
	expectScreen(t, m, "MOUSE", "y         copy URL or message", "m         mouse off/on", "Option (iTerm2)", "Shift (Ghostty, WezTerm)", "set -g mouse on", "? or esc closes")
	all := strings.Split(m.View().Content, "\n")
	if len(all) > 24 {
		t.Errorf("the key sheet takes %d lines", len(all))
	}
	for i, line := range all {
		if width := lipgloss.Width(line); width > 80 {
			t.Errorf("line %d is %d wide", i, width)
		}
	}
}

func TestLongDetailEndsWithEllipsis(t *testing.T) {
	f := newFake()
	f.skills[0].Detail = strings.Repeat("very-long-detail-", 10)
	m := newModel(f)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, y := find(t, m, 1, "pdf")
	line := lines(m)[y]
	if !strings.HasSuffix(strings.TrimRight(line, " "), "…") || lipgloss.Width(line) > 100 {
		t.Errorf("the detail is not cut with an ellipsis: %q", line)
	}
}

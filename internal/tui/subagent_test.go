package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/subagent"
	"github.com/CTHua/hearth/internal/tui"
)

// subagentsStart is how many Mine rows come before the subagents: 3
// protocol, 2 agent, 2 repo, 3 skill and 2 server rows.
const subagentsStart = 12

const brokenDetail = "subagents/broken.md: name is missing"

func subagentFake() *fake {
	f := newFake()
	f.subagents = []subagent.Row{
		{Name: "reviewer", State: subagent.Off, Detail: "Reviews a diff."},
		{Name: "helper", Enabled: true, State: subagent.OK, Detail: "Helps."},
		{Name: "broken", State: subagent.Invalid, Detail: brokenDetail, Invalid: brokenDetail},
		{Name: "gone", Enabled: true, Stale: true, State: subagent.Stale, Detail: "no such file"},
	}
	return f
}

// tallModel is a UI tall enough to show every Mine row at once.
func tallModel(f *fake) *tui.Model {
	m := newModel(f)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 50})
	return m
}

// toSubagent moves the cursor to the subagent row at index.
func toSubagent(m *tui.Model, index int) {
	press(m, "1")
	for range subagentsStart + index {
		press(m, "down")
	}
}

func TestSubagentRowsToggleChoices(t *testing.T) {
	f := subagentFake()
	m := tallModel(f)
	expectScreen(t, m, "SUBAGENT", "reviewer", "Reviews a diff.", "subagent-invalid", "stale")
	toSubagent(m, 0)
	press(m, "space")
	press(m, "down", "space")
	expectCalls(t, f, "EnableSubagents [reviewer]", "DisableSubagents [helper]")
	press(m, "down", "space")
	expectCalls(t, f)
	expectScreen(t, m, "cannot enable: "+brokenDetail)
	expectClipboard(t, press(m, "y"), brokenDetail)
	// A stale choice counts as enabled: space drops it.
	press(m, "down", "space")
	expectCalls(t, f, "DisableSubagents [gone]")
	// An invalid one that is still chosen can be turned off.
	f.subagents[2].Enabled = true
	m = tallModel(f)
	toSubagent(m, 2)
	press(m, "space")
	expectCalls(t, f, "DisableSubagents [broken]")
}

func TestClickOnSubagentCheckboxToggles(t *testing.T) {
	f := subagentFake()
	m := tallModel(f)
	_, y := find(t, m, 1, "reviewer")
	x := lipgloss.Width(lines(m)[y][:strings.Index(lines(m)[y], "[ ]")])
	click(m, x+1, y)
	expectCalls(t, f, "EnableSubagents [reviewer]")
	_, y = find(t, m, 1, "broken")
	x = lipgloss.Width(lines(m)[y][:strings.Index(lines(m)[y], "[ ]")])
	click(m, x+1, y)
	expectCalls(t, f)
}

func TestNoSubagentsIsOneLineTheCursorSkips(t *testing.T) {
	f := newFake()
	m := tallModel(f)
	expectScreen(t, m, "no subagents; the team adds one as subagents/<name>.md")
	for range subagentsStart + 3 {
		press(m, "down")
	}
	if row := cursorRow(t, m); !strings.Contains(row, "tracker") {
		t.Errorf("the cursor left the last server row: %q", row)
	}
	press(m, "space")
	expectCalls(t, f, "DisableMCP [tracker]")
}

func TestSubagentListingErrorIsShown(t *testing.T) {
	f := newFake()
	f.subagentsErr = apperr.New(apperr.Config, "state is broken")
	m := newModel(f)
	expectScreen(t, m, "error [config] state is broken")
}

func TestSubagentRowsFitTheTerminal(t *testing.T) {
	f := subagentFake()
	long := strings.Repeat("very-long-detail-", 10)
	f.subagents[0].Detail = long
	f.subagents[1].Name = strings.Repeat("long-name-", 4)
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m := newModel(f)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		toSubagent(m, len(f.subagents)-1)
		content := m.View().Content
		if !strings.Contains(screen(m), "gone") {
			t.Errorf("%dx%d: the cursor row is not on screen:\n%s", size[0], size[1], screen(m))
		}
		lines := strings.Split(content, "\n")
		if len(lines) > size[1] {
			t.Errorf("%dx%d has %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := lipgloss.Width(line); width > size[0] {
				t.Errorf("%dx%d line %d is %d wide: %q", size[0], size[1], i, width, line)
			}
		}
	}
}

func TestInvalidRowWithEditedLeftoverCannotBeEnabled(t *testing.T) {
	f := subagentFake()
	f.subagents[2].State = subagent.Modified
	f.subagents[2].Detail = ".claude/agents/broken.md changed"
	m := tallModel(f)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	expectScreen(t, m, ".claude/agents/broken.md changed; source: "+brokenDetail)
	toSubagent(m, 2)
	press(m, "space")
	expectCalls(t, f)
	expectScreen(t, m, "cannot enable: "+brokenDetail)
}

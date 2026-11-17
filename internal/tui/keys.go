package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

const helpSep = "  "

// keyMap is every key the UI reacts to; the help line is built from it.
type keyMap struct {
	Quit      key.Binding
	Interrupt key.Binding
	NextTab   key.Binding
	PrevTab   key.Binding
	Tabs      key.Binding
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	Choose    key.Binding
	Add       key.Binding
	AddSkill  key.Binding
	Remove    key.Binding
	Clone     key.Binding
	Fix       key.Binding
	SkillSync key.Binding
	Sync      key.Binding
	NextField key.Binding
	PrevField key.Binding
	Save      key.Binding
	Cancel    key.Binding
	Erase     key.Binding
}

var keys = keyMap{
	Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	Interrupt: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel/quit")),
	NextTab:   key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab/1-4", "tabs")),
	PrevTab:   key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "prev tab")),
	Tabs:      key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "tab")),
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up/down", "move")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("down/j", "down")),
	Toggle:    key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	Choose:    key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "choose")),
	Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
	AddSkill:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add skill")),
	Remove:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
	Clone:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clone all")),
	Fix:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix remote")),
	SkillSync: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "skill sync")),
	Sync:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
	NextField: key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "next field")),
	PrevField: key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("shift+tab", "prev field")),
	Save:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
	Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Erase:     key.NewBinding(key.WithKeys("backspace")),
}

// tabKeys are each tab's own keys, shown first in the help line.
// commonKeys put quit before navigation, so a narrow terminal clips the
// navigation hints rather than the way out.
var tabKeys = [tabCount][]key.Binding{
	{keys.Add, keys.Remove, keys.Clone, keys.Fix},
	{keys.Choose},
	{keys.Toggle, keys.AddSkill, keys.SkillSync},
	{keys.Toggle, keys.Sync},
}

var commonKeys = []key.Binding{keys.Quit, keys.Interrupt, keys.NextTab, keys.Up}

var formKeys = []key.Binding{keys.NextField, keys.PrevField, keys.Save, keys.Cancel}

func helpLine(bindings []key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		parts = append(parts, help.Key+" "+help.Desc)
	}
	return strings.Join(parts, helpSep)
}

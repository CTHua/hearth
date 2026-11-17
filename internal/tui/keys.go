package tui

import (
	"charm.land/bubbles/v2/key"
)

// keyMap is every key the UI reacts to; the help line and the key sheet
// are built from it.
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
	Remove    key.Binding
	Update    key.Binding
	Clone     key.Binding
	Fix       key.Binding
	Sync      key.Binding
	Copy      key.Binding
	Mouse     key.Binding
	Help      key.Binding
	NextField key.Binding
	PrevField key.Binding
	Save      key.Binding
	Cancel    key.Binding
	Erase     key.Binding
}

var keys = keyMap{
	Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	Interrupt: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel job / quit")),
	NextTab:   key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "tabs")),
	PrevTab:   key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "previous tab")),
	Tabs:      key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "go to tab")),
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑ k", "move up")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓ j", "move down")),
	Toggle:    key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "use")),
	Choose:    key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "choose protocol")),
	Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
	Remove:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
	Update:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "update pin")),
	Clone:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clone all")),
	Fix:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix remote")),
	Sync:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
	Copy:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy")),
	Mouse:     key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mouse on/off")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),
	NextField: key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "next field")),
	PrevField: key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("shift+tab", "previous field")),
	Save:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
	Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Erase:     key.NewBinding(key.WithKeys("backspace")),
}

// tabKeys are each tab's most used keys, shown on the left of the help
// line, most important first: the last ones are dropped when the line is
// too narrow. commonKeys are on its right on every tab and always kept.
var tabKeys = [tabCount][]key.Binding{
	{keys.Toggle, keys.Sync, keys.Copy},
	{keys.Add, keys.Remove, keys.Clone, keys.Fix, keys.Copy},
	{keys.Add, keys.Remove, keys.Update, keys.Copy},
	{keys.Add, keys.Remove, keys.Copy},
}

var commonKeys = []key.Binding{keys.NextTab, keys.Help, keys.Quit}

var formKeys = []key.Binding{keys.Save, keys.NextField, keys.Cancel}

// keySheet is what ? shows: one column for everywhere and Mine, one for
// the team tabs, forms and the mouse. A desc is used as a heading when its
// key is empty. The sheet is as tall as an 80×24 terminal allows, so a new
// line here needs one taken out.
var keySheet = [][]key.Binding{
	{
		heading("everywhere"), keys.Tabs, keys.NextTab, keys.Up, keys.Down,
		labeled(keys.Copy, "copy URL or message"), labeled(keys.Mouse, "mouse off/on, to select"),
		keys.Interrupt, keys.Quit,
		heading(""), heading("mine"), keys.Toggle, keys.Choose, keys.Sync,
	},
	{
		heading("team"), keys.Add, double(keys.Remove), keys.Update,
		labeled(keys.Clone, "clone all (Repos)"), labeled(keys.Fix, "fix remote (Repos)"),
		heading(""), heading("forms"), keys.Save, keys.NextField, keys.Cancel,
		heading(""), heading("mouse"),
		note("click", "tab, row, box, key hint"),
		note("wheel", "move the cursor"),
		// Terminals pass a drag to the app while it has the mouse; a
		// modifier, or m, gives the drag back to the terminal.
		note("select", "hold Option (iTerm2) or"),
		note(continued, "Shift (Ghostty, WezTerm)"),
		note("tmux", "set -g mouse on"),
	},
}

// continued is the key of a line that goes on from the line above; it is
// not empty, which would make the line a heading.
const continued = " "

func heading(title string) key.Binding {
	return key.NewBinding(key.WithHelp("", title))
}

// note is a key sheet line that is not a key binding, such as a mouse action.
func note(action string, desc string) key.Binding {
	return key.NewBinding(key.WithHelp(action, desc))
}

// double is a key that acts on its second press in a row.
func double(binding key.Binding) key.Binding {
	help := binding.Help()
	return key.NewBinding(key.WithHelp(help.Key+" "+help.Key, help.Desc))
}

func labeled(binding key.Binding, desc string) key.Binding {
	return key.NewBinding(key.WithHelp(binding.Help().Key, desc))
}

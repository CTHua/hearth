package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// fieldCursor marks the end of the focused field. bubbles' textinput is not
// used: it pulls in a clipboard module this build does not depend on.
const fieldCursor = "_"

// form is a few one-line text fields; enter submits them all, esc closes it.
type form struct {
	title  string
	labels []string
	values [][]rune
	focus  int
	submit func(values []string) tea.Cmd
}

func newForm(title string, labels []string) *form {
	return &form{title: title, labels: labels, values: make([][]rune, len(labels))}
}

// repoForm asks for a new repo; blank optional fields get core's defaults.
func repoForm() *form {
	return newForm("add a repo (blank fields use the defaults; a blank branch is asked from the remote)",
		[]string{"url", "name", "path", "branch"})
}

// skillForm asks for a new external skill.
func skillForm() *form {
	return newForm("add an external skill (a blank commit pins the remote's HEAD)",
		[]string{"url", "name", "subdir", "commit"})
}

func (m *Model) openForm(f *form, submit func([]string) tea.Cmd) tea.Cmd {
	f.submit = submit
	m.form = f
	return nil
}

func (m *Model) formKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.form
	switch {
	case key.Matches(msg, keys.Cancel):
		m.form = nil
		m.messages = []string{msgFormClosed}
	case key.Matches(msg, keys.Save):
		m.form = nil
		values := make([]string, 0, len(f.values))
		for _, value := range f.values {
			values = append(values, string(value))
		}
		return f.submit(values)
	case key.Matches(msg, keys.NextField):
		f.focus = (f.focus + 1) % len(f.values)
	case key.Matches(msg, keys.PrevField):
		f.focus = (f.focus + len(f.values) - 1) % len(f.values)
	case key.Matches(msg, keys.Erase):
		value := f.values[f.focus]
		if len(value) > 0 {
			f.values[f.focus] = value[:len(value)-1]
		}
	default:
		f.insert(msg.Text)
	}
	return nil
}

// insert adds typed or pasted text to the focused field.
func (f *form) insert(text string) {
	// Every field is one line; a pasted line break would split the view.
	text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
	f.values[f.focus] = append(f.values[f.focus], []rune(text)...)
}

func (f *form) view(index int) string {
	text := string(f.values[index])
	if index == f.focus {
		return text + fieldCursor
	}
	return text
}

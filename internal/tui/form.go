package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
)

// fieldCursor marks the end of the focused field. bubbles' textinput is not
// used: it pulls in a clipboard module this build does not depend on.
const fieldCursor = "_"

// Hints for blank fields; the ones that depend on other fields are built
// in the hint functions below.
const (
	hintURL         = "git@host:org/repo.git or https://..."
	hintFromURL     = "from the URL"
	hintAskRemote   = "ask the remote"
	hintRemoteHead  = "the remote's HEAD (needs the network)"
	hintTopOfRepo   = config.RootSubdir + " (the top of the repo)"
	hintMCPName     = "letters, digits, - and _"
	hintCommand     = "for a stdio server: command args"
	hintMCPURL      = "or an http server"
	hintEnv         = "VAR, VAR (names only; stdio only)"
	pathSep         = "/"
	noteRepo        = "c clones it later."
	noteSkill       = "a blank commit pins the remote's HEAD."
	noteMCP         = "Fill command or URL, not both. Tokens and headers of an http server are set with the CLI."
	noteUpdateSkill = "A blank commit moves the pin to the remote's HEAD."
	formTitleRepo   = "Add repo"
	formTitleSkill  = "Add external skill"
	formTitleMCP    = "Add MCP server"
	formTitleUpdate = "Update pin of %s"
	labelURL        = "URL"
	labelName       = "Name"
	labelPath       = "Path"
	labelBranch     = "Branch"
	labelSubdir     = "Subdir"
	labelCommit     = "Commit"
	labelCommand    = "Command"
	labelEnv        = "Env"
)

// form is a few one-line text fields; enter submits them all, esc closes it.
type form struct {
	title  string
	note   string
	labels []string
	values [][]rune
	focus  int
	// hints returns what each blank field defaults to, given the values so
	// far, such as the name a URL gives.
	hints func(values []string) []string
	// problems are why the last submit was refused; the form stays open.
	problems []string
	// submit returns an error to keep the form open and show it.
	submit func(values []string) (tea.Cmd, error)
}

func newForm(title string, note string, labels []string, hints func([]string) []string) *form {
	return &form{title: title, note: note, labels: labels, values: make([][]rune, len(labels)), hints: hints}
}

// repoForm asks for a new repo; blank optional fields get core's defaults.
func repoForm() *form {
	return newForm(formTitleRepo, noteRepo, []string{labelURL, labelName, labelPath, labelBranch}, func(values []string) []string {
		name := urlName(values[0])
		path := hintFromURL
		if typed := strings.TrimSpace(values[1]); typed != "" {
			name = typed
		}
		if name != hintFromURL {
			path = config.ProductsDir + pathSep + name
		}
		return []string{hintURL, name, path, hintAskRemote}
	})
}

// skillForm asks for a new external skill; the name hint follows core: the
// last subdir segment, or the URL's name at the top of the repo.
func skillForm() *form {
	return newForm(formTitleSkill, noteSkill, []string{labelURL, labelName, labelSubdir, labelCommit}, func(values []string) []string {
		name := urlName(values[0])
		subdir := strings.Trim(strings.TrimSpace(values[2]), pathSep)
		if subdir != "" && subdir != config.RootSubdir {
			segments := strings.Split(subdir, pathSep)
			name = segments[len(segments)-1]
		}
		return []string{hintURL, name, hintTopOfRepo, hintRemoteHead}
	})
}

// mcpForm asks for a new MCP server; submitMCP turns it into an entry.
func mcpForm() *form {
	return newForm(formTitleMCP, noteMCP, []string{labelName, labelCommand, labelURL, labelEnv}, func([]string) []string {
		return []string{hintMCPName, hintCommand, hintMCPURL, hintEnv}
	})
}

// updateForm asks where to move a skill's pin.
func updateForm(title string) *form {
	return newForm(title, noteUpdateSkill, []string{labelCommit}, func([]string) []string {
		return []string{hintRemoteHead}
	})
}

// urlName is the name core would give a repo or skill at rawURL.
func urlName(rawURL string) string {
	u, err := giturl.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return hintFromURL
	}
	return u.DefaultName()
}

func (m *Model) openForm(f *form, submit func([]string) (tea.Cmd, error)) tea.Cmd {
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
		// The form stays open while an action runs, so nothing typed is lost.
		if m.busy() {
			return nil
		}
		cmd, err := f.submit(f.texts())
		if err != nil {
			f.problems = reportLines(err)
			return nil
		}
		m.form = nil
		return cmd
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

func (f *form) texts() []string {
	values := make([]string, 0, len(f.values))
	for _, value := range f.values {
		values = append(values, string(value))
	}
	return values
}

// insert adds typed or pasted text to the focused field.
func (f *form) insert(text string) {
	// Every field is one line; a pasted line break would split the view.
	text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
	f.values[f.focus] = append(f.values[f.focus], []rune(text)...)
}

// Package tui is the terminal UI: a personal tab and three team tabs over
// core.Actions. It holds no logic of its own beyond choosing which action
// to call.
package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/skill"
)

type tab int

// summaryIndent starts each group line under a summary.
const summaryIndent = "  "

// Mine edits the personal file; the tabs from tabRepos on edit the team file.
const (
	tabMine tab = iota
	tabRepos
	tabSkills
	tabMCP
	tabCount
)

var tabNames = [tabCount]string{"Mine", "Repos", "Skills", "MCP"}

// firstTabKey is the digit of the first tab in keys.Tabs.
const firstTabKey = '1'

// Protocol rows at the top of Mine.
const (
	optionSSH = iota
	optionHTTPS
	optionDetect
	optionCount
)

// agentRows is how many rows the agent checkboxes take on Mine, one per
// agent, right under the protocol rows.
var agentRows = len(config.Agents())

const (
	msgBusy        = "busy: %s is running; wait for it or press ctrl+c to cancel it"
	msgRunning     = "running: %s (ctrl+c cancels)"
	msgCanceling   = "canceling %s; waiting for git to stop"
	msgDone        = "done: %s"
	msgConfirm     = "press d again to remove %s from %s; any other key keeps it"
	msgBuiltin     = "the built-in skill is installed whenever an agent is chosen; it cannot be toggled"
	msgURLRequired = "the URL is required"
	msgFormClosed  = "closed the form; nothing was saved"
	msgNothing     = "nothing selected"
)

// Job labels, shown while a background action runs.
const (
	jobClone       = "clone"
	jobFixRemote   = "fix-remote"
	jobDetect      = "protocol detection"
	jobSync        = "sync"
	jobSkillAdd    = "skill add"
	jobSkillUpdate = "skill update"
	jobRepoAdd     = "repo add"
	problemError   = "error ["
	problemWarn    = "warning ["
	lineSeparator  = "\n"
)

// snapshot is what the screen shows, read after every action.
type snapshot struct {
	home    config.Home
	repos   []core.RepoRow
	skills  []skill.Row
	servers []mcp.Row
	// changes are the team-file entries that differ from HEAD.
	changes []core.TeamChange
	// problems are listing errors, shown above the tab's content.
	problems []string
}

func load(ctx context.Context, actions core.Actions) snapshot {
	snap := snapshot{home: actions.Home(), servers: actions.ListMCP()}
	repos, err := actions.ListRepos(ctx)
	if err != nil {
		snap.problems = append(snap.problems, reportLines(err)...)
	}
	snap.repos = repos
	skills, err := actions.ListSkills(ctx)
	if err != nil {
		snap.problems = append(snap.problems, reportLines(err)...)
	}
	snap.skills = skills
	changes, err := actions.TeamChanges(ctx)
	if err != nil {
		snap.problems = append(snap.problems, reportLines(err)...)
	}
	snap.changes = changes
	return snap
}

// job is the one background action that may run at a time: core.Actions
// is not safe for concurrent use.
type job struct {
	label     string
	cancel    context.CancelFunc
	canceling bool
}

// jobFunc runs a background action. lines are its result lines; teamChanged
// says it rewrote the team file when it succeeds.
type jobFunc func(ctx context.Context, out core.Output) (lines []string, teamChanged bool, err error)

type jobDoneMsg struct {
	lines []string
	snap  snapshot
}

// Model is the Bubble Tea model of the UI.
type Model struct {
	ctx           context.Context
	actions       core.Actions
	snap          snapshot
	tab           tab
	cursor        [tabCount]int
	width         int
	height        int
	form          *form
	showKeys      bool
	pendingRemove string
	job           *job
	messages      []string
	quitting      bool
	// mouseOff gives the mouse back to the terminal, to select text.
	mouseOff bool
	// hover is where the mouse last moved; hovering is false until it does.
	hover    point
	hovering bool
	// layout is where render last drew what a click can hit.
	layout layout
}

// New returns the UI over actions, already showing the current state.
// Background actions stop when ctx is done.
func New(ctx context.Context, actions core.Actions) *Model {
	return &Model{ctx: ctx, actions: actions, snap: load(ctx, actions), width: defaultWidth, height: defaultHeight}
}

// Run shows the UI until the user quits.
func Run(ctx context.Context, actions core.Actions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := tea.NewProgram(New(ctx, actions)).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return context.Canceled
	}
	return err
}

// Init starts nothing: New already loaded the state.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case jobDoneMsg:
		m.job = nil
		m.snap = msg.snap
		m.messages = msg.lines
		m.clampCursors()
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		if m.form != nil {
			m.form.insert(msg.Content)
		}
	case tea.MouseMsg:
		return m, m.mouse(msg)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	if key.Matches(msg, keys.Interrupt) {
		return m.interrupt()
	}
	if m.form != nil {
		return m.formKey(msg)
	}
	if m.showKeys {
		m.showKeys = !key.Matches(msg, keys.Help, keys.Cancel)
		return nil
	}
	confirm := m.dropQuestion()
	switch {
	case key.Matches(msg, keys.Help):
		m.showKeys = true
		return nil
	case key.Matches(msg, keys.Copy):
		return m.copy()
	case key.Matches(msg, keys.Mouse):
		m.toggleMouse()
		return nil
	case key.Matches(msg, keys.Tabs):
		m.tab = tab(msg.String()[0] - firstTabKey)
		return nil
	case key.Matches(msg, keys.Quit):
		if m.busy() {
			return nil
		}
		m.quitting = true
		return tea.Quit
	case key.Matches(msg, keys.NextTab):
		m.tab = (m.tab + 1) % tabCount
		return nil
	case key.Matches(msg, keys.PrevTab):
		m.tab = (m.tab + tabCount - 1) % tabCount
		return nil
	case key.Matches(msg, keys.Up):
		m.move(-1)
		return nil
	case key.Matches(msg, keys.Down):
		m.move(1)
		return nil
	}
	switch m.tab {
	case tabMine:
		return m.mineKey(msg)
	case tabRepos:
		return m.reposKey(msg, confirm)
	case tabSkills:
		return m.skillsKey(msg, confirm)
	}
	return m.mcpKey(msg, confirm)
}

// dropQuestion forgets a pending remove and returns its name. Any key or
// click but a second d drops the question; the second d replaces it.
func (m *Model) dropQuestion() string {
	confirm := m.pendingRemove
	m.pendingRemove = ""
	if confirm != "" {
		m.messages = nil
	}
	return confirm
}

// interrupt cancels the running action and stays on screen until it ends;
// with nothing running it leaves the UI.
func (m *Model) interrupt() tea.Cmd {
	if m.job == nil {
		m.quitting = true
		return tea.Quit
	}
	m.job.cancel()
	m.job.canceling = true
	m.messages = []string{fmt.Sprintf(msgCanceling, m.job.label)}
	return nil
}

// busy reports, and tells the user, that an action is running and no other
// may start.
func (m *Model) busy() bool {
	if m.job == nil {
		return false
	}
	m.messages = []string{fmt.Sprintf(msgBusy, m.job.label)}
	return true
}

func (m *Model) rowCount(t tab) int {
	switch t {
	case tabMine:
		return optionCount + agentRows + len(m.snap.skills) + len(m.snap.servers)
	case tabRepos:
		return len(m.snap.repos)
	case tabSkills:
		return len(m.snap.home.Team.Skills)
	}
	return len(m.snap.home.Team.MCP)
}

func (m *Model) move(delta int) {
	m.cursor[m.tab] += delta
	m.clampCursors()
}

func (m *Model) clampCursors() {
	for t := range tabCount {
		m.cursor[t] = max(0, min(m.cursor[t], m.rowCount(t)-1))
	}
}

// selected is the cursor row of the current tab, or false when it has none.
func (m *Model) selected() (int, bool) {
	m.clampCursors()
	return m.cursor[m.tab], m.rowCount(m.tab) > 0
}

// selectedName is the name on the cursor row of a team tab; with no rows it
// tells the user so.
func (m *Model) selectedName(names []string) (string, bool) {
	row, ok := m.selected()
	if !ok {
		m.messages = []string{msgNothing}
		return "", false
	}
	return names[row], true
}

// confirmRemove asks for a second d before removing name; it reports
// whether this press is that second d and nothing else runs.
func (m *Model) confirmRemove(name string, confirm string) bool {
	if confirm != name {
		m.pendingRemove = name
		m.messages = []string{fmt.Sprintf(msgConfirm, name, brand.TeamFile)}
		return false
	}
	return !m.busy()
}

// finishNow shows the outcome of an action that ran in Update and reloads.
func (m *Model) finishNow(lines []string, teamChanged bool, err error) {
	m.messages = outcome(nil, lines, teamChanged, err)
	m.snap = load(m.ctx, m.actions)
	m.clampCursors()
}

// start runs an action in the background with its own cancel. The state is
// reloaded in the same goroutine, so only one goroutine ever calls core.
func (m *Model) start(label string, run jobFunc) tea.Cmd {
	if m.busy() {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.job = &job{label: label, cancel: cancel}
	m.messages = []string{fmt.Sprintf(msgRunning, label)}
	parent := m.ctx
	actions := m.actions
	return func() tea.Msg {
		var out bytes.Buffer
		var errw bytes.Buffer
		lines, teamChanged, err := run(ctx, core.Output{Out: &out, Err: &errw})
		cancel()
		// Problems come first so the summary, which ends stdout, survives
		// when the message area keeps only the last lines.
		transcript := pinSummary(append(problemLines(errw.String()), splitLines(out.String())...))
		messages := outcome(transcript, lines, teamChanged, err)
		if len(messages) == 0 {
			messages = []string{fmt.Sprintf(msgDone, label)}
		}
		return jobDoneMsg{lines: messages, snap: load(parent, actions)}
	}
}

// onSuccess drops the lines that report a change when the change failed.
func onSuccess(lines []string, err error) []string {
	if err != nil {
		return nil
	}
	return lines
}

// outcome is what the CLI would print: the transcript, the result lines on
// success, the error as `error [...]` lines, and the PR hint after a
// team-file change.
func outcome(transcript []string, lines []string, teamChanged bool, err error) []string {
	messages := slices.Clone(transcript)
	if err != nil {
		// Detection lines are kept: the protocol was saved even so.
		messages = append(messages, lines...)
		return append(messages, reportLines(err)...)
	}
	messages = append(messages, lines...)
	if teamChanged {
		messages = append(messages, brand.TeamFileChangedHint)
	}
	return messages
}

func reportLines(err error) []string {
	var buf bytes.Buffer
	apperr.Report(&buf, err)
	return splitLines(buf.String())
}

// problemLines keeps the error and warning lines of a stderr transcript and
// drops git's progress output.
func problemLines(text string) []string {
	var problems []string
	for _, line := range splitLines(text) {
		if strings.HasPrefix(line, problemError) || strings.HasPrefix(line, problemWarn) {
			problems = append(problems, line)
		}
	}
	return problems
}

// pinSummary moves the batch summary to the end, so it survives when the
// message area keeps only the last lines; sync prints MCP lines after it.
func pinSummary(lines []string) []string {
	start := slices.IndexFunc(lines, func(line string) bool {
		return strings.HasPrefix(line, core.SummaryPrefix)
	})
	if start < 0 {
		return lines
	}
	end := start + 1
	for end < len(lines) && strings.HasPrefix(lines[end], summaryIndent) {
		end++
	}
	pinned := slices.Concat(lines[:start], lines[end:], lines[start:end])
	return pinned
}

func splitLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, lineSeparator) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func detectedLines(protocol string) []string {
	if protocol == "" {
		return nil
	}
	return []string{core.ProtocolNotice(protocol)}
}

// optional is nil for a field left blank, so core applies its default.
func optional(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

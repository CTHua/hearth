// Package tui is the terminal UI: four tabs over core.Actions. It holds no
// logic of its own beyond choosing which action to call.
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
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/skill"
)

type tab int

// summaryIndent starts each group line under a summary.
const summaryIndent = "  "

const (
	tabRepos tab = iota
	tabProtocol
	tabSkills
	tabMCP
	tabCount
)

var tabNames = [tabCount]string{"Repos", "Protocol", "Skills", "MCP"}

// firstTabKey is the digit of the first tab in keys.Tabs.
const firstTabKey = '1'

// Protocol tab rows.
const (
	optionSSH = iota
	optionHTTPS
	optionDetect
	optionCount
)

// agentRows is how many rows the agent checkboxes take at the top of the
// Skills tab, one per agent.
var agentRows = len(config.Agents())

const (
	msgBusy        = "busy: %s is running; wait for it or press ctrl+c to cancel it"
	msgRunning     = "running: %s (ctrl+c cancels)"
	msgCanceling   = "canceling %s; waiting for git to stop"
	msgDone        = "done: %s"
	msgConfirm     = "press d again to remove %s from %s; any other key keeps it"
	msgBuiltin     = "the built-in skill is installed whenever an agent is chosen; it cannot be toggled"
	msgURLRequired = "the URL is required; nothing was added"
	msgFormClosed  = "nothing was added"
	msgNothing     = "nothing selected"
)

// Job labels, shown while a background action runs.
const (
	jobClone      = "clone"
	jobFixRemote  = "fix-remote"
	jobDetect     = "protocol detection"
	jobSkillSync  = "skill sync"
	jobSync       = "sync"
	jobSkillAdd   = "skill add"
	jobRepoAdd    = "repo add"
	problemError  = "error ["
	problemWarn   = "warning ["
	lineSeparator = "\n"
)

// snapshot is what the screen shows, read after every action.
type snapshot struct {
	home    config.Home
	repos   []core.RepoRow
	skills  []skill.Row
	servers []mcp.Row
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
	pendingRemove string
	job           *job
	messages      []string
	quitting      bool
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
	confirm := m.pendingRemove
	m.pendingRemove = ""
	// Any key but a second d drops the question; the second d replaces it.
	if confirm != "" {
		m.messages = nil
	}
	switch {
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
	case tabRepos:
		return m.reposKey(msg, confirm)
	case tabProtocol:
		return m.protocolKey(msg)
	case tabSkills:
		return m.skillsKey(msg)
	}
	return m.mcpKey(msg)
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
	case tabRepos:
		return len(m.snap.repos)
	case tabProtocol:
		return optionCount
	case tabSkills:
		return agentRows + len(m.snap.skills)
	}
	return len(m.snap.servers)
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

func (m *Model) reposKey(msg tea.KeyPressMsg, confirm string) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Add):
		return m.openForm(repoForm(), m.submitRepo)
	case key.Matches(msg, keys.Clone):
		return m.start(jobClone, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.Clone(ctx, nil, out)
			return nil, false, err
		})
	}
	fix := key.Matches(msg, keys.Fix)
	if !fix && !key.Matches(msg, keys.Remove) {
		return nil
	}
	row, ok := m.selected()
	if !ok {
		m.messages = []string{msgNothing}
		return nil
	}
	name := m.snap.repos[row].Repo.Name
	if fix {
		return m.start(jobFixRemote, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.FixRemote(ctx, []string{name}, out)
			return nil, false, err
		})
	}
	if confirm != name {
		m.pendingRemove = name
		m.messages = []string{fmt.Sprintf(msgConfirm, name, brand.TeamFile)}
		return nil
	}
	if m.busy() {
		return nil
	}
	result, err := m.actions.RemoveRepo(name)
	lines := []string{fmt.Sprintf("removed %s from %s", name, brand.TeamFile)}
	if result.CheckoutKept {
		lines = append(lines, fmt.Sprintf("the checkout at %s was not touched", result.Repo.Path))
	}
	m.finishNow(onSuccess(lines, err), true, err)
	return nil
}

func (m *Model) submitRepo(values []string) tea.Cmd {
	url := strings.TrimSpace(values[0])
	if url == "" {
		m.messages = []string{msgURLRequired}
		return nil
	}
	params := core.AddRepoParams{URL: url, Name: optional(values[1]), Path: optional(values[2]), Branch: optional(values[3])}
	add := func(ctx context.Context) ([]string, error) {
		result, err := m.actions.AddRepo(ctx, params)
		lines := detectedLines(result.DetectedProtocol)
		if err == nil {
			lines = append(lines, fmt.Sprintf("added %s -> %s (%s)", result.Repo.Name, result.Repo.Path, result.Repo.Branch))
		}
		return lines, err
	}
	// Without a branch the remote is asked for one, which may take a while.
	if params.Branch == nil {
		return m.start(jobRepoAdd, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
			lines, err := add(ctx)
			return lines, true, err
		})
	}
	if m.busy() {
		return nil
	}
	lines, err := add(m.ctx)
	m.finishNow(lines, true, err)
	return nil
}

func (m *Model) protocolKey(msg tea.KeyPressMsg) tea.Cmd {
	if !key.Matches(msg, keys.Choose) {
		return nil
	}
	option, _ := m.selected()
	if option == optionDetect {
		if len(m.snap.home.Team.Repos) == 0 {
			m.messages = []string{core.ErrNoRepos.Message}
			return nil
		}
		return m.start(jobDetect, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
			protocol, err := m.actions.DetectProtocol(ctx)
			return detectedLines(protocol), false, err
		})
	}
	if m.busy() {
		return nil
	}
	protocol := giturl.SSH
	if option == optionHTTPS {
		protocol = giturl.HTTPS
	}
	err := m.actions.SetProtocol(protocol)
	m.finishNow(onSuccess([]string{fmt.Sprintf("protocol: %s (saved to %s)", protocol, brand.LocalFile)}, err), false, err)
	return nil
}

func (m *Model) skillsKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.AddSkill):
		return m.openForm(skillForm(), m.submitSkill)
	case key.Matches(msg, keys.SkillSync):
		return m.start(jobSkillSync, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.SyncSkills(ctx, out)
			return nil, false, err
		})
	case !key.Matches(msg, keys.Toggle):
		return nil
	}
	row, _ := m.selected()
	if row < agentRows {
		if m.busy() {
			return nil
		}
		agents := toggled(m.snap.home.Personal.Agents, config.Agents()[row])
		err := m.actions.SetAgents(agents)
		m.finishNow(onSuccess([]string{fmt.Sprintf("agents: %s (saved to %s)", core.LabelList(agents), brand.LocalFile)}, err), false, err)
		return nil
	}
	item := m.snap.skills[row-agentRows]
	if item.Source == skill.Builtin {
		m.messages = []string{msgBuiltin}
		return nil
	}
	if m.busy() {
		return nil
	}
	m.toggleChoice(item.Name, item.Enabled, m.actions.EnableSkills, m.actions.DisableSkills)
	return nil
}

func (m *Model) submitSkill(values []string) tea.Cmd {
	url := strings.TrimSpace(values[0])
	if url == "" {
		m.messages = []string{msgURLRequired}
		return nil
	}
	params := core.AddSkillParams{URL: url, Name: optional(values[1]), Subdir: optional(values[2]), Commit: optional(values[3])}
	return m.start(jobSkillAdd, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
		result, err := m.actions.AddSkill(ctx, params)
		lines := detectedLines(result.DetectedProtocol)
		if err == nil {
			entry := result.Skill
			lines = append(lines, fmt.Sprintf("added skill %s at %s (subdir %s)", entry.Name, entry.Commit, entry.Subdir))
		}
		return lines, true, err
	})
}

func (m *Model) mcpKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Sync):
		return m.start(jobSync, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.Sync(ctx, false, out)
			return nil, false, err
		})
	case !key.Matches(msg, keys.Toggle):
		return nil
	}
	row, ok := m.selected()
	if !ok {
		m.messages = []string{msgNothing}
		return nil
	}
	if m.busy() {
		return nil
	}
	server := m.snap.servers[row]
	// A stale choice counts as enabled: toggling it removes it.
	m.toggleChoice(server.Name, server.Enabled || server.Stale, m.actions.EnableMCP, m.actions.DisableMCP)
	return nil
}

// toggleChoice enables or disables one personal choice.
func (m *Model) toggleChoice(name string, enabled bool, enable func([]string) ([]string, error), disable func([]string) ([]string, error)) {
	if enabled {
		removed, err := disable([]string{name})
		m.finishNow(onSuccess([]string{fmt.Sprintf("disabled: %s (saved to %s)", core.LabelList(removed), brand.LocalFile)}, err), false, err)
		return
	}
	added, err := enable([]string{name})
	m.finishNow(onSuccess([]string{fmt.Sprintf("enabled: %s (saved to %s)", core.LabelList(added), brand.LocalFile)}, err), false, err)
}

// toggled returns the known agents in their usual order, with agent flipped.
func toggled(current []string, agent string) []string {
	var next []string
	for _, known := range config.Agents() {
		on := slices.Contains(current, known)
		if known == agent {
			on = !on
		}
		if on {
			next = append(next, known)
		}
	}
	return next
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

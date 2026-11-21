package tui_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/skill"
	"github.com/CTHua/hearth/internal/subagent"
	"github.com/CTHua/hearth/internal/tui"
)

const (
	newURL    = "https://git.example.invalid/acme/new.git"
	skillURL  = "https://git.example.invalid/acme/skills.git"
	nilValue  = "<nil>"
	cloneDone = "Summary: 1 cloned, 0 skipped, 0 failed"
)

var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;]*m")

// fake records every call that changes something and keeps just enough
// state for the screen to follow.
type fake struct {
	home  config.Home
	repos []core.RepoRow
	// reposErr is what ListRepos returns.
	reposErr error
	skills   []skill.Row
	servers  []mcp.Row
	// subagents and subagentsErr are what ListSubagents returns.
	subagents    []subagent.Row
	subagentsErr error
	calls        []string
	loads        int
	// enableErr is what EnableSkills returns; cloneFail lists repos Clone fails.
	enableErr error
	// mcpErr is what AddMCP returns.
	mcpErr error
	// changes and changesErr are what TeamChanges returns.
	changes    []core.TeamChange
	changesErr error
	// syncTail makes Sync print MCP lines after the skill summary; syncErr
	// is what Sync returns.
	syncTail  bool
	syncErr   error
	cloneFail []string
	// block makes Clone and UpdateSkill wait for their context; started is
	// closed once they do.
	block   bool
	started chan struct{}
}

func newFake() *fake {
	repos := []config.Repo{{Name: "app", URL: "https://git.example.invalid/acme/app.git", Path: "products/app", Branch: "main"}, {Name: "web", URL: "https://git.example.invalid/acme/web.git", Path: "products/web", Branch: "main"}}
	f := &fake{started: make(chan struct{})}
	f.home.Team.Repos = repos
	f.home.Personal.Agents = []string{config.AgentClaude}
	f.home.Team.Skills = []config.Skill{{Name: "pdf", URL: skillURL, Commit: "0123456789abcdef", Subdir: "pdf"}}
	f.home.Team.MCP = []config.MCP{
		{Name: "docs", Transport: config.TransportStdio, Command: "npx", Args: []string{"-y", "@acme/docs-mcp"}},
		{Name: "tracker", Transport: config.TransportHTTP, URL: "https://mcp.example.invalid/mcp"},
	}
	for _, repo := range repos {
		// No repos key in the personal file: every repo is chosen.
		f.repos = append(f.repos, core.RepoRow{Repo: repo, Status: checkout.Status{State: checkout.Missing}, Chosen: true})
	}
	f.skills = []skill.Row{
		{Name: "pdf", Source: skill.External, State: skill.Disabled},
		{Name: "notes", Source: skill.Team, Enabled: true, State: skill.OK},
		{Name: brand.BuiltinSkill, Source: skill.Builtin, Enabled: true, State: skill.OK},
	}
	f.servers = []mcp.Row{{Name: "docs", Transport: config.TransportStdio}, {Name: "tracker", Transport: config.TransportHTTP, Enabled: true}}
	return f
}

func (f *fake) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func show(value *string) string {
	if value == nil {
		return nilValue
	}
	return *value
}

func (f *fake) Home() config.Home { return f.home }

func (f *fake) AddRepo(_ context.Context, p core.AddRepoParams) (core.AddRepoResult, error) {
	f.record("AddRepo %s name=%s path=%s branch=%s description=%s", p.URL, show(p.Name), show(p.Path), show(p.Branch), show(p.Description))
	return core.AddRepoResult{Repo: config.Repo{Name: "new", Path: "products/new", Branch: "main"}}, nil
}

func (f *fake) RemoveRepo(name string) (core.RemoveRepoResult, error) {
	f.record("RemoveRepo %s", name)
	return core.RemoveRepoResult{Repo: config.Repo{Name: name}}, nil
}

func (f *fake) ListRepos(context.Context) ([]core.RepoRow, error) {
	f.loads++
	return f.repos, f.reposErr
}

func (f *fake) Clone(ctx context.Context, names []string, out core.Output) (core.BatchResult, error) {
	f.record("Clone %v", names)
	result := core.BatchResult{DoneLabel: "cloned", SkippedLabel: "skipped"}
	if f.block {
		close(f.started)
		<-ctx.Done()
		return result, ctx.Err()
	}
	if len(f.cloneFail) > 0 {
		for _, name := range f.cloneFail {
			fmt.Fprintf(out.Err, "error [env] %s: clone failed\n", name)
		}
		result.Failed = f.cloneFail
		fmt.Fprint(out.Out, result.Summary())
		return result, apperr.Exit(apperr.ExitFailed)
	}
	result.Done = []string{"app"}
	fmt.Fprint(out.Out, result.Summary())
	return result, nil
}

func (f *fake) FixRemote(_ context.Context, names []string, _ core.Output) (core.BatchResult, error) {
	f.record("FixRemote %v", names)
	return core.BatchResult{}, nil
}

func (f *fake) SetProtocol(protocol string) error {
	f.record("SetProtocol %s", protocol)
	f.home.Personal.Protocol = protocol
	return nil
}

func (f *fake) DetectProtocol(context.Context) (string, error) {
	f.record("DetectProtocol")
	return "ssh", nil
}

func (f *fake) EnableRepos(names []string) ([]string, error) {
	f.record("EnableRepos %v", names)
	f.chooseRepos(names, true)
	return names, nil
}

func (f *fake) DisableRepos(names []string) ([]string, error) {
	f.record("DisableRepos %v", names)
	f.chooseRepos(names, false)
	return names, nil
}

// chooseRepos keeps the repo rows in step with a choice, as core would.
func (f *fake) chooseRepos(names []string, chosen bool) {
	for i := range f.repos {
		if slices.Contains(names, f.repos[i].Repo.Name) {
			f.repos[i].Chosen = chosen
		}
	}
}

func (f *fake) SetAgents(agents []string) error {
	f.record("SetAgents %v", agents)
	f.home.Personal.Agents = agents
	return nil
}

func (f *fake) EnableSkills(names []string) ([]string, error) {
	f.record("EnableSkills %v", names)
	if f.enableErr != nil {
		return nil, f.enableErr
	}
	return names, nil
}

func (f *fake) DisableSkills(names []string) ([]string, error) {
	f.record("DisableSkills %v", names)
	return names, nil
}

func (f *fake) AddSkill(_ context.Context, p core.AddSkillParams) (core.AddSkillResult, error) {
	subdir := nilValue
	if len(p.Subdirs) > 0 {
		subdir = strings.Join(p.Subdirs, ",")
	}
	f.record("AddSkill %s name=%s subdir=%s commit=%s", p.URL, show(p.Name), subdir, show(p.Commit))
	if len(p.Subdirs) == 0 {
		// Without a subdir the remote is listed; this one holds a group.
		group := config.Skill{Name: "tools", URL: p.URL, Commit: "abc", Subdirs: []string{"skills/grilling", "skills/grill-me"}}
		skipped := []core.SkippedSkill{{Dir: "extra/other", Reason: "not listed in " + skill.PluginFile}}
		return core.AddSkillResult{Skill: group, Added: group.Subdirs, Listed: true, Found: 3, Skipped: skipped}, nil
	}
	return core.AddSkillResult{Skill: config.Skill{Name: "skills", Commit: "abc", Subdir: "."}}, nil
}

func (f *fake) RemoveSkill(name string) (core.RemoveSkillResult, error) {
	f.record("RemoveSkill %s", name)
	return core.RemoveSkillResult{Name: name}, nil
}

func (f *fake) UpdateSkill(ctx context.Context, name string, commit *string) (core.UpdateSkillResult, error) {
	f.record("UpdateSkill %s commit=%s", name, show(commit))
	result := core.UpdateSkillResult{Name: name, From: "abc", To: "def"}
	if f.block {
		close(f.started)
		<-ctx.Done()
		return result, ctx.Err()
	}
	return result, nil
}

func (f *fake) ListSkills(context.Context) ([]skill.Row, error) {
	return f.skills, nil
}

func (f *fake) SyncSkills(context.Context, core.Output) (core.BatchResult, error) {
	f.record("SyncSkills")
	return core.BatchResult{}, nil
}

func (f *fake) AddMCP(server config.MCP) error {
	f.record("AddMCP %s transport=%s command=%s args=%q url=%s env=%q", server.Name, server.Transport, server.Command, server.Args, server.URL, server.EnvVars)
	return f.mcpErr
}

func (f *fake) RemoveMCP(name string) (config.MCP, error) {
	f.record("RemoveMCP %s", name)
	return config.MCP{Name: name}, nil
}

func (f *fake) TeamChanges(context.Context) ([]core.TeamChange, error) {
	return f.changes, f.changesErr
}

func (f *fake) EnableMCP(names []string) ([]string, error) {
	f.record("EnableMCP %v", names)
	return names, nil
}

func (f *fake) DisableMCP(names []string) ([]string, error) {
	f.record("DisableMCP %v", names)
	return names, nil
}

func (f *fake) ListMCP() []mcp.Row {
	return f.servers
}

func (f *fake) EnableSubagents(names []string) ([]string, error) {
	f.record("EnableSubagents %v", names)
	return names, nil
}

func (f *fake) DisableSubagents(names []string) ([]string, error) {
	f.record("DisableSubagents %v", names)
	return names, nil
}

func (f *fake) ListSubagents() ([]subagent.Row, error) {
	return f.subagents, f.subagentsErr
}

func (f *fake) Sync(_ context.Context, force bool, out core.Output) (core.SyncResult, error) {
	f.record("Sync force=%v", force)
	if f.syncTail {
		// Like core.Sync: the skill summary, then the MCP lines.
		fmt.Fprint(out.Out, core.BatchResult{DoneLabel: "installed", SkippedLabel: "skipped"}.Summary())
		fmt.Fprint(out.Out, "no agent chosen\nremoved .mcp.json\nremoved .codex/config.toml\n")
	}
	return core.SyncResult{}, f.syncErr
}

var _ core.Actions = (*fake)(nil)

func keyMsg(name string) tea.KeyPressMsg {
	special := map[string]tea.KeyPressMsg{
		"space":     {Code: tea.KeySpace, Text: " "},
		"enter":     {Code: tea.KeyEnter},
		"tab":       {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"down":      {Code: tea.KeyDown},
		"up":        {Code: tea.KeyUp},
		"esc":       {Code: tea.KeyEscape},
		"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	}
	if msg, found := special[name]; found {
		return msg
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

// press sends each key and returns the command of the last one.
func press(m *tui.Model, names ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, name := range names {
		_, cmd = m.Update(keyMsg(name))
	}
	return cmd
}

func typeText(m *tui.Model, text string) {
	for _, r := range text {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// finish runs a background command to its end and hands the result back.
func finish(t *testing.T, m *tui.Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no background command")
	}
	m.Update(cmd())
}

func screen(m *tui.Model) string {
	return ansiCodes.ReplaceAllString(m.View().Content, "")
}

func expectCalls(t *testing.T, f *fake, want ...string) {
	t.Helper()
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
	f.calls = nil
}

func expectScreen(t *testing.T, m *tui.Model, wants ...string) {
	t.Helper()
	text := screen(m)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("screen lacks %q:\n%s", want, text)
		}
	}
}

func newModel(f *fake) *tui.Model {
	return tui.New(context.Background(), f)
}

func TestRepoAddWithBranchRunsNowAndShowsHint(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "a")
	typeText(m, newURL)
	press(m, "tab")
	typeText(m, "new")
	press(m, "tab", "tab")
	typeText(m, "main")
	if cmd := press(m, "enter"); cmd != nil {
		t.Error("a repo add with a branch ran in the background")
	}
	expectCalls(t, f, "AddRepo "+newURL+" name=new path=<nil> branch=main description=<nil>")
	expectScreen(t, m, "added new -> products/new (main)", brand.TeamFileChangedHint)
}

func TestRepoAddWithoutBranchRunsInBackground(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "a")
	typeText(m, newURL)
	cmd := press(m, "enter")
	expectScreen(t, m, "running: repo add")
	expectCalls(t, f)
	loads := f.loads
	finish(t, m, cmd)
	expectCalls(t, f, "AddRepo "+newURL+" name=<nil> path=<nil> branch=<nil> description=<nil>")
	expectScreen(t, m, brand.TeamFileChangedHint)
	if f.loads == loads {
		t.Error("the lists were not read again after the action")
	}
}

func TestRepoAddPassesDescription(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "a")
	expectScreen(t, m, "Description", "optional")
	typeText(m, newURL)
	press(m, "tab", "tab", "tab")
	typeText(m, "main")
	press(m, "tab")
	typeText(m, "Orders API")
	press(m, "enter")
	expectCalls(t, f, "AddRepo "+newURL+" name=<nil> path=<nil> branch=main description=Orders API")
}

func TestRepoAddNeedsURL(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "a", "enter")
	expectCalls(t, f)
	expectScreen(t, m, "the URL is required")
}

func TestRepoRemoveNeedsTwoPresses(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "d")
	expectCalls(t, f)
	expectScreen(t, m, "press d again to remove app")
	press(m, "j")
	if strings.Contains(screen(m), "press d again") {
		t.Error("the question stayed after another key")
	}
	press(m, "d")
	expectCalls(t, f)
	expectScreen(t, m, "press d again to remove web")
	press(m, "d")
	expectCalls(t, f, "RemoveRepo web")
	expectScreen(t, m, "removed web from "+brand.TeamFile, brand.TeamFileChangedHint)
}

func TestCloneAndFixRemote(t *testing.T) {
	f := newFake()
	m := newModel(f)
	cmd := press(m, "2", "c")
	expectScreen(t, m, "running: clone")
	finish(t, m, cmd)
	expectCalls(t, f, "Clone []")
	expectScreen(t, m, cloneDone)
	finish(t, m, press(m, "j", "f"))
	expectCalls(t, f, "FixRemote [web]")
	expectScreen(t, m, "done: fix-remote")
}

func TestProtocolChoices(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "1", "enter")
	press(m, "down", "space")
	expectCalls(t, f, "SetProtocol ssh", "SetProtocol https")
	expectScreen(t, m, "protocol: https (saved to "+brand.LocalFile+")")
	finish(t, m, press(m, "down", "enter"))
	expectCalls(t, f, "DetectProtocol")
	expectScreen(t, m, "protocol: detected ssh")
}

func TestDetectWithoutReposIsDisabled(t *testing.T) {
	f := newFake()
	f.home.Team.Repos = nil
	f.repos = nil
	m := newModel(f)
	if cmd := press(m, "1", "down", "down", "enter"); cmd != nil {
		t.Error("detection started without repos")
	}
	expectCalls(t, f)
	expectScreen(t, m, core.ErrNoRepos.Message)
}

func TestSkillsTab(t *testing.T) {
	f := newFake()
	m := newModel(f)
	// Agents and skills are on Mine, under the three protocol rows; the two
	// repo rows are between them.
	press(m, "1", "down", "down", "down", "space")
	press(m, "down", "space")
	expectCalls(t, f, "SetAgents []", "SetAgents [codex]")
	press(m, "down", "down", "down", "space")
	press(m, "down", "space")
	expectCalls(t, f, "EnableSkills [pdf]", "DisableSkills [notes]")
	press(m, "down", "space")
	expectCalls(t, f)
	expectScreen(t, m, "the built-in skill is installed whenever an agent is chosen")
	// s on Mine is the top-level sync; skill sync has no key of its own.
	finish(t, m, press(m, "s"))
	expectCalls(t, f, "Sync force=false")
	// Adding an external skill is on the team Skills tab.
	press(m, "3", "a")
	typeText(m, skillURL)
	press(m, "tab", "tab")
	typeText(m, "pdf")
	cmd := press(m, "enter")
	expectScreen(t, m, "running: skill add")
	finish(t, m, cmd)
	expectCalls(t, f, "AddSkill "+skillURL+" name=<nil> subdir=pdf commit=<nil>")
	expectScreen(t, m, "added skill skills at abc", brand.TeamFileChangedHint)
}

func TestMCPTab(t *testing.T) {
	f := newFake()
	m := newModel(f)
	// Servers are the last rows of Mine: 3 protocol, 2 agent, 2 repo and 3
	// skill rows come first.
	press(m, "1", "down", "down", "down", "down", "down", "down", "down", "down", "down", "down", "space")
	press(m, "down", "space")
	expectCalls(t, f, "EnableMCP [docs]", "DisableMCP [tracker]")
	finish(t, m, press(m, "s"))
	expectCalls(t, f, "Sync force=false")
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, quit := cmd().(tea.QuitMsg)
	return quit
}

func TestCtrlCCancelsRunningActionThenQuits(t *testing.T) {
	f := newFake()
	f.block = true
	m := newModel(f)
	cmd := press(m, "2", "c")
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	<-f.started
	// Nothing else may call core while the clone runs.
	if c := press(m, "q"); isQuit(c) {
		t.Fatal("q quit while an action ran")
	}
	expectScreen(t, m, "busy: clone is running")
	press(m, "d", "d", "f", "2", "enter")
	if c := press(m, "ctrl+c"); isQuit(c) {
		t.Fatal("ctrl+c quit while an action ran")
	}
	expectScreen(t, m, "canceling clone")
	m.Update(<-done)
	expectCalls(t, f, "Clone []")
	expectScreen(t, m, "interrupted")
	if !isQuit(press(m, "ctrl+c")) {
		t.Error("ctrl+c with nothing running did not quit")
	}
}

func TestQuitKey(t *testing.T) {
	if !isQuit(press(newModel(newFake()), "q")) {
		t.Error("q did not quit")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	f := newFake()
	long := strings.Repeat("very-long-detail-", 10)
	f.repos[0].Status.Detail = long
	f.skills[0].Detail = long
	f.servers[0].MissingEnv = []string{strings.Repeat("LONG_NAME_", 12)}
	f.changes = []core.TeamChange{{Table: core.TableRepos, Name: long, Kind: core.Removed}}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {200, 50}} {
		m := newModel(f)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		screens := map[string]func(){
			"mine":       func() { press(m, "1", "down", "down", "down", "down", "down", "down", "down", "down", "down") },
			"repos":      func() { press(m, "2") },
			"skills":     func() { press(m, "3") },
			"mcp":        func() { press(m, "4") },
			"repo form":  func() { press(m, "2", "a"); typeText(m, long) },
			"skill form": func() { press(m, "3", "a"); typeText(m, long) },
			"mcp form": func() {
				press(m, "4", "a")
				typeText(m, long)
				press(m, "tab", "tab")
				typeText(m, long)
				press(m, "enter")
			},
			"keys":     func() { press(m, "?") },
			"messages": func() { finish(t, m, press(m, "1", "s")) },
		}
		for name, open := range screens {
			open()
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) > size[1] {
				t.Errorf("%dx%d %s has %d lines", size[0], size[1], name, len(lines))
			}
			for i, line := range lines {
				if width := lipgloss.Width(line); width > size[0] {
					t.Errorf("%dx%d %s line %d is %d wide: %q", size[0], size[1], name, i, width, line)
				}
			}
			press(m, "esc")
		}
	}
}

func TestSmallTerminalShowsOnlyAHint(t *testing.T) {
	m := newModel(newFake())
	for _, size := range [][2]int{{79, 24}, {80, 23}, {40, 10}, {1, 1}, {0, 0}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		text := screen(m)
		if size[0] >= 40 {
			if !strings.Contains(text, fmt.Sprintf("this terminal is %d×%d", size[0], size[1])) || !strings.Contains(text, "needs at least 80×24") {
				t.Errorf("%dx%d:\n%s", size[0], size[1], text)
			}
			if strings.Contains(text, "Repos") {
				t.Errorf("%dx%d drew the tabs:\n%s", size[0], size[1], text)
			}
		}
		for i, line := range strings.Split(m.View().Content, "\n") {
			if width := lipgloss.Width(line); width > max(1, size[0]) {
				t.Errorf("%dx%d line %d is %d wide", size[0], size[1], i, width)
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	expectScreen(t, m, "Repos", "MINE · "+brand.LocalFile)
}

func TestFailedActionShowsNoSuccessLine(t *testing.T) {
	f := newFake()
	f.enableErr = apperr.New(apperr.Usage, "cannot enable pdf")
	m := newModel(f)
	press(m, "1", "down", "down", "down", "down", "down", "down", "down", "space")
	expectCalls(t, f, "EnableSkills [pdf]")
	expectScreen(t, m, "cannot enable pdf")
	if text := screen(m); strings.Contains(text, "enabled:") {
		t.Errorf("a refused enable reported success:\n%s", text)
	}
}

func TestManyFailuresKeepTheSummary(t *testing.T) {
	f := newFake()
	f.cloneFail = []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}
	m := newModel(f)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	finish(t, m, press(m, "2", "c"))
	expectScreen(t, m, "Summary: 0 cloned, 0 skipped, 8 failed")
}

func TestPasteDropsLineBreaks(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "2", "a")
	m.Update(tea.PasteMsg{Content: newURL[:20] + "\r\n" + newURL[20:]})
	press(m, "tab", "tab", "tab")
	typeText(m, "main")
	press(m, "enter")
	expectCalls(t, f, "AddRepo "+newURL+" name=<nil> path=<nil> branch=main description=<nil>")
}

func TestSyncKeepsTheSkillSummary(t *testing.T) {
	f := newFake()
	f.syncTail = true
	m := newModel(f)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	finish(t, m, press(m, "1", "s"))
	expectScreen(t, m, core.SummaryPrefix+"0 installed", "removed .codex/config.toml")
}

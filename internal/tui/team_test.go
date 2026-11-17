package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/tui"
	"github.com/CTHua/hearth/internal/version"
)

// Truecolor foregrounds and backgrounds as lipgloss writes them.
const (
	accentSGR   = "38;2;232;163;61"
	dimSGR      = "38;2;75;80;92"
	selectedSGR = "48;2;48;48;48"
	mcpURL      = "https://mcp.example.invalid/x"
	docsCommand = "npx -y @acme/docs-mcp"
	mcpTitle    = "Add MCP server"
)

func lines(m *tui.Model) []string {
	return strings.Split(screen(m), "\n")
}

func TestOpensOnMineAndSwitchesTabs(t *testing.T) {
	f := newFake()
	m := newModel(f)
	bar := lines(m)[0]
	at := -1
	for _, name := range []string{"Mine", "team", "Repos", "Skills", "MCP"} {
		next := strings.Index(bar, name)
		if next <= at {
			t.Fatalf("tab bar is not Mine, team, Repos, Skills, MCP: %q", bar)
		}
		at = next
	}
	expectScreen(t, m, "MINE · "+brand.LocalFile)
	// The cursor starts on the first protocol row.
	for _, line := range lines(m) {
		if strings.Contains(line, "▌") && !strings.Contains(line, "ssh") {
			t.Errorf("cursor row is %q", line)
		}
	}
	tabs := []struct {
		keys  []string
		title string
	}{
		{[]string{"tab"}, "Add repo"},
		{[]string{"tab", "tab"}, "Add external skill"},
		{[]string{"shift+tab"}, mcpTitle},
		{[]string{"2"}, "Add repo"},
		{[]string{"3"}, "Add external skill"},
		{[]string{"4"}, mcpTitle},
	}
	for _, c := range tabs {
		m := newModel(f)
		press(m, c.keys...)
		expectScreen(t, m, "TEAM · "+brand.TeamFile)
		press(m, "a")
		expectScreen(t, m, c.title)
	}
	press(m, "a")
	if strings.Contains(screen(m), "Add ") {
		t.Error("a opened a form on Mine")
	}
}

func TestTeamSkillsTab(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "3", "space")
	expectCalls(t, f)
	text := screen(m)
	if !strings.Contains(text, "pdf") || strings.Contains(text, "notes") || strings.Contains(text, "builtin") {
		t.Errorf("the team Skills tab lists more than external skills:\n%s", text)
	}
	press(m, "d")
	expectCalls(t, f)
	expectScreen(t, m, "press d again to remove pdf")
	press(m, "d")
	expectCalls(t, f, "RemoveSkill pdf")
	expectScreen(t, m, "removed skill pdf from "+brand.TeamFile, brand.TeamFileChangedHint)
	press(m, "u")
	expectScreen(t, m, "Update pin of pdf", "the remote's HEAD")
	cmd := press(m, "enter")
	expectScreen(t, m, "running: skill update")
	expectCalls(t, f)
	finish(t, m, cmd)
	expectCalls(t, f, "UpdateSkill pdf commit=<nil>")
	expectScreen(t, m, "updated skill pdf: abc -> def", brand.TeamFileChangedHint)
	press(m, "u")
	typeText(m, "def")
	if cmd := press(m, "enter"); cmd != nil {
		t.Error("an update to a given commit ran in the background")
	}
	expectCalls(t, f, "UpdateSkill pdf commit=def")
}

func TestSkillUpdateCanBeCanceled(t *testing.T) {
	f := newFake()
	f.block = true
	m := newModel(f)
	cmd := press(m, "3", "u", "enter")
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	<-f.started
	if isQuit(press(m, "ctrl+c")) {
		t.Fatal("ctrl+c quit while the update ran")
	}
	expectScreen(t, m, "canceling skill update")
	m.Update(<-done)
	expectCalls(t, f, "UpdateSkill pdf commit=<nil>")
	expectScreen(t, m, "interrupted")
}

// fillMCP opens the MCP form and fills name, command, URL and env vars.
func fillMCP(m *tui.Model, values ...string) tea.Cmd {
	press(m, "4", "a")
	for i, value := range values {
		if i > 0 {
			press(m, "tab")
		}
		typeText(m, value)
	}
	return press(m, "enter")
}

func TestTeamMCPTab(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "4", "space")
	expectCalls(t, f)
	if cmd := fillMCP(m, "docs2", docsCommand, "", "DOCS_TOKEN, OTHER"); cmd != nil {
		t.Error("adding an MCP server ran in the background")
	}
	expectCalls(t, f, `AddMCP docs2 transport=stdio command=npx args=["-y" "@acme/docs-mcp"] url= env=["DOCS_TOKEN" "OTHER"]`)
	expectScreen(t, m, "added MCP server docs2 (stdio)", brand.TeamFileChangedHint)
	if strings.Contains(screen(m), mcpTitle) {
		t.Error("the form stayed open after a successful add")
	}
	fillMCP(m, "web", "", mcpURL)
	expectCalls(t, f, "AddMCP web transport=http command= args=[] url="+mcpURL+" env=[]")
	press(m, "up", "d")
	expectCalls(t, f)
	press(m, "d")
	expectCalls(t, f, "RemoveMCP docs")
	expectScreen(t, m, "removed MCP server docs from "+brand.TeamFile, brand.TeamFileChangedHint)
}

func TestMCPFormRefusalsKeepTheForm(t *testing.T) {
	refused := []struct {
		values []string
		reason string
	}{
		{[]string{"x", docsCommand, mcpURL}, "not both"},
		{[]string{"x", "", ""}, "fill command for a stdio server or URL"},
		{[]string{"x", "", mcpURL, "TOKEN"}, "env vars are for stdio servers"},
	}
	for _, c := range refused {
		f := newFake()
		m := newModel(f)
		fillMCP(m, c.values...)
		expectCalls(t, f)
		expectScreen(t, m, mcpTitle, c.reason)
	}
	f := newFake()
	f.mcpErr = apperr.New(apperr.Usage, "env var bad-name is not a valid name")
	m := newModel(f)
	fillMCP(m, "x", "npx", "", "bad-name")
	expectCalls(t, f, `AddMCP x transport=stdio command=npx args=[] url= env=["bad-name"]`)
	expectScreen(t, m, mcpTitle, "bad-name is not a valid name")
	if strings.Contains(screen(m), brand.TeamFileChangedHint) {
		t.Error("a refused add told the user to open a PR")
	}
	press(m, "esc")
	if strings.Contains(screen(m), mcpTitle) {
		t.Error("esc did not close the form")
	}
}

// TestLongFormErrorKeepsKeyHints fills the smallest screen with errors; the
// key hints must stay inside the box.
func TestLongFormErrorKeepsKeyHints(t *testing.T) {
	f := newFake()
	f.mcpErr = apperr.New(apperr.Usage, "%s", strings.Repeat("env var bad-name is not a valid name; ", 40))
	m := newModel(f)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	fillMCP(m, "x", "npx", "", "bad-name")
	expectScreen(t, m, mcpTitle, "esc cancel", "…")
	if lines := strings.Split(screen(m), "\n"); len(lines) > 24 {
		t.Errorf("screen has %d lines, want at most 24", len(lines))
	}
}

// TestRefusedMCPLeavesTeamFile runs the form against the real core, whose
// env var check refuses the server.
func TestRefusedMCPLeavesTeamFile(t *testing.T) {
	ctx := context.Background()
	g, err := gitx.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "home")
	if err := scaffold.Init(ctx, g, dir); err != nil {
		t.Fatal(err)
	}
	home, err := config.Load(dir, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	teamFile := filepath.Join(dir, brand.TeamFile)
	before, err := os.ReadFile(teamFile)
	if err != nil {
		t.Fatal(err)
	}
	m := tui.New(ctx, core.New(home))
	fillMCP(m, "docs", docsCommand, "", "bad-name")
	expectScreen(t, m, mcpTitle, "bad-name")
	after, err := os.ReadFile(teamFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("%s changed:\n%s", brand.TeamFile, after)
	}
	press(m, "esc")
	fillMCP(m, "docs", docsCommand, "", "DOCS_TOKEN")
	expectScreen(t, m, "added MCP server docs (stdio)")
	written, err := config.Load(dir, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	if server, found := written.Team.FindMCP("docs"); !found || server.Command != "npx" || strings.Join(server.Args, " ") != "-y @acme/docs-mcp" {
		t.Errorf("written server %+v", server)
	}
}

func TestTeamChangesAreMarked(t *testing.T) {
	f := newFake()
	f.changes = []core.TeamChange{
		{Table: core.TableRepos, Name: "app", Kind: core.Modified},
		{Table: core.TableSkills, Name: "pdf", Kind: core.Added},
		{Table: core.TableMCP, Name: "gone", Kind: core.Removed},
	}
	m := newModel(f)
	if strings.Contains(screen(m), "since HEAD") {
		t.Error("Mine shows the team-file changes")
	}
	header := brand.TeamFile + ": 3 changes since HEAD"
	press(m, "2")
	expectScreen(t, m, header, "removed: gone", "open a PR")
	expectRow(t, m, `~\s+app\s`)
	expectRow(t, m, `^\s+web\s`)
	press(m, "3")
	expectScreen(t, m, header)
	expectRow(t, m, `\+\s+pdf\s`)
	press(m, "4")
	expectRow(t, m, `^\s+docs\s`)

	f.changes = f.changes[:1]
	f.changesErr = nil
	m = newModel(f)
	press(m, "2")
	expectScreen(t, m, brand.TeamFile+": 1 change since HEAD")
	if strings.Contains(screen(m), "removed:") {
		t.Error("removed names shown with nothing removed")
	}

	f.changes = nil
	f.changesErr = apperr.New(apperr.Config, "cannot compare %s with HEAD: broken", brand.TeamFile)
	m = newModel(f)
	press(m, "2")
	expectScreen(t, m, "cannot compare "+brand.TeamFile+" with HEAD")
	if strings.Contains(screen(m), "since HEAD") {
		t.Error("changes shown although HEAD could not be read")
	}
}

// expectRow checks that a line, with the cursor bar blanked, matches.
func expectRow(t *testing.T, m *tui.Model, pattern string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	for _, line := range lines(m) {
		if re.MatchString(strings.ReplaceAll(line, "▌", " ")) {
			return
		}
	}
	t.Errorf("no row matches %s:\n%s", pattern, screen(m))
}

func TestFormIsCenteredOverDimmedScreen(t *testing.T) {
	m := newModel(newFake())
	press(m, "2", "a")
	typeText(m, newURL)
	raw := strings.Split(m.View().Content, "\n")
	if strings.Contains(raw[0], accentSGR) || !strings.Contains(raw[0], dimSGR) {
		t.Errorf("the tab bar under the form is not dimmed: %q", raw[0])
	}
	expectScreen(t, m, "╭", "╯", "Add repo")
	// Blank fields show what they default to, from the URL typed so far.
	expectScreen(t, m, "products/new", "ask the remote")
	labelled := false
	for _, line := range raw {
		if i := strings.Index(line, "URL"); i > 0 && strings.Contains(line[:i], accentSGR) {
			labelled = true
		}
	}
	if !labelled {
		t.Error("the focused label is not in the accent color")
	}
	press(m, "esc")
	if strings.Contains(screen(m), "Add repo") {
		t.Error("esc did not close the form")
	}
}

func TestStatesHaveSymbolsAndWords(t *testing.T) {
	f := newFake()
	f.repos[0].Status.State = checkout.PathConflict
	f.repos[1].Status.State = checkout.RemoteMismatch
	f.servers[1].MissingEnv = []string{"TRACKER_TOKEN"}
	m := newModel(f)
	expectScreen(t, m, "✓ ok", "○ disabled", "○ off", "! env-missing")
	selected := false
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, "▌") && strings.Contains(line, selectedSGR) {
			selected = true
		}
	}
	if !selected {
		t.Error("the cursor row has no bar and highlight")
	}
	press(m, "2")
	expectScreen(t, m, "✗ path-conflict", "! remote-mismatch")
	m = newModel(newFake())
	press(m, "2")
	expectScreen(t, m, "○ "+string(checkout.Missing))
}

func TestKeySheet(t *testing.T) {
	m := newModel(newFake())
	press(m, "?")
	expectScreen(t, m, "Keys", "EVERYWHERE", "TEAM", "update pin", "? or esc closes")
	if isQuit(press(m, "q")) {
		t.Error("q quit under the key sheet")
	}
	press(m, "?")
	if strings.Contains(screen(m), "EVERYWHERE") {
		t.Error("? did not close the key sheet")
	}
	press(m, "?", "esc")
	if strings.Contains(screen(m), "EVERYWHERE") {
		t.Error("esc did not close the key sheet")
	}
}

func TestHelpLineHasTheTabsKeys(t *testing.T) {
	m := newModel(newFake())
	common := []string{"tab tabs", "? keys", "q quit"}
	cases := []struct {
		tab  string
		want []string
		not  string
	}{
		{"1", []string{"space use", "s sync"}, "a add"},
		{"2", []string{"a add", "d remove", "c clone all", "f fix remote"}, "space"},
		{"3", []string{"a add", "d remove", "u update pin"}, "clone"},
		{"4", []string{"a add", "d remove"}, "update"},
	}
	for _, c := range cases {
		press(m, c.tab)
		all := lines(m)
		help := all[len(all)-1]
		for _, want := range append(c.want, common...) {
			if !strings.Contains(help, want) {
				t.Errorf("tab %s help line lacks %q: %q", c.tab, want, help)
			}
		}
		if strings.Contains(help, c.not) {
			t.Errorf("tab %s help line has %q: %q", c.tab, c.not, help)
		}
	}
}

func TestOneLineResultIsAToast(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "1", "down", "space")
	all := lines(m)
	if toast := all[len(all)-2]; !strings.Contains(toast, "protocol: https") {
		t.Errorf("the line above the help line is %q", toast)
	}
}

package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/tui"
)

// Mine rows of newFake, counted from the top: 3 protocol rows, then the
// agents, the repos, the skills and the servers.
const (
	rowCodex   = 4
	rowApp     = 5
	rowWeb     = 6
	rowPDF     = 7
	rowTracker = 11
)

// toMineRow moves the cursor to Mine's row at index.
func toMineRow(m *tui.Model, index int) {
	press(m, "1")
	for range index {
		press(m, "down")
	}
}

// lineIndex is the first screen line that holds text, or -1.
func lineIndex(m *tui.Model, text string) int {
	for i, line := range lines(m) {
		if strings.Contains(line, text) {
			return i
		}
	}
	return -1
}

func TestMineListsReposBetweenAgentsAndSkills(t *testing.T) {
	f := newFake()
	f.repos[1].Chosen = false
	m := tallModel(f)
	agents := lineIndex(m, "AGENTS")
	repos := lineIndex(m, "REPO")
	skills := lineIndex(m, "SKILL")
	if agents < 0 || repos <= agents || skills <= repos {
		t.Fatalf("AGENTS at %d, REPO at %d, SKILL at %d:\n%s", agents, repos, skills, screen(m))
	}
	app := lines(m)[lineIndex(m, "products/app")]
	for _, want := range []string{"[✓]", "app", "○ " + string(checkout.Missing), "cloned by the next sync"} {
		if !strings.Contains(app, want) {
			t.Errorf("app row lacks %q: %q", want, app)
		}
	}
	web := lines(m)[lineIndex(m, "products/web")]
	for _, want := range []string{"[ ]", "web", core.NotSelectedLabel} {
		if !strings.Contains(web, want) {
			t.Errorf("web row lacks %q: %q", want, web)
		}
	}
	// A repo that is not chosen but cloned shows its checkout, which stays.
	f.repos[1].Status.State = checkout.OK
	m = tallModel(f)
	web = lines(m)[lineIndex(m, "products/web")]
	for _, want := range []string{"[ ]", "✓ ok", "not chosen; the checkout is kept"} {
		if !strings.Contains(web, want) {
			t.Errorf("cloned web row lacks %q: %q", want, web)
		}
	}
	toMineRow(m, rowApp)
	if row := cursorRow(t, m); !strings.Contains(row, "products/app") {
		t.Errorf("the cursor is on %q, want the app row", row)
	}
	expectClipboard(t, press(m, "y"), f.repos[0].Repo.URL)
}

func TestMineWithoutReposShowsEmptyStateAndKeepsRowsInPlace(t *testing.T) {
	f := newFake()
	f.home.Team.Repos = nil
	f.repos = nil
	m := tallModel(f)
	expectScreen(t, m, "no repos in "+brand.TeamFile+"; add one on the Repos tab")
	// Without repo rows, the first skill is right under the agents.
	toMineRow(m, rowWeb-1)
	if row := cursorRow(t, m); !strings.Contains(row, "pdf") {
		t.Fatalf("the cursor is on %q, want pdf", row)
	}
	press(m, "space")
	expectCalls(t, f, "EnableSkills [pdf]")
	_, y := find(t, m, 1, "pdf")
	x, _ := find(t, m, y, "[ ]")
	click(m, x+1, y)
	expectCalls(t, f, "EnableSkills [pdf]")
}

// When listing the repos fails, the problems say why; the empty-team line
// is not shown and the rows below stay in place.
func TestMineWithReposThatFailToListShowsNoEmptyState(t *testing.T) {
	f := newFake()
	f.repos = nil
	f.reposErr = apperr.New(apperr.Env, "listing the repos failed")
	m := tallModel(f)
	expectScreen(t, m, "listing the repos failed")
	if text := screen(m); strings.Contains(text, "no repos in "+brand.TeamFile) {
		t.Errorf("empty-team line shown:\n%s", text)
	}
	toMineRow(m, rowWeb-1)
	if row := cursorRow(t, m); !strings.Contains(row, "pdf") {
		t.Fatalf("the cursor is on %q, want pdf", row)
	}
	press(m, "space")
	expectCalls(t, f, "EnableSkills [pdf]")
	_, y := find(t, m, 1, "pdf")
	x, _ := find(t, m, y, "[ ]")
	click(m, x+1, y)
	expectCalls(t, f, "EnableSkills [pdf]")
}

func TestSpaceOnRepoRowTogglesTheChoice(t *testing.T) {
	f := newFake()
	m := tallModel(f)
	toMineRow(m, rowApp)
	press(m, "space")
	expectCalls(t, f, "DisableRepos [app]")
	expectScreen(t, m, "disabled: app (saved to "+brand.LocalFile+")")
	press(m, "space")
	expectCalls(t, f, "EnableRepos [app]")
	expectScreen(t, m, "enabled: app (saved to "+brand.LocalFile+")")
	// A click on the checkbox is space on that row.
	_, y := find(t, m, 1, "products/web")
	x, _ := find(t, m, y, "[✓]")
	click(m, x+1, y)
	expectCalls(t, f, "DisableRepos [web]")
	expectScreen(t, m, "disabled: web (saved to "+brand.LocalFile+")")
	_, y = find(t, m, 1, "products/web")
	x, _ = find(t, m, y, "[ ]")
	click(m, x+1, y)
	expectCalls(t, f, "EnableRepos [web]")
}

func TestRepoChoiceWaitsForTheRunningAction(t *testing.T) {
	f := newFake()
	f.block = true
	m := tallModel(f)
	cmd := press(m, "2", "c")
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	<-f.started
	toMineRow(m, rowApp)
	press(m, "space")
	_, y := find(t, m, 1, "products/web")
	x, _ := find(t, m, y, "[✓]")
	click(m, x+1, y)
	expectScreen(t, m, "busy: clone is running")
	press(m, "ctrl+c")
	m.Update(<-done)
	expectCalls(t, f, "Clone []")
}

func TestMineSyncKeySaysItClones(t *testing.T) {
	f := newFake()
	m := newModel(f)
	help := lines(m)[len(lines(m))-1]
	if !strings.Contains(help, "s clone + sync") {
		t.Errorf("Mine's help line: %q", help)
	}
	finish(t, m, press(m, "s"))
	expectCalls(t, f, "Sync force=false")
	press(m, "?")
	expectScreen(t, m, "clone + sync", "clone chosen (Repos)")
}

func TestReposTabClonesChosenAndShowsNotSelected(t *testing.T) {
	f := newFake()
	f.repos[1].Chosen = false
	m := newModel(f)
	press(m, "2")
	expectScreen(t, m, "○ "+string(checkout.Missing), "○ "+core.NotSelectedLabel)
	if web := lines(m)[lineIndex(m, "web")]; !strings.Contains(web, core.NotSelectedLabel) {
		t.Errorf("web row: %q", web)
	}
	help := lines(m)[len(lines(m))-1]
	if !strings.Contains(help, "c clone chosen") || strings.Contains(help, "clone all") {
		t.Errorf("Repos help line: %q", help)
	}
	// core picks the chosen repos when Clone gets no names.
	finish(t, m, press(m, "c"))
	expectCalls(t, f, "Clone []")
}

func TestQuitReportsWhetherToSync(t *testing.T) {
	cases := []struct {
		name      string
		subagents bool
		// refused makes the skill enable fail.
		refused bool
		keys    []string
		want    bool
	}{
		{"nothing changed", false, false, nil, false},
		{"agent", false, false, []string{"1", "down", "down", "down", "space"}, true},
		{"repo", false, false, mineRowKeys(rowWeb, "space"), true},
		{"skill", false, false, mineRowKeys(rowPDF, "space"), true},
		{"MCP", false, false, mineRowKeys(rowTracker, "space"), true},
		{"subagent", true, false, mineRowKeys(subagentsStart, "space"), true},
		{"protocol only", false, false, []string{"1", "enter", "down", "space"}, false},
		{"team tabs only", false, false, []string{"2", "d", "d", "4", "d", "d"}, false},
		{"refused change", false, true, mineRowKeys(rowPDF, "space"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake()
			if c.subagents {
				f = subagentFake()
			}
			if c.refused {
				f.enableErr = apperr.New(apperr.Usage, "cannot enable pdf")
			}
			m := tallModel(f)
			press(m, c.keys...)
			if len(f.calls) == 0 && c.keys != nil {
				t.Fatalf("the keys called nothing")
			}
			if !isQuit(press(m, "q")) {
				t.Fatal("q did not quit")
			}
			if got := m.SyncOnQuit(); got != c.want {
				t.Errorf("SyncOnQuit = %v, want %v; calls %v", got, c.want, f.calls)
			}
			if got, err := m.Finish(nil); got != c.want || err != nil {
				t.Errorf("Finish(nil) = %v, %v", got, err)
			}
		})
	}
}

// mineRowKeys are the keys that move to Mine's row at index, then more.
func mineRowKeys(index int, more ...string) []string {
	keys := []string{"1"}
	for range index {
		keys = append(keys, "down")
	}
	return append(keys, more...)
}

func TestSuccessfulSyncClearsTheChangeAndAFailedOneKeepsIt(t *testing.T) {
	f := newFake()
	m := tallModel(f)
	toMineRow(m, rowCodex)
	press(m, "space")
	finish(t, m, press(m, "s"))
	press(m, "q")
	if m.SyncOnQuit() {
		t.Error("a sync succeeded after the change, yet quit asks for another")
	}
	f.syncErr = apperr.Exit(apperr.ExitFailed)
	m = tallModel(f)
	toMineRow(m, rowApp)
	press(m, "space")
	finish(t, m, press(m, "s"))
	press(m, "q")
	if !m.SyncOnQuit() {
		t.Error("a failed sync cleared the change")
	}
	expectCalls(t, f, "SetAgents [claude codex]", "Sync force=false", "DisableRepos [app]", "Sync force=false")
}

func TestCtrlCNeverAsksForASync(t *testing.T) {
	f := newFake()
	m := tallModel(f)
	toMineRow(m, rowApp)
	press(m, "space")
	if !isQuit(press(m, "ctrl+c")) {
		t.Fatal("ctrl+c with nothing running did not quit")
	}
	if m.SyncOnQuit() {
		t.Error("ctrl+c asked for a sync")
	}
	// An interrupted program, as Run sees it, asks for none either.
	m = tallModel(f)
	toMineRow(m, rowApp)
	press(m, "space", "q")
	got, err := m.Finish(tea.ErrInterrupted)
	if got || !errors.Is(err, context.Canceled) {
		t.Errorf("Finish(ErrInterrupted) = %v, %v", got, err)
	}
	failure := errors.New("no terminal")
	if got, err := m.Finish(failure); got || !errors.Is(err, failure) {
		t.Errorf("Finish(failure) = %v, %v", got, err)
	}
}

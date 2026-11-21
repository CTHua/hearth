package tui_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/skill"
	"github.com/CTHua/hearth/internal/tui"
)

// groupFake is the fake with a group of two skills, whose choices follow
// EnableSkills and DisableSkills so the screen can show them.
type groupFake struct {
	*fake
}

func newGroupFake() *groupFake {
	f := newFake()
	f.home.Team.Skills = append(f.home.Team.Skills, config.Skill{Name: "kit", URL: newURL, Commit: "fedcba9876543210", Subdirs: []string{"a/one", "a/two"}})
	f.skills = []skill.Row{
		{Name: "pdf", Source: skill.External, State: skill.Disabled},
		{Name: "one", Group: "kit", Source: skill.External, State: skill.Disabled},
		{Name: "two", Group: "kit", Source: skill.External, State: skill.Disabled},
		{Name: "notes", Source: skill.Team, Enabled: true, State: skill.OK},
		{Name: brand.BuiltinSkill, Source: skill.Builtin, Enabled: true, State: skill.OK},
	}
	return &groupFake{fake: f}
}

func (g *groupFake) choose(names []string, on bool) {
	for i := range g.skills {
		if slices.Contains(names, g.skills[i].Name) {
			g.skills[i].Enabled = on
		}
	}
}

func (g *groupFake) EnableSkills(names []string) ([]string, error) {
	added, err := g.fake.EnableSkills(names)
	g.choose(names, true)
	return added, err
}

func (g *groupFake) DisableSkills(names []string) ([]string, error) {
	removed, err := g.fake.DisableSkills(names)
	g.choose(names, false)
	return removed, err
}

// kitTitle is the cursor presses from the top of Mine to the group's title
// row: 3 protocol rows, 2 agent rows, 2 repo rows, then pdf.
var kitTitle = []string{"1", "down", "down", "down", "down", "down", "down", "down", "down"}

func TestMineShowsGroupTitleAndTogglesTheGroup(t *testing.T) {
	g := newGroupFake()
	m := tui.New(context.Background(), g)
	press(m, "1")
	expectScreen(t, m, "kit (0/2)", "  one", "  two")
	press(m, kitTitle...)
	if row := cursorRow(t, m); !strings.Contains(row, "kit (0/2)") {
		t.Fatalf("cursor on %q", row)
	}
	press(m, "space")
	expectCalls(t, g.fake, "EnableSkills [one two]")
	expectScreen(t, m, "kit (2/2)")
	press(m, "space")
	expectCalls(t, g.fake, "DisableSkills [one two]")
	expectScreen(t, m, "kit (0/2)")
	// With one member on, the title turns the rest on.
	press(m, "down", "space")
	expectCalls(t, g.fake, "EnableSkills [one]")
	expectScreen(t, m, "kit (1/2)", "[-]")
	press(m, "up", "space")
	expectCalls(t, g.fake, "EnableSkills [two]")
	// The rows after the group are still reached: notes is two below two.
	press(m, "down", "down", "down", "space")
	expectCalls(t, g.fake, "DisableSkills [notes]")
}

func TestClickOnGroupTitleTogglesTheGroup(t *testing.T) {
	g := newGroupFake()
	m := tui.New(context.Background(), g)
	press(m, "1")
	_, y := find(t, m, 1, "kit (0/2)")
	x, _ := find(t, m, y, "[ ]")
	click(m, x+1, y)
	expectCalls(t, g.fake, "EnableSkills [one two]")
	_, y = find(t, m, 1, "kit (2/2)")
	x, _ = find(t, m, y, "[✓]")
	click(m, x+1, y)
	expectCalls(t, g.fake, "DisableSkills [one two]")
}

func TestInvalidMembersAreLeftOffByTheTitle(t *testing.T) {
	g := newGroupFake()
	g.skills[2].Invalid = "name is also used"
	m := tui.New(context.Background(), g)
	press(m, kitTitle...)
	press(m, "space")
	expectCalls(t, g.fake, "EnableSkills [one]")
	// Every member that can be on is on, so the title turns them off.
	press(m, "space")
	expectCalls(t, g.fake, "DisableSkills [one]")
	g.skills[1].Invalid = "name is also used"
	press(m, "space")
	expectCalls(t, g.fake)
	expectScreen(t, m, "no skill of group kit can be turned on")
}

func TestSkillsTabShowsGroupAsOneRow(t *testing.T) {
	g := newGroupFake()
	m := tui.New(context.Background(), g)
	press(m, "3")
	expectScreen(t, m, "kit", "group of 2")
	press(m, "down", "d", "d")
	expectCalls(t, g.fake, "RemoveSkill kit")
}

func TestAddSkillWithBlankSubdirShowsTheGroup(t *testing.T) {
	f := newFake()
	m := newModel(f)
	press(m, "3", "a")
	expectScreen(t, m, "blank finds every SKILL.md", "several as a group")
	typeText(m, skillURL)
	finish(t, m, press(m, "enter"))
	expectCalls(t, f, "AddSkill "+skillURL+" name=<nil> subdir=<nil> commit=<nil>")
	expectScreen(t, m, "added group tools at abc: 2 skill(s) added", "found 3 skill(s)", "extra/other: not listed in")
}

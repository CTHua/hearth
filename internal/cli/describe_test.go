package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/version"
)

const (
	webDescription = "Customer-facing web app"
	pdfSkill       = "\n[[skills]]\nname = \"pdf\"\nurl = \"https://git.example.invalid/acme/skills.git\"\ncommit = \"0123456789abcdef0123456789abcdef01234567\"\nsubdir = \"pdf\"\n"
)

func loadTeam(t *testing.T, home string) config.Team {
	t.Helper()
	loaded, err := config.Load(home, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	return loaded.Team
}

// describedHome has web with a description, api without, a skill and an
// MCP server.
func describedHome(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	run(t, "-C", home, "repo", "add", httpsURL("web"), "--branch", "main", "--description", webDescription).expect(t, exitOK, "")
	run(t, "-C", home, "repo", "add", httpsURL("api"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "add", "m", "--", "npx", "server").expect(t, exitOK, "")
	write(t, teamFile(home), read(t, teamFile(home))+pdfSkill)
	return home
}

func TestRepoAddWritesDescriptionAfterBranch(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main", "--description", "  訂單 \"API\"  ")
	r.expect(t, exitOK, "")
	want := "branch = \"main\"\ndescription = \"訂單 \\\"API\\\"\"\n"
	if text := read(t, teamFile(home)); !strings.HasSuffix(text, want) {
		t.Errorf("team file:\n%s", text)
	}
	other := newHome(t)
	run(t, "-C", other, "repo", "add", appHTTPS, "--branch", "main", "--description", "   ").expect(t, exitOK, "")
	if text := read(t, teamFile(other)); strings.Contains(text, config.FieldDescription) {
		t.Errorf("blank --description written:\n%s", text)
	}
}

func TestRepoAddRejectsBadDescriptionBeforeConnecting(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, appHTTPS, filepath.Join(t.TempDir(), "missing.git"))
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	before := read(t, teamFile(home))
	tooLong := strings.Repeat("x", config.DescriptionMaxLength+1)
	cases := [][]string{
		{appHTTPS, "--branch", "main", "--description", "a\tb"},
		{appHTTPS, "--branch", "main", "--description", "a\nb"},
		{appHTTPS, "--branch", "main", "--description", tooLong},
		// Without --branch the remote would be asked; the description fails first.
		{appHTTPS, "--description", tooLong},
	}
	for _, args := range cases {
		r := run(t, append([]string{"-C", home, "repo", "add"}, args...)...)
		if r.code != exitBad || !strings.Contains(r.stderr, "error [usage]") || !strings.Contains(r.stderr, "--description") {
			t.Errorf("%q: exit %d stderr %q", args, r.code, r.stderr)
		}
		if read(t, teamFile(home)) != before {
			t.Fatalf("%q changed the team file", args)
		}
	}
}

func TestRepoDescribeChangesOnlyThatRepo(t *testing.T) {
	home := describedHome(t)
	before := loadTeam(t, home)
	r := run(t, "-C", home, "repo", "describe", "api", "  訂單 API ")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "described api: 訂單 API") || !strings.Contains(r.stdout, brand.TeamFileChangedHint) {
		t.Errorf("stdout: %s", r.stdout)
	}
	after := loadTeam(t, home)
	if after.Repos[1].Description != "訂單 API" {
		t.Errorf("api description = %q", after.Repos[1].Description)
	}
	after.Repos[1].Description = ""
	if !reflect.DeepEqual(after, before) {
		t.Errorf("other entries changed:\n got %+v\nwant %+v", after, before)
	}
	r = run(t, "-C", home, "repo", "describe", "api", "")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "removed the description of api") {
		t.Errorf("stdout: %s", r.stdout)
	}
	if !reflect.DeepEqual(loadTeam(t, home), before) {
		t.Errorf("after clearing: %+v, want %+v", loadTeam(t, home), before)
	}
	if strings.Count(read(t, teamFile(home)), config.FieldDescription+" = ") != 1 {
		t.Errorf("api still has the key:\n%s", read(t, teamFile(home)))
	}
}

func TestRepoDescribeRejectsAndLeavesFileAlone(t *testing.T) {
	home := describedHome(t)
	before := read(t, teamFile(home))
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"nope", "x"}, "unknown repo name(s): nope; available: web, api"},
		{[]string{"api"}, "error [usage]"},
		{[]string{"api", "x", "y"}, "error [usage]"},
		{[]string{}, "error [usage]"},
		{[]string{"api", "a\nb"}, "description"},
		{[]string{"api", strings.Repeat("x", config.DescriptionMaxLength+1)}, "description"},
	}
	for _, c := range cases {
		r := run(t, append([]string{"-C", home, "repo", "describe"}, c.args...)...)
		if r.code != exitBad || !strings.Contains(r.stderr, "error [usage]") || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%q: exit %d stderr %q, want usage error with %q", c.args, r.code, r.stderr, c.want)
		}
		if read(t, teamFile(home)) != before {
			t.Fatalf("%q changed the team file", c.args)
		}
	}
}

func TestRepoDescribeSameValueIsUnchanged(t *testing.T) {
	home := describedHome(t)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(teamFile(home), past, past); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"web", webDescription}, {"api", ""}} {
		r := run(t, append([]string{"-C", home, "repo", "describe"}, args...)...)
		r.expect(t, exitOK, "")
		if !strings.HasPrefix(r.stdout, "unchanged "+args[0]) || strings.Contains(r.stdout, brand.TeamFileChangedHint) {
			t.Errorf("%q: stdout %s", args, r.stdout)
		}
	}
	info, err := os.Stat(teamFile(home))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("mtime %v, want %v", info.ModTime(), past)
	}
}

func TestRepoListShowsDescriptionLast(t *testing.T) {
	home := describedHome(t)
	r := run(t, "-C", home, "repo", "list")
	r.expect(t, exitOK, "")
	lines := strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
	if got := strings.Join(strings.Fields(lines[0]), " "); got != "NAME PATH BRANCH STATUS DETAIL DESCRIPTION" {
		t.Errorf("header %q", got)
	}
	column := strings.Index(lines[0], "DESCRIPTION")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "web ") || lines[1][column:] != webDescription {
		t.Errorf("web row:\n%s", r.stdout)
	}
	if strings.TrimSpace(lines[2][min(column, len(lines[2])):]) != "" {
		t.Errorf("api row has a description:\n%s", r.stdout)
	}
}

func TestDoctorWarnsAboutMissingDescriptions(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", httpsURL("web"), "--branch", "main", "--description", webDescription).expect(t, exitOK, "")
	run(t, "-C", home, "repo", "add", httpsURL("api"), "--branch", "main").expect(t, exitOK, "")
	r := run(t, "-C", home, "doctor")
	// The same exit as without the warning: the repos are not cloned yet,
	// which is only a warning too.
	r.expect(t, exitOK, "")
	var warnings []string
	for _, line := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(line, "warning: ") && strings.Contains(line, config.FieldDescription) {
			warnings = append(warnings, line)
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "api") || !strings.Contains(warnings[0], "repo describe api") {
		t.Errorf("description warnings %q in:\n%s", warnings, r.stdout)
	}
	run(t, "-C", home, "repo", "describe", "api", "Orders API").expect(t, exitOK, "")
	r = run(t, "-C", home, "doctor")
	r.expect(t, exitOK, "")
	if strings.Contains(r.stdout, "repo describe") {
		t.Errorf("warning with every description set:\n%s", r.stdout)
	}
}

func TestHelpListsDescription(t *testing.T) {
	wants := map[string][]string{
		"help":     {"--description <text>", "repo describe <name> <text>"},
		"add":      {"--description <text>"},
		"describe": {"repo describe <name> <text>"},
	}
	for command, want := range wants {
		args := []string{"help"}
		if command != "help" {
			args = []string{"repo", command, "--help"}
		}
		r := run(t, args...)
		r.expect(t, exitOK, "")
		for _, text := range want {
			if !strings.Contains(r.stdout, text) {
				t.Errorf("%v lacks %q:\n%s", args, text, r.stdout)
			}
		}
	}
	run(t, "-C", newHome(t), "repo", "frobnicate").expect(t, exitBad, "describe")
}

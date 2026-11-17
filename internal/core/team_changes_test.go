package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/version"
)

const (
	teamHeader = "schema_version = 1\n" + brand.MinVersionKey + " = \"0.1.0\"\n"
	repoA      = "\n[[repos]]\nname = \"a\"\nurl = \"git@git.example.invalid:acme/a.git\"\npath = \"products/a\"\nbranch = \"main\"\n"
	repoADev   = "\n[[repos]]\nname = \"a\"\nurl = \"git@git.example.invalid:acme/a.git\"\npath = \"products/a\"\nbranch = \"dev\"\n"
	repoB      = "\n[[repos]]\nname = \"b\"\nurl = \"git@git.example.invalid:acme/b.git\"\npath = \"products/b\"\nbranch = \"main\"\n"
	repoC      = "\n[[repos]]\nname = \"c\"\nurl = \"git@git.example.invalid:acme/c.git\"\npath = \"products/c\"\nbranch = \"main\"\n"
	serverM    = "\n[[mcp]]\nname = \"m\"\ntransport = \"stdio\"\ncommand = \"npx\"\n"
	// committedTeam and editedTeam differ in all three ways: a is modified,
	// c added and b removed; the server is the same.
	committedTeam = teamHeader + repoA + repoB + serverM
	editedTeam    = teamHeader + repoADev + repoC + serverM
	otherFile     = "README.md"
)

var editedChanges = []core.TeamChange{
	{Table: core.TableRepos, Name: "a", Kind: core.Modified},
	{Table: core.TableRepos, Name: "c", Kind: core.Added},
	{Table: core.TableRepos, Name: "b", Kind: core.Removed},
}

// homeIn makes a git repo with the home root at sub (empty for the top),
// commits committed as the team file (nothing when empty, or no commit at
// all when commit is false), then writes current and loads it.
func homeIn(t *testing.T, sub string, commit bool, committed string, current string) *core.Service {
	t.Helper()
	top := t.TempDir()
	gittest.Git(t, top, "init", "--quiet")
	root := filepath.Join(top, sub)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if commit {
		file := otherFile
		content := "x\n"
		if committed != "" {
			file = brand.TeamFile
			content = committed
		}
		writeFile(t, filepath.Join(root, file), content)
		gittest.Git(t, top, "add", "--all")
		gittest.Git(t, top, "commit", "--quiet", "-m", "seed")
	}
	writeFile(t, filepath.Join(root, brand.TeamFile), current)
	return load(t, root)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, root string) *core.Service {
	t.Helper()
	home, err := config.Load(root, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	return core.New(home)
}

func expectChanges(t *testing.T, s *core.Service, want []core.TeamChange) {
	t.Helper()
	got, err := s.TeamChanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("changes %+v, want %+v", got, want)
	}
}

func TestTeamChangesAgainstHead(t *testing.T) {
	expectChanges(t, homeIn(t, "", true, committedTeam, editedTeam), editedChanges)
	expectChanges(t, homeIn(t, "", true, committedTeam, committedTeam), nil)
}

func TestTeamChangesWithHomeInSubdirectory(t *testing.T) {
	expectChanges(t, homeIn(t, filepath.Join("teams", "home"), true, committedTeam, editedTeam), editedChanges)
	// git prints the prefix as is; only its newline is not part of the path.
	expectChanges(t, homeIn(t, filepath.Join(" teams", "home "), true, committedTeam, editedTeam), editedChanges)
}

func TestTeamChangesSkillsAndServers(t *testing.T) {
	s := homeIn(t, "", true, committedTeam+pdfSkillEntry, committedTeam)
	server := config.MCP{Name: "n", Transport: config.TransportHTTP, URL: "https://mcp.example.invalid/mcp"}
	if err := s.AddMCP(server); err != nil {
		t.Fatal(err)
	}
	// Written back with an empty list rather than none: still the same entry.
	if _, err := s.RemoveMCP("m"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCP(config.MCP{Name: "m", Transport: config.TransportStdio, Command: "npx", Args: []string{}}); err != nil {
		t.Fatal(err)
	}
	expectChanges(t, s, []core.TeamChange{
		{Table: core.TableSkills, Name: "pdf", Kind: core.Removed},
		{Table: core.TableMCP, Name: "n", Kind: core.Added},
	})
}

const pdfSkillEntry = "\n[[skills]]\nname = \"pdf\"\nurl = \"https://git.example.invalid/acme/skills.git\"\ncommit = \"0123456789abcdef0123456789abcdef01234567\"\nsubdir = \"pdf\"\n"

func TestTeamChangesWithNothingToCompare(t *testing.T) {
	noRepo := t.TempDir()
	writeFile(t, filepath.Join(noRepo, brand.TeamFile), committedTeam)
	cases := map[string]*core.Service{
		"not a git repo":       load(t, noRepo),
		"no commit":            homeIn(t, "", false, "", committedTeam),
		"no team file in HEAD": homeIn(t, "", true, "", committedTeam),
	}
	for name, s := range cases {
		got, err := s.TeamChanges(context.Background())
		if err != nil || got != nil {
			t.Errorf("%s: changes %+v, err %v; want neither", name, got, err)
		}
	}
}

func TestTeamChangesWithUnreadableHead(t *testing.T) {
	s := homeIn(t, "", true, teamHeader+"extra = 1\nmore = 2\n", committedTeam)
	got, err := s.TeamChanges(context.Background())
	if got != nil {
		t.Errorf("changes %+v with an unreadable HEAD", got)
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Category != apperr.Config {
		t.Fatalf("err %T %v, want a config error", err, err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error is more than one line: %q", err)
	}
}

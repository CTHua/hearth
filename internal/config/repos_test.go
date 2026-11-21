package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
)

const (
	personalHeader = "schema_version = 3\n"
	apiRepo        = `
[[repos]]
name = "api"
url = "git@git.example.invalid:acme/api.git"
path = "products/api"
branch = "main"
`
)

// chosenNames lists the team repos home's personal file chooses.
func chosenNames(home *config.Home) []string {
	var names []string
	for _, repo := range home.Team.Repos {
		if home.Personal.RepoChosen(repo.Name) {
			names = append(names, repo.Name)
		}
	}
	return names
}

func TestRepoChoiceLoadsAndWritesBack(t *testing.T) {
	cases := []struct {
		name     string
		personal string
		chosen   string
		// line is the repos line the file has before and after a rewrite;
		// empty means no repos key.
		line string
	}{
		{"no key chooses every repo", personalHeader + "protocol = \"ssh\"\n", "web-app api", ""},
		{"listed names only", personalHeader + "repos = [\"api\"]\n", "api", "repos = [\"api\"]"},
		{"empty list chooses none", personalHeader + "repos = []\n", "", "repos = []"},
		{"stale name is not chosen", personalHeader + "repos = [\"gone\", \"web-app\"]\n", "web-app", "repos = [\"gone\", \"web-app\"]"},
		{"names compare case-insensitively", personalHeader + "repos = [\"API\"]\n", "api", "repos = [\"API\"]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := writeFiles(t, header+webApp+apiRepo, c.personal)
			home, err := config.Load(root, released)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(chosenNames(home), " "); got != c.chosen {
				t.Errorf("chosen %q, want %q", got, c.chosen)
			}
			if err := config.WritePersonal(root, home.Personal); err != nil {
				t.Fatal(err)
			}
			text := readText(t, filepath.Join(root, brand.LocalFile))
			if c.line == "" && strings.Contains(text, "repos") {
				t.Errorf("rewritten file has a repos key:\n%s", text)
			}
			if c.line != "" && !strings.Contains(text, c.line+"\n") {
				t.Errorf("rewritten file lacks %q:\n%s", c.line, text)
			}
			again, err := config.Load(root, released)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(chosenNames(again), " "); got != c.chosen {
				t.Errorf("after rewrite chosen %q, want %q", got, c.chosen)
			}
		})
	}
}

func TestWritePersonalPutsReposAfterAgents(t *testing.T) {
	root := writeFiles(t, header+webApp, "")
	personal := config.Personal{}.WithProtocol("ssh").WithAgents([]string{"claude"}).WithSkills([]string{"pdf"}).WithRepos([]string{"web-app"})
	if err := config.WritePersonal(root, personal); err != nil {
		t.Fatal(err)
	}
	want := personalHeader + "protocol = \"ssh\"\nagents = [\"claude\"]\nrepos = [\"web-app\"]\nskills = [\"pdf\"]\n"
	if got := readText(t, filepath.Join(root, brand.LocalFile)); got != want {
		t.Errorf("personal file %q, want %q", got, want)
	}
}

func TestRepoChoiceRulesNameFileAndKey(t *testing.T) {
	for _, body := range []string{
		"repos = \"api\"\n",
		"repos = [1]\n",
		"repos = [\"bad name\"]\n",
		"repos = [\"\"]\n",
		"repos = [\"api\", \"API\"]\n",
	} {
		lines, categories := loadErrors(t, writeFiles(t, header+apiRepo, personalHeader+body), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.LocalFile+": repos") {
			t.Errorf("%q: %s", body, lines)
		}
	}
}

// Every choice a personal command makes starts a missing personal file at
// the personal schema, while init's team file stays at the team schema.
func TestNewFilesUseEachFilesSchema(t *testing.T) {
	started := map[string]config.Personal{
		"protocol":  config.Personal{}.WithProtocol("ssh"),
		"agents":    config.Personal{}.WithAgents([]string{"claude"}),
		"skills":    config.Personal{}.WithSkills([]string{"pdf"}),
		"mcp":       config.Personal{}.WithMCPChoices([]string{"docs"}),
		"subagents": config.Personal{}.WithSubagents([]string{"reviewer"}),
		"repos":     config.Personal{}.WithRepos([]string{}),
	}
	for name, personal := range started {
		if personal.SchemaVersion != config.PersonalSchemaVersion || config.PersonalSchemaVersion != 3 {
			t.Errorf("%s: schema_version %d", name, personal.SchemaVersion)
		}
	}
	team := config.NewTeam()
	if team.SchemaVersion != 2 || team.MinVersion != "0.2.0" {
		t.Errorf("new team file at %d, %s", team.SchemaVersion, team.MinVersion)
	}
}

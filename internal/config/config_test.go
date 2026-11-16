package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
)

const (
	released = "0.1.0"
	header   = "schema_version = 1\n" + brand.MinVersionKey + " = \"0.1.0\"\n"
	webApp   = `
[[repos]]
name = "web-app"
url = "git@git.example.invalid:acme/web-app.git"
path = "products/web-app"
branch = "main"
`
)

func writeFiles(t *testing.T, team string, personal string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, brand.TeamFile), []byte(team), 0o644); err != nil {
		t.Fatal(err)
	}
	if personal != "" {
		if err := os.WriteFile(filepath.Join(root, brand.LocalFile), []byte(personal), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// loadErrors returns the reported lines and the categories they carry.
func loadErrors(t *testing.T, root string, running string) (string, map[apperr.Category]bool) {
	t.Helper()
	_, err := config.Load(root, running)
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
	var list apperr.List
	if !errors.As(err, &list) {
		t.Fatalf("Load error %T %v, want apperr.List", err, err)
	}
	categories := map[apperr.Category]bool{}
	for _, e := range list {
		categories[e.Category] = true
	}
	return list.Error(), categories
}

func TestLoadValidTeamAndPersonal(t *testing.T) {
	root := writeFiles(t, header+webApp, "schema_version = 1\nprotocol = \"https\"\n")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.Repo{{Name: "web-app", URL: "git@git.example.invalid:acme/web-app.git", Path: "products/web-app", Branch: "main"}}
	if !reflect.DeepEqual(home.Team.Repos, want) {
		t.Errorf("repos = %+v, want %+v", home.Team.Repos, want)
	}
	if home.Personal.Protocol != "https" {
		t.Errorf("protocol = %q, want https", home.Personal.Protocol)
	}
	if len(home.Warnings) != 0 {
		t.Errorf("warnings = %v, want none for a released build", home.Warnings)
	}
}

func TestLoadAcceptsEmptyRepoList(t *testing.T) {
	for _, team := range []string{header, header + "repos = []\n"} {
		home, err := config.Load(writeFiles(t, team, ""), released)
		if err != nil {
			t.Fatalf("%q: %v", team, err)
		}
		if len(home.Team.Repos) != 0 || home.Personal.Protocol != "" {
			t.Errorf("%q: got %+v", team, home)
		}
	}
}

func TestLoadMissingTeamFileIsConfigError(t *testing.T) {
	_, err := config.Load(t.TempDir(), released)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Category != apperr.Config {
		t.Fatalf("err = %v, want config error", err)
	}
}

func TestPersonalFileProtocolRules(t *testing.T) {
	home, err := config.Load(writeFiles(t, header, "schema_version = 1\n"), released)
	if err != nil || home.Personal.Protocol != "" {
		t.Fatalf("protocol-less personal file: %+v, %v", home, err)
	}
	for _, personal := range []string{"schema_version = 1\nprotocol = \"git\"\n", "schema_version = 1\nprotocol = 1\n", "protocol = \"ssh\"\n"} {
		lines, categories := loadErrors(t, writeFiles(t, header, personal), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.LocalFile+": ") {
			t.Errorf("%q: %s", personal, lines)
		}
	}
}

func TestUnknownKeysAreConfigErrorsWithLocation(t *testing.T) {
	cases := map[string]string{
		header + "extra = 1\n":                             brand.TeamFile + ": unknown key extra",
		header + webApp + "owner = \"x\"\n":                brand.TeamFile + ": repos[0].owner: unknown key",
		header + "[team]\nname = \"x\"\n":                  brand.TeamFile + ": unknown key team",
		header + "repos = [{ name = \"a\" }]\n":            brand.TeamFile + ": repos[0].url: missing",
		header + "repos = \"x\"\n":                         brand.TeamFile + ": repos: must be an array of tables",
		header + strings.Replace(webApp, `"main"`, "1", 1): brand.TeamFile + ": repos[0].branch: must be a string",
	}
	for team, want := range cases {
		lines, categories := loadErrors(t, writeFiles(t, team, ""), released)
		if !categories[apperr.Config] || !strings.Contains(lines, want) {
			t.Errorf("team file %q:\n got %s\nwant %s", team, lines, want)
		}
	}
	lines, _ := loadErrors(t, writeFiles(t, header, "schema_version = 1\ncolor = \"red\"\n"), released)
	if !strings.Contains(lines, brand.LocalFile+": unknown key color") {
		t.Errorf("personal unknown key: %s", lines)
	}
}

func TestRepoValueRules(t *testing.T) {
	entry := func(name, url, path, branch string) string {
		return header + "[[repos]]\nname = \"" + name + "\"\nurl = \"" + url + "\"\npath = \"" + path + "\"\nbranch = \"" + branch + "\"\n"
	}
	url := "https://git.example.invalid/acme/app.git"
	cases := map[string]string{
		entry("-app", url, "products/app", "main"):                             "repos[0].name:",
		entry("a;b", url, "products/app", "main"):                              "repos[0].name:",
		entry("app", url, "app", "main"):                                       "repos[0].path:",
		entry("app", url, "products", "main"):                                  "repos[0].path:",
		entry("app", url, "products/../escape", "main"):                        "repos[0].path:",
		entry("app", url, "products/a b", "main"):                              "repos[0].path:",
		entry("app", url, "products/.git", "main"):                             "repos[0].path:",
		entry("app", url, "products/a;b", "main"):                              "repos[0].path:",
		entry("app", url, "products/$(touch x)", "main"):                       "repos[0].path:",
		entry("app", url, "../x", "main"):                                      "repos[0].path:",
		entry("app", url, "/tmp/outside", "main"):                              "repos[0].path:",
		entry("a b", url, "products/app", "main"):                              "repos[0].name:",
		entry("$(touch x)", url, "products/app", "main"):                       "repos[0].name:",
		entry("app", url, "products/app", "--upload-pack=x"):                   "repos[0].branch:",
		entry("app", "remotes/app", "products/app", "main"):                    "repos[0].url:",
		entry("app", url, "products/app", "-main"):                             "repos[0].branch:",
		entry("app", url, "products/app", "a..b"):                              "repos[0].branch:",
		entry("app", url, "products/app", ""):                                  "repos[0].branch:",
		entry("app", "/srv/git/app.git", "products/app", "main"):               "repos[0].url:",
		entry("app", "--upload-pack=x", "products/app", "main"):                "repos[0].url:",
		entry("app", "http://git.example.invalid/a/b", "products/app", "main"): "repos[0].url:",
	}
	for team, want := range cases {
		lines, categories := loadErrors(t, writeFiles(t, team, ""), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.TeamFile+": "+want) {
			t.Errorf("%s\n got %s\nwant %s", team, lines, want)
		}
	}
}

func TestDuplicateAndNestedEntriesAreCaseInsensitive(t *testing.T) {
	team := header + webApp + `
[[repos]]
name = "WEB-APP"
url = "https://git.example.invalid/acme/other.git"
path = "products/Web-App/sub"
branch = "main"

[[repos]]
name = "third"
url = "https://git.example.invalid/acme/third.git"
path = "products/WEB-APP"
branch = "main"
`
	lines, _ := loadErrors(t, writeFiles(t, team, ""), released)
	for _, want := range []string{"repos[1].name: duplicate name", "repos[1].path: nested paths", "repos[2].path: duplicate path"} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in:\n%s", want, lines)
		}
	}
	if count := strings.Count(lines, "\n") + 1; count < 3 {
		t.Errorf("want every problem listed, got %d line(s):\n%s", count, lines)
	}
}

func TestVersionGate(t *testing.T) {
	newer := "schema_version = 1\n" + brand.MinVersionKey + " = \"0.2.0\"\n"
	if _, categories := loadErrors(t, writeFiles(t, newer, ""), released); !categories[apperr.Version] {
		t.Error("older build than min version: want version error")
	}
	if _, err := config.Load(writeFiles(t, newer, ""), "0.2.0"); err != nil {
		t.Errorf("equal version: %v", err)
	}
	if _, err := config.Load(writeFiles(t, newer, ""), "0.10.0"); err != nil {
		t.Errorf("newer build: %v", err)
	}
	home, err := config.Load(writeFiles(t, newer, ""), "dev")
	if err != nil || len(home.Warnings) != 1 {
		t.Errorf("dev build: want one warning and no error, got %+v, %v", home, err)
	}
	// A newer schema fails as version even though it also has keys this build
	// does not know: the version check runs before strict parsing.
	future := "schema_version = 2\n" + brand.MinVersionKey + " = \"0.1.0\"\nnew_key = true\n"
	for _, running := range []string{released, "dev"} {
		lines, categories := loadErrors(t, writeFiles(t, future, ""), running)
		if !categories[apperr.Version] || categories[apperr.Config] {
			t.Errorf("schema 2 with %s: %s", running, lines)
		}
	}
	lines, categories := loadErrors(t, writeFiles(t, header, "schema_version = 2\nnew_key = 1\n"), released)
	if !categories[apperr.Version] || categories[apperr.Config] {
		t.Errorf("personal schema 2: %s", lines)
	}
	for _, bad := range []string{"schema_version = 1\n", brand.MinVersionKey + " = \"0.1.0\"\n", "schema_version = 1\n" + brand.MinVersionKey + " = \"v0.1.0\"\n"} {
		if _, categories := loadErrors(t, writeFiles(t, bad, ""), released); !categories[apperr.Config] {
			t.Errorf("%q: want config error", bad)
		}
	}
}

func TestWriteTeamKeepsOnlyHeaderComment(t *testing.T) {
	root := writeFiles(t, "# my own note\n"+header+webApp, "")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	team, err := home.Team.WithRepo(config.Repo{Name: "api", URL: "https://git.example.invalid/acme/api.git", Path: "products/api", Branch: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteTeam(root, team); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, brand.TeamFile))
	if err != nil {
		t.Fatal(err)
	}
	want := brand.TeamFileHeader + "\n" + header + webApp + `
[[repos]]
name = "api"
url = "https://git.example.invalid/acme/api.git"
path = "products/api"
branch = "develop"
`
	if string(data) != want {
		t.Errorf("team file:\n%s\nwant:\n%s", data, want)
	}
	reloaded, err := config.Load(root, released)
	if err != nil || !reflect.DeepEqual(reloaded.Team, team) {
		t.Errorf("round trip: %+v, %v; want %+v", reloaded.Team, err, team)
	}
}

func TestWriteNewTeamMatchesSpecExample(t *testing.T) {
	root := t.TempDir()
	if err := config.WriteTeam(root, config.NewTeam()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, brand.TeamFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := brand.TeamFileHeader + "\n" + header; string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestWritePersonal(t *testing.T) {
	root := writeFiles(t, header, "")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WritePersonal(root, home.Personal.WithProtocol("ssh")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, brand.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := "schema_version = 1\nprotocol = \"ssh\"\n"; string(data) != want {
		t.Errorf("personal file %q, want %q", data, want)
	}
}

func TestWithRepoRejectsConflictsAndWithoutRemoves(t *testing.T) {
	home, err := config.Load(writeFiles(t, header+webApp, ""), released)
	if err != nil {
		t.Fatal(err)
	}
	_, err = home.Team.WithRepo(config.Repo{Name: "Web-App", URL: "https://git.example.invalid/a/b.git", Path: "products/web-app/x", Branch: "main"})
	var problems config.FieldErrors
	if !errors.As(err, &problems) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	fields := map[string]bool{}
	for _, p := range problems {
		fields[p.Field] = true
	}
	if !fields[config.FieldName] || !fields[config.FieldPath] {
		t.Errorf("problems = %v, want name and path", problems)
	}
	team, removed := home.Team.Without("web-app")
	if !removed || len(team.Repos) != 0 || len(home.Team.Repos) != 1 {
		t.Errorf("Without: removed=%v team=%+v original=%+v", removed, team, home.Team)
	}
	if _, removed := home.Team.Without("nope"); removed {
		t.Error("Without(unknown) reported a removal")
	}
}

func TestCheckNewSkipsOnlyPendingFields(t *testing.T) {
	home, err := config.Load(writeFiles(t, header+webApp, ""), released)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://git.example.invalid/acme/x.git"
	cases := map[string]config.Repo{
		config.FieldName: {Name: "WEB-APP", URL: url, Path: "products/x"},
		config.FieldPath: {Name: "x", URL: url, Path: "products/web-app/inner"},
	}
	for field, repo := range cases {
		err := home.Team.CheckNew(repo, config.FieldBranch)
		var problems config.FieldErrors
		if !errors.As(err, &problems) || len(problems) != 1 || problems[0].Field != field {
			t.Errorf("%s conflict with a pending branch: %v", field, err)
		}
	}
	if err := home.Team.CheckNew(config.Repo{Name: "x", URL: url, Path: "products/x"}, config.FieldBranch); err != nil {
		t.Errorf("valid entry with a pending branch: %v", err)
	}
	if err := home.Team.CheckNew(config.Repo{Name: "x", URL: url, Path: "products/x"}); err == nil {
		t.Error("empty branch accepted when it is not pending")
	}
}

func TestFindRootWalksUp(t *testing.T) {
	root := writeFiles(t, header, "")
	nested := filepath.Join(root, "products", "web-app", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := config.FindRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, _ := os.Stat(root)
	gotInfo, _ := os.Stat(got)
	if !os.SameFile(wantInfo, gotInfo) {
		t.Errorf("FindRoot = %s, want %s", got, root)
	}
	_, err = config.FindRoot(t.TempDir())
	var e *apperr.Error
	if !errors.As(err, &e) || e.Category != apperr.Config {
		t.Errorf("no team file: %v, want config error", err)
	}
}

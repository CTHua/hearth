package upgrade_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/upgrade"
)

const (
	target       = 3
	personalText = "schema_version = 1\nprotocol = \"ssh\"\n"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

func newHome(t *testing.T) string {
	t.Helper()
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "home")
	if err := scaffold.Init(context.Background(), g, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// schemaOneHome is a new home repo whose team file is at schema 1, as the
// first release wrote it.
func schemaOneHome(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	path := filepath.Join(home, brand.TeamFile)
	current := "schema_version = " + strconv.Itoa(config.TeamSchemaVersion)
	text := strings.Replace(read(t, path), current, "schema_version = 1", 1)
	text = strings.Replace(text, "\""+config.MinVersion+"\"", "\"0.1.0\"", 1)
	writeText(t, path, text)
	return home
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeText(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func schemaOf(t *testing.T, path string) int64 {
	t.Helper()
	values := map[string]any{}
	if _, err := toml.DecodeFile(path, &values); err != nil {
		t.Fatal(err)
	}
	return values["schema_version"].(int64)
}

// fakePlan migrates 1 -> 2 -> 3, recording each step as "<file>:<from>" and
// appending "# <file> <from>" to the text; failAt makes the step from that
// schema fail.
func fakePlan(steps *[]string, failAt int64) upgrade.Plan {
	chain := func(file string) []upgrade.Migration {
		var migrations []upgrade.Migration
		// Listed out of order: Run must pick each step by its From.
		for _, from := range []int64{2, 1} {
			migrations = append(migrations, upgrade.Migration{From: from, Apply: func(text []byte) ([]byte, error) {
				*steps = append(*steps, file+":"+strconv.FormatInt(from, 10))
				if !strings.Contains(string(text), "schema_version = "+strconv.FormatInt(from, 10)+"\n") {
					return nil, errors.New("applied out of order")
				}
				if from == failAt {
					return nil, errors.New("step failed")
				}
				return append(text, []byte("# "+file+" "+strconv.FormatInt(from, 10)+"\n")...), nil
			}})
		}
		return migrations
	}
	return upgrade.Plan{
		Team:           chain("team"),
		Personal:       chain("personal"),
		TeamTarget:     target,
		PersonalTarget: target,
		Validate:       func(string) error { return nil },
	}
}

func TestMigrationsRunInOrderToTarget(t *testing.T) {
	home := schemaOneHome(t)
	teamPath := filepath.Join(home, brand.TeamFile)
	team := read(t, teamPath)
	personal := filepath.Join(home, brand.LocalFile)
	writeText(t, personal, personalText)
	var steps []string
	changes, err := upgrade.Run(home, fakePlan(&steps, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps, " "); got != "team:1 team:2 personal:1 personal:2" {
		t.Errorf("steps %s", got)
	}
	for _, path := range []string{teamPath, personal} {
		if got := schemaOf(t, path); got != target {
			t.Errorf("%s: schema_version %d, want %d", path, got, target)
		}
	}
	// Each step saw the text the step before it left.
	wantTeam := strings.Replace(team, "schema_version = 1", "schema_version = 3", 1) + "# team 1\n# team 2\n"
	if got := read(t, teamPath); got != wantTeam {
		t.Errorf("team file:\n%s\nwant:\n%s", got, wantTeam)
	}
	if len(changes) != 2 || changes[0].Detail != "schema_version 1 -> 3" {
		t.Errorf("changes %+v", changes)
	}
	steps = nil
	if changes, err := upgrade.Run(home, fakePlan(&steps, 0), false); err != nil || len(changes) != 0 || len(steps) != 0 {
		t.Errorf("second run: %+v, %v, steps %v", changes, err, steps)
	}
}

func TestFailedMigrationLeavesFilesUnchanged(t *testing.T) {
	home := schemaOneHome(t)
	personal := filepath.Join(home, brand.LocalFile)
	writeText(t, personal, personalText)
	team := read(t, filepath.Join(home, brand.TeamFile))
	var steps []string
	// The personal file's second step fails after the team file migrated.
	plan := fakePlan(&steps, 0)
	plan.Personal = fakePlan(&steps, 2).Personal
	if _, err := upgrade.Run(home, plan, false); err == nil {
		t.Fatal("no error")
	}
	if read(t, filepath.Join(home, brand.TeamFile)) != team || read(t, personal) != personalText {
		t.Error("a config file changed")
	}
}

func TestFailedValidationLeavesFilesUnchanged(t *testing.T) {
	home := schemaOneHome(t)
	team := read(t, filepath.Join(home, brand.TeamFile))
	var steps []string
	plan := fakePlan(&steps, 0)
	var validated string
	plan.Validate = func(dir string) error {
		validated = read(t, filepath.Join(dir, brand.TeamFile))
		return errors.New("invalid")
	}
	if _, err := upgrade.Run(home, plan, false); err == nil {
		t.Fatal("no error")
	}
	if !strings.Contains(validated, "schema_version = 3") {
		t.Errorf("validated:\n%s", validated)
	}
	if read(t, filepath.Join(home, brand.TeamFile)) != team {
		t.Error("team file changed")
	}
}

func TestCheckWritesNothingAndMissingPersonalIsSkipped(t *testing.T) {
	home := schemaOneHome(t)
	team := read(t, filepath.Join(home, brand.TeamFile))
	var steps []string
	changes, err := upgrade.Run(home, fakePlan(&steps, 0), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].File != brand.TeamFile {
		t.Errorf("changes %+v", changes)
	}
	if read(t, filepath.Join(home, brand.TeamFile)) != team {
		t.Error("--check wrote the team file")
	}
	if _, err := os.Stat(filepath.Join(home, brand.LocalFile)); err == nil {
		t.Error("personal file created")
	}
	if _, err := upgrade.Run(home, fakePlan(&steps, 0), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, brand.LocalFile)); err == nil {
		t.Error("personal file created")
	}
}

func TestMissingStepIsConfigError(t *testing.T) {
	home := schemaOneHome(t)
	plan := upgrade.Plan{TeamTarget: 2, PersonalTarget: 2, Validate: func(string) error { return nil }}
	_, err := upgrade.Run(home, plan, true)
	if err == nil || !strings.Contains(err.Error(), "error [config]") {
		t.Errorf("err %v", err)
	}
}

// schemaOneTeam is a schema 1 team file with comments, blank lines, an
// [[mcp]] entry whose header_envs are not in sorted order, and the two
// version lines spelled as given.
func schemaOneTeam(schemaLine string, minLine string) string {
	return brand.TeamFileHeader + "\n" +
		"# A comment the team wrote.\n" +
		schemaLine + "\n" +
		"\n" +
		minLine + "\n" +
		"\n" +
		"[[skills]]  # one skill\n" +
		"name = \"pdf\"\n" +
		"url = \"https://git.example.invalid/acme/skills.git\"\n" +
		"commit = \"0123456789abcdef0123456789abcdef01234567\"\n" +
		"subdir = \"pdf\"\n" +
		"\n" +
		"[[mcp]]\n" +
		"name = \"tracker\"\n" +
		"transport = \"http\"\n" +
		"url = \"https://mcp.example.invalid/mcp\"\n" +
		"header_envs = { \"X-Workspace\" = \"TRACKER_WORKSPACE\", \"A-First\" = \"TRACKER_A\" }\n" +
		"# schema_version = 1 in a table is not the top-level key\n"
}

// TestDefaultPlanRewritesOnlyTheVersionValues is the 1 -> 2 step of this
// build: every spelling of the two lines, and every other byte kept.
func TestDefaultPlanRewritesOnlyTheVersionValues(t *testing.T) {
	min := brand.MinVersionKey
	cases := []struct {
		name       string
		schemaLine string
		minLine    string
		wantSchema string
		wantMin    string
	}{
		{"plain", "schema_version = 1", min + " = \"0.1.0\"", "schema_version = 2", min + " = \"" + config.MinVersion + "\""},
		{"no spaces, comments", "schema_version=1 # first", min + "=\"0.1.0\"   # oldest", "schema_version=2 # first", min + "=\"" + config.MinVersion + "\"   # oldest"},
		{"single quotes, tabs", "schema_version\t=\t1", min + " = '0.1.0'#c", "schema_version\t=\t2", min + " = '" + config.MinVersion + "'#c"},
		{"min already higher", "schema_version = 1", min + " = \"9.0.0\"", "schema_version = 2", min + " = \"9.0.0\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			teamPath := filepath.Join(home, brand.TeamFile)
			personal := filepath.Join(home, brand.LocalFile)
			writeText(t, teamPath, schemaOneTeam(c.schemaLine, c.minLine))
			personalBefore := "# mine\nschema_version = 1 # personal\nprotocol = \"ssh\"\n\nskills = [\"pdf\"]\n"
			writeText(t, personal, personalBefore)
			changes, err := upgrade.Run(home, upgrade.Default(), false)
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) != 2 {
				t.Errorf("changes %+v", changes)
			}
			if got, want := read(t, teamPath), schemaOneTeam(c.wantSchema, c.wantMin); got != want {
				t.Errorf("team file:\n%s\nwant:\n%s", got, want)
			}
			if got, want := read(t, personal), strings.Replace(personalBefore, "= 1 #", "= 3 #", 1); got != want {
				t.Errorf("personal file %q, want %q", got, want)
			}
			if changes, err := upgrade.Run(home, upgrade.Default(), false); err != nil || len(changes) != 0 {
				t.Errorf("second run: %+v, %v", changes, err)
			}
		})
	}
}

// A home already at team schema 2 with a schema 2 personal file: only the
// personal file's schema value changes, and the team file keeps every byte.
func TestDefaultPlanRaisesOnlyThePersonalFileFromTwo(t *testing.T) {
	home := newHome(t)
	teamPath := filepath.Join(home, brand.TeamFile)
	personal := filepath.Join(home, brand.LocalFile)
	team := read(t, teamPath)
	personalBefore := "# mine\nschema_version = 2 # personal\nprotocol = \"ssh\"\n\nskills = [\"pdf\"]\n"
	writeText(t, personal, personalBefore)
	changes, err := upgrade.Run(home, upgrade.Default(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := []upgrade.Change{{File: brand.LocalFile, Detail: "schema_version 2 -> 3"}}
	if !slices.Equal(changes, want) {
		t.Errorf("--check changes %+v, want %+v", changes, want)
	}
	if changes, err := upgrade.Run(home, upgrade.Default(), false); err != nil || !slices.Equal(changes, want) {
		t.Errorf("changes %+v, %v", changes, err)
	}
	if got := read(t, teamPath); got != team {
		t.Errorf("team file changed:\n%s\nwant:\n%s", got, team)
	}
	if got, want := read(t, personal), strings.Replace(personalBefore, "= 2 #", "= 3 #", 1); got != want {
		t.Errorf("personal file %q, want %q", got, want)
	}
	if _, err := config.Load(home, "0.2.0"); err != nil {
		t.Errorf("load after upgrade: %v", err)
	}
	if changes, err := upgrade.Run(home, upgrade.Default(), true); err != nil || len(changes) != 0 {
		t.Errorf("--check at team 2, personal 3: %+v, %v", changes, err)
	}
}

// A home at team schema 1 and personal schema 1 ends at 2 and 3, and loads.
func TestDefaultPlanTakesEachFileToItsOwnTarget(t *testing.T) {
	home := schemaOneHome(t)
	personal := filepath.Join(home, brand.LocalFile)
	writeText(t, personal, personalText)
	if _, err := upgrade.Run(home, upgrade.Default(), false); err != nil {
		t.Fatal(err)
	}
	if got := schemaOf(t, filepath.Join(home, brand.TeamFile)); got != config.TeamSchemaVersion {
		t.Errorf("team schema_version %d", got)
	}
	if got := schemaOf(t, personal); got != config.PersonalSchemaVersion {
		t.Errorf("personal schema_version %d", got)
	}
	if _, err := config.Load(home, "0.2.0"); err != nil {
		t.Errorf("load after upgrade: %v", err)
	}
}

// Files with "\r\n" line ends and no comments: the version lines have nothing
// after the value but the "\r", and only the two values change.
func TestDefaultPlanKeepsCRLFLineEnds(t *testing.T) {
	min := brand.MinVersionKey
	teamLines := func(schema string, minVersion string) string {
		return strings.Join([]string{
			"schema_version = " + schema,
			min + " = \"" + minVersion + "\"",
			"",
			"[[skills]]",
			"name = \"pdf\"",
			"url = \"https://git.example.invalid/acme/skills.git\"",
			"commit = \"0123456789abcdef0123456789abcdef01234567\"",
			"subdir = \"pdf\"",
			"",
		}, "\r\n")
	}
	personalLines := func(schema string) string {
		return strings.Join([]string{
			"schema_version = " + schema,
			"protocol = \"ssh\"",
			"skills = [\"pdf\"]",
			"",
		}, "\r\n")
	}
	home := newHome(t)
	teamPath := filepath.Join(home, brand.TeamFile)
	personal := filepath.Join(home, brand.LocalFile)
	writeText(t, teamPath, teamLines("1", "0.1.0"))
	writeText(t, personal, personalLines("1"))
	changes, err := upgrade.Run(home, upgrade.Default(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Errorf("changes %+v", changes)
	}
	if got, want := read(t, teamPath), teamLines("2", config.MinVersion); got != want {
		t.Errorf("team file %q, want %q", got, want)
	}
	if got, want := read(t, personal), personalLines("3"); got != want {
		t.Errorf("personal file %q, want %q", got, want)
	}
}

// A line the text replacement cannot find fails the run before either file
// is written, even when the other file could be upgraded.
func TestDefaultPlanWithoutTheLineWritesNothing(t *testing.T) {
	min := brand.MinVersionKey
	cases := map[string]string{
		"quoted schema key":      schemaOneTeam("\"schema_version\" = 1", min+" = \"0.1.0\""),
		"quoted min version key": schemaOneTeam("schema_version = 1", "\""+min+"\" = \"0.1.0\""),
		"multi-line min version": schemaOneTeam("schema_version = 1", min+" = \"\"\"0.1.0\"\"\""),
	}
	for name, team := range cases {
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			teamPath := filepath.Join(home, brand.TeamFile)
			personal := filepath.Join(home, brand.LocalFile)
			writeText(t, teamPath, team)
			writeText(t, personal, personalText)
			_, err := upgrade.Run(home, upgrade.Default(), false)
			if err == nil || !strings.Contains(err.Error(), "error [config]") {
				t.Errorf("err %v", err)
			}
			if read(t, teamPath) != team || read(t, personal) != personalText {
				t.Error("a config file changed")
			}
		})
	}
}

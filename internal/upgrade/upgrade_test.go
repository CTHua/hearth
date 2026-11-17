package upgrade_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/brand"
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

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func schemaOf(t *testing.T, path string) int64 {
	t.Helper()
	values := map[string]any{}
	if _, err := toml.DecodeFile(path, &values); err != nil {
		t.Fatal(err)
	}
	return values["schema_version"].(int64)
}

// fakePlan migrates 1 -> 2 -> 3, recording each step as "<file>:<from>";
// failAt makes the step from that schema fail.
func fakePlan(steps *[]string, failAt int64) upgrade.Plan {
	chain := func(file string) []upgrade.Migration {
		var migrations []upgrade.Migration
		// Listed out of order: Run must pick each step by its From.
		for _, from := range []int64{2, 1} {
			migrations = append(migrations, upgrade.Migration{From: from, Apply: func(values map[string]any) error {
				*steps = append(*steps, file+":"+strconv.FormatInt(from, 10))
				if values["schema_version"] != from {
					return errors.New("applied out of order")
				}
				if from == failAt {
					return errors.New("step failed")
				}
				return nil
			}})
		}
		return migrations
	}
	return upgrade.Plan{
		Team:     chain("team"),
		Personal: chain("personal"),
		Target:   target,
		Validate: func(string) error { return nil },
	}
}

func TestMigrationsRunInOrderToTarget(t *testing.T) {
	home := newHome(t)
	personal := filepath.Join(home, brand.LocalFile)
	if err := os.WriteFile(personal, []byte(personalText), 0o644); err != nil {
		t.Fatal(err)
	}
	var steps []string
	changes, err := upgrade.Run(home, fakePlan(&steps, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps, " "); got != "team:1 team:2 personal:1 personal:2" {
		t.Errorf("steps %s", got)
	}
	for _, path := range []string{filepath.Join(home, brand.TeamFile), personal} {
		if got := schemaOf(t, path); got != target {
			t.Errorf("%s: schema_version %d, want %d", path, got, target)
		}
	}
	if !strings.HasPrefix(read(t, filepath.Join(home, brand.TeamFile)), brand.TeamFileHeader+"\n") {
		t.Error("team file lost its header")
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
	home := newHome(t)
	personal := filepath.Join(home, brand.LocalFile)
	if err := os.WriteFile(personal, []byte(personalText), 0o644); err != nil {
		t.Fatal(err)
	}
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
	plan := upgrade.Plan{Target: 2, Validate: func(string) error { return nil }}
	_, err := upgrade.Run(home, plan, true)
	if err == nil || !strings.Contains(err.Error(), "error [config]") {
		t.Errorf("err %v", err)
	}
}

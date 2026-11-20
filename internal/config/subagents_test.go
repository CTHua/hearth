package config_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
)

func TestLoadAndWriteSubagents(t *testing.T) {
	root := writeFiles(t, header, "schema_version = 2\nmcp = [\"docs\"]\nsubagents = [\"code-reviewer\", \"gone\"]\n")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(home.Personal.Subagents, []string{"code-reviewer", "gone"}) {
		t.Errorf("subagents = %v", home.Personal.Subagents)
	}
	if err := config.WritePersonal(root, home.Personal.WithSubagents([]string{"reviewer"})); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(root, brand.LocalFile)); got != "schema_version = 2\nmcp = [\"docs\"]\nsubagents = [\"reviewer\"]\n" {
		t.Errorf("personal file %q", got)
	}
}

func TestSubagentNameRule(t *testing.T) {
	for _, name := range []string{"reviewer", "code-reviewer", "a_b2"} {
		if problem := config.SubagentNameProblem(name); problem != "" {
			t.Errorf("%s: %s", name, problem)
		}
	}
	for _, name := range []string{"", "Reviewer", "1st", "-a", "a.b", "a b"} {
		if config.SubagentNameProblem(name) == "" {
			t.Errorf("%q accepted", name)
		}
	}
	personal := map[string]string{
		"subagents = [\"a\", \"a\"]\n": "subagents[1]: duplicate",
		"subagents = [\"Bad\"]\n":      "subagents[0]: ",
		"subagents = \"a\"\n":          "subagents: must be an array of strings",
	}
	for body, want := range personal {
		lines, _ := loadErrors(t, writeFiles(t, header, "schema_version = 2\n"+body), released)
		if !strings.Contains(lines, brand.LocalFile+": "+want) {
			t.Errorf("%q: %s", body, lines)
		}
	}
}

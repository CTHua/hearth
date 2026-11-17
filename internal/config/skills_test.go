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
	pin      = "0123456789abcdef0123456789abcdef01234567"
	skillURL = "https://git.example.invalid/acme/skills.git"
	pdfSkill = `
[[skills]]
name = "pdf-tools"
url = "` + skillURL + `"
commit = "` + pin + `"
subdir = "pdf"
`
)

func skillEntry(name string, commit string, subdir string) string {
	return "\n[[skills]]\nname = \"" + name + "\"\nurl = \"" + skillURL + "\"\ncommit = \"" + commit + "\"\nsubdir = \"" + subdir + "\"\n"
}

func TestLoadSkillsAndPersonalLists(t *testing.T) {
	personal := "schema_version = 1\nprotocol = \"ssh\"\nagents = [\"claude\", \"codex\"]\nskills = [\"pdf-tools\", \"gone\"]\n"
	home, err := config.Load(writeFiles(t, header+webApp+pdfSkill, personal), released)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.Skill{{Name: "pdf-tools", URL: skillURL, Commit: pin, Subdir: "pdf"}}
	if !reflect.DeepEqual(home.Team.Skills, want) {
		t.Errorf("skills = %+v, want %+v", home.Team.Skills, want)
	}
	if !reflect.DeepEqual(home.Personal.Agents, []string{"claude", "codex"}) || !reflect.DeepEqual(home.Personal.Skills, []string{"pdf-tools", "gone"}) {
		t.Errorf("personal = %+v", home.Personal)
	}
}

func TestSkillValueRulesNameFileAndLocation(t *testing.T) {
	sha256 := strings.Repeat("a", 64)
	valid := []string{skillEntry("a", pin, "."), skillEntry("b", sha256, "skills/x.y"), skillEntry("c", pin, "a/b-c_d")}
	for _, entry := range valid {
		if _, err := config.Load(writeFiles(t, header+entry, ""), released); err != nil {
			t.Errorf("%q: %v", entry, err)
		}
	}
	cases := map[string]string{
		skillEntry("pdf", "main", "."):                                      "skills[0].commit: ",
		skillEntry("pdf", "abc1234", "."):                                   "skills[0].commit: ",
		skillEntry("pdf", strings.ToUpper(pin), "."):                        "skills[0].commit: ",
		skillEntry("pdf", pin, "../x"):                                      "skills[0].subdir: ",
		skillEntry("pdf", pin, "a/.git"):                                    "skills[0].subdir: ",
		skillEntry("pdf", pin, "a/./b"):                                     "skills[0].subdir: ",
		skillEntry("pdf", pin, "/abs"):                                      "skills[0].subdir: ",
		skillEntry("pdf", pin, ""):                                          "skills[0].subdir: ",
		skillEntry("Hearth", pin, "."):                                      "skills[0].name: ",
		skillEntry("-x", pin, "."):                                          "skills[0].name: ",
		skillEntry("a", pin, ".") + skillEntry("A", pin, "."):               "skills[1].name: duplicate",
		strings.Replace(skillEntry("pdf", pin, "."), skillURL, "skills", 1): "skills[0].url: ",
		strings.Replace(skillEntry("pdf", pin, "."), "subdir", "path", 1):   "skills[0].path: unknown key",
		"skills = \"x\"\n":                                                  "skills: must be an array of tables",
	}
	for entry, want := range cases {
		lines, categories := loadErrors(t, writeFiles(t, header+entry, ""), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.TeamFile+": "+want) {
			t.Errorf("%q:\n got %s\nwant %s", entry, lines, want)
		}
	}
}

func TestPersonalListRules(t *testing.T) {
	cases := map[string]string{
		"agents = [\"cursor\"]\n":             "agents[0]: ",
		"agents = [\"claude\", \"claude\"]\n": "agents[1]: duplicate",
		"agents = \"claude\"\n":               "agents: must be an array of strings",
		"agents = [1]\n":                      "agents[0]: must be a string",
		"[agents]\nenabled = []\n":            "agents: must be an array of strings",
		"skills = [\"pdf\", \"PDF\"]\n":       "skills[1]: duplicate",
		"skills = [\"ok\", \"" + strings.ToUpper(brand.BuiltinSkill) + "\"]\n": "skills[1]: ",
		"skills = [\"bad name\"]\n": "skills[0]: ",
	}
	for body, want := range cases {
		lines, categories := loadErrors(t, writeFiles(t, header, "schema_version = 1\n"+body), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.LocalFile+": "+want) {
			t.Errorf("%q:\n got %s\nwant %s", body, lines, want)
		}
	}
	home, err := config.Load(writeFiles(t, header, "schema_version = 1\nagents = []\nskills = [\"not-anywhere\"]\n"), released)
	if err != nil || len(home.Personal.Agents) != 0 || home.Personal.Skills[0] != "not-anywhere" {
		t.Errorf("an unknown chosen name must load: %+v, %v", home, err)
	}
}

func TestWriteTeamPutsReposBeforeSkills(t *testing.T) {
	root := writeFiles(t, header+pdfSkill+webApp, "")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	team, err := home.Team.WithSkill(config.Skill{Name: "notes", URL: skillURL, Commit: pin, Subdir: "."})
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
	want := brand.TeamFileHeader + "\n" + header + webApp + pdfSkill +
		"\n[[skills]]\nname = \"notes\"\nurl = \"" + skillURL + "\"\ncommit = \"" + pin + "\"\nsubdir = \".\"\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
	if _, err := config.Load(root, released); err != nil {
		t.Errorf("written file does not load: %v", err)
	}
}

func TestWritePersonalKeepsEveryKey(t *testing.T) {
	root := writeFiles(t, header, "schema_version = 1\nprotocol = \"https\"\nskills = [\"pdf\"]\n")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WritePersonal(root, home.Personal.WithAgents([]string{"codex"}).WithProtocol("ssh")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, brand.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := "schema_version = 1\nprotocol = \"ssh\"\nagents = [\"codex\"]\nskills = [\"pdf\"]\n"; string(data) != want {
		t.Errorf("personal file %q, want %q", data, want)
	}
	if err := config.WritePersonal(root, home.Personal.WithAgents(nil).WithSkills(nil)); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, brand.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := "schema_version = 1\nprotocol = \"https\"\n"; string(data) != want {
		t.Errorf("emptied lists: %q, want %q", data, want)
	}
}

func TestSkillEditsRejectConflicts(t *testing.T) {
	home, err := config.Load(writeFiles(t, header+pdfSkill, ""), released)
	if err != nil {
		t.Fatal(err)
	}
	_, err = home.Team.WithSkill(config.Skill{Name: "PDF-tools", URL: skillURL, Commit: pin, Subdir: "."})
	var problems config.FieldErrors
	if !errors.As(err, &problems) || problems[0].Field != config.FieldName || !strings.Contains(err.Error(), "skills[1].name") {
		t.Errorf("duplicate name: %v", err)
	}
	if err := home.Team.CheckNewSkill(config.Skill{Name: "x", URL: skillURL, Subdir: "."}, config.FieldCommit); err != nil {
		t.Errorf("pending commit: %v", err)
	}
	if err := home.Team.CheckNewSkill(config.Skill{Name: "x", URL: skillURL, Subdir: "."}); err == nil {
		t.Error("empty commit accepted when it is not pending")
	}
	if _, err := home.Team.WithSkillCommit("pdf-tools", "abc"); err == nil {
		t.Error("short commit accepted")
	}
	next, err := home.Team.WithSkillCommit("pdf-tools", strings.Repeat("b", 40))
	if err != nil || next.Skills[0].Commit != strings.Repeat("b", 40) || home.Team.Skills[0].Commit != pin {
		t.Errorf("WithSkillCommit: %+v, %v", next.Skills, err)
	}
	if team, removed := home.Team.WithoutSkill("pdf-tools"); !removed || len(team.Skills) != 0 {
		t.Errorf("WithoutSkill: %+v", team)
	}
}

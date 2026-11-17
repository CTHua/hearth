package config_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/version"
)

func TestParseTeamMatchesLoad(t *testing.T) {
	content := header + webApp + pdfSkill + docsServer + trackerServer
	home, err := config.Load(writeFiles(t, content, ""), version.Current())
	if err != nil {
		t.Fatal(err)
	}
	team, err := config.ParseTeam([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(team, home.Team) {
		t.Errorf("ParseTeam %+v, Load %+v", team, home.Team)
	}
}

func TestParseTeamRejectsWhatLoadRejects(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		category apperr.Category
	}{
		{"not toml", "schema_version = \n", apperr.Config},
		{"unknown key", header + "extra = 1\n", apperr.Config},
		{"newer schema", "schema_version = 2\n" + brand.MinVersionKey + " = \"0.1.0\"\nnew_key = true\n", apperr.Version},
		{"missing schema", brand.MinVersionKey + " = \"0.1.0\"\n", apperr.Config},
		{"bad repo name", header + "[[repos]]\nname = \"a b\"\nurl = \"git@git.example.invalid:acme/a.git\"\npath = \"products/a\"\nbranch = \"main\"\n", apperr.Config},
	}
	for _, c := range cases {
		_, err := config.ParseTeam([]byte(c.content))
		var list apperr.List
		if !errors.As(err, &list) {
			t.Errorf("%s: error %T %v, want apperr.List", c.name, err, err)
			continue
		}
		if list[0].Category != c.category {
			t.Errorf("%s: category %s, want %s: %v", c.name, list[0].Category, c.category, err)
		}
		if _, loadErr := config.Load(writeFiles(t, c.content, ""), version.Current()); loadErr == nil {
			t.Errorf("%s: Load accepted what ParseTeam rejected", c.name)
		}
	}
}

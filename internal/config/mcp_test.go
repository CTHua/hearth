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
	docsServer = `
[[mcp]]
name = "docs"
transport = "stdio"
command = "npx"
args = ["-y", "@acme/docs-mcp"]
env_vars = ["DOCS_TOKEN"]
`
	trackerServer = `
[[mcp]]
name = "tracker"
transport = "http"
url = "https://mcp.example.invalid/mcp"
bearer_token_env = "TRACKER_TOKEN"
header_envs = { "X-Workspace" = "TRACKER_WORKSPACE", "A-First" = "TRACKER_A" }
`
)

func TestLoadAndWriteMCPKeepsOrder(t *testing.T) {
	root := writeFiles(t, header+webApp+pdfSkill+docsServer+trackerServer, "schema_version = 2\nmcp = [\"tracker\", \"gone\"]\n")
	home, err := config.Load(root, released)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.MCP{
		{Name: "docs", Transport: config.TransportStdio, Command: "npx", Args: []string{"-y", "@acme/docs-mcp"}, EnvVars: []string{"DOCS_TOKEN"}},
		{Name: "tracker", Transport: config.TransportHTTP, URL: "https://mcp.example.invalid/mcp", BearerTokenEnv: "TRACKER_TOKEN",
			HeaderEnvs: []config.HeaderEnv{{Header: "X-Workspace", Var: "TRACKER_WORKSPACE"}, {Header: "A-First", Var: "TRACKER_A"}}},
	}
	if !reflect.DeepEqual(home.Team.MCP, want) {
		t.Errorf("mcp = %+v\nwant %+v", home.Team.MCP, want)
	}
	if !reflect.DeepEqual(home.Personal.MCP, []string{"tracker", "gone"}) {
		t.Errorf("personal mcp = %v", home.Personal.MCP)
	}
	if err := config.WriteTeam(root, home.Team); err != nil {
		t.Fatal(err)
	}
	if got, want := readText(t, filepath.Join(root, brand.TeamFile)), brand.TeamFileHeader+"\n"+header+webApp+pdfSkill+docsServer+trackerServer; got != want {
		t.Errorf("team file:\n%s\nwant:\n%s", got, want)
	}
	if err := config.WritePersonal(root, home.Personal.WithMCPChoices([]string{"docs"})); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(root, brand.LocalFile)); got != "schema_version = 2\nmcp = [\"docs\"]\n" {
		t.Errorf("personal file %q", got)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWrittenValuesRoundTrip(t *testing.T) {
	root := writeFiles(t, header, "")
	server := config.MCP{Name: "odd", Transport: config.TransportStdio, Command: "run \"x\"\\", Args: []string{"tab\there", "line\nbreak", "\x01", "&<>"}}
	team, err := config.NewTeam().WithMCP(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteTeam(root, team); err != nil {
		t.Fatal(err)
	}
	home, err := config.Load(root, released)
	if err != nil || !reflect.DeepEqual(home.Team.MCP, []config.MCP{server}) {
		t.Errorf("round trip %+v, %v", home, err)
	}
}

func TestMCPRulesNameFileAndLocation(t *testing.T) {
	stdio := func(extra string) string {
		return header + "[[mcp]]\nname = \"s\"\ntransport = \"stdio\"\ncommand = \"x\"\n" + extra
	}
	http := func(url string, extra string) string {
		return header + "[[mcp]]\nname = \"h\"\ntransport = \"http\"\nurl = \"" + url + "\"\n" + extra
	}
	cases := map[string]string{
		stdio("env = { DOCS_TOKEN = \"literal\" }\n"):                                     "mcp[0].env: unknown key",
		http("https://a.invalid", "headers = { X = \"value\" }\n"):                        "mcp[0].headers: unknown key",
		stdio("url = \"https://a.invalid\"\n"):                                            "mcp[0].url: must not be set when transport is \"stdio\"",
		stdio("header_envs = {}\n"):                                                       "mcp[0].header_envs: must not be set",
		http("https://a.invalid", "args = []\n"):                                          "mcp[0].args: must not be set when transport is \"http\"",
		stdio("env_vars = [\"lower\"]\n"):                                                 "mcp[0].env_vars[0]: ",
		stdio("env_vars = [\"A\", \"A\"]\n"):                                              "mcp[0].env_vars[1]: duplicate",
		stdio("args = [1]\n"):                                                             "mcp[0].args[0]: must be a string",
		stdio("args = \"x\"\n"):                                                           "mcp[0].args: must be an array of strings",
		http("https://a.invalid", "header_envs = { \"Authorization\" = \"T\" }\n"):        "mcp[0].header_envs: ",
		http("https://a.invalid", "header_envs = { \"x-a\" = \"T\", \"X-A\" = \"U\" }\n"): "mcp[0].header_envs: duplicate header",
		http("https://a.invalid", "header_envs = { \"X\" = 1 }\n"):                        "mcp[0].header_envs: must be a table",
		http("https://a.invalid", "header_envs = { \"Bad Header\" = \"T\" }\n"):           "mcp[0].header_envs: ",
		http("https://a.invalid", "bearer_token_env = \"x-y\"\n"):                         "mcp[0].bearer_token_env: ",
		http("http://127.0.0.10", ""):                                                     "mcp[0].url: ",
		http("http://localhost.evil.invalid", ""):                                         "mcp[0].url: ",
		http("https://user:pw@a.invalid", ""):                                             "mcp[0].url: must not contain",
		http("ftp://a.invalid", ""):                                                       "mcp[0].url: ",
		header + "[[mcp]]\nname = \"a.b\"\ntransport = \"stdio\"\ncommand = \"x\"\n":      "mcp[0].name: ",
		header + "[[mcp]]\nname = \"a\"\ntransport = \"sse\"\n":                           "mcp[0].transport: ",
		header + "[[mcp]]\nname = \"a\"\ntransport = \"stdio\"\n":                         "mcp[0].command: missing",
		header + "[[mcp]]\nname = \"a\"\ntransport = \"http\"\n":                          "mcp[0].url: missing",
		stdio("") + "[[mcp]]\nname = \"S\"\ntransport = \"stdio\"\ncommand = \"y\"\n":     "mcp[1].name: duplicate",
		header + "mcp = \"x\"\n":                                                          "mcp: must be an array of tables",
	}
	for team, want := range cases {
		lines, categories := loadErrors(t, writeFiles(t, team, ""), released)
		if !categories[apperr.Config] || !strings.Contains(lines, brand.TeamFile+": "+want) {
			t.Errorf("%q:\n got %s\nwant %s", team, lines, want)
		}
	}
	for _, url := range []string{"http://127.0.0.1:3000/mcp", "http://localhost/mcp", "https://mcp.example.invalid"} {
		if _, err := config.Load(writeFiles(t, http(url, ""), ""), released); err != nil {
			t.Errorf("%s: %v", url, err)
		}
	}
	personal := map[string]string{
		"mcp = [\"a\", \"A\"]\n": "mcp[1]: duplicate",
		"mcp = [\"a.b\"]\n":      "mcp[0]: ",
	}
	for body, want := range personal {
		lines, _ := loadErrors(t, writeFiles(t, header, "schema_version = 2\n"+body), released)
		if !strings.Contains(lines, brand.LocalFile+": "+want) {
			t.Errorf("%q: %s", body, lines)
		}
	}
}

func TestMCPEditsRejectConflicts(t *testing.T) {
	home, err := config.Load(writeFiles(t, header+docsServer, ""), released)
	if err != nil {
		t.Fatal(err)
	}
	_, err = home.Team.WithMCP(config.MCP{Name: "DOCS", Transport: config.TransportStdio, Command: "x"})
	var problems config.FieldErrors
	if !errors.As(err, &problems) || !strings.Contains(problems.Error(), "mcp[1].name: duplicate") {
		t.Errorf("err = %v", err)
	}
	team, removed := home.Team.WithoutMCP("Docs")
	if !removed || len(team.MCP) != 0 || len(home.Team.MCP) != 1 {
		t.Errorf("WithoutMCP: %+v %v", team, removed)
	}
	if found, ok := home.Team.FindMCP("DOCS"); !ok || found.Name != "docs" {
		t.Errorf("FindMCP: %+v %v", found, ok)
	}
}

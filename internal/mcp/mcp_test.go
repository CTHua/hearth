package mcp_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/mcp"
)

const (
	claude    = config.AgentClaude
	codex     = config.AgentCodex
	codexDir  = ".codex"
	userJSON  = "{\"mine\": true}\n"
	userTOML  = "# mine\n"
	failedMod = "[" + "generated-modified" + "]"
	failedPC  = "[" + "path-conflict" + "]"
)

// servers covers both transports with and without their optional keys; the
// team order differs from the order they are chosen in.
var servers = []config.MCP{
	{Name: "docs", Transport: config.TransportStdio, Command: "npx", Args: []string{"-y", "@acme/docs-mcp"}, EnvVars: []string{"DOCS_TOKEN", "DOCS_ORG"}},
	{Name: "unused", Transport: config.TransportStdio, Command: "unused"},
	{Name: "Local", Transport: config.TransportStdio, Command: "./bin/serve"},
	{Name: "tracker", Transport: config.TransportHTTP, URL: "https://mcp.example.invalid/mcp", BearerTokenEnv: "TRACKER_TOKEN",
		HeaderEnvs: []config.HeaderEnv{{Header: "X-Workspace", Var: "TRACKER_WORKSPACE"}, {Header: "X-Team", Var: "TRACKER_TEAM"}}},
	{Name: "dev", Transport: config.TransportHTTP, URL: "http://127.0.0.1:3000/mcp"},
}

var chosen = []string{"dev", "tracker", "local", "DOCS"}

func home(t *testing.T, agents []string, names []string) *config.Home {
	t.Helper()
	return &config.Home{
		Root:     t.TempDir(),
		Team:     config.Team{MCP: servers},
		Personal: config.Personal{SchemaVersion: config.SchemaVersion, Agents: agents, MCP: names},
	}
}

type outcome struct {
	result mcp.Result
	stdout string
	stderr string
}

func syncHome(t *testing.T, h *config.Home, force bool) outcome {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := mcp.Sync(h, force, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	return outcome{result: result, stdout: stdout.String(), stderr: stderr.String()}
}

func path(h *config.Home, rel string) string {
	return filepath.Join(h.Root, filepath.FromSlash(rel))
}

func read(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func write(t *testing.T, file string, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(file string) bool {
	_, err := os.Lstat(file)
	return err == nil
}

func recorded(t *testing.T, h *config.Home) map[string]string {
	t.Helper()
	if !exists(path(h, brand.StateFile)) {
		return map[string]string{}
	}
	var st struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(read(t, path(h, brand.StateFile))), &st); err != nil {
		t.Fatal(err)
	}
	return st.Files
}

func sum(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func TestGoldenFiles(t *testing.T) {
	cases := map[string][]string{"servers": chosen, "empty": nil}
	for name, names := range cases {
		h := home(t, []string{claude, codex}, names)
		res := syncHome(t, h, false)
		if len(res.result.Failed) != 0 {
			t.Fatalf("%s: %v %s", name, res.result.Failed, res.stderr)
		}
		golden := map[string]string{mcp.ClaudeFile: name + ".mcp.json", mcp.CodexFile: name + ".config.toml"}
		for rel, file := range golden {
			want := read(t, filepath.Join("testdata", file))
			if got := read(t, path(h, rel)); got != want {
				t.Errorf("%s %s:\n%s\nwant:\n%s", name, rel, got, want)
			}
		}
	}
}

func TestCodexFileDecodesToExpectedStructure(t *testing.T) {
	h := home(t, []string{codex}, chosen)
	syncHome(t, h, false)
	type server struct {
		Command           string            `toml:"command"`
		Args              []string          `toml:"args"`
		EnvVars           []string          `toml:"env_vars"`
		URL               string            `toml:"url"`
		BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
		EnvHTTPHeaders    map[string]string `toml:"env_http_headers"`
	}
	var got struct {
		MCPServers map[string]server `toml:"mcp_servers"`
	}
	meta, err := toml.Decode(read(t, path(h, mcp.CodexFile)), &got)
	if err != nil {
		t.Fatal(err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) != 0 {
		t.Errorf("unexpected keys %v", undecoded)
	}
	want := map[string]server{
		"docs":  {Command: "npx", Args: []string{"-y", "@acme/docs-mcp"}, EnvVars: []string{"DOCS_TOKEN", "DOCS_ORG"}},
		"Local": {Command: "./bin/serve", Args: []string{}},
		"tracker": {URL: "https://mcp.example.invalid/mcp", BearerTokenEnvVar: "TRACKER_TOKEN",
			EnvHTTPHeaders: map[string]string{"X-Workspace": "TRACKER_WORKSPACE", "X-Team": "TRACKER_TEAM"}},
		"dev": {URL: "http://127.0.0.1:3000/mcp"},
	}
	if !reflect.DeepEqual(got.MCPServers, want) {
		t.Errorf("decoded %+v\nwant %+v", got.MCPServers, want)
	}
}

func TestEachAgentGetsOnlyItsFile(t *testing.T) {
	cases := map[string]struct {
		has     string
		hasNot  string
		hasNot2 string
	}{
		claude: {has: mcp.ClaudeFile, hasNot: mcp.CodexFile, hasNot2: codexDir},
		codex:  {has: mcp.CodexFile, hasNot: mcp.ClaudeFile},
	}
	for agent, c := range cases {
		h := home(t, []string{agent}, chosen)
		syncHome(t, h, false)
		if !exists(path(h, c.has)) || exists(path(h, c.hasNot)) || (c.hasNot2 != "" && exists(path(h, c.hasNot2))) {
			t.Errorf("%s: want only %s", agent, c.has)
		}
		files := recorded(t, h)
		if len(files) != 1 || files[c.has] != sum(read(t, path(h, c.has))) {
			t.Errorf("%s: state %v", agent, files)
		}
	}
}

func TestDroppedAgentRemovesOnlyUntouchedFiles(t *testing.T) {
	h := home(t, []string{claude, codex}, chosen)
	syncHome(t, h, false)
	h.Personal.Agents = nil
	res := syncHome(t, h, false)
	if len(res.result.Failed) != 0 || exists(path(h, mcp.ClaudeFile)) || exists(path(h, codexDir)) {
		t.Errorf("untouched files not removed: %+v", res)
	}
	if exists(path(h, brand.StateFile)) || exists(path(h, brand.StateDir)) {
		t.Error("empty state left behind")
	}

	h = home(t, []string{claude, codex}, chosen)
	syncHome(t, h, false)
	write(t, path(h, mcp.ClaudeFile), userJSON)
	h.Personal.Agents = nil
	res = syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+" "+mcp.ClaudeFile+":") || read(t, path(h, mcp.ClaudeFile)) != userJSON {
		t.Errorf("edited file removed or not reported: %+v", res)
	}
	if exists(path(h, mcp.CodexFile)) || len(res.result.Failed) != 1 {
		t.Errorf("the other file should still be removed: %+v", res)
	}
	res = syncHome(t, h, true)
	if len(res.result.Failed) != 0 || exists(path(h, mcp.ClaudeFile)) || len(recorded(t, h)) != 0 {
		t.Errorf("force did not remove: %+v", res)
	}
}

func TestUnmanagedFileOfUnchosenAgentIsLeftAlone(t *testing.T) {
	h := home(t, nil, chosen)
	write(t, path(h, mcp.ClaudeFile), userJSON)
	write(t, path(h, mcp.CodexFile), userTOML)
	for _, force := range []bool{false, true} {
		res := syncHome(t, h, force)
		if len(res.result.Failed) != 0 || strings.Contains(res.stderr, "error") {
			t.Errorf("force=%v: %+v", force, res)
		}
	}
	if read(t, path(h, mcp.ClaudeFile)) != userJSON || read(t, path(h, mcp.CodexFile)) != userTOML || exists(path(h, brand.StateFile)) {
		t.Error("unmanaged files were touched")
	}
}

func TestEditedFileBlocksOnlyItselfUntilForce(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"docs"})
	syncHome(t, h, false)
	write(t, path(h, mcp.ClaudeFile), userJSON)
	h.Personal.MCP = chosen
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+" "+mcp.ClaudeFile+":") || len(res.result.Failed) != 1 {
		t.Fatalf("not reported: %+v", res)
	}
	if read(t, path(h, mcp.ClaudeFile)) != userJSON {
		t.Error("edited file overwritten")
	}
	if want := read(t, filepath.Join("testdata", "servers.config.toml")); read(t, path(h, mcp.CodexFile)) != want {
		t.Error("the other file was not updated")
	}
	res = syncHome(t, h, true)
	want := read(t, filepath.Join("testdata", "servers.mcp.json"))
	if len(res.result.Failed) != 0 || read(t, path(h, mcp.ClaudeFile)) != want || recorded(t, h)[mcp.ClaudeFile] != sum(want) {
		t.Errorf("force: %+v", res)
	}
	if res = syncHome(t, h, false); len(res.result.Failed) != 0 {
		t.Errorf("rerun after force: %+v", res)
	}
}

func TestExistingFileWithoutRecordIsEdited(t *testing.T) {
	h := home(t, []string{claude}, chosen)
	write(t, path(h, mcp.ClaudeFile), userJSON)
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+" "+mcp.ClaudeFile+":") || read(t, path(h, mcp.ClaudeFile)) != userJSON {
		t.Errorf("%+v", res)
	}
}

func TestSymlinksAreRefusedEvenWithForce(t *testing.T) {
	links := []struct {
		link   string
		target string
	}{
		{mcp.ClaudeFile, "outside.json"},
		{codexDir, "outside-dir"},
		{mcp.CodexFile, "outside.toml"},
		{brand.StateDir, "outside-state"},
		{brand.StateFile, "outside-state.json"},
	}
	for _, c := range links {
		for _, force := range []bool{false, true} {
			h := home(t, []string{claude, codex}, chosen)
			outside := filepath.Join(t.TempDir(), c.target)
			if strings.HasSuffix(c.target, "dir") || strings.HasSuffix(c.target, "state") {
				if err := os.MkdirAll(outside, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, outside, userJSON)
			}
			if err := os.MkdirAll(filepath.Dir(path(h, c.link)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path(h, c.link)); err != nil {
				t.Fatal(err)
			}
			res := syncHome(t, h, force)
			if !strings.Contains(res.stderr, failedPC) || len(res.result.Failed) == 0 {
				t.Errorf("%s force=%v: %+v", c.link, force, res)
			}
			info, err := os.Lstat(outside)
			if err != nil {
				t.Fatal(err)
			}
			if info.IsDir() {
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 0 {
					t.Errorf("%s force=%v: wrote through the link: %v %v", c.link, force, entries, err)
				}
			} else if read(t, outside) != userJSON {
				t.Errorf("%s force=%v: target changed", c.link, force)
			}
		}
	}
}

func TestFileOfWrongTypeIsPathConflict(t *testing.T) {
	h := home(t, []string{claude, codex}, chosen)
	if err := os.MkdirAll(path(h, mcp.ClaudeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, path(h, codexDir), "")
	res := syncHome(t, h, true)
	if strings.Count(res.stderr, failedPC) != 2 {
		t.Errorf("%+v", res)
	}
}

func TestStaleChoiceIsWarnedAndLeftOut(t *testing.T) {
	h := home(t, []string{claude}, []string{"docs", "gone"})
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, "warning [unknown] gone:") || len(res.result.Failed) != 0 {
		t.Errorf("%+v", res)
	}
	if text := read(t, path(h, mcp.ClaudeFile)); strings.Contains(text, "gone") || !strings.Contains(text, "docs") {
		t.Errorf(".mcp.json: %s", text)
	}
}

func TestMissingEnvIsWarnedByNameOnly(t *testing.T) {
	value := "secret-value-17"
	t.Setenv("DOCS_TOKEN", value)
	t.Setenv("DOCS_ORG", "")
	t.Setenv("PATH", t.TempDir())
	h := home(t, []string{claude}, []string{"docs"})
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, "warning [env-missing] docs: environment variable DOCS_ORG") ||
		!strings.Contains(res.stderr, "warning [env-missing] docs: command npx") {
		t.Errorf("%+v", res)
	}
	if strings.Contains(res.stderr, "DOCS_TOKEN is not set") || strings.Contains(res.stdout+res.stderr+read(t, path(h, mcp.ClaudeFile)), value) {
		t.Errorf("value leaked or set variable reported: %+v", res)
	}
	rows := mcp.Inspect(h)
	if len(rows) != len(servers) || !reflect.DeepEqual(rows[0].MissingEnv, []string{"DOCS_ORG"}) || rows[0].MissingCommand != "npx" || !rows[0].Enabled || rows[1].Enabled {
		t.Errorf("rows %+v", rows)
	}
}

func TestCorruptStateIsAnErrorAndWritesNothing(t *testing.T) {
	h := home(t, []string{claude}, chosen)
	write(t, path(h, brand.StateFile), "not json")
	if _, err := mcp.Sync(h, true, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || exists(path(h, mcp.ClaudeFile)) {
		t.Errorf("err %v", err)
	}
}

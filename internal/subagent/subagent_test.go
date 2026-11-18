package subagent_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/subagent"
)

const (
	claude     = config.AgentClaude
	codex      = config.AgentCodex
	claudeFile = subagent.ClaudeDir + "/reviewer.md"
	codexFile  = subagent.CodexDir + "/reviewer.toml"
	userText   = "mine\n"
	failedMod  = "error [" + "generated-modified" + "] "
	failedPC   = "error [" + "path-conflict" + "] "
	failedBad  = "error [" + "subagent-invalid" + "] "
	// simple is a valid definition with nothing but the required parts.
	simple = "---\nname: %s\ndescription: Does one thing.\n---\n\nDo the thing.\n"
)

func source(t *testing.T) string {
	t.Helper()
	return read(t, filepath.Join("testdata", "reviewer.md"))
}

func simpleSource(name string) string {
	return strings.Replace(simple, "%s", name, 1)
}

// home has the reviewer source and the given agents and choices.
func home(t *testing.T, agents []string, names []string) *config.Home {
	t.Helper()
	h := &config.Home{
		Root:     t.TempDir(),
		Personal: config.Personal{SchemaVersion: config.SchemaVersion, Agents: agents, Subagents: names},
	}
	write(t, path(h, subagent.SourceDir+"/reviewer.md"), source(t))
	return h
}

type outcome struct {
	result subagent.Result
	stdout string
	stderr string
}

func syncHome(t *testing.T, h *config.Home, force bool) outcome {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := subagent.Sync(h, force, &stdout, &stderr)
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

func rows(t *testing.T, h *config.Home) map[string]subagent.Row {
	t.Helper()
	list, err := subagent.Inspect(h)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]subagent.Row{}
	for _, row := range list {
		byName[row.Name] = row
	}
	return byName
}

func TestGoldenFiles(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	res := syncHome(t, h, false)
	if len(res.result.Failed) != 0 || res.stderr != "" {
		t.Fatalf("%+v", res)
	}
	golden := map[string]string{claudeFile: "reviewer.claude.md", codexFile: "reviewer.codex.toml"}
	for rel, file := range golden {
		want := read(t, filepath.Join("testdata", file))
		if got := read(t, path(h, rel)); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", rel, got, want)
		}
	}
	if row := rows(t, h)["reviewer"]; row.State != subagent.OK || !row.Enabled {
		t.Errorf("row %+v", row)
	}
	if res = syncHome(t, h, false); !strings.Contains(res.stdout, "up to date") || len(res.result.Failed) != 0 {
		t.Errorf("rerun: %+v", res)
	}
}

func TestClaudeFieldsKeepTheirStructure(t *testing.T) {
	h := home(t, []string{claude}, []string{"reviewer"})
	syncHome(t, h, false)
	text := read(t, path(h, claudeFile))
	front := strings.SplitN(text, "---\n", 3)[1]
	var got map[string]any
	if err := yaml.Unmarshal([]byte(front), &got); err != nil {
		t.Fatal(err)
	}
	hooks := map[string]any{"PreToolUse": []any{map[string]any{
		"matcher": "Bash",
		"hooks":   []any{map[string]any{"type": "command", "command": "./bin/check --strict"}},
	}}}
	want := map[string]any{
		"name": "reviewer", "description": "Reviews a diff: correctness first, then style.",
		"tools": []any{"Read", "Grep", "Glob"}, "model": "sonnet", "effort": "medium",
		"hooks": hooks, "version": "2", "color": "blue",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %#v\nwant %#v", got, want)
	}
	if exists(path(h, subagent.CodexDir)) || exists(path(h, ".codex")) {
		t.Error("codex is not chosen but its directory was written")
	}
}

func TestCodexFieldsKeepTheirTypes(t *testing.T) {
	h := home(t, []string{codex}, []string{"reviewer"})
	syncHome(t, h, false)
	var got map[string]any
	if _, err := toml.Decode(read(t, path(h, codexFile)), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"name":                   "reviewer",
		"description":            "Reviews a diff: correctness first, then style.",
		"developer_instructions": "You review diffs. Report each finding with \"file:line\".\n\nDo not edit files.",
		"model":                  "gpt-5.4",
		"model_reasoning_effort": "high",
		"max_turns":              int64(12),
		"temperature":            0.5,
		"budget":                 3.0,
		"strict":                 true,
		"features":               map[string]any{"shell_tool": false, "web": map[string]any{"search": true}},
		"nicknames":              []any{"Rev", "R2", int64(3), false},
		"odd key":                "x",
		"empty":                  map[string]any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %#v\nwant %#v", got, want)
	}
	if exists(path(h, subagent.ClaudeDir)) {
		t.Error("claude is not chosen but its directory was written")
	}
}

func TestInvalidSourcesAreReportedAndSkipped(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"nofront":     {"name: nofront\n\nbody\n", "no frontmatter"},
		"unclosed":    {"---\nname: unclosed\ndescription: d\n\nbody\n", "no frontmatter"},
		"badyaml":     {"---\nname: [badyaml\n---\nbody\n", "not valid YAML"},
		"notmap":      {"---\n- a\n---\nbody\n", "must be a YAML mapping"},
		"noname":      {"---\ndescription: d\n---\nbody\n", "name is missing"},
		"nodesc":      {"---\nname: nodesc\n---\nbody\n", "description must be a non-empty string"},
		"blankdesc":   {"---\nname: blankdesc\ndescription: \"  \"\n---\nbody\n", "description must be a non-empty string"},
		"othername":   {"---\nname: other\ndescription: d\n---\nbody\n", "must equal the file name"},
		"Upper":       {"---\nname: Upper\ndescription: d\n---\nbody\n", "file name \"Upper\" must match"},
		"numname":     {"---\nname: 12\ndescription: d\n---\nbody\n", "name must be a string"},
		"nobody":      {"---\nname: nobody\ndescription: d\n---\n \n\t\n", "body after the frontmatter is empty"},
		"extra":       {"---\nname: extra\ndescription: d\ntools: Read\n---\nbody\n", "unknown key \"tools\""},
		"dupkey":      {"---\nname: dupkey\nname: dupkey\ndescription: d\n---\nbody\n", "duplicate key \"name\""},
		"claudelist":  {"---\nname: claudelist\ndescription: d\nclaude: [a]\n---\nbody\n", "claude must be a mapping"},
		"codexstr":    {"---\nname: codexstr\ndescription: d\ncodex: x\n---\nbody\n", "codex must be a mapping"},
		"claudename":  {"---\nname: claudename\ndescription: d\nclaude:\n  name: x\n---\nbody\n", "claude.name is not allowed"},
		"codexdev":    {"---\nname: codexdev\ndescription: d\ncodex:\n  developer_instructions: x\n---\nbody\n", "codex.developer_instructions is not allowed"},
		"codexdesc":   {"---\nname: codexdesc\ndescription: d\ncodex:\n  description: x\n---\nbody\n", "codex.description is not allowed"},
		"codexnull":   {"---\nname: codexnull\ndescription: d\ncodex:\n  model: null\n---\nbody\n", "codex.model: null is not supported"},
		"codextime":   {"---\nname: codextime\ndescription: d\ncodex:\n  at: 2026-11-18\n---\nbody\n", "codex.at: a timestamp is not supported"},
		"codexalias":  {"---\nname: codexalias\ndescription: d\nclaude:\n  model: &m sonnet\ncodex:\n  model: *m\n---\nbody\n", "codex.model: anchors and aliases"},
		"codexanchr":  {"---\nname: codexanchr\ndescription: d\ncodex:\n  model: &m x\n---\nbody\n", "codex.model: anchors and aliases"},
		"claudecycle": {"---\nname: claudecycle\ndescription: d\nclaude: {custom: &a {self: *a}}\n---\nbody\n", "claude.custom: anchors and aliases are not supported"},
		"claudealias": {"---\nname: claudealias\ndescription: d\nclaude:\n  model: &m sonnet\n  fallback: *m\n---\nbody\n", "claude.fallback: anchors and aliases are not supported"},
		"topanchor":   {"---\nname: &n topanchor\ndescription: d\n---\nbody\n", "name: anchors and aliases are not supported"},
		"codexkey":    {"---\nname: codexkey\ndescription: d\ncodex:\n  1: x\n---\nbody\n", "codex.1: keys must be strings"},
		"codexmaps":   {"---\nname: codexmaps\ndescription: d\ncodex:\n  list: [{a: 1}]\n---\nbody\n", "codex.list[0]: a list may hold only"},
		"codexdeep":   {"---\nname: codexdeep\ndescription: d\ncodex:\n  f:\n    a: 1\n    a: 2\n---\nbody\n", "duplicate key codex.f.a"},
		"codexbig":    {"---\nname: codexbig\ndescription: d\ncodex:\n  n: 18446744073709551615\n---\nbody\n", "does not fit"},
	}
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	for name, c := range cases {
		write(t, path(h, subagent.SourceDir+"/"+name+".md"), c.text)
	}
	write(t, path(h, subagent.SourceDir+"/_template.md"), "not a definition\n")
	write(t, path(h, subagent.SourceDir+"/notes.txt"), "not a definition\n")
	list := rows(t, h)
	for name, c := range cases {
		row, found := list[name]
		if !found || row.State != subagent.Invalid || !strings.Contains(row.Detail, c.want) || !strings.HasPrefix(row.Detail, subagent.SourceDir+"/"+name+".md: ") {
			t.Errorf("%s: %+v, want %q", name, row, c.want)
		}
	}
	for _, name := range []string{"_template", "notes"} {
		if _, found := list[name]; found {
			t.Errorf("%s is listed", name)
		}
	}
	if len(list) != len(cases)+1 {
		t.Errorf("%d rows, want %d", len(list), len(cases)+1)
	}
	res := syncHome(t, h, false)
	if len(res.result.Failed) != 0 || !exists(path(h, claudeFile)) || !exists(path(h, codexFile)) {
		t.Errorf("the valid file was not generated: %+v", res)
	}
	files := recorded(t, h)
	if len(files) != 2 {
		t.Errorf("state %v", files)
	}
	cat, err := subagent.Discover(h.Root)
	if err != nil {
		t.Fatal(err)
	}
	if got := cat.Choosable(); !reflect.DeepEqual(got, []string{"reviewer"}) {
		t.Errorf("choosable %v", got)
	}
}

func TestSourceThatBecomesInvalidKeepsItsFiles(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	syncHome(t, h, false)
	claudeBefore := read(t, path(h, claudeFile))
	codexBefore := read(t, path(h, codexFile))
	write(t, path(h, subagent.SourceDir+"/reviewer.md"), "---\nname: reviewer\n---\nbody\n")
	for _, force := range []bool{false, true} {
		res := syncHome(t, h, force)
		if !strings.Contains(res.stderr, failedBad+"reviewer:") || len(res.result.Failed) != 1 {
			t.Errorf("force=%v: %+v", force, res)
		}
		if read(t, path(h, claudeFile)) != claudeBefore || read(t, path(h, codexFile)) != codexBefore || len(recorded(t, h)) != 2 {
			t.Errorf("force=%v: generated files changed", force)
		}
	}
	if row := rows(t, h)["reviewer"]; row.State != subagent.Invalid || !row.Enabled {
		t.Errorf("row %+v", row)
	}
}

func TestDisableRemovesFilesAndEmptyDirectories(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	syncHome(t, h, false)
	h.Personal.Subagents = nil
	res := syncHome(t, h, false)
	if len(res.result.Failed) != 0 || exists(path(h, subagent.ClaudeDir)) || exists(path(h, ".codex")) {
		t.Errorf("not removed: %+v", res)
	}
	if !exists(path(h, ".claude")) {
		t.Error(".claude/ was removed")
	}
	if exists(path(h, brand.StateFile)) {
		t.Error("empty state left behind")
	}

	h = home(t, []string{claude, codex}, []string{"reviewer"})
	write(t, path(h, ".codex/config.toml"), userText)
	write(t, path(h, subagent.ClaudeDir+"/mine.md"), userText)
	syncHome(t, h, false)
	h.Personal.Agents = nil
	res = syncHome(t, h, false)
	if len(res.result.Failed) != 0 || exists(path(h, subagent.CodexDir)) || exists(path(h, claudeFile)) {
		t.Errorf("not removed: %+v", res)
	}
	if read(t, path(h, ".codex/config.toml")) != userText || read(t, path(h, subagent.ClaudeDir+"/mine.md")) != userText {
		t.Error("a file sync did not write was touched")
	}
}

func TestEditedFileIsLeftAloneUntilForce(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	syncHome(t, h, false)
	write(t, path(h, claudeFile), userText)
	write(t, path(h, subagent.SourceDir+"/reviewer.md"), strings.Replace(source(t), "Do not edit files.", "Never edit files.", 1))
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+claudeFile+": ") || !strings.Contains(res.stderr, "subagents/reviewer.md") || len(res.result.Failed) != 1 {
		t.Fatalf("not reported: %+v", res)
	}
	if read(t, path(h, claudeFile)) != userText || !strings.Contains(read(t, path(h, codexFile)), "Never edit") {
		t.Error("edited file overwritten, or the other one not updated")
	}
	if row := rows(t, h)["reviewer"]; row.State != subagent.Modified {
		t.Errorf("row %+v", row)
	}
	h.Personal.Subagents = nil
	res = syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+claudeFile+": ") || read(t, path(h, claudeFile)) != userText || exists(path(h, codexFile)) {
		t.Errorf("edited file removed: %+v", res)
	}
	res = syncHome(t, h, true)
	if len(res.result.Failed) != 0 || exists(path(h, claudeFile)) || len(recorded(t, h)) != 0 {
		t.Errorf("force did not remove: %+v", res)
	}

	h = home(t, []string{claude}, []string{"reviewer"})
	write(t, path(h, claudeFile), userText)
	res = syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+claudeFile+": ") || read(t, path(h, claudeFile)) != userText {
		t.Errorf("unrecorded file overwritten: %+v", res)
	}
	res = syncHome(t, h, true)
	if len(res.result.Failed) != 0 || read(t, path(h, claudeFile)) != read(t, filepath.Join("testdata", "reviewer.claude.md")) {
		t.Errorf("force did not overwrite: %+v", res)
	}
}

func TestUnrecordedFilesOutsideTheTargetsAreNeverTouched(t *testing.T) {
	h := home(t, []string{claude, codex}, nil)
	mine := map[string]string{
		claudeFile:                        userText,
		codexFile:                         userText,
		subagent.ClaudeDir + "/other.md":  userText,
		subagent.CodexDir + "/other.toml": userText,
	}
	for rel, text := range mine {
		write(t, path(h, rel), text)
	}
	for _, force := range []bool{false, true} {
		res := syncHome(t, h, force)
		if len(res.result.Failed) != 0 || res.stderr != "" {
			t.Errorf("force=%v: %+v", force, res)
		}
	}
	h.Personal.Subagents = []string{"reviewer"}
	h.Personal.Agents = []string{claude}
	syncHome(t, h, true)
	h.Personal.Subagents = nil
	syncHome(t, h, true)
	for rel, text := range mine {
		if rel == claudeFile {
			continue
		}
		if !exists(path(h, rel)) || read(t, path(h, rel)) != text {
			t.Errorf("%s was touched", rel)
		}
	}
}

func TestSymlinksAreRefusedEvenWithForce(t *testing.T) {
	for _, link := range []string{claudeFile, subagent.ClaudeDir, subagent.CodexDir, ".codex", brand.StateFile} {
		for _, force := range []bool{false, true} {
			h := home(t, []string{claude, codex}, []string{"reviewer"})
			outside := filepath.Join(t.TempDir(), "outside")
			if strings.HasSuffix(link, ".md") || strings.HasSuffix(link, ".json") {
				write(t, outside, userText)
			} else if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path(h, link)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path(h, link)); err != nil {
				t.Fatal(err)
			}
			res := syncHome(t, h, force)
			if !strings.Contains(res.stderr, failedPC) || len(res.result.Failed) == 0 {
				t.Errorf("%s force=%v: %+v", link, force, res)
			}
			info, err := os.Stat(outside)
			if err != nil {
				t.Fatal(err)
			}
			if info.IsDir() {
				if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
					t.Errorf("%s force=%v: wrote through the link: %v %v", link, force, entries, err)
				}
			} else if read(t, outside) != userText {
				t.Errorf("%s force=%v: target changed", link, force)
			}
			if row := rows(t, h)["reviewer"]; row.State != subagent.PathConflict {
				t.Errorf("%s: row %+v", link, row)
			}
		}
	}
}

func TestStaleChoiceRemovesItsFiles(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	syncHome(t, h, false)
	if err := os.Remove(path(h, subagent.SourceDir+"/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	row := rows(t, h)["reviewer"]
	if row.State != subagent.Stale || !row.Enabled || !strings.Contains(row.Detail, "subagent disable reviewer") {
		t.Errorf("row %+v", row)
	}
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, "warning [unknown] reviewer:") || len(res.result.Failed) != 0 {
		t.Errorf("%+v", res)
	}
	if exists(path(h, claudeFile)) || exists(path(h, codexFile)) || len(recorded(t, h)) != 0 {
		t.Error("stale files not removed")
	}
}

func TestSharedStateKeepsTheOtherSidesRecords(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	h.Team.MCP = []config.MCP{{Name: "docs", Transport: config.TransportStdio, Command: "npx"}}
	h.Personal.MCP = []string{"docs"}
	if _, err := mcp.Sync(h, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	syncHome(t, h, false)
	all := []string{mcp.ClaudeFile, mcp.CodexFile, claudeFile, codexFile}
	files := recorded(t, h)
	for _, rel := range all {
		if files[rel] == "" {
			t.Fatalf("%s not recorded: %v", rel, files)
		}
	}
	h.Personal.Subagents = nil
	syncHome(t, h, false)
	files = recorded(t, h)
	if len(files) != 2 || files[mcp.ClaudeFile] == "" || files[mcp.CodexFile] == "" || !exists(path(h, mcp.CodexFile)) {
		t.Errorf("subagent sync dropped MCP records: %v", files)
	}
	h.Personal.Subagents = []string{"reviewer"}
	syncHome(t, h, false)
	h.Personal.Agents = nil
	if _, err := mcp.Sync(h, false, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	files = recorded(t, h)
	if len(files) != 2 || files[claudeFile] == "" || files[codexFile] == "" || !exists(path(h, codexFile)) {
		t.Errorf("MCP sync dropped subagent records: %v", files)
	}
}

func TestNoAgentChosenIsANote(t *testing.T) {
	h := home(t, nil, []string{"reviewer"})
	write(t, path(h, subagent.SourceDir+"/helper.md"), simpleSource("helper"))
	res := syncHome(t, h, false)
	if !strings.Contains(res.stdout, "no agent is chosen") || len(res.result.Failed) != 0 || exists(path(h, ".claude")) {
		t.Errorf("%+v", res)
	}
	list := rows(t, h)
	if list["reviewer"].State != subagent.OK || list["helper"].State != subagent.Off || list["helper"].Detail != "Does one thing." {
		t.Errorf("rows %+v", list)
	}
}

func TestCorruptStateIsAnErrorAndWritesNothing(t *testing.T) {
	h := home(t, []string{claude}, []string{"reviewer"})
	write(t, path(h, brand.StateFile), "not json")
	if _, err := subagent.Sync(h, true, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || exists(path(h, claudeFile)) {
		t.Errorf("err %v", err)
	}
	if _, err := subagent.Inspect(h); err == nil {
		t.Error("Inspect read a corrupt state")
	}
}

func TestSourceDirThatIsNotADirectoryIsAPathConflict(t *testing.T) {
	for _, kind := range []string{"symlink", "file"} {
		h := home(t, []string{claude, codex}, []string{"reviewer"})
		syncHome(t, h, false)
		before := recorded(t, h)
		source := path(h, subagent.SourceDir)
		moved := filepath.Join(t.TempDir(), "moved")
		if err := os.Rename(source, moved); err != nil {
			t.Fatal(err)
		}
		if kind == "symlink" {
			if err := os.Symlink(moved, source); err != nil {
				t.Fatal(err)
			}
		} else {
			write(t, source, userText)
		}
		if _, err := subagent.Sync(h, true, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), failedPC+subagent.SourceDir) {
			t.Errorf("%s: sync err %v", kind, err)
		}
		if _, err := subagent.Inspect(h); err == nil {
			t.Errorf("%s: Inspect did not fail", kind)
		}
		if !exists(path(h, claudeFile)) || !exists(path(h, codexFile)) || !reflect.DeepEqual(recorded(t, h), before) {
			t.Errorf("%s: generated files or records changed", kind)
		}
	}
}

func TestInspectReportsEditedLeftovers(t *testing.T) {
	h := home(t, []string{claude, codex}, []string{"reviewer"})
	syncHome(t, h, false)
	h.Personal.Agents = []string{claude}
	write(t, path(h, codexFile), userText)
	if row := rows(t, h)["reviewer"]; row.State != subagent.Modified || !strings.Contains(row.Detail, codexFile) {
		t.Errorf("agent not chosen: %+v", row)
	}
	h.Personal.Subagents = nil
	if row := rows(t, h)["reviewer"]; row.State != subagent.Modified || row.Enabled {
		t.Errorf("disabled: %+v", row)
	}
	if err := os.Remove(path(h, subagent.SourceDir+"/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	if row, found := rows(t, h)["reviewer"]; !found || row.State != subagent.Modified || row.Enabled {
		t.Errorf("orphan: %+v %v", row, found)
	}
	h.Personal.Subagents = []string{"reviewer"}
	if row := rows(t, h)["reviewer"]; !row.Stale || row.State != subagent.Modified {
		t.Errorf("stale: %+v", row)
	}
	res := syncHome(t, h, false)
	if !strings.Contains(res.stderr, failedMod+codexFile) {
		t.Errorf("sync and Inspect disagree: %+v", res)
	}
	if exists(path(h, claudeFile)) {
		t.Error("untouched stale file not removed")
	}
	if _, found := rows(t, h)["ghost"]; found {
		t.Error("listed a name with nothing left")
	}
}

func TestUntouchedLeftoverIsNotAProblem(t *testing.T) {
	h := home(t, []string{claude}, []string{"reviewer"})
	syncHome(t, h, false)
	h.Personal.Subagents = nil
	if row := rows(t, h)["reviewer"]; row.State != subagent.Off {
		t.Errorf("row %+v", row)
	}
}

func TestInvalidUnchosenSourceWithEditedLeftoverKeepsBoth(t *testing.T) {
	h := home(t, []string{claude}, []string{"reviewer"})
	syncHome(t, h, false)
	h.Personal.Subagents = nil
	write(t, path(h, claudeFile), userText)
	write(t, path(h, subagent.SourceDir+"/reviewer.md"), "---\nname: reviewer\n---\nbody\n")
	row := rows(t, h)["reviewer"]
	if row.State != subagent.Modified || !strings.Contains(row.Detail, claudeFile) || !strings.Contains(row.Invalid, "description must be") {
		t.Errorf("row %+v", row)
	}
	if full := row.FullDetail(); !strings.Contains(full, claudeFile) || !strings.Contains(full, "description must be") {
		t.Errorf("full detail %q", full)
	}
	h.Personal.Subagents = []string{"reviewer"}
	if row := rows(t, h)["reviewer"]; row.State != subagent.Invalid || row.Invalid == "" || row.FullDetail() != row.Invalid {
		t.Errorf("chosen: %+v", row)
	}
}

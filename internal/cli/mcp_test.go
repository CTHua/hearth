package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/mcp"
)

const (
	docsURL    = "https://mcp.example.invalid/mcp"
	docsSecret = "placeholder-value-42"
)

// mcpHome has a stdio server docs and an http server tracker, both chosen,
// with both agents.
func mcpHome(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	run(t, "-C", home, "mcp", "add", "docs", "--env-var", "DOCS_TOKEN", "--", "npx", "-y", "@acme/docs-mcp").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "add", "tracker", "--url", docsURL, "--bearer-token-env", "TRACKER_TOKEN").expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", bothAgents).expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "docs", "tracker").expect(t, exitOK, "")
	return home
}

func TestMCPAddRejectsBadEntriesWithoutWriting(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "mcp", "add", "docs", "--", "npx").expect(t, exitOK, "")
	before := read(t, teamFile(home))
	cases := [][]string{
		{"DOCS", "--", "npx"},
		{"a.b", "--", "npx"},
		{"x", "--env-var", "lower", "--", "npx"},
		{"x", "--url", docsURL, "--header-env", "X-A=A", "--header-env", "x-a=B"},
		{"x", "--url", docsURL, "--header-env", "authorization=T"},
		{"x", "--url", docsURL, "--header-env", "AUTHORIZATION=T"},
		{"x", "--url", docsURL, "--header-env", "no-separator"},
		{"x", "--url", "http://127.0.0.10/mcp"},
		{"x", "--url", "http://localhost.evil.invalid/mcp"},
		{"x", "--url", "https://user:pw@mcp.example.invalid/mcp"},
		{"x", "--url", "ftp://mcp.example.invalid"},
		{"x", "--url", docsURL, "--", "npx"},
		{"x", "--url", docsURL, "--env-var", "A"},
		{"x", "--bearer-token-env", "T", "--", "npx"},
		{"x", "--header-env", "X=T", "--", "npx"},
		// Flags of the other transport count when given empty.
		{"x", "--bearer-token-env", "", "--", "npx"},
		{"x", "--header-env", "=", "--", "npx"},
		{"x", "--url", "", "--", "npx"},
		{"x", "--url", ""},
		{"x", "--url", docsURL, "--env-var", ""},
		{"x", "--url", docsURL, "--", ""},
		{"x"},
		{},
	}
	for _, args := range cases {
		res := run(t, append([]string{"-C", home, "mcp", "add"}, args...)...)
		if res.code != exitBad || !strings.Contains(res.stderr, "error [usage]") {
			t.Errorf("%v: exit %d %s", args, res.code, res.stderr)
		}
		if strings.Contains(res.stderr, ":pw@") {
			t.Errorf("%v: the password was printed: %s", args, res.stderr)
		}
	}
	if read(t, teamFile(home)) != before {
		t.Errorf("team file changed:\n%s", read(t, teamFile(home)))
	}
}

func TestMCPAddKeepsRepeatedFlagsInOrder(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "mcp", "add", "local", "--url", "http://127.0.0.1:3000/mcp",
		"--header-env", "X-Zed=ZED", "--header-env", "X-Alpha=ALPHA=1").expect(t, exitBad, "error [usage]")
	r := run(t, "-C", home, "mcp", "add", "local", "--url", "http://127.0.0.1:3000/mcp", "--header-env", "X-Zed=ZED", "--header-env", "X-Alpha=ALPHA")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, brand.TeamFileChangedHint) {
		t.Errorf("stdout: %s", r.stdout)
	}
	run(t, "-C", home, "mcp", "add", "docs", "--env-var", "ZED", "--env-var", "ALPHA", "--", "npx", "-y", "--flag", "@acme/docs-mcp").expect(t, exitOK, "")
	text := read(t, teamFile(home))
	for _, want := range []string{
		`header_envs = { "X-Zed" = "ZED", "X-Alpha" = "ALPHA" }`,
		`env_vars = ["ZED", "ALPHA"]`,
		`args = ["-y", "--flag", "@acme/docs-mcp"]`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("team file lacks %s:\n%s", want, text)
		}
	}
	run(t, "-C", home, "mcp", "list").expect(t, exitOK, "")
}

func TestSyncWritesBothFilesAndForceOverwritesAnEditedOne(t *testing.T) {
	home := mcpHome(t)
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	codex := read(t, at(home, mcp.CodexFile))
	write(t, at(home, mcp.ClaudeFile), "{}\n")
	run(t, "-C", home, "config", "agents", "claude").expect(t, exitOK, "")
	res := run(t, "-C", home, "sync")
	res.expect(t, exitFailed, "error [generated-modified] "+mcp.ClaudeFile+":")
	if read(t, at(home, mcp.ClaudeFile)) != "{}\n" {
		t.Error("edited .mcp.json overwritten")
	}
	if exists(at(home, mcp.CodexFile)) {
		t.Errorf("codex file not removed after dropping codex; it was %q", codex)
	}
	run(t, "-C", home, "mcp", "sync", "--force").expect(t, exitOK, "")
	if !strings.Contains(read(t, at(home, mcp.ClaudeFile)), "\"tracker\"") {
		t.Error("force did not overwrite")
	}
	run(t, "-C", home, "mcp", "sync").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "sync", "extra").expect(t, exitBad, "error [usage]")
}

func TestTopLevelSyncRunsMCPAfterSkillFailure(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, strings.Repeat("0", 40), "claude")
	run(t, "-C", home, "mcp", "add", "docs", "--", "npx").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "docs").expect(t, exitOK, "")
	write(t, at(home, mcp.ClaudeFile), "{}\n")
	res := run(t, "-C", home, "sync", "--force")
	res.expect(t, exitFailed, "error [clone-failed] demo:")
	if strings.Contains(res.stderr, "[usage]") || !strings.Contains(read(t, at(home, mcp.ClaudeFile)), "\"docs\"") {
		t.Errorf("MCP did not run after the skill failure: %+v", res)
	}
	run(t, "-C", home, "sync", "--bogus").expect(t, exitBad, "error [usage]")
}

func TestRemovedServerIsAStaleChoice(t *testing.T) {
	home := mcpHome(t)
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "remove", "Tracker").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "list").expect(t, exitOK, "warning [unknown] tracker:")
	doctor := run(t, "-C", home, "doctor")
	if doctor.code == exitBad || !strings.Contains(doctor.stdout, "warning: [unknown] tracker:") {
		t.Errorf("doctor: %+v", doctor)
	}
	run(t, "-C", home, "sync").expect(t, exitOK, "warning [unknown] tracker:")
	for _, rel := range []string{mcp.ClaudeFile, mcp.CodexFile} {
		if strings.Contains(read(t, at(home, rel)), "tracker") {
			t.Errorf("%s still has the removed server", rel)
		}
	}
	run(t, "-C", home, "mcp", "disable", "tracker").expect(t, exitOK, "")
	if strings.Contains(read(t, localFile(home)), "tracker") {
		t.Errorf("disable kept the stale name: %s", read(t, localFile(home)))
	}
	run(t, "-C", home, "mcp", "enable", "nope").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "mcp", "disable", "nope").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "mcp", "remove", "nope").expect(t, exitBad, "error [usage]")
}

func TestDoctorWarnsAboutEnvByNameOnly(t *testing.T) {
	home := mcpHome(t)
	t.Setenv("DOCS_TOKEN", docsSecret)
	gittest.Unsetenv(t, "TRACKER_TOKEN")
	res := run(t, "-C", home, "doctor")
	if res.code != exitOK || !strings.Contains(res.stdout, "warning: [env-missing] tracker: environment variable TRACKER_TOKEN") {
		t.Errorf("doctor: %+v", res)
	}
	if strings.Contains(res.stdout+res.stderr, docsSecret) {
		t.Errorf("value printed: %+v", res)
	}
	// Without git the MCP checks still run.
	t.Setenv("PATH", t.TempDir())
	res = run(t, "-C", home, "doctor")
	res.expect(t, exitBad, "error [env] git not found")
	for _, want := range []string{"warning: [env-missing] docs: command npx not found in PATH", "warning: [env-missing] tracker:"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("without git, doctor lacks %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout+res.stderr, docsSecret) {
		t.Errorf("value printed: %+v", res)
	}
}

func TestMCPListAndDoctorNeitherWriteNorStartServers(t *testing.T) {
	home := newHome(t)
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	server := filepath.Join(bin, "fake-server")
	write(t, server, "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(server, 0o755); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	symlink(t, git, filepath.Join(bin, "git"))
	run(t, "-C", home, "mcp", "add", "fake", "--", "fake-server").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "add", "remote", "--url", docsURL).expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", bothAgents).expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "fake", "remote").expect(t, exitOK, "")
	t.Setenv("PATH", bin)
	before := tree(t, home)
	for _, args := range [][]string{{"mcp", "list"}, {"doctor"}} {
		res := run(t, append([]string{"-C", home}, args...)...)
		if res.code == exitBad {
			t.Errorf("%v: exit %d %s", args, res.code, res.stderr)
		}
	}
	assertSameTree(t, before, tree(t, home))
	if exists(marker) {
		t.Error("list or doctor started the server")
	}
}

func TestSyncLeavesNewHomeRepoClean(t *testing.T) {
	home := mcpHome(t)
	gittest.Git(t, home, "add", "-A")
	gittest.Git(t, home, "commit", "--quiet", "-m", "init")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	for _, rel := range []string{mcp.ClaudeFile, mcp.CodexFile, brand.StateFile} {
		if !exists(at(home, rel)) {
			t.Errorf("%s not written", rel)
		}
	}
	if status := gittest.Git(t, home, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("sync left changes:\n%s", status)
	}
}

func TestRelativeCommandIsResolvedAgainstHomeRepo(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "mcp", "add", "local", "--", "./bin/serve").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "local").expect(t, exitOK, "")
	const missing = "command ./bin/serve not found"
	elsewhere := t.TempDir()
	mkdir(t, filepath.Join(elsewhere, "bin"))
	write(t, filepath.Join(elsewhere, "bin", "serve"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(elsewhere, "bin", "serve"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)
	if res := run(t, "-C", home, "doctor"); !strings.Contains(res.stdout, missing) {
		t.Errorf("a same-named file in the cwd counted: %s", res.stdout)
	}
	mkdir(t, at(home, "bin"))
	write(t, at(home, "bin/serve"), "#!/bin/sh\n")
	if res := run(t, "-C", home, "doctor"); !strings.Contains(res.stdout, missing) {
		t.Errorf("a file that is not executable counted: %s", res.stdout)
	}
	if err := os.Chmod(at(home, "bin/serve"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := at(home, "memory")
	for _, dir := range []string{elsewhere, sub} {
		t.Chdir(dir)
		for _, args := range [][]string{{"-C", home, "doctor"}, {"doctor"}} {
			if dir == elsewhere && len(args) == 1 {
				continue
			}
			if res := run(t, args...); strings.Contains(res.stdout+res.stderr, missing) || !strings.Contains(res.stdout, "ok: mcp server local") {
				t.Errorf("%s %v: %s", dir, args, res.stdout)
			}
		}
		list := run(t, "-C", home, "mcp", "list")
		if list.code != exitOK {
			t.Errorf("%s list: %+v", dir, list)
		}
	}
}

func TestSyncPrintsConfigErrorsAndWarningsOnce(t *testing.T) {
	home := mcpHome(t)
	res := run(t, "-C", home, "sync")
	res.expect(t, exitOK, "")
	if n := strings.Count(res.stderr, "warning: dev build"); n != 1 {
		t.Errorf("dev warning printed %d times:\n%s", n, res.stderr)
	}
	write(t, localFile(home), "schema_version = 2\ncolor = \"red\"\n")
	res = run(t, "-C", home, "sync")
	res.expect(t, exitBad, "unknown key color")
	if n := strings.Count(res.stderr, "unknown key color"); n != 1 {
		t.Errorf("config error printed %d times:\n%s", n, res.stderr)
	}
}

func TestStaleChoiceIsNeitherEnabledNorDisabledInList(t *testing.T) {
	home := mcpHome(t)
	run(t, "-C", home, "mcp", "remove", "tracker").expect(t, exitOK, "")
	res := run(t, "-C", home, "mcp", "list")
	for _, line := range lines(res.stdout) {
		fields := strings.Fields(line)
		if len(fields) == 4 && fields[0] == "tracker" && (fields[1] != "-" || fields[2] != "-") {
			t.Errorf("stale row: %q", line)
		}
		if len(fields) == 4 && fields[0] == "docs" && fields[2] != "yes" {
			t.Errorf("docs row: %q", line)
		}
	}
	if !strings.Contains(res.stdout, "tracker") {
		t.Errorf("no stale row:\n%s", res.stdout)
	}
}

func TestSymlinkedFileOfDroppedAgentIsLeftAlone(t *testing.T) {
	home := mcpHome(t)
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	outside := filepath.Join(t.TempDir(), "outside.json")
	write(t, outside, "{}\n")
	if err := os.Remove(at(home, mcp.ClaudeFile)); err != nil {
		t.Fatal(err)
	}
	symlink(t, outside, at(home, mcp.ClaudeFile))
	run(t, "-C", home, "config", "agents", "none").expect(t, exitOK, "")
	for _, args := range [][]string{{"sync"}, {"sync", "--force"}} {
		res := run(t, append([]string{"-C", home}, args...)...)
		res.expect(t, exitFailed, "error [path-conflict] "+mcp.ClaudeFile+":")
		dest, err := os.Readlink(at(home, mcp.ClaudeFile))
		if err != nil || dest != outside {
			t.Errorf("%v: link changed: %q %v", args, dest, err)
		}
		if read(t, outside) != "{}\n" {
			t.Errorf("%v: target changed", args)
		}
	}
}

func TestUnchosenAgentsOwnFileIsNotTouched(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "mcp", "add", "docs", "--", "npx").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "docs").expect(t, exitOK, "")
	write(t, at(home, mcp.ClaudeFile), "{}\n")
	res := run(t, "-C", home, "sync")
	if res.code != exitOK || strings.Contains(res.stderr, "error") || read(t, at(home, mcp.ClaudeFile)) != "{}\n" {
		t.Errorf("%+v", res)
	}
}

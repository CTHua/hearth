package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/cli"
	"github.com/CTHua/hearth/internal/gittest"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

const (
	appHTTPS   = "https://git.example.invalid/acme/app.git"
	appSSH     = "git@git.example.invalid:acme/app.git"
	exitOK     = 0
	exitFailed = 1
	exitBad    = 2
)

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), args, &stdout, &stderr, false)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// runInTerminal runs as if stdin and stdout were terminals.
func runInTerminal(t *testing.T, args ...string) result {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), args, &stdout, &stderr, true)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (r result) expect(t *testing.T, code int, stderrHas string) {
	t.Helper()
	if r.code != code || !strings.Contains(r.stderr, stderrHas) {
		t.Fatalf("exit %d, want %d with stderr containing %q\nstdout: %s\nstderr: %s", r.code, code, stderrHas, r.stdout, r.stderr)
	}
}

func newHome(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "home")
	run(t, "init", dir).expect(t, exitOK, "")
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

func write(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func teamFile(home string) string {
	return filepath.Join(home, brand.TeamFile)
}

func localFile(home string) string {
	return filepath.Join(home, brand.LocalFile)
}

func TestVersionHelpAndUnknownCommand(t *testing.T) {
	r := run(t, "version")
	if r.code != exitOK || r.stdout != "dev\n" {
		t.Errorf("version: %+v", r)
	}
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		r := run(t, args...)
		if r.code != exitOK || !strings.Contains(r.stdout, "usage: "+brand.Name) {
			t.Errorf("%v: %+v", args, r)
		}
	}
	// No command outside a terminal prints the help like `help` does.
	if r := run(t); r.code != exitOK || !strings.HasPrefix(r.stdout, "usage: "+brand.Name) || r.stderr != "" {
		t.Errorf("no command: %+v", r)
	}
	run(t, "frobnicate").expect(t, exitBad, "error [usage] unknown command")
	run(t, "-x").expect(t, exitBad, "error [usage]")
}

func TestInitRefusesNonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "keep"), "x")
	run(t, "init", dir).expect(t, exitBad, "error [path-conflict]")
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("init wrote into a non-empty directory: %v", entries)
	}
	run(t, "init").expect(t, exitBad, "error [usage]")
}

func TestInitWithRelativeDirUnderC(t *testing.T) {
	base := t.TempDir()
	run(t, "-C", base, "init", "home").expect(t, exitOK, "")
	if _, err := os.Stat(teamFile(filepath.Join(base, "home"))); err != nil {
		t.Error(err)
	}
}

func TestRepoAddDefaultsWithBranch(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "added app -> products/app (main)") || !strings.Contains(r.stdout, brand.TeamFileChangedHint) {
		t.Errorf("stdout: %s", r.stdout)
	}
	want := "[[repos]]\nname = \"app\"\nurl = \"" + appHTTPS + "\"\npath = \"products/app\"\nbranch = \"main\"\n"
	if text := read(t, teamFile(home)); !strings.HasSuffix(text, "\n\n"+want) || !strings.HasPrefix(text, brand.TeamFileHeader+"\n") {
		t.Errorf("team file:\n%s", text)
	}
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("repo add with --branch wrote the personal file")
	}
}

func TestRepoAddExplicitNameAndPathAndFlagOrder(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", "--name", "api-service", appSSH, "--path", "products/team/api", "-branch", "develop").expect(t, exitOK, "")
	text := read(t, teamFile(home))
	for _, want := range []string{`name = "api-service"`, `url = "` + appSSH + `"`, `path = "products/team/api"`, `branch = "develop"`} {
		if !strings.Contains(text, want) {
			t.Errorf("team file lacks %s:\n%s", want, text)
		}
	}
}

func TestRepoAddAsksRemoteForDefaultBranch(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, appHTTPS, gittest.BareRemote(t, "trunk"))
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	// The team URL is ssh; the query uses the URL converted to the personal protocol.
	r := run(t, "-C", home, "repo", "add", appSSH)
	r.expect(t, exitOK, "")
	if !strings.Contains(read(t, teamFile(home)), `branch = "trunk"`) {
		t.Errorf("branch not taken from remote HEAD:\n%s", read(t, teamFile(home)))
	}
}

func TestRepoAddUnreachableRemoteNeedsBranch(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, appHTTPS, filepath.Join(t.TempDir(), "missing.git"))
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	before := read(t, teamFile(home))
	run(t, "-C", home, "repo", "add", appHTTPS).expect(t, exitBad, "--branch")
	if read(t, teamFile(home)) != before {
		t.Error("team file changed")
	}
}

func TestRepoAddWithoutProtocolOrBranchDetectsProtocol(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, appSSH, gittest.BareRemote(t, "trunk"))
	gittest.Redirect(t, appHTTPS, filepath.Join(t.TempDir(), "missing.git"))
	r := run(t, "-C", home, "repo", "add", appHTTPS)
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "detected ssh") {
		t.Errorf("stdout lacks the detection result: %s", r.stdout)
	}
	if got := read(t, localFile(home)); got != "schema_version = 1\nprotocol = \"ssh\"\n" {
		t.Errorf("personal file %q", got)
	}
	if !strings.Contains(read(t, teamFile(home)), `branch = "trunk"`) {
		t.Errorf("branch not asked over the detected protocol:\n%s", read(t, teamFile(home)))
	}
}

func TestRepoAddWithoutBranchChecksConflictsBeforeConnecting(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	other := "https://git.example.invalid/acme/other.git"
	// Any connection would fail and report an env error instead of usage.
	for _, url := range []string{appSSH, appHTTPS, other} {
		gittest.Redirect(t, url, filepath.Join(t.TempDir(), "missing.git"))
	}
	gittest.Redirect(t, "git@git.example.invalid:acme/other.git", filepath.Join(t.TempDir(), "missing.git"))
	before := read(t, teamFile(home))
	cases := []struct {
		args []string
		flag string
	}{
		{[]string{"https://git.example.invalid/acme/APP.git", "--path", "products/elsewhere"}, "--name"},
		{[]string{other, "--path", "products/app/inner"}, "--path"},
	}
	for _, c := range cases {
		r := run(t, append([]string{"-C", home, "repo", "add"}, c.args...)...)
		if r.code != exitBad || !strings.Contains(r.stderr, "error [usage]") || !strings.Contains(r.stderr, c.flag) {
			t.Errorf("%v: exit %d stderr %q, want a usage error naming %s", c.args, r.code, r.stderr, c.flag)
		}
		if strings.Contains(r.stderr, "error [env]") {
			t.Errorf("%v: contacted the remote before checking conflicts", c.args)
		}
		if read(t, teamFile(home)) != before {
			t.Fatalf("%v changed the team file", c.args)
		}
		if _, err := os.Stat(localFile(home)); err == nil {
			t.Fatalf("%v wrote the personal file", c.args)
		}
	}
}

func TestRepoAddDetectionFailureWritesNothing(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, appSSH, filepath.Join(t.TempDir(), "missing.git"))
	gittest.Redirect(t, appHTTPS, filepath.Join(t.TempDir(), "missing.git"))
	before := read(t, teamFile(home))
	run(t, "-C", home, "repo", "add", appHTTPS).expect(t, exitBad, "error [env]")
	if read(t, teamFile(home)) != before {
		t.Error("team file changed")
	}
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("personal file written")
	}
}

func TestRepoAddRejectsBadOrConflictingEntries(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	before := read(t, teamFile(home))
	cases := []struct {
		args []string
		want string
	}{
		{[]string{appHTTPS, "--branch", "main"}, "--name"},
		{[]string{"https://git.example.invalid/acme/APP.git", "--branch", "main"}, "--name"},
		{[]string{appHTTPS, "--name", "other", "--path", "products/APP", "--branch", "main"}, "--path"},
		{[]string{appHTTPS, "--name", "other", "--path", "products/app/inner", "--branch", "main"}, "--path"},
		{[]string{appHTTPS, "--name", "-x", "--path", "products/x", "--branch", "main"}, "--name"},
		{[]string{appHTTPS, "--name", "x", "--path", "elsewhere/x", "--branch", "main"}, "--path"},
		{[]string{appHTTPS, "--name", "x", "--branch", "a..b"}, "--branch"},
		{[]string{appHTTPS, "--name", "", "--branch", "main"}, "--name"},
		{[]string{"/srv/git/app.git", "--branch", "main"}, "url"},
		{[]string{"https://git.example.invalid:8443/acme/x.git", "--branch", "main"}, "url"},
		{[]string{"ext::sh -c touch% /tmp/x", "--branch", "main"}, "url"},
		{[]string{appHTTPS, "--bogus"}, "error [usage]"},
		{[]string{}, "error [usage]"},
		{[]string{appHTTPS, "extra", "--branch", "main"}, "error [usage]"},
	}
	for _, c := range cases {
		r := run(t, append([]string{"-C", home, "repo", "add"}, c.args...)...)
		if r.code != exitBad || !strings.Contains(r.stderr, "error [usage]") || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%v: exit %d stderr %q, want usage error mentioning %q", c.args, r.code, r.stderr, c.want)
		}
		if read(t, teamFile(home)) != before {
			t.Fatalf("%v changed the team file", c.args)
		}
	}
}

func TestRepoAddSeparatorKeepsDashArgumentPositional(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "repo", "add", "--branch", "main", "--", "-https://git.example.invalid/acme/x.git")
	r.expect(t, exitBad, "url")
}

func TestRepoRemoveKeepsCheckout(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	checkout := filepath.Join(home, "products", "app")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(checkout, "work.txt"), "unsaved")
	r := run(t, "-C", home, "repo", "remove", "app")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "products/app") || !strings.Contains(r.stdout, brand.TeamFileChangedHint) {
		t.Errorf("stdout: %s", r.stdout)
	}
	if read(t, filepath.Join(checkout, "work.txt")) != "unsaved" {
		t.Error("checkout changed")
	}
	if strings.Contains(read(t, teamFile(home)), "[[repos]]") {
		t.Errorf("entry still listed:\n%s", read(t, teamFile(home)))
	}
	run(t, "-C", home, "repo", "remove", "app").expect(t, exitBad, "unknown repo name(s): app; available: (none)")
}

// listHome builds a home repo with one repo in each checkout state.
func listHome(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	remote := gittest.BareRemote(t, "main")
	names := []string{"ok", "mismatch", "conflict", "missing"}
	for _, name := range names {
		url := "git@git.example.invalid:acme/" + name + ".git"
		run(t, "-C", home, "repo", "add", url, "--branch", "main").expect(t, exitOK, "")
	}
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	products := filepath.Join(home, "products")
	gittest.Git(t, "", "clone", "--quiet", remote, filepath.Join(products, "ok"))
	gittest.Git(t, filepath.Join(products, "ok"), "remote", "set-url", "origin", "https://git.example.invalid/acme/ok.git")
	gittest.Git(t, "", "clone", "--quiet", remote, filepath.Join(products, "mismatch"))
	gittest.Git(t, filepath.Join(products, "mismatch"), "remote", "set-url", "origin", "git@git.example.invalid:acme/mismatch.git")
	if err := os.MkdirAll(filepath.Join(products, "conflict"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func statusOf(t *testing.T, stdout string, name string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == name {
			return fields[3]
		}
	}
	t.Fatalf("no row for %s in:\n%s", name, stdout)
	return ""
}

func TestRepoListReportsFourStatesWithoutWriting(t *testing.T) {
	home := listHome(t)
	team := read(t, teamFile(home))
	local := read(t, localFile(home))
	r := run(t, "-C", home, "repo", "list")
	r.expect(t, exitOK, "")
	want := map[string]string{"ok": "ok", "mismatch": "remote-mismatch", "conflict": "path-conflict", "missing": "missing"}
	for name, state := range want {
		if got := statusOf(t, r.stdout, name); got != state {
			t.Errorf("%s: %s, want %s", name, got, state)
		}
	}
	if !strings.Contains(r.stdout, "products/ok") || !strings.Contains(r.stdout, "main") {
		t.Errorf("rows lack path or branch:\n%s", r.stdout)
	}
	if read(t, teamFile(home)) != team || read(t, localFile(home)) != local {
		t.Error("repo list wrote a config file")
	}
	if _, err := os.Stat(filepath.Join(home, "products", "missing")); err == nil {
		t.Error("repo list created a checkout")
	}
}

func TestRepoListWithoutProtocolComparesTeamURL(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appSSH, "--branch", "main").expect(t, exitOK, "")
	target := filepath.Join(home, "products", "app")
	gittest.Git(t, "", "clone", "--quiet", gittest.BareRemote(t, "main"), target)
	gittest.Git(t, target, "remote", "set-url", "origin", appSSH)
	r := run(t, "-C", home, "repo", "list")
	r.expect(t, exitOK, "")
	if got := statusOf(t, r.stdout, "app"); got != "ok" {
		t.Errorf("status %s, want ok", got)
	}
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("repo list wrote the personal file")
	}
}

func TestRepoListEmpty(t *testing.T) {
	r := run(t, "-C", newHome(t), "repo", "list")
	if r.code != exitOK || !strings.Contains(r.stdout, "No repos configured in "+brand.TeamFile) {
		t.Errorf("%+v", r)
	}
}

func TestInheritedGitDirDoesNotLeak(t *testing.T) {
	home := listHome(t)
	other := t.TempDir()
	gittest.Git(t, other, "init", "--quiet")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	r := run(t, "-C", home, "repo", "list")
	r.expect(t, exitOK, "")
	if got := statusOf(t, r.stdout, "ok"); got != "ok" {
		t.Errorf("ok repo shows %s with an inherited GIT_DIR", got)
	}
}

func TestConfigProtocol(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "config", "protocol")
	if r.code != exitOK || !strings.Contains(r.stdout, "not set") {
		t.Errorf("unset: %+v", r)
	}
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	if got := read(t, localFile(home)); got != "schema_version = 1\nprotocol = \"ssh\"\n" {
		t.Errorf("personal file %q", got)
	}
	r = run(t, "-C", home, "config", "protocol")
	if r.code != exitOK || !strings.Contains(r.stdout, "ssh") {
		t.Errorf("set: %+v", r)
	}
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "git").expect(t, exitBad, "error [usage]")
	if !strings.Contains(read(t, localFile(home)), "https") {
		t.Error("invalid value changed the personal file")
	}
	run(t, "-C", home, "config").expect(t, exitBad, "error [usage]")
}

func TestConfigErrorsNameFileAndLocation(t *testing.T) {
	home := newHome(t)
	write(t, teamFile(home), read(t, teamFile(home))+"\n[[repos]]\nname = \"app\"\nurl = \""+appHTTPS+"\"\npath = \"app\"\nbranch = \"main\"\n")
	run(t, "-C", home, "repo", "list").expect(t, exitBad, "error [config] "+brand.TeamFile+": repos[0].path: ")
}

func TestVersionGateBlocksCommandsAndWrites(t *testing.T) {
	home := newHome(t)
	text := strings.Replace(read(t, teamFile(home)), "schema_version = 1", "schema_version = 2", 1)
	write(t, teamFile(home), text+"future = true\n")
	for _, args := range [][]string{{"repo", "list"}, {"repo", "add", appHTTPS, "--branch", "main"}, {"repo", "remove", "app"}, {"config", "protocol", "ssh"}, {"config", "protocol"}} {
		run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, "error [version]")
	}
	if read(t, teamFile(home)) != text+"future = true\n" {
		t.Error("team file changed")
	}
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("personal file written past the version gate")
	}
	for _, args := range [][]string{{"version"}, {"help"}, {"init", filepath.Join(t.TempDir(), "new")}} {
		run(t, append([]string{"-C", home}, args...)...).expect(t, exitOK, "")
	}
}

func TestVersionGateOnPersonalFile(t *testing.T) {
	home := newHome(t)
	write(t, localFile(home), "schema_version = 2\nprotocol = \"ssh\"\n")
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitBad, "error [version] "+brand.LocalFile)
	if !strings.Contains(read(t, localFile(home)), "schema_version = 2") {
		t.Error("personal file rewritten")
	}
}

func TestDevBuildWarnsAboutSkippedMinVersion(t *testing.T) {
	r := run(t, "-C", newHome(t), "repo", "list")
	r.expect(t, exitOK, "warning: dev build")
}

func TestRootIsFoundFromInsideProducts(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	nested := filepath.Join(home, "products", "app", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	r := run(t, "-C", nested, "repo", "list")
	if r.code != exitOK || statusOf(t, r.stdout, "app") != "path-conflict" {
		t.Errorf("-C nested: %+v", r)
	}
	t.Chdir(nested)
	r = run(t, "repo", "list")
	if r.code != exitOK || statusOf(t, r.stdout, "app") != "path-conflict" {
		t.Errorf("cwd nested: %+v", r)
	}
}

func TestRelativeDirectoryOption(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	t.Chdir(filepath.Dir(home))
	r := run(t, "-C", filepath.Base(home), "repo", "list")
	if r.code != exitOK || statusOf(t, r.stdout, "app") != "missing" {
		t.Errorf("relative -C: %+v", r)
	}
}

func TestOutsideHomeRepoIsConfigError(t *testing.T) {
	run(t, "-C", t.TempDir(), "repo", "list").expect(t, exitBad, "error [config] "+brand.TeamFile+" not found")
	run(t, "-C", filepath.Join(t.TempDir(), "missing"), "repo", "list").expect(t, exitBad, "error [usage]")
}

func TestInterruptedRunExits130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := cli.Run(ctx, []string{"-C", newHome(t), "repo", "list"}, &stdout, &stderr, false); code != 130 {
		t.Errorf("exit %d, want 130; stderr %s", code, stderr.String())
	}
}

package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/cli"
	"github.com/CTHua/hearth/internal/gittest"
)

func httpsURL(name string) string {
	return "https://git.example.invalid/acme/" + name + ".git"
}

func sshURL(name string) string {
	return "git@git.example.invalid:acme/" + name + ".git"
}

// pushBranch adds a commit on a new branch of remote and returns its sha.
func pushBranch(t *testing.T, remote string, branch string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	gittest.Git(t, "", "clone", "--quiet", remote, work)
	gittest.Git(t, work, "checkout", "--quiet", "-b", branch)
	gittest.Git(t, work, "commit", "--quiet", "--allow-empty", "-m", branch)
	gittest.Git(t, work, "push", "--quiet", "origin", "refs/heads/"+branch)
	return strings.TrimSpace(gittest.Git(t, work, "rev-parse", "HEAD"))
}

// advanceBranch adds a commit on an existing branch of remote and returns its sha.
func advanceBranch(t *testing.T, remote string, branch string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	gittest.Git(t, "", "clone", "--quiet", "--branch", branch, remote, work)
	gittest.Git(t, work, "commit", "--quiet", "--allow-empty", "-m", "later")
	gittest.Git(t, work, "push", "--quiet", "origin", "refs/heads/"+branch)
	return strings.TrimSpace(gittest.Git(t, work, "rev-parse", "HEAD"))
}

func pushTag(t *testing.T, remote string, tag string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	gittest.Git(t, "", "clone", "--quiet", remote, work)
	gittest.Git(t, work, "tag", tag)
	gittest.Git(t, work, "push", "--quiet", "origin", "refs/tags/"+tag)
	return strings.TrimSpace(gittest.Git(t, work, "rev-parse", "HEAD"))
}

type entry struct {
	name   string
	branch string
	path   string
	remote string // local path the https URL is redirected to
}

// homeWith creates a home repo listing entries with https URLs, each
// redirected to its remote, and protocol set to https.
func homeWith(t *testing.T, entries ...entry) string {
	t.Helper()
	home := newHome(t)
	for _, e := range entries {
		gittest.Redirect(t, httpsURL(e.name), e.remote)
		args := []string{"-C", home, "repo", "add", httpsURL(e.name), "--branch", e.branch}
		if e.path != "" {
			args = append(args, "--path", e.path)
		}
		run(t, args...).expect(t, exitOK, "")
	}
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	return home
}

func simple(t *testing.T, names ...string) []entry {
	t.Helper()
	entries := make([]entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, entry{name: name, branch: "main", remote: gittest.BareRemote(t, "main")})
	}
	return entries
}

func product(home string, name string) string {
	return filepath.Join(home, "products", name)
}

func headRef(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gittest.Git(t, dir, "symbolic-ref", "HEAD"))
}

func origin(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gittest.Git(t, dir, "config", "--local", "--get", "remote.origin.url"))
}

func assertNoTempDirs(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), brand.CloneTempPrefix) {
			t.Errorf("temporary directory left behind: %s", e.Name())
		}
	}
}

func lines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestCloneUsesConfiguredBranchNotTagAndSetsOrigin(t *testing.T) {
	alpha := gittest.BareRemote(t, "main")
	pushBranch(t, alpha, "develop")
	bravo := gittest.BareRemote(t, "main")
	tagCommit := pushTag(t, bravo, "release")
	branchCommit := pushBranch(t, bravo, "release")
	if tagCommit == branchCommit {
		t.Fatal("tag and branch must differ for this test")
	}
	home := homeWith(t, entry{name: "alpha", branch: "develop", remote: alpha}, entry{name: "bravo", branch: "release", remote: bravo})
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	out := lines(r.stdout)
	for _, want := range []string{"cloned alpha -> products/alpha (develop)", "cloned bravo -> products/bravo (release)"} {
		if !contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
	if got := headRef(t, product(home, "alpha")); got != "refs/heads/develop" {
		t.Errorf("alpha HEAD %s", got)
	}
	if got := strings.TrimSpace(gittest.Git(t, product(home, "bravo"), "rev-parse", "HEAD")); got != branchCommit {
		t.Errorf("bravo is at %s, want the branch commit %s", got, branchCommit)
	}
	if got := origin(t, product(home, "alpha")); got != httpsURL("alpha") {
		t.Errorf("origin %s", got)
	}
	assertNoTempDirs(t, filepath.Join(home, "products"))
}

func TestRerunSkipsAndLeavesCheckoutUntouched(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	dir := product(home, "app")
	gittest.Git(t, dir, "checkout", "--quiet", "-b", "local-work")
	write(t, filepath.Join(dir, "untracked.txt"), "mine")
	before := gittest.Git(t, dir, "status", "--porcelain", "--branch") + gittest.Git(t, dir, "rev-parse", "HEAD")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if !contains(lines(r.stdout), "skipped app: already cloned at products/app") {
		t.Errorf("stdout: %s", r.stdout)
	}
	after := gittest.Git(t, dir, "status", "--porcelain", "--branch") + gittest.Git(t, dir, "rev-parse", "HEAD")
	if after != before || read(t, filepath.Join(dir, "untracked.txt")) != "mine" {
		t.Errorf("checkout changed:\n%s\n%s", before, after)
	}
}

func TestCloneSelectsNamesInTeamFileOrder(t *testing.T) {
	home := homeWith(t, simple(t, "a", "b", "c")...)
	r := run(t, "-C", home, "clone", "c", "a", "c")
	r.expect(t, exitOK, "")
	var cloned []string
	for _, line := range lines(r.stdout) {
		if strings.HasPrefix(line, "cloned ") {
			cloned = append(cloned, line)
		}
	}
	want := []string{"cloned a -> products/a (main)", "cloned c -> products/c (main)"}
	if strings.Join(cloned, "\n") != strings.Join(want, "\n") {
		t.Errorf("cloned %v, want %v", cloned, want)
	}
	if exists(product(home, "b")) {
		t.Error("b was cloned")
	}
	run(t, "-C", home, "clone", "b").expect(t, exitOK, "")
	if !exists(product(home, "b")) {
		t.Error("single name not cloned")
	}
}

func TestCloneUnknownNameIsUsageErrorWithoutCloning(t *testing.T) {
	home := homeWith(t, simple(t, "a")...)
	r := run(t, "-C", home, "clone", "a", "nope")
	r.expect(t, exitBad, "error [usage]")
	if !strings.Contains(r.stderr, "nope") {
		t.Errorf("stderr: %s", r.stderr)
	}
	if exists(filepath.Join(home, "products")) {
		t.Error("products/ created")
	}
}

func TestExistingTargetsThatAreNotCheckouts(t *testing.T) {
	home := homeWith(t, simple(t, "plain", "file", "nested")...)
	products := filepath.Join(home, "products")
	if err := os.MkdirAll(product(home, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(product(home, "plain"), "notes.txt"), "mine")
	write(t, product(home, "file"), "a file")
	nested := product(home, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [path-conflict] plain: products/plain is not a checkout root; git resolves it to")
	r.expect(t, exitFailed, "error [path-conflict] file: products/file is not a directory")
	if read(t, filepath.Join(product(home, "plain"), "notes.txt")) != "mine" || read(t, product(home, "file")) != "a file" {
		t.Error("existing content changed")
	}
	if strings.Contains(r.stdout, "skipped plain") {
		t.Errorf("stdout: %s", r.stdout)
	}
	assertNoTempDirs(t, products)
}

func TestNonGitDirectoryOutsideAnyRepoIsPathConflict(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	// Without the home repo's .git, the directory is not inside any repo.
	if err := os.RemoveAll(filepath.Join(home, ".git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(home))
	if err := os.MkdirAll(product(home, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", home, "clone").expect(t, exitFailed, "is not a git checkout")
}

func TestOriginMismatchIsReportedAndNotChanged(t *testing.T) {
	home := homeWith(t, simple(t, "app", "bare")...)
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	gittest.Git(t, product(home, "app"), "remote", "set-url", "origin", "https://git.example.invalid/acme/other.git")
	gittest.Git(t, product(home, "bare"), "remote", "remove", "origin")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [remote-mismatch] app:")
	r.expect(t, exitFailed, "error [remote-mismatch] bare:")
	if !strings.Contains(r.stderr, httpsURL("app")) || !strings.Contains(r.stderr, "acme/other.git") {
		t.Errorf("stderr lacks both URLs: %s", r.stderr)
	}
	if got := origin(t, product(home, "app")); got != "https://git.example.invalid/acme/other.git" {
		t.Errorf("origin changed to %s", got)
	}
	if got := gittest.Git(t, product(home, "bare"), "remote"); got != "" {
		t.Errorf("remote added: %q", got)
	}
}

func TestCloneConfigErrorsCloneNothing(t *testing.T) {
	home := newHome(t)
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	write(t, teamFile(home), read(t, teamFile(home))+`
[[repos]]
name = "alpha"
url = "`+httpsURL("alpha")+`"
path = "other/alpha"
branch = "main"

[[repos]]
name = "bravo"
url = "`+httpsURL("bravo")+`"
path = "products/bravo"
branch = "--upload-pack=touch `+sentinel+`"
`)
	r := run(t, "-C", home, "clone")
	r.expect(t, exitBad, "error [config]")
	var configLines []string
	for _, line := range lines(r.stderr) {
		if strings.HasPrefix(line, "error [config]") {
			configLines = append(configLines, line)
		}
	}
	if len(configLines) != 2 || !strings.Contains(configLines[0], "repos[0]") || !strings.Contains(configLines[1], "repos[1]") {
		t.Errorf("want one error per entry, got:\n%s", r.stderr)
	}
	if exists(filepath.Join(home, "products")) || exists(sentinel) {
		t.Error("something ran or was created")
	}
	for _, bad := range []string{"\n[[repos]]\nname = \"a\"\n", "\nrepos = 1\n", "\nnot toml ["} {
		write(t, teamFile(home), strings.SplitN(read(t, teamFile(home)), "\n[[repos]]", 2)[0]+bad)
		run(t, "-C", home, "clone").expect(t, exitBad, "error [config]")
	}
	write(t, teamFile(home), "\xff\xfe")
	run(t, "-C", home, "clone").expect(t, exitBad, "error [config]")
	if exists(filepath.Join(home, "products")) {
		t.Error("products/ created")
	}
}

func TestCloneWithMissingTeamFileIsConfigError(t *testing.T) {
	home := newHome(t)
	if err := os.Remove(teamFile(home)); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", home, "clone").expect(t, exitBad, "error [config]")
}

func TestCloneEmptyListPrintsMessageOnly(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "clone")
	if r.code != exitOK || r.stdout != "No repos configured in "+brand.TeamFile+"\n" {
		t.Errorf("%+v", r)
	}
	if strings.Contains(r.stderr, "error") || exists(filepath.Join(home, "products")) || exists(localFile(home)) {
		t.Errorf("empty list did more than print: %+v", r)
	}
}

func TestSymlinksOnThePathAreRejected(t *testing.T) {
	outside := t.TempDir()
	external := filepath.Join(outside, "checkout")
	remote := gittest.BareRemote(t, "main")
	gittest.Git(t, "", "clone", "--quiet", remote, external)
	home := homeWith(t,
		entry{name: "target", branch: "main", remote: remote},
		entry{name: "grouped", branch: "main", path: "products/group/grouped", remote: remote},
	)
	products := filepath.Join(home, "products")
	if err := os.MkdirAll(products, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, product(home, "target")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, product(home, "group")); err != nil {
		t.Fatal(err)
	}
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [path-conflict] target: products/target is a symlink")
	r.expect(t, exitFailed, "error [path-conflict] grouped: products/group is a symlink")
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Errorf("something was written outside: %v", entries)
	}
}

func TestProductsSymlinkIsRejected(t *testing.T) {
	for _, broken := range []bool{false, true} {
		home := homeWith(t, simple(t, "app")...)
		dest := t.TempDir()
		if broken {
			dest = filepath.Join(dest, "missing-dir")
		}
		if err := os.Symlink(dest, filepath.Join(home, "products")); err != nil {
			t.Fatal(err)
		}
		run(t, "-C", home, "clone").expect(t, exitFailed, "products is a symlink")
		if broken && exists(dest) {
			t.Error("broken symlink target was created")
		}
	}
}

func TestCloneFailuresLeaveNoTargetOrTempDir(t *testing.T) {
	tagged := gittest.BareRemote(t, "main")
	pushTag(t, tagged, "v1")
	home := homeWith(t,
		entry{name: "missing", branch: "main", remote: filepath.Join(t.TempDir(), "does-not-exist")},
		entry{name: "nobranch", branch: "nope", remote: gittest.BareRemote(t, "main")},
		entry{name: "tagged", branch: "v1", remote: tagged},
	)
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [clone-failed] missing: git clone exited with")
	r.expect(t, exitFailed, "error [clone-failed] nobranch:")
	r.expect(t, exitFailed, "error [clone-failed] tagged: clone of "+httpsURL("tagged")+" is not on branch v1")
	entries, err := os.ReadDir(filepath.Join(home, "products"))
	if err != nil || len(entries) != 0 {
		t.Errorf("products/ has %v, %v", entries, err)
	}
}

func TestRerunAfterFixingRemoteClones(t *testing.T) {
	late := filepath.Join(t.TempDir(), "late.git")
	home := homeWith(t, entry{name: "late", branch: "main", remote: late})
	run(t, "-C", home, "clone").expect(t, exitFailed, "error [clone-failed] late:")
	if err := os.Rename(gittest.BareRemote(t, "main"), late); err != nil {
		t.Fatal(err)
	}
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if !contains(lines(r.stdout), "cloned late -> products/late (main)") {
		t.Errorf("stdout: %s", r.stdout)
	}
}

func TestFileAsParentFailsOnlyThatRepo(t *testing.T) {
	home := homeWith(t,
		entry{name: "app", branch: "main", path: "products/group/app", remote: gittest.BareRemote(t, "main")},
		entry{name: "other", branch: "main", remote: gittest.BareRemote(t, "main")},
	)
	if err := os.MkdirAll(filepath.Join(home, "products"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, product(home, "group"), "keep")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [path-conflict] app: products/group is not a directory")
	if !contains(lines(r.stdout), "cloned other -> products/other (main)") || read(t, product(home, "group")) != "keep" {
		t.Errorf("stdout: %s", r.stdout)
	}
}

func TestMixedResultsAndOutputFormat(t *testing.T) {
	home := homeWith(t,
		entry{name: "good", branch: "main", remote: gittest.BareRemote(t, "main")},
		entry{name: "bad", branch: "main", remote: filepath.Join(t.TempDir(), "missing")},
		entry{name: "old", branch: "main", remote: gittest.BareRemote(t, "main")},
	)
	run(t, "-C", home, "clone", "old").expect(t, exitOK, "")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [clone-failed] bad:")
	want := []string{
		"cloned good -> products/good (main)",
		"skipped old: already cloned at products/old",
		"Summary: 1 cloned, 1 skipped, 1 failed",
		"  cloned: good",
		"  skipped: old",
		"  failed: bad (clone-failed)",
	}
	if got := lines(r.stdout); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, strings.Join(want, "\n"))
	}
	if strings.Count(r.stderr, "error [clone-failed] bad:") != 1 {
		t.Errorf("stderr: %s", r.stderr)
	}
}

func TestCloneFromOtherDirectories(t *testing.T) {
	home := homeWith(t, simple(t, "root", "nested")...)
	t.Chdir("/")
	run(t, "-C", home, "clone", "root").expect(t, exitOK, "")
	if err := os.MkdirAll(filepath.Join(product(home, "root"), "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(product(home, "root"), "src"))
	run(t, "clone", "nested").expect(t, exitOK, "")
	if headRef(t, product(home, "nested")) != "refs/heads/main" || exists(filepath.Join(product(home, "root"), "src", "products")) {
		t.Error("clone from inside products/ did not use the home repo root")
	}
}

func TestHomeRepoGitIgnoresProducts(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	status := gittest.Git(t, home, "status", "--porcelain", "--untracked-files=all")
	if strings.Contains(status, "products") || strings.Contains(status, brand.LocalFile) {
		t.Errorf("status shows ignored files:\n%s", status)
	}
	if !strings.Contains(status, brand.TeamFile) {
		t.Errorf("team file not trackable:\n%s", status)
	}
}

func TestCloneInheritedGitDirDoesNotLeak(t *testing.T) {
	other := t.TempDir()
	gittest.Git(t, other, "init", "--quiet")
	gittest.Git(t, other, "commit", "--quiet", "--allow-empty", "-m", "x")
	before := gittest.Git(t, other, "rev-parse", "HEAD") + gittest.Git(t, other, "branch", "--list")
	home := homeWith(t, simple(t, "app")...)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	first := run(t, "-C", home, "clone")
	first.expect(t, exitOK, "")
	second := run(t, "-C", home, "clone")
	second.expect(t, exitOK, "")
	if !contains(lines(second.stdout), "skipped app: already cloned at products/app") {
		t.Errorf("second run: %s", second.stdout)
	}
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	if after := gittest.Git(t, other, "rev-parse", "HEAD") + gittest.Git(t, other, "branch", "--list"); after != before {
		t.Errorf("other repo changed: %s -> %s", before, after)
	}
}

func TestCloneExtTransportIsBlocked(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	home := homeWith(t, entry{name: "ext", branch: "main", remote: "ext::sh -c touch% " + sentinel})
	gittest.Git(t, "", "config", "--global", "protocol.ext.allow", "always")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "protocol.ext.allow") })
	run(t, "-C", home, "clone").expect(t, exitFailed, "error [clone-failed] ext:")
	if exists(sentinel) {
		t.Fatal("ext:: command ran")
	}
}

func TestCloneWithoutGitIsEnvError(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	t.Setenv("PATH", t.TempDir())
	run(t, "-C", home, "clone").expect(t, exitBad, "error [env]")
	if exists(filepath.Join(home, "products")) {
		t.Error("products/ created")
	}
}

func TestCloneConvertsURLToPersonalProtocol(t *testing.T) {
	cases := []struct {
		listed, protocol, want string
	}{
		{sshURL("app"), "https", httpsURL("app")},
		{httpsURL("app"), "ssh", sshURL("app")},
		{"ssh://git@git.example.invalid/acme/app.git", "https", httpsURL("app")},
		{"ssh://git@git.example.invalid/acme/app.git", "ssh", sshURL("app")},
	}
	for _, c := range cases {
		home := newHome(t)
		remote := gittest.BareRemote(t, "main")
		gittest.Redirect(t, c.want, remote)
		run(t, "-C", home, "repo", "add", c.listed, "--branch", "main").expect(t, exitOK, "")
		run(t, "-C", home, "config", "protocol", c.protocol).expect(t, exitOK, "")
		run(t, "-C", home, "clone").expect(t, exitOK, "")
		if got := origin(t, product(home, "app")); got != c.want {
			t.Errorf("%s with %s: origin %s, want %s", c.listed, c.protocol, got, c.want)
		}
		if !strings.Contains(read(t, teamFile(home)), c.listed) {
			t.Error("team file was rewritten with the converted URL")
		}
	}
}

func TestProtocolChangeReportsMismatchAndLeavesCheckout(t *testing.T) {
	home := newHome(t)
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, sshURL("app"), remote)
	gittest.Redirect(t, httpsURL("app"), remote)
	run(t, "-C", home, "repo", "add", sshURL("app"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	tracking := gittest.Git(t, product(home, "app"), "rev-parse", "refs/remotes/origin/main")
	advanced := advanceBranch(t, remote, "main")
	pushBranch(t, remote, "later")
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitFailed, "error [remote-mismatch] app:")
	if !strings.Contains(r.stderr, httpsURL("app")) || !strings.Contains(r.stderr, "protocol = https") {
		t.Errorf("stderr: %s", r.stderr)
	}
	if origin(t, product(home, "app")) != sshURL("app") {
		t.Error("origin was changed")
	}
	after := gittest.Git(t, product(home, "app"), "rev-parse", "refs/remotes/origin/main")
	if after != tracking || strings.TrimSpace(after) == advanced {
		t.Errorf("origin/main moved from %s to %s: the checkout was fetched", tracking, after)
	}
	if refs := gittest.Git(t, product(home, "app"), "for-each-ref", "refs/remotes/origin/later"); refs != "" {
		t.Errorf("origin/later appeared: the checkout was fetched: %s", refs)
	}
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	r = run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if !contains(lines(r.stdout), "skipped app: already cloned at products/app") {
		t.Errorf("stdout: %s", r.stdout)
	}
}

func TestCloneDetectsProtocolWhenUnset(t *testing.T) {
	home := newHome(t)
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, sshURL("app"), filepath.Join(t.TempDir(), "no-ssh"))
	gittest.Redirect(t, httpsURL("app"), remote)
	run(t, "-C", home, "repo", "add", sshURL("app"), "--branch", "main").expect(t, exitOK, "")
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "detected https") || !strings.Contains(r.stdout, "config protocol") {
		t.Errorf("stdout lacks the result and how to change it: %s", r.stdout)
	}
	if got := read(t, localFile(home)); got != "schema_version = 1\nprotocol = \"https\"\n" {
		t.Errorf("personal file %q", got)
	}
	if got := origin(t, product(home, "app")); got != httpsURL("app") {
		t.Errorf("origin %s", got)
	}
}

func TestCloneDetectionFailureClonesAndWritesNothing(t *testing.T) {
	home := newHome(t)
	gittest.Redirect(t, sshURL("app"), filepath.Join(t.TempDir(), "no-ssh"))
	gittest.Redirect(t, httpsURL("app"), filepath.Join(t.TempDir(), "no-https"))
	run(t, "-C", home, "repo", "add", sshURL("app"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "clone").expect(t, exitBad, "error [env]")
	if exists(filepath.Join(home, "products")) || exists(localFile(home)) {
		t.Error("detection failure cloned or wrote something")
	}
}

func TestCloneWithProtocolSetDoesNotDetect(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if strings.Contains(r.stdout, "detected") {
		t.Errorf("detected although protocol was set: %s", r.stdout)
	}
}

func TestInvalidPersonalFileFailsEvenWithEmptyList(t *testing.T) {
	home := newHome(t)
	for _, personal := range []string{"[github\n", "schema_version = 1\nprotocol = \"git\"\n", "schema_version = 1\ntoken = \"x\"\n", "schema_version = 1\nprotocol = 1\n"} {
		write(t, localFile(home), personal)
		r := run(t, "-C", home, "clone")
		r.expect(t, exitBad, "error [config] "+brand.LocalFile)
	}
}

func TestInterruptedCloneRemovesTempDirAndExits130(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", sshURL("app"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	// A fake ssh that never answers keeps git clone running until cancelled.
	bin := t.TempDir()
	started := filepath.Join(bin, "started")
	write(t, filepath.Join(bin, "ssh"), "#!/bin/sh\ntouch "+started+"\nexec sleep 30\n")
	if err := os.Chmod(filepath.Join(bin, "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", filepath.Join(bin, "ssh"))
	gittest.Git(t, "", "config", "--global", "protocol.ssh.allow", "always")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "protocol.ssh.allow") })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for !exists(started) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Run(ctx, []string{"-C", home, "clone"}, &stdout, &stderr, false)
	if code != 130 || !strings.Contains(stderr.String(), "interrupted") {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	if !exists(started) {
		t.Fatal("clone never reached the remote; test is invalid")
	}
	assertNoTempDirs(t, filepath.Join(home, "products"))
	if exists(product(home, "app")) {
		t.Error("target exists after interrupt")
	}
}

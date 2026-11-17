package cli_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gittest"
)

const (
	skillHTTPS  = "https://git.example.invalid/acme/skills.git"
	skillSSH    = "git@git.example.invalid:acme/skills.git"
	demoSubdir  = "skills/demo"
	skillV1     = "---\nname: demo\ndescription: v1\n---\n"
	skillV2     = "---\nname: demo\ndescription: v2\n---\n"
	vendorDemo  = "vendor/skills/demo"
	claudeDemo  = ".claude/skills/demo"
	agentsDemo  = ".agents/skills/demo"
	bothAgents  = "claude,codex"
	builtinFile = brand.StateDir + "/skills/" + brand.BuiltinSkill + "/SKILL.md"
)

// skillRemote is a bare remote whose main branch has two commits that both
// hold skills/demo/SKILL.md; HEAD is the newer one.
type skillRemote struct {
	bare   string
	work   string
	pinned string
	newer  string
}

func newSkillRemote(t *testing.T) skillRemote {
	t.Helper()
	base := t.TempDir()
	r := skillRemote{bare: filepath.Join(base, "skills.git"), work: filepath.Join(base, "work")}
	gittest.Git(t, base, "init", "--quiet", "--initial-branch", "main", r.work)
	r.pinned = r.commit(t, demoSubdir+"/SKILL.md", skillV1)
	r.newer = r.commit(t, demoSubdir+"/SKILL.md", skillV2)
	gittest.Git(t, base, "clone", "--quiet", "--bare", r.work, r.bare)
	return r
}

// commit writes path in the work tree, commits it and returns the sha. It
// does not push.
func (r skillRemote) commit(t *testing.T, path string, text string) string {
	t.Helper()
	full := filepath.Join(r.work, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, full, text)
	gittest.Git(t, r.work, "add", "-A")
	gittest.Git(t, r.work, "commit", "--quiet", "-m", path)
	return strings.TrimSpace(gittest.Git(t, r.work, "rev-parse", "HEAD"))
}

func (r skillRemote) push(t *testing.T) {
	t.Helper()
	gittest.Git(t, r.work, "push", "--quiet", r.bare, "main")
}

// skillHome has demo pinned at commit, protocol https, agents chosen (none
// when empty) and demo enabled.
func skillHome(t *testing.T, r skillRemote, commit string, agents string) string {
	t.Helper()
	home := newHome(t)
	gittest.Redirect(t, skillHTTPS, r.bare)
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "add", skillHTTPS, "--name", "demo", "--subdir", demoSubdir, "--commit", commit).expect(t, exitOK, "")
	if agents != "" {
		run(t, "-C", home, "config", "agents", agents).expect(t, exitOK, "")
	}
	run(t, "-C", home, "skill", "enable", "demo").expect(t, exitOK, "")
	return home
}

func at(home string, rel string) string {
	return filepath.Join(home, filepath.FromSlash(rel))
}

func head(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD"))
}

func isDetached(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "symbolic-ref", "--quiet", "HEAD")
	return cmd.Run() != nil
}

// assertLinked checks that link is a relative symlink resolving to the demo
// skill with text.
func assertLinked(t *testing.T, home string, link string, text string) {
	t.Helper()
	dest, err := os.Readlink(at(home, link))
	if err != nil {
		t.Fatalf("%s: %v", link, err)
	}
	if filepath.IsAbs(dest) {
		t.Errorf("%s points to absolute %s", link, dest)
	}
	if want := "../../" + vendorDemo + "/" + demoSubdir; dest != want {
		t.Errorf("%s -> %s, want %s", link, dest, want)
	}
	if got := read(t, at(home, link+"/SKILL.md")); got != text {
		t.Errorf("%s/SKILL.md = %q, want %q", link, got, text)
	}
}

// tree records every path below dir: file contents, symlink targets and
// directories, so a comparison catches any write.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			dest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files[rel] = "-> " + dest
		case entry.IsDir():
			files[rel] = "dir " + info.Mode().String()
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[rel] = info.Mode().String() + " " + string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertSameTree(t *testing.T, before map[string]string, after map[string]string) {
	t.Helper()
	for path, value := range before {
		if after[path] != value {
			t.Errorf("%s changed: %q -> %q", path, value, after[path])
		}
	}
	for path := range after {
		if _, found := before[path]; !found {
			t.Errorf("%s was created", path)
		}
	}
}

// listStatus returns the STATUS column of the skill list row for name.
func listStatus(t *testing.T, home string, name string) string {
	t.Helper()
	r := run(t, "-C", home, "skill", "list")
	if r.code != exitOK {
		t.Fatalf("skill list exit %d: %s", r.code, r.stderr)
	}
	for _, line := range lines(r.stdout) {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == name {
			return fields[3]
		}
	}
	t.Fatalf("no row for %s:\n%s", name, r.stdout)
	return ""
}

func TestSkillAddWithoutCommitPinsRemoteHead(t *testing.T) {
	r := newSkillRemote(t)
	home := newHome(t)
	gittest.Redirect(t, skillHTTPS, r.bare)
	run(t, "-C", home, "repo", "add", appHTTPS, "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "add", skillHTTPS, "--subdir", demoSubdir)
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, brand.TeamFileChangedHint) {
		t.Errorf("no PR hint: %s", res.stdout)
	}
	want := "\n\n[[skills]]\nname = \"demo\"\nurl = \"" + skillHTTPS + "\"\ncommit = \"" + r.newer + "\"\nsubdir = \"" + demoSubdir + "\"\n"
	text := read(t, teamFile(home))
	if !strings.HasSuffix(text, want) || strings.Index(text, "[[repos]]") > strings.Index(text, "[[skills]]") {
		t.Errorf("team file:\n%s", text)
	}
	run(t, "-C", home, "skill", "add", skillHTTPS).expect(t, exitOK, "")
	if !strings.Contains(read(t, teamFile(home)), "name = \"skills\"\nurl = \""+skillHTTPS+"\"\ncommit = \""+r.newer+"\"\nsubdir = \".\"") {
		t.Errorf("default name from URL:\n%s", read(t, teamFile(home)))
	}
}

func TestSkillAddWithFullCommitDoesNotConnectOrDetect(t *testing.T) {
	home := newHome(t)
	pin := strings.Repeat("c", 40)
	run(t, "-C", home, "skill", "add", skillHTTPS, "--subdir", "pdf", "--commit", pin).expect(t, exitOK, "")
	if !strings.Contains(read(t, teamFile(home)), "commit = \""+pin+"\"") {
		t.Errorf("team file:\n%s", read(t, teamFile(home)))
	}
	if exists(localFile(home)) {
		t.Error("protocol was detected although --commit was given")
	}
	run(t, "-C", home, "skill", "update", "pdf", "--commit", strings.Repeat("d", 64)).expect(t, exitOK, "")
	if exists(localFile(home)) {
		t.Error("update with --commit detected the protocol")
	}
}

func TestSkillAddRejectsBadValuesAndConflicts(t *testing.T) {
	home := newHome(t)
	pin := strings.Repeat("c", 40)
	run(t, "-C", home, "skill", "add", skillHTTPS, "--name", "pdf", "--commit", pin).expect(t, exitOK, "")
	if err := os.MkdirAll(at(home, "skills/notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, at(home, "skills/notes/SKILL.md"), skillV1)
	before := read(t, teamFile(home))
	cases := map[string][]string{
		"short commit":               {"--name", "x", "--commit", "abc1234"},
		"uppercase commit":           {"--name", "x", "--commit", strings.ToUpper(pin)},
		"branch commit":              {"--name", "x", "--commit", "main"},
		"reserved name":              {"--name", "Hearth", "--commit", pin},
		"duplicate name":             {"--name", "PDF", "--commit", pin},
		"team skill name":            {"--name", "Notes", "--commit", pin},
		"escaping subdir":            {"--subdir", "../x", "--commit", pin},
		"git subdir":                 {"--subdir", ".git", "--commit", pin},
		"team name, no commit given": {"--name", "notes"},
	}
	for label, flags := range cases {
		res := run(t, append([]string{"-C", home, "skill", "add", skillHTTPS}, flags...)...)
		if res.code != exitBad || !strings.Contains(res.stderr, "error [usage]") || !strings.Contains(res.stderr, "is unchanged") {
			t.Errorf("%s: exit %d stderr %s", label, res.code, res.stderr)
		}
	}
	run(t, "-C", home, "skill", "add", "skills", "--commit", pin).expect(t, exitBad, "error [usage]")
	if read(t, teamFile(home)) != before {
		t.Error("a rejected add changed the team file")
	}
	if exists(localFile(home)) {
		t.Error("a rejected add wrote the personal file")
	}
}

func TestSkillUpdateAndRemove(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	run(t, "-C", home, "skill", "update", "demo", "--commit", "abc").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "skill", "update", "nope", "--commit", r.newer).expect(t, exitBad, "error [usage] unknown skill")
	if !strings.Contains(read(t, teamFile(home)), r.pinned) {
		t.Fatal("rejected update changed the pin")
	}
	res := run(t, "-C", home, "skill", "update", "demo")
	res.expect(t, exitOK, "")
	if !strings.Contains(read(t, teamFile(home)), "commit = \""+r.newer+"\"") || !strings.Contains(res.stdout, brand.TeamFileChangedHint) {
		t.Errorf("update to remote HEAD: %s\n%s", res.stdout, read(t, teamFile(home)))
	}
	res = run(t, "-C", home, "skill", "update", "demo", "--commit", r.newer)
	if res.code != exitOK || !strings.Contains(res.stdout, "already pinned") {
		t.Errorf("same pin: %+v", res)
	}
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	res = run(t, "-C", home, "skill", "remove", "demo")
	res.expect(t, exitOK, "")
	if strings.Contains(read(t, teamFile(home)), "[[skills]]") || !strings.Contains(res.stdout, vendorDemo+" was not touched") {
		t.Errorf("remove: %s\n%s", res.stdout, read(t, teamFile(home)))
	}
	if !exists(at(home, vendorDemo)) {
		t.Error("remove deleted the checkout")
	}
	run(t, "-C", home, "skill", "remove", "demo").expect(t, exitBad, "error [usage] unknown skill")
}

func TestPersonalCommandsKeepOtherKeys(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	if err := os.MkdirAll(at(home, "skills/notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, at(home, "skills/notes/SKILL.md"), skillV1)
	team := read(t, teamFile(home))
	res := run(t, "-C", home, "config", "agents")
	if res.code != exitOK || res.stdout != "agents: none\n" {
		t.Errorf("unset agents: %+v", res)
	}
	run(t, "-C", home, "config", "agents", "codex,claude,codex").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "enable", "NOTES", "demo").expect(t, exitOK, "")
	want := "schema_version = 1\nprotocol = \"https\"\nagents = [\"codex\", \"claude\"]\nskills = [\"demo\", \"notes\"]\n"
	if got := read(t, localFile(home)); got != want {
		t.Errorf("personal file %q, want %q", got, want)
	}
	res = run(t, "-C", home, "config", "agents")
	if res.stdout != "agents: codex, claude\n" {
		t.Errorf("agents shown as %q", res.stdout)
	}
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", "none").expect(t, exitOK, "")
	want = "schema_version = 1\nprotocol = \"ssh\"\nskills = [\"demo\", \"notes\"]\n"
	if got := read(t, localFile(home)); got != want {
		t.Errorf("after protocol and none: %q, want %q", got, want)
	}
	for _, args := range [][]string{
		{"config", "agents", "cursor"}, {"config", "agents", "claude,"}, {"config", "agents", "none,claude"},
		{"skill", "enable", "nope"}, {"skill", "enable", brand.BuiltinSkill}, {"skill", "disable", "nope"},
		{"skill", "enable"}, {"skill", "disable"},
	} {
		run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, "error [usage]")
	}
	res = run(t, "-C", home, "skill", "enable", "nope")
	if !strings.Contains(res.stderr, "available: demo, notes") {
		t.Errorf("enable does not list the choices: %s", res.stderr)
	}
	run(t, "-C", home, "skill", "disable", "Demo").expect(t, exitOK, "")
	if got := read(t, localFile(home)); got != "schema_version = 1\nprotocol = \"ssh\"\nskills = [\"notes\"]\n" {
		t.Errorf("after disable: %q", got)
	}
	if read(t, teamFile(home)) != team {
		t.Error("a personal command changed the team file")
	}
}

func TestSyncInstallsPinnedCommitDetachedAndRerunDoesNotFetch(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	vendor := at(home, vendorDemo)
	if head(t, vendor) != r.pinned || !isDetached(t, vendor) {
		t.Errorf("checkout at %s, detached %v; want %s detached", head(t, vendor), isDetached(t, vendor), r.pinned)
	}
	assertLinked(t, home, claudeDemo, skillV1)
	assertLinked(t, home, agentsDemo, skillV1)
	for _, want := range []string{"skill demo: installed " + vendorDemo + " at " + r.pinned, "link " + claudeDemo + ": created", "Summary: 2 ready"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, res.stdout)
		}
	}
	assertNoTempDirs(t, at(home, "vendor/skills"))
	tracking := strings.TrimSpace(gittest.Git(t, vendor, "rev-parse", "refs/remotes/origin/main"))
	r.commit(t, "later.txt", "later\n")
	r.push(t)
	res = run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "skill demo: already at "+r.pinned) || !strings.Contains(res.stdout, "link "+claudeDemo+": ok") {
		t.Errorf("rerun stdout:\n%s", res.stdout)
	}
	if got := strings.TrimSpace(gittest.Git(t, vendor, "rev-parse", "refs/remotes/origin/main")); got != tracking {
		t.Errorf("rerun fetched: origin/main %s -> %s", tracking, got)
	}
	if listStatus(t, home, "demo") != "ok" || listStatus(t, home, brand.BuiltinSkill) != "ok" {
		t.Error("list after sync is not ok")
	}
}

func TestSyncMovesToNewPinWhenNothingIsLost(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "update", "demo", "--commit", r.newer).expect(t, exitOK, "")
	if got := listStatus(t, home, "demo"); got != "pin-mismatch" {
		t.Errorf("list after update: %s", got)
	}
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "updated "+vendorDemo+" from "+r.pinned+" to "+r.newer) {
		t.Errorf("stdout:\n%s", res.stdout)
	}
	vendor := at(home, vendorDemo)
	if head(t, vendor) != r.newer || !isDetached(t, vendor) {
		t.Errorf("checkout at %s", head(t, vendor))
	}
	assertLinked(t, home, claudeDemo, skillV2)
	assertNoTempDirs(t, at(home, "vendor/skills"))
}

// Each case breaks exactly one of the three conditions for replacing a
// checkout, so removing that condition from the code fails its case.
func TestSyncKeepsCheckoutThatHoldsLocalState(t *testing.T) {
	cases := map[string]func(t *testing.T, vendor string){
		"uncommitted change": func(t *testing.T, vendor string) {
			write(t, filepath.Join(vendor, demoSubdir, "SKILL.md"), "mine\n")
		},
		"ignored file": func(t *testing.T, vendor string) {
			write(t, filepath.Join(vendor, ".git", "info", "exclude"), "*.log\n")
			write(t, filepath.Join(vendor, "notes.log"), "mine\n")
		},
		"local commit on detached HEAD": func(t *testing.T, vendor string) {
			write(t, filepath.Join(vendor, "mine.txt"), "mine\n")
			gittest.Git(t, vendor, "add", "mine.txt")
			gittest.Git(t, vendor, "commit", "--quiet", "-m", "mine")
		},
		"branch checked out": func(t *testing.T, vendor string) {
			gittest.Git(t, vendor, "checkout", "--quiet", "-b", "mine")
		},
	}
	for label, prepare := range cases {
		t.Run(label, func(t *testing.T) {
			r := newSkillRemote(t)
			home := skillHome(t, r, r.pinned, bothAgents)
			run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
			vendor := at(home, vendorDemo)
			prepare(t, vendor)
			run(t, "-C", home, "skill", "update", "demo", "--commit", r.newer).expect(t, exitOK, "")
			before := tree(t, vendor)
			res := run(t, "-C", home, "skill", "sync")
			res.expect(t, exitFailed, "error [pin-mismatch] demo:")
			if !strings.Contains(res.stderr, r.newer) {
				t.Errorf("message does not name the new pin: %s", res.stderr)
			}
			assertSameTree(t, before, tree(t, vendor))
			assertNoTempDirs(t, at(home, "vendor/skills"))
			if listStatus(t, home, "demo") != "pin-mismatch" {
				t.Error("list does not report pin-mismatch")
			}
		})
	}
}

func TestSubdirMustHoldSkillFileInsideCheckout(t *testing.T) {
	r := newSkillRemote(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "SKILL.md"), skillV1)
	if err := os.Symlink(outside, filepath.Join(r.work, "escape")); err != nil {
		t.Fatal(err)
	}
	escape := r.commit(t, "escape-marker", "x\n")
	r.push(t)
	cases := map[string][]string{
		"missing SKILL.md":        {"--subdir", "skills", "--commit", r.pinned},
		"missing subdir":          {"--subdir", "nope", "--commit", r.pinned},
		"symlink out of checkout": {"--subdir", "escape", "--commit", escape},
	}
	for label, flags := range cases {
		t.Run(label, func(t *testing.T) {
			home := newHome(t)
			gittest.Redirect(t, skillHTTPS, r.bare)
			run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
			run(t, append([]string{"-C", home, "skill", "add", skillHTTPS, "--name", "demo"}, flags...)...).expect(t, exitOK, "")
			run(t, "-C", home, "config", "agents", "claude").expect(t, exitOK, "")
			run(t, "-C", home, "skill", "enable", "demo").expect(t, exitOK, "")
			run(t, "-C", home, "skill", "sync").expect(t, exitFailed, "error [skill-invalid] demo:")
			if exists(at(home, vendorDemo)) || exists(at(home, claudeDemo)) {
				t.Error("an invalid skill was installed or linked")
			}
			assertNoTempDirs(t, at(home, "vendor/skills"))
		})
	}
}

func TestUnknownCommitIsCloneFailedWithoutLeftovers(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, strings.Repeat("0", 40), "claude")
	run(t, "-C", home, "skill", "sync").expect(t, exitFailed, "error [clone-failed] demo:")
	if exists(at(home, vendorDemo)) || exists(at(home, claudeDemo)) {
		t.Error("a failed clone left a checkout or a link")
	}
	assertNoTempDirs(t, at(home, "vendor/skills"))
}

func TestProtocolAppliesToSkillURL(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	gittest.Redirect(t, skillSSH, r.bare)
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if got := origin(t, at(home, vendorDemo)); got != skillSSH {
		t.Errorf("origin %s, want %s", got, skillSSH)
	}
}

func TestSyncDetectsProtocolOnlyWhenItClones(t *testing.T) {
	r := newSkillRemote(t)
	home := newHome(t)
	gittest.Redirect(t, skillSSH, r.bare)
	run(t, "-C", home, "skill", "add", skillHTTPS, "--name", "demo", "--subdir", demoSubdir, "--commit", r.pinned).expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", "claude").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "enable", "demo").expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	if !strings.Contains(read(t, localFile(home)), "protocol = \"ssh\"\nagents = [\"claude\"]\nskills = [\"demo\"]") {
		t.Errorf("detection did not keep the other keys: %s", read(t, localFile(home)))
	}
}

func TestUserFilesAndForeignLinksAreNeverTouched(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	if err := os.MkdirAll(at(home, claudeDemo), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, at(home, claudeDemo+"/SKILL.md"), "mine\n")
	foreign := t.TempDir()
	if err := os.MkdirAll(at(home, ".agents/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, at(home, agentsDemo)); err != nil {
		t.Fatal(err)
	}
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitFailed, "error [link-conflict] demo:")
	if strings.Count(res.stderr, "error [link-conflict] demo:") != 2 {
		t.Errorf("want two conflicts:\n%s", res.stderr)
	}
	if read(t, at(home, claudeDemo+"/SKILL.md")) != "mine\n" {
		t.Error("user file overwritten")
	}
	if dest, _ := os.Readlink(at(home, agentsDemo)); dest != foreign {
		t.Errorf("foreign link changed to %s", dest)
	}
	if got := listStatus(t, home, "demo"); got != "link-conflict" {
		t.Errorf("list: %s", got)
	}
	run(t, "-C", home, "config", "agents", "none").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if dest, _ := os.Readlink(at(home, agentsDemo)); dest != foreign || read(t, at(home, claudeDemo+"/SKILL.md")) != "mine\n" {
		t.Error("prune touched a file or link it does not manage")
	}
}

func TestListReportsBrokenLinksAndSyncRepairsThem(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if err := os.Remove(at(home, claudeDemo)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(at(home, agentsDemo)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../"+vendorDemo, at(home, agentsDemo)); err != nil {
		t.Fatal(err)
	}
	before := tree(t, home)
	res := run(t, "-C", home, "skill", "list")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "link-stale") || !strings.Contains(res.stdout, agentsDemo) {
		t.Errorf("list shows the worst state and its path:\n%s", res.stdout)
	}
	assertSameTree(t, before, tree(t, home))
	if err := os.Remove(at(home, agentsDemo)); err != nil {
		t.Fatal(err)
	}
	if got := listStatus(t, home, "demo"); got != "link-missing" {
		t.Errorf("both links missing: %s", got)
	}
	if err := os.Symlink("../../"+vendorDemo, at(home, agentsDemo)); err != nil {
		t.Fatal(err)
	}
	res = run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "link "+claudeDemo+": created") || !strings.Contains(res.stdout, "link "+agentsDemo+": updated") {
		t.Errorf("stdout:\n%s", res.stdout)
	}
	assertLinked(t, home, claudeDemo, skillV1)
	assertLinked(t, home, agentsDemo, skillV1)
}

func TestDisablingRemovesOnlyManagedLinks(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	foreign := t.TempDir()
	if err := os.Symlink(foreign, at(home, ".claude/skills/other")); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", home, "config", "agents", "claude").expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "link "+agentsDemo+": removed") || exists(at(home, agentsDemo)) {
		t.Errorf("codex link kept:\n%s", res.stdout)
	}
	assertLinked(t, home, claudeDemo, skillV1)
	run(t, "-C", home, "skill", "disable", "demo").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if exists(at(home, claudeDemo)) {
		t.Error("disabled skill still linked")
	}
	if dest, _ := os.Readlink(at(home, ".claude/skills/other")); dest != foreign {
		t.Error("user link removed")
	}
	if !exists(at(home, vendorDemo+"/.git")) {
		t.Error("sync deleted the vendor checkout")
	}
	if got := listStatus(t, home, "demo"); got != "disabled" {
		t.Errorf("list: %s", got)
	}
	doctor := run(t, "-C", home, "doctor")
	doctor.expect(t, exitOK, "")
	if strings.Contains(doctor.stdout+doctor.stderr, "demo") {
		t.Errorf("doctor mentions a disabled skill:\n%s%s", doctor.stdout, doctor.stderr)
	}
}

func TestPathConflictsOnSkillDirectories(t *testing.T) {
	cases := map[string]struct {
		prepare func(t *testing.T, home string, outside string)
		item    string
	}{
		"vendor/skills is a symlink": {func(t *testing.T, home string, outside string) {
			mkdir(t, at(home, "vendor"))
			symlink(t, outside, at(home, "vendor/skills"))
		}, "demo"},
		"vendor is a file": {func(t *testing.T, home string, outside string) {
			write(t, at(home, "vendor"), "x")
		}, "demo"},
		"state dir is a symlink": {func(t *testing.T, home string, outside string) {
			symlink(t, outside, at(home, brand.StateDir))
		}, brand.BuiltinSkill},
		".claude/skills is a symlink": {func(t *testing.T, home string, outside string) {
			symlink(t, outside, at(home, ".claude/skills"))
		}, "demo"},
		".agents is a file": {func(t *testing.T, home string, outside string) {
			write(t, at(home, ".agents"), "x")
		}, "demo"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			r := newSkillRemote(t)
			home := skillHome(t, r, r.pinned, bothAgents)
			outside := t.TempDir()
			c.prepare(t, home, outside)
			if got := listStatus(t, home, c.item); got != "path-conflict" {
				t.Errorf("list %s: %s", c.item, got)
			}
			run(t, "-C", home, "skill", "sync").expect(t, exitFailed, "error [path-conflict] "+c.item+":")
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Errorf("wrote through the symlink: %v", entries)
			}
			run(t, "-C", home, "doctor").expect(t, exitFailed, "error [path-conflict] "+c.item+":")
		})
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target string, link string) {
	t.Helper()
	mkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinSkillFollowsAgentChoice(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if exists(at(home, brand.StateDir)) {
		t.Error("built-in skill written with no agent chosen")
	}
	if got := listStatus(t, home, brand.BuiltinSkill); got != "disabled" {
		t.Errorf("list with no agent: %s", got)
	}
	run(t, "-C", home, "config", "agents", bothAgents).expect(t, exitOK, "")
	if got := listStatus(t, home, brand.BuiltinSkill); got != "not-installed" {
		t.Errorf("list before sync: %s", got)
	}
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	text := read(t, at(home, builtinFile))
	wants := []string{
		"name: " + brand.BuiltinSkill,
		brand.Name + " repo add", brand.Name + " repo remove", brand.Name + " skill add", brand.Name + " skill remove",
		brand.Name + " skill enable", brand.Name + " skill disable", brand.TeamFile, brand.LocalFile, "by hand", "sandbox",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("built-in SKILL.md lacks %q", want)
		}
	}
	for _, link := range []string{".claude/skills/" + brand.BuiltinSkill, ".agents/skills/" + brand.BuiltinSkill} {
		if read(t, at(home, link+"/SKILL.md")) != text {
			t.Errorf("%s does not reach the built-in skill", link)
		}
	}
	write(t, at(home, builtinFile), "old\n")
	if got := listStatus(t, home, brand.BuiltinSkill); got != "not-installed" {
		t.Errorf("out-of-date built-in: %s", got)
	}
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if read(t, at(home, builtinFile)) != text {
		t.Error("sync did not rewrite the built-in skill")
	}
	run(t, "-C", home, "config", "agents", "none").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	for _, path := range []string{brand.StateDir, ".claude/skills/" + brand.BuiltinSkill, ".agents/skills/" + brand.BuiltinSkill} {
		if exists(at(home, path)) {
			t.Errorf("%s left after choosing no agent", path)
		}
	}
}

func TestRemovedSkillIsAStaleChoice(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "claude")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "remove", "demo").expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "list")
	res.expect(t, exitOK, "warning [unknown] demo:")
	if listStatus(t, home, "demo") != "unknown" {
		t.Error("list does not show unknown")
	}
	doctor := run(t, "-C", home, "doctor")
	if doctor.code != exitOK || !strings.Contains(doctor.stdout, "warning: [unknown] demo:") {
		t.Errorf("doctor: %+v", doctor)
	}
	res = run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "warning [unknown] demo:")
	if exists(at(home, claudeDemo)) {
		t.Error("sync kept the link of a removed skill")
	}
	run(t, "-C", home, "skill", "disable", "demo").expect(t, exitOK, "")
	if strings.Contains(read(t, localFile(home)), "demo") {
		t.Errorf("disable kept the stale name: %s", read(t, localFile(home)))
	}
}

func TestTeamSkillsCanBeChosenAndConflictsAreInvalid(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "claude")
	mkdir(t, at(home, "skills/notes"))
	write(t, at(home, "skills/notes/SKILL.md"), skillV1)
	mkdir(t, at(home, "skills/bad name"))
	write(t, at(home, "skills/bad name/SKILL.md"), skillV1)
	run(t, "-C", home, "skill", "enable", "bad name").expect(t, exitBad, "error [usage] cannot enable bad name (skill-invalid")
	run(t, "-C", home, "skill", "enable", "notes").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if dest, _ := os.Readlink(at(home, ".claude/skills/notes")); dest != "../../skills/notes" {
		t.Errorf("team link -> %s", dest)
	}
	mkdir(t, at(home, "skills/Demo"))
	write(t, at(home, "skills/Demo/SKILL.md"), skillV2)
	if listStatus(t, home, "demo") != "skill-invalid" || listStatus(t, home, "Demo") != "skill-invalid" {
		t.Error("a name used by two sources is not invalid")
	}
	run(t, "-C", home, "skill", "enable", "Demo").expect(t, exitBad, "error [usage]")
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitFailed, "error [skill-invalid] demo:")
	if exists(at(home, claudeDemo)) || !exists(at(home, ".claude/skills/notes")) {
		t.Error("the conflict should unlink only demo")
	}
	if listStatus(t, home, "notes") != "ok" {
		t.Error("other skills stop working")
	}
}

func TestMissingAgentCLIsAreOnlyWarnings(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	symlink(t, git, filepath.Join(bin, "git"))
	write(t, filepath.Join(bin, "claude"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "warning [agent-missing] codex:")
	if strings.Contains(res.stderr, "agent-missing] claude") {
		t.Errorf("claude is on PATH: %s", res.stderr)
	}
	assertLinked(t, home, agentsDemo, skillV1)
	doctor := run(t, "-C", home, "doctor")
	if doctor.code != exitOK || !strings.Contains(doctor.stdout, "warning: [agent-missing] codex:") {
		t.Errorf("doctor: %+v", doctor)
	}
}

func TestListAndDoctorNeitherConnectNorWrite(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "update", "demo", "--commit", r.newer).expect(t, exitOK, "")
	other := "https://git.example.invalid/acme/other.git"
	run(t, "-C", home, "skill", "add", other, "--name", "other", "--commit", r.pinned).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "enable", "other").expect(t, exitOK, "")
	gittest.Redirect(t, other, filepath.Join(t.TempDir(), "missing.git"))
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	before := tree(t, home)
	for _, args := range [][]string{{"skill", "list"}, {"doctor"}} {
		res := run(t, append([]string{"-C", home}, args...)...)
		if res.code == exitBad {
			t.Errorf("%v: exit %d %s", args, res.code, res.stderr)
		}
	}
	assertSameTree(t, before, tree(t, home))
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"ls-remote", "fetch", "clone", "remote-"} {
		if strings.Contains(string(data), network) {
			t.Errorf("list or doctor ran git %s:\n%s", network, data)
		}
	}
}

func TestSyncLeavesHomeRepoClean(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, bothAgents)
	gittest.Git(t, home, "add", "-A")
	gittest.Git(t, home, "commit", "--quiet", "-m", "init")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	if status := gittest.Git(t, home, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("sync left changes:\n%s", status)
	}
}

// A directory the link cannot be written into is a path problem, not a file
// system without symlinks; that split is tested in internal/skill.
func TestUnwritableLinkDirIsPathConflictWithoutCopying(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	run(t, "-C", home, "skill", "sync").expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", "claude").expect(t, exitOK, "")
	dir := at(home, ".claude/skills")
	mkdir(t, dir)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Error(err)
		}
	})
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitFailed, "error [path-conflict] demo: cannot create symlink "+claudeDemo)
	if !strings.Contains(res.stderr, "permission denied") || exists(at(home, claudeDemo)) {
		t.Errorf("stderr: %s", res.stderr)
	}
}

func TestSkillUpdateChecksTeamSkillConflictsBeforeConnecting(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	mkdir(t, at(home, "skills/Demo"))
	write(t, at(home, "skills/Demo/SKILL.md"), skillV1)
	team := read(t, teamFile(home))
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	for _, flags := range [][]string{{"--commit", r.newer}, nil} {
		res := run(t, append([]string{"-C", home, "skill", "update", "demo"}, flags...)...)
		res.expect(t, exitBad, "error [usage] cannot update skill: name \"demo\" is already used by the team skill at skills/Demo")
	}
	if read(t, teamFile(home)) != team {
		t.Error("a rejected update changed the team file")
	}
	if data, err := os.ReadFile(trace); err == nil && strings.Contains(string(data), "ls-remote") {
		t.Errorf("update contacted the remote:\n%s", data)
	}
}

func TestSkillNamesAreMatchedCaseInsensitively(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	run(t, "-C", home, "skill", "update", "DEMO", "--commit", r.newer).expect(t, exitOK, "")
	if text := read(t, teamFile(home)); !strings.Contains(text, "name = \"demo\"\n") || !strings.Contains(text, r.newer) {
		t.Errorf("update by another case:\n%s", text)
	}
	res := run(t, "-C", home, "skill", "remove", "Demo")
	res.expect(t, exitOK, "")
	if strings.Contains(read(t, teamFile(home)), "[[skills]]") || !strings.Contains(res.stdout, "removed skill demo") {
		t.Errorf("remove by another case: %s\n%s", res.stdout, read(t, teamFile(home)))
	}
}

func TestLinksEscapingVendorAreNotManaged(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "claude")
	escape := home + "/vendor/skills/../../elsewhere"
	symlink(t, escape, at(home, claudeDemo))
	symlink(t, escape, at(home, ".claude/skills/x"))
	run(t, "-C", home, "skill", "sync").expect(t, exitFailed, "error [link-conflict] demo:")
	for _, link := range []string{claudeDemo, ".claude/skills/x"} {
		if dest, err := os.Readlink(at(home, link)); err != nil || dest != escape {
			t.Errorf("%s -> %q, %v; want it left at %s", link, dest, err, escape)
		}
	}
}

func TestSkillUsageErrors(t *testing.T) {
	home := newHome(t)
	for _, args := range [][]string{{"skill"}, {"skill", "frob"}, {"skill", "add"}, {"skill", "list", "x"}, {"skill", "sync", "x"}, {"config", "agents", "a", "b"}} {
		run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, "error [usage]")
	}
	res := run(t, "-C", home, "skill", "sync")
	if res.code != exitOK || !strings.Contains(res.stdout, "Nothing enabled") {
		t.Errorf("empty sync: %+v", res)
	}
	if exists(at(home, "vendor")) || exists(at(home, ".agents")) {
		t.Error("empty sync wrote directories")
	}
}

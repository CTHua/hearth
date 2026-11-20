package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gittest"
)

const (
	toolkitHTTPS = "https://git.example.invalid/acme/toolkit.git"
	toolkitSSH   = "git@git.example.invalid:acme/toolkit.git"
	moreHTTPS    = "https://git.example.invalid/acme/more.git"
	oneDir       = "skills/a/one"
	twoDir       = "skills/b/two"
	threeDir     = "skills/c/three"
	vendorKit    = "vendor/skills/toolkit"
	pluginPath   = ".claude-plugin/plugin.json"
)

// threeSkills is a repo with three skills and no plugin manifest.
var threeSkills = map[string]string{
	oneDir + "/SKILL.md":   "---\nname: one\n---\n",
	twoDir + "/SKILL.md":   "---\nname: two\n---\n",
	threeDir + "/SKILL.md": "---\nname: three\n---\n",
	"README.md":            "toolkit\n",
}

// groupRemote is a bare remote with one commit holding files, and that
// commit's sha.
type groupRemote struct {
	bare   string
	commit string
}

func newGroupRemote(t *testing.T, files map[string]string) groupRemote {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	r := groupRemote{bare: filepath.Join(base, "remote.git")}
	gittest.Git(t, base, "init", "--quiet", "--initial-branch", "main", work)
	for rel, text := range files {
		full := filepath.Join(work, filepath.FromSlash(rel))
		mkdir(t, filepath.Dir(full))
		write(t, full, text)
	}
	gittest.Git(t, work, "add", "-A")
	gittest.Git(t, work, "commit", "--quiet", "--allow-empty", "-m", "skills")
	r.commit = strings.TrimSpace(gittest.Git(t, work, "rev-parse", "HEAD"))
	gittest.Git(t, base, "clone", "--quiet", "--bare", work, r.bare)
	return r
}

// withFiles returns files with extra added.
func withFiles(files map[string]string, extra map[string]string) map[string]string {
	all := map[string]string{}
	for rel, text := range files {
		all[rel] = text
	}
	for rel, text := range extra {
		all[rel] = text
	}
	return all
}

// groupHome is a home repo with protocol https and url redirected to r.
func groupHome(t *testing.T, url string, r groupRemote) string {
	t.Helper()
	home := newHome(t)
	gittest.Redirect(t, url, r.bare)
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	return home
}

// teamSkillsBlock is the [[skills]] part of the team file.
func teamSkillsBlock(t *testing.T, home string) string {
	t.Helper()
	text := read(t, teamFile(home))
	if i := strings.Index(text, "[[skills]]"); i >= 0 {
		return text[i:]
	}
	return ""
}

func groupBlock(name string, url string, commit string, subdirs ...string) string {
	quoted := make([]string, len(subdirs))
	for i, subdir := range subdirs {
		quoted[i] = "\"" + subdir + "\""
	}
	return "[[skills]]\nname = \"" + name + "\"\nurl = \"" + url + "\"\ncommit = \"" + commit + "\"\nsubdirs = [\n  " + strings.Join(quoted, ",\n  ") + ",\n]\n"
}

// emptyDir fails unless dir holds nothing.
func emptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%s holds %v", dir, entries)
	}
}

func TestSkillAddListsRemoteIntoGroupWithoutLeavingFiles(t *testing.T) {
	r := newGroupRemote(t, threeSkills)
	home := groupHome(t, toolkitHTTPS, r)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	before := tree(t, home)
	res := run(t, "-C", home, "skill", "add", toolkitHTTPS)
	res.expect(t, exitOK, "")
	want := groupBlock("toolkit", toolkitHTTPS, r.commit, oneDir, twoDir, threeDir)
	if got := teamSkillsBlock(t, home); got != want {
		t.Errorf("team file skills:\n%s\nwant:\n%s", got, want)
	}
	for _, line := range []string{"added group toolkit at " + r.commit + ": 3 skill(s) added", "found 3 skill(s) in " + toolkitHTTPS, brand.TeamFileChangedHint} {
		if !strings.Contains(res.stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "not added") {
		t.Errorf("nothing was left out:\n%s", res.stdout)
	}
	// Only the team file changed: no vendor/ and no other new file.
	after := tree(t, home)
	delete(before, brand.TeamFile)
	delete(after, brand.TeamFile)
	assertSameTree(t, before, after)
	emptyDir(t, temp)
}

func TestSkillAddDetectsProtocolAndWritesOnlyTheTwoFiles(t *testing.T) {
	r := newGroupRemote(t, threeSkills)
	home := newHome(t)
	gittest.Redirect(t, toolkitHTTPS, r.bare)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	before := tree(t, home)
	run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitOK, "")
	if !strings.Contains(read(t, localFile(home)), "protocol = \"https\"") {
		t.Errorf("personal file: %s", read(t, localFile(home)))
	}
	after := tree(t, home)
	for _, file := range []string{brand.TeamFile, brand.LocalFile} {
		delete(before, file)
		delete(after, file)
	}
	assertSameTree(t, before, after)
	emptyDir(t, temp)
}

func TestPluginManifestPicksMembersAndNamesGroup(t *testing.T) {
	manifest := `{"name": "picked", "skills": ["./skills/a/one/", "./skills/b"]}`
	r := newGroupRemote(t, withFiles(threeSkills, map[string]string{pluginPath: manifest}))
	home := groupHome(t, toolkitHTTPS, r)
	res := run(t, "-C", home, "skill", "add", toolkitHTTPS)
	res.expect(t, exitOK, "")
	if got, want := teamSkillsBlock(t, home), groupBlock("picked", toolkitHTTPS, r.commit, oneDir, twoDir); got != want {
		t.Errorf("team file skills:\n%s\nwant:\n%s", got, want)
	}
	for _, line := range []string{"added group picked at", "2 skill(s) added", "found 3 skill(s)", "not added (1):", "  " + threeDir + ": not listed in " + pluginPath,
		"`" + brand.Name + " skill add " + toolkitHTTPS + " --name picked --subdir <path>`"} {
		if !strings.Contains(res.stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, res.stdout)
		}
	}
}

// A skill the manifest leaves out does not clash by name with one it picks.
func TestUnselectedSkillDoesNotClashWithAPickedOne(t *testing.T) {
	unselected := "other/one"
	files := map[string]string{
		oneDir + "/SKILL.md":     "---\nname: one\n---\n",
		unselected + "/SKILL.md": "---\nname: one\n---\n",
		pluginPath:               `{"name": "picked", "skills": ["./skills/a/one"]}`,
	}
	r := newGroupRemote(t, files)
	home := groupHome(t, toolkitHTTPS, r)
	res := run(t, "-C", home, "skill", "add", toolkitHTTPS)
	res.expect(t, exitOK, "")
	if got, want := teamSkillsBlock(t, home), groupBlock("picked", toolkitHTTPS, r.commit, oneDir); got != want {
		t.Errorf("team file skills:\n%s\nwant:\n%s", got, want)
	}
	for _, line := range []string{"1 skill(s) added", "not added (1):", "  " + unselected + ": not listed in " + pluginPath} {
		if !strings.Contains(res.stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "same last path segment") {
		t.Errorf("stdout reports a clash:\n%s", res.stdout)
	}
}

// Each manifest is read by one rule below the add table; the members each
// should pick are listed.
func TestPluginManifestRules(t *testing.T) {
	outside := "tools/four"
	files := withFiles(threeSkills, map[string]string{outside + "/SKILL.md": "four\n"})
	cases := map[string]struct {
		manifest string
		group    string
		members  []string
	}{
		"broken JSON":             {`{"name": "x", `, "toolkit", []string{oneDir, twoDir, threeDir, outside}},
		"string skills":           {`{"name": "kit", "skills": "./tools"}`, "kit", []string{outside}},
		"no skills field":         {`{"name": "kit"}`, "kit", []string{oneDir, twoDir, threeDir}},
		"picks nothing":           {`{"name": "kit", "skills": ["./nowhere"]}`, "kit", []string{oneDir, twoDir, threeDir, outside}},
		"skills of another type":  {`{"name": "kit", "skills": [{"path": "skills"}]}`, "toolkit", []string{oneDir, twoDir, threeDir, outside}},
		"unusable name":           {`{"name": "bad name", "skills": "skills"}`, "toolkit", []string{oneDir, twoDir, threeDir}},
		"bigger than the maximum": {`{"name": "kit", "skills": "./tools", "pad": "` + strings.Repeat("x", 1<<20) + `"}`, "toolkit", []string{oneDir, twoDir, threeDir, outside}},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			r := newGroupRemote(t, withFiles(files, map[string]string{pluginPath: c.manifest}))
			home := groupHome(t, toolkitHTTPS, r)
			run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitOK, "")
			slices.Sort(c.members)
			if got, want := teamSkillsBlock(t, home), groupBlock(c.group, toolkitHTTPS, r.commit, c.members...); got != want {
				t.Errorf("team file skills:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// The first row of the add table: what the listing found decides the entry.
func TestSkillAddWithoutSubdirByWhatIsFound(t *testing.T) {
	t.Run("nothing", func(t *testing.T) {
		r := newGroupRemote(t, map[string]string{"README.md": "x\n"})
		home := groupHome(t, toolkitHTTPS, r)
		team := read(t, teamFile(home))
		run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitBad, "error [usage] cannot add skill: no SKILL.md found")
		if read(t, teamFile(home)) != team {
			t.Error("team file changed")
		}
	})
	t.Run("top of the repo", func(t *testing.T) {
		r := newGroupRemote(t, withFiles(threeSkills, map[string]string{"SKILL.md": "top\n"}))
		home := groupHome(t, toolkitHTTPS, r)
		run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitOK, "")
		want := "[[skills]]\nname = \"toolkit\"\nurl = \"" + toolkitHTTPS + "\"\ncommit = \"" + r.commit + "\"\nsubdir = \".\"\n"
		if got := teamSkillsBlock(t, home); got != want {
			t.Errorf("team file skills:\n%s", got)
		}
	})
	t.Run("one skill", func(t *testing.T) {
		r := newGroupRemote(t, map[string]string{twoDir + "/SKILL.md": "two\n"})
		home := groupHome(t, toolkitHTTPS, r)
		run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitOK, "")
		want := "[[skills]]\nname = \"two\"\nurl = \"" + toolkitHTTPS + "\"\ncommit = \"" + r.commit + "\"\nsubdir = \"" + twoDir + "\"\n"
		if got := teamSkillsBlock(t, home); got != want {
			t.Errorf("team file skills:\n%s", got)
		}
	})
	t.Run("given commit", func(t *testing.T) {
		r := newGroupRemote(t, threeSkills)
		home := groupHome(t, toolkitHTTPS, r)
		run(t, "-C", home, "skill", "add", toolkitHTTPS, "--commit", strings.Repeat("0", 40)).expect(t, exitBad, "error [usage] commit")
		run(t, "-C", home, "skill", "add", toolkitHTTPS, "--commit", r.commit, "--name", "kit").expect(t, exitOK, "")
		if got := teamSkillsBlock(t, home); got != groupBlock("kit", toolkitHTTPS, r.commit, oneDir, twoDir, threeDir) {
			t.Errorf("team file skills:\n%s", got)
		}
	})
}

// The third row: several --subdir are the group, and the remote is not
// listed; with --commit it is not contacted at all.
func TestSkillAddWithSeveralSubdirsDoesNotList(t *testing.T) {
	home := newHome(t)
	pin := strings.Repeat("c", 40)
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	res := run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", "skills/x/four", "--commit", pin)
	res.expect(t, exitOK, "")
	if got := teamSkillsBlock(t, home); got != groupBlock("toolkit", toolkitHTTPS, pin, oneDir, "skills/x/four") {
		t.Errorf("team file skills:\n%s", got)
	}
	if strings.Contains(res.stdout, "found") {
		t.Errorf("reports a listing:\n%s", res.stdout)
	}
	if data, err := os.ReadFile(trace); err == nil && (strings.Contains(string(data), "clone") || strings.Contains(string(data), "ls-remote")) {
		t.Errorf("add contacted the remote:\n%s", data)
	}
	if exists(localFile(home)) {
		t.Error("protocol was detected although nothing was contacted")
	}
}

// The fourth row: --name of a group of the same repo adds to it.
func TestSkillAddToExistingGroup(t *testing.T) {
	r := newGroupRemote(t, threeSkills)
	home := groupHome(t, toolkitHTTPS, r)
	gittest.Redirect(t, toolkitSSH, r.bare)
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", twoDir).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--name", "single", "--subdir", threeDir).expect(t, exitOK, "")
	before := read(t, teamFile(home))
	for label, args := range map[string][]string{
		"commit given":    {toolkitSSH, "--name", "toolkit", "--subdir", threeDir, "--commit", r.commit},
		"already in it":   {toolkitSSH, "--name", "Toolkit", "--subdir", twoDir},
		"a single skill":  {toolkitHTTPS, "--name", "single", "--subdir", oneDir},
		"bad subdir":      {toolkitSSH, "--name", "toolkit", "--subdir", "../x"},
		"duplicate name":  {toolkitSSH, "--name", "toolkit", "--subdir", "skills/z/one"},
		"group name used": {toolkitSSH, "--name", "toolkit", "--subdir", "skills/z/toolkit"},
	} {
		res := run(t, append([]string{"-C", home, "skill", "add"}, args...)...)
		if res.code != exitBad || !strings.Contains(res.stderr, "error [usage]") || !strings.Contains(res.stderr, "is unchanged") {
			t.Errorf("%s: exit %d stderr %s", label, res.code, res.stderr)
		}
	}
	if read(t, teamFile(home)) != before {
		t.Fatal("a rejected add changed the team file")
	}
	// The ssh spelling names the same repo; the pin stays.
	res := run(t, "-C", home, "skill", "add", toolkitSSH, "--name", "toolkit", "--subdir", threeDir)
	res.expect(t, exitOK, "")
	if !strings.Contains(teamSkillsBlock(t, home), groupBlock("toolkit", toolkitHTTPS, r.commit, oneDir, twoDir, threeDir)) {
		t.Errorf("team file skills:\n%s", teamSkillsBlock(t, home))
	}
	if !strings.Contains(res.stdout, "added 1 skill(s) to group toolkit at "+r.commit+": three") || !strings.Contains(res.stdout, "skill enable three") {
		t.Errorf("stdout:\n%s", res.stdout)
	}
}

// The rules below the add table about names: skipped when found, refused
// when given.
func TestSkillAddNameRules(t *testing.T) {
	files := withFiles(threeSkills, map[string]string{
		"skills/d/_hidden/SKILL.md": "x\n",
		"skills/e/Hearth/SKILL.md":  "x\n",
		"skills/f/dup/SKILL.md":     "x\n",
		"skills/g/DUP/SKILL.md":     "x\n",
		"skills/h/notes/SKILL.md":   "x\n",
	})
	r := newGroupRemote(t, files)
	home := groupHome(t, toolkitHTTPS, r)
	mkdir(t, at(home, "skills/notes"))
	write(t, at(home, "skills/notes/SKILL.md"), skillV1)
	res := run(t, "-C", home, "skill", "add", toolkitHTTPS)
	res.expect(t, exitOK, "")
	if got := teamSkillsBlock(t, home); got != groupBlock("toolkit", toolkitHTTPS, r.commit, oneDir, twoDir, threeDir) {
		t.Errorf("team file skills:\n%s", got)
	}
	for _, line := range []string{
		"not added (5):",
		"  skills/d/_hidden: name ",
		"  skills/e/Hearth: name \"" + brand.BuiltinSkill + "\" is reserved",
		"  skills/f/dup: another skill found has the same last path segment \"dup\"",
		"  skills/g/DUP: another skill found has the same last path segment \"DUP\"",
		"  skills/h/notes: name \"notes\" is already used by the team skill at skills/notes",
		"found 8 skill(s)",
	} {
		if !strings.Contains(res.stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, res.stdout)
		}
	}
	before := read(t, teamFile(home))
	for label, args := range map[string][]string{
		"invalid member name": {"--name", "x", "--subdir", oneDir, "--subdir", "skills/d/_hidden"},
		"reserved member":     {"--name", "x", "--subdir", oneDir, "--subdir", "skills/e/Hearth"},
		"repeated last part":  {"--name", "x", "--subdir", "skills/f/dup", "--subdir", "skills/g/DUP"},
		"team skill member":   {"--name", "x", "--subdir", oneDir, "--subdir", "skills/h/notes"},
		"group like member":   {"--name", "one", "--subdir", oneDir, "--subdir", twoDir},
		"group like team":     {"--name", "notes", "--subdir", oneDir, "--subdir", twoDir},
		"group like builtin":  {"--name", brand.BuiltinSkill, "--subdir", oneDir, "--subdir", twoDir},
	} {
		res := run(t, append([]string{"-C", home, "skill", "add", moreHTTPS}, args...)...)
		if res.code != exitBad || !strings.Contains(res.stderr, "error [usage]") {
			t.Errorf("%s: exit %d stderr %s", label, res.code, res.stderr)
		}
	}
	if read(t, teamFile(home)) != before {
		t.Error("a rejected add changed the team file")
	}
}

// A group name found on the remote that clashes asks for --name.
func TestSkillAddFoundGroupNameMustBeFree(t *testing.T) {
	cases := map[string]string{
		"a member":   `{"name": "one"}`,
		"a team one": `{"name": "notes"}`,
	}
	for label, manifest := range cases {
		t.Run(label, func(t *testing.T) {
			r := newGroupRemote(t, withFiles(threeSkills, map[string]string{pluginPath: manifest}))
			home := groupHome(t, toolkitHTTPS, r)
			mkdir(t, at(home, "skills/notes"))
			write(t, at(home, "skills/notes/SKILL.md"), skillV1)
			team := read(t, teamFile(home))
			run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitBad, "--name")
			if read(t, teamFile(home)) != team {
				t.Error("team file changed")
			}
			run(t, "-C", home, "skill", "add", toolkitHTTPS, "--name", "kit").expect(t, exitOK, "")
		})
	}
}

// groupSyncHome has the toolkit group pinned with one and two enabled and
// both agents chosen.
func groupSyncHome(t *testing.T, subdirs ...string) (string, groupRemote) {
	t.Helper()
	r := newGroupRemote(t, threeSkills)
	home := groupHome(t, toolkitHTTPS, r)
	args := []string{"-C", home, "skill", "add", toolkitHTTPS, "--commit", r.commit}
	for _, subdir := range subdirs {
		args = append(args, "--subdir", subdir)
	}
	run(t, args...).expect(t, exitOK, "")
	run(t, "-C", home, "config", "agents", bothAgents).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "enable", "one", "two").expect(t, exitOK, "")
	return home, r
}

func TestGroupSyncsOneCheckoutWithALinkPerMember(t *testing.T) {
	home, r := groupSyncHome(t, oneDir, twoDir, threeDir)
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitOK, "")
	entries, err := os.ReadDir(at(home, "vendor/skills"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "toolkit" {
		t.Errorf("vendor/skills holds %v, %v", entries, err)
	}
	if n := strings.Count(res.stdout, ": installed "); n != 1 || !strings.Contains(res.stdout, "skill toolkit: installed "+vendorKit+" at "+r.commit) {
		t.Errorf("%d installed lines:\n%s", n, res.stdout)
	}
	if head(t, at(home, vendorKit)) != r.commit || !isDetached(t, at(home, vendorKit)) {
		t.Errorf("checkout at %s, detached %v", head(t, at(home, vendorKit)), isDetached(t, at(home, vendorKit)))
	}
	for _, dir := range []string{".claude/skills", ".agents/skills"} {
		entries, err := os.ReadDir(at(home, dir))
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		if err != nil || !slices.Equal(names, []string{brand.BuiltinSkill, "one", "two"}) {
			t.Errorf("%s holds %v, %v", dir, names, err)
		}
		for name, subdir := range map[string]string{"one": oneDir, "two": twoDir} {
			dest, err := os.Readlink(at(home, dir+"/"+name))
			if want := "../../" + vendorKit + "/" + subdir; err != nil || dest != want {
				t.Errorf("%s/%s -> %s, %v; want %s", dir, name, dest, err, want)
			}
		}
	}
	if !strings.Contains(res.stdout, "Summary: 3 ready") {
		t.Errorf("stdout:\n%s", res.stdout)
	}
	assertNoTempDirs(t, at(home, "vendor/skills"))
	rerun := run(t, "-C", home, "skill", "sync")
	rerun.expect(t, exitOK, "")
	if n := strings.Count(rerun.stdout, "skill toolkit: already at"); n != 1 {
		t.Errorf("rerun:\n%s", rerun.stdout)
	}
	if listStatus(t, home, "one") != "ok" || listStatus(t, home, "three") != "disabled" {
		t.Error("list after sync")
	}
}

func TestBrokenMemberDoesNotStopItsGroup(t *testing.T) {
	home, _ := groupSyncHome(t, oneDir, "skills/x/two")
	res := run(t, "-C", home, "skill", "sync")
	res.expect(t, exitFailed, "error [skill-invalid] two:")
	if !exists(at(home, ".claude/skills/one")) || exists(at(home, ".claude/skills/two")) {
		t.Error("the valid member should be linked and only it")
	}
	if listStatus(t, home, "two") != "skill-invalid" || listStatus(t, home, "one") != "ok" {
		t.Error("list does not show the broken member alone")
	}
}

func TestEnableAndDisableWholeGroup(t *testing.T) {
	r := newGroupRemote(t, threeSkills)
	home := groupHome(t, toolkitHTTPS, r)
	run(t, "-C", home, "skill", "add", toolkitHTTPS).expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "enable", "toolkit")
	res.expect(t, exitOK, "")
	if want := "schema_version = 2\nprotocol = \"https\"\nskills = [\"one\", \"two\", \"three\"]\n"; read(t, localFile(home)) != want {
		t.Errorf("personal file %q, want %q", read(t, localFile(home)), want)
	}
	run(t, "-C", home, "skill", "disable", "one").expect(t, exitOK, "")
	run(t, "-C", home, "skill", "disable", "Toolkit").expect(t, exitOK, "")
	if want := "schema_version = 2\nprotocol = \"https\"\n"; read(t, localFile(home)) != want {
		t.Errorf("personal file %q, want %q", read(t, localFile(home)), want)
	}
	run(t, "-C", home, "skill", "disable", "toolkit").expect(t, exitBad, "error [usage] not enabled")
	// A team skill named like the group is what the name means.
	mkdir(t, at(home, "skills/toolkit"))
	write(t, at(home, "skills/toolkit/SKILL.md"), skillV1)
	run(t, "-C", home, "skill", "enable", "toolkit").expect(t, exitOK, "")
	if want := "schema_version = 2\nprotocol = \"https\"\nskills = [\"toolkit\"]\n"; read(t, localFile(home)) != want {
		t.Errorf("personal file %q, want %q", read(t, localFile(home)), want)
	}
}

func TestSameMemberInTwoGroupsIsInvalid(t *testing.T) {
	home := newHome(t)
	pin := strings.Repeat("c", 40)
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", twoDir, "--commit", pin).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "add", moreHTTPS, "--subdir", "x/one", "--subdir", "x/four", "--commit", pin).expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "list")
	res.expect(t, exitOK, "")
	invalid := 0
	for _, line := range lines(res.stdout) {
		fields := strings.Fields(line)
		if len(fields) > listStatusColumn && fields[0] == "one" && fields[listStatusColumn] == "disabled" && strings.Contains(line, "is also used by the external skill") {
			invalid++
		}
	}
	if invalid != 2 {
		t.Errorf("want both members called one marked invalid:\n%s", res.stdout)
	}
	run(t, "-C", home, "skill", "enable", "one").expect(t, exitBad, "error [usage] cannot enable one (skill-invalid")
	run(t, "-C", home, "skill", "enable", "two", "four").expect(t, exitOK, "")
}

func TestRemoveAndUpdateGroupsAndMembers(t *testing.T) {
	r := newGroupRemote(t, threeSkills)
	home := groupHome(t, toolkitHTTPS, r)
	pin := strings.Repeat("c", 40)
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", twoDir, "--commit", pin).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "update", "one").expect(t, exitBad, "error [usage] cannot update skill one: it is pinned with its group toolkit")
	res := run(t, "-C", home, "skill", "update", "toolkit")
	res.expect(t, exitOK, "")
	if got := teamSkillsBlock(t, home); got != groupBlock("toolkit", toolkitHTTPS, r.commit, oneDir, twoDir) {
		t.Errorf("after update:\n%s", got)
	}
	res = run(t, "-C", home, "skill", "remove", "ONE")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "removed skill one from group toolkit") || teamSkillsBlock(t, home) != groupBlock("toolkit", toolkitHTTPS, r.commit, twoDir) {
		t.Errorf("remove member: %s\n%s", res.stdout, teamSkillsBlock(t, home))
	}
	run(t, "-C", home, "skill", "remove", "two").expect(t, exitBad, "error [usage] cannot remove skill two: it is the last member of group toolkit; remove the group")
	run(t, "-C", home, "skill", "remove", "toolkit").expect(t, exitOK, "")
	if teamSkillsBlock(t, home) != "" {
		t.Errorf("remove group:\n%s", read(t, teamFile(home)))
	}
	run(t, "-C", home, "skill", "remove", "toolkit").expect(t, exitBad, "error [usage] unknown skill")
}

// A member name two groups share must be qualified with its group.
func TestRemoveMemberSharedByTwoGroups(t *testing.T) {
	home := newHome(t)
	pin := strings.Repeat("c", 40)
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", twoDir, "--commit", pin).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "add", moreHTTPS, "--subdir", "x/One", "--subdir", "x/four", "--subdir", "x/five", "--commit", pin).expect(t, exitOK, "")
	run(t, "-C", home, "skill", "add", "https://git.example.invalid/acme/pdf.git", "--name", "pdf", "--subdir", "pdf", "--commit", pin).expect(t, exitOK, "")
	before := read(t, teamFile(home))
	res := run(t, "-C", home, "skill", "remove", "one")
	res.expect(t, exitBad, "error [usage] cannot remove skill one: 2 groups have a skill of that name")
	for _, form := range []string{"toolkit/one", "more/One"} {
		if !strings.Contains(res.stderr, form) {
			t.Errorf("stderr lacks %s: %s", form, res.stderr)
		}
	}
	for _, args := range [][]string{{"nope/one"}, {"pdf/pdf"}, {"toolkit/four"}} {
		run(t, append([]string{"-C", home, "skill", "remove"}, args...)...).expect(t, exitBad, "error [usage] cannot remove skill "+args[0])
	}
	if read(t, teamFile(home)) != before {
		t.Fatal("a rejected remove changed the team file")
	}
	res = run(t, "-C", home, "skill", "remove", "MORE/one")
	res.expect(t, exitOK, "")
	if !strings.Contains(res.stdout, "removed skill One from group more") {
		t.Errorf("stdout: %s", res.stdout)
	}
	text := read(t, teamFile(home))
	if !strings.Contains(text, groupBlock("toolkit", toolkitHTTPS, pin, oneDir, twoDir)) || !strings.Contains(text, groupBlock("more", moreHTTPS, pin, "x/four", "x/five")) {
		t.Errorf("team file:\n%s", text)
	}
	// Only one group has it now, so the plain name works again.
	run(t, "-C", home, "skill", "remove", "one").expect(t, exitOK, "")
	if !strings.Contains(read(t, teamFile(home)), groupBlock("toolkit", toolkitHTTPS, pin, twoDir)) {
		t.Errorf("team file:\n%s", read(t, teamFile(home)))
	}
}

func TestListShowsGroupColumn(t *testing.T) {
	r := newSkillRemote(t)
	home := skillHome(t, r, r.pinned, "")
	run(t, "-C", home, "skill", "add", toolkitHTTPS, "--subdir", oneDir, "--subdir", twoDir, "--commit", r.pinned).expect(t, exitOK, "")
	res := run(t, "-C", home, "skill", "list")
	res.expect(t, exitOK, "")
	header := strings.Fields(lines(res.stdout)[0])
	if !slices.Equal(header, []string{"NAME", "GROUP", "SOURCE", "ENABLED", "STATUS", "DETAIL"}) {
		t.Errorf("header %v", header)
	}
	for name, want := range map[string]string{"demo": "-", "one": "toolkit", "two": "toolkit", brand.BuiltinSkill: "-"} {
		if got := listColumn(t, home, name, listGroupColumn); got != want {
			t.Errorf("%s: GROUP %s, want %s", name, got, want)
		}
	}
}

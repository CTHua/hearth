package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/subagent"
)

const (
	reviewerSource = "---\nname: reviewer\ndescription: Reviews a diff.\nclaude:\n  model: sonnet\ncodex:\n  model_reasoning_effort: high\n---\n\nReview the diff.\n"
	reviewerClaude = subagent.ClaudeDir + "/reviewer.md"
	reviewerCodex  = subagent.CodexDir + "/reviewer.toml"
	brokenSource   = "---\nname: reviewer\n---\n\nReview the diff.\n"
)

// subagentHome has the template's subagent and reviewer, with the given
// agents chosen.
func subagentHome(t *testing.T, agents string) string {
	t.Helper()
	home := newHome(t)
	write(t, at(home, subagent.SourceDir+"/reviewer.md"), reviewerSource)
	run(t, "-C", home, "config", "agents", agents).expect(t, exitOK, "")
	return home
}

func TestSubagentEnableWritesChoiceAndSyncWritesChosenAgentsOnly(t *testing.T) {
	home := subagentHome(t, "claude")
	r := run(t, "-C", home, "subagent", "enable", "reviewer")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "enabled: reviewer (saved to "+brand.LocalFile+")") {
		t.Errorf("stdout: %s", r.stdout)
	}
	if !strings.Contains(read(t, localFile(home)), `subagents = ["reviewer"]`) {
		t.Errorf("personal file:\n%s", read(t, localFile(home)))
	}
	r = run(t, "-C", home, "sync")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "subagent "+reviewerClaude+": wrote") || !strings.Contains(read(t, at(home, reviewerClaude)), "model: sonnet") {
		t.Errorf("sync: %+v", r)
	}
	if exists(at(home, subagent.CodexDir)) || exists(at(home, subagent.ClaudeDir+"/subagent.md")) {
		t.Error("wrote a file for an agent or a subagent that is not chosen")
	}
	list := run(t, "-C", home, "subagent", "list")
	list.expect(t, exitOK, "")
	for _, want := range []string{"NAME", "reviewer  yes", "ok", "subagent  no", "off"} {
		if !strings.Contains(list.stdout, want) {
			t.Errorf("list lacks %q:\n%s", want, list.stdout)
		}
	}
	run(t, "-C", home, "config", "agents", bothAgents).expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "sync").expect(t, exitOK, "")
	if !strings.Contains(read(t, at(home, reviewerCodex)), "model_reasoning_effort = \"high\"") {
		t.Error("codex file not written")
	}
}

func TestSubagentEnableRefusesUnknownAndInvalidNames(t *testing.T) {
	home := subagentHome(t, "claude")
	write(t, at(home, subagent.SourceDir+"/broken.md"), "---\nname: broken\n---\nbody\n")
	run(t, "-C", home, "subagent", "enable", "subagent").expect(t, exitOK, "")
	before := read(t, localFile(home))
	for _, names := range [][]string{{"nope"}, {"broken"}, {"reviewer", "broken"}} {
		r := run(t, append([]string{"-C", home, "subagent", "enable"}, names...)...)
		r.expect(t, exitBad, "error [usage] cannot enable")
		if read(t, localFile(home)) != before {
			t.Errorf("%v: personal file changed:\n%s", names, read(t, localFile(home)))
		}
	}
	r := run(t, "-C", home, "subagent", "enable", "broken")
	if !strings.Contains(r.stderr, "subagent-invalid") || !strings.Contains(r.stderr, "description must be") {
		t.Errorf("no reason: %s", r.stderr)
	}
	list := run(t, "-C", home, "subagent", "list")
	if !strings.Contains(list.stdout, "subagent-invalid") {
		t.Errorf("list:\n%s", list.stdout)
	}
	run(t, "-C", home, "subagent", "disable", "nope").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "subagent").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "subagent", "bogus").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "subagent", "enable").expect(t, exitBad, "error [usage]")
	run(t, "-C", home, "subagent", "list", "extra").expect(t, exitBad, "error [usage]")
}

func TestSubagentDisableRemovesGeneratedFiles(t *testing.T) {
	home := subagentHome(t, bothAgents)
	run(t, "-C", home, "mcp", "add", "docs", "--", "npx").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "docs").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "disable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	if exists(at(home, subagent.ClaudeDir)) || exists(at(home, subagent.CodexDir)) {
		t.Error("generated files or their directories left behind")
	}
	if !exists(at(home, mcp.CodexFile)) || !exists(at(home, ".claude/settings.json")) {
		t.Error("removed what subagent sync does not own")
	}
}

func TestTopLevelSyncRunsSubagentsLastAndPassesForce(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "mcp", "add", "docs", "--", "npx").expect(t, exitOK, "")
	run(t, "-C", home, "mcp", "enable", "docs").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	r := run(t, "-C", home, "sync")
	r.expect(t, exitOK, "")
	summary := strings.Index(r.stdout, "Summary: ")
	mcpLine := strings.Index(r.stdout, "mcp "+mcp.ClaudeFile)
	subagentLine := strings.Index(r.stdout, "subagent "+reviewerClaude)
	if summary < 0 || mcpLine < summary || subagentLine < mcpLine {
		t.Errorf("order: skill, MCP, subagent:\n%s", r.stdout)
	}
	write(t, at(home, reviewerClaude), "mine\n")
	run(t, "-C", home, "sync").expect(t, exitFailed, "error [generated-modified] "+reviewerClaude+":")
	run(t, "-C", home, "subagent", "sync").expect(t, exitFailed, "error [generated-modified] "+reviewerClaude+":")
	if read(t, at(home, reviewerClaude)) != "mine\n" {
		t.Error("edited file overwritten without --force")
	}
	run(t, "-C", home, "sync", "--force").expect(t, exitOK, "")
	if !strings.Contains(read(t, at(home, reviewerClaude)), "Review the diff.") {
		t.Error("top-level --force did not overwrite")
	}
	write(t, at(home, reviewerClaude), "mine\n")
	run(t, "-C", home, "subagent", "sync", "--force").expect(t, exitOK, "")
	if !strings.Contains(read(t, at(home, reviewerClaude)), "Review the diff.") {
		t.Error("subagent sync --force did not overwrite")
	}
	run(t, "-C", home, "subagent", "sync", "extra").expect(t, exitBad, "error [usage]")
}

func TestBrokenSourceKeepsFilesAndFailsSync(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	before := read(t, at(home, reviewerClaude))
	write(t, at(home, subagent.SourceDir+"/reviewer.md"), brokenSource)
	run(t, "-C", home, "sync").expect(t, exitFailed, "error [subagent-invalid] reviewer:")
	if read(t, at(home, reviewerClaude)) != before {
		t.Error("generated file changed after the source broke")
	}
	doctor := run(t, "-C", home, "doctor")
	doctor.expect(t, exitFailed, "error [subagent-invalid] reviewer:")
}

func TestDoctorReportsInvalidSourcesAndEditedFiles(t *testing.T) {
	home := subagentHome(t, "claude")
	fresh := run(t, "-C", home, "doctor")
	if fresh.code != exitOK || strings.Contains(fresh.stdout+fresh.stderr, "subagent") {
		t.Errorf("nothing chosen, nothing to report: %+v", fresh)
	}
	write(t, at(home, subagent.SourceDir+"/broken.md"), "no frontmatter\n")
	r := run(t, "-C", home, "doctor")
	if r.code != exitOK || !strings.Contains(r.stdout, "warning: [subagent-invalid] broken: subagents/broken.md: no frontmatter") {
		t.Errorf("unchosen invalid source: %+v", r)
	}
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	r = run(t, "-C", home, "doctor")
	if !strings.Contains(r.stdout, "warning: [not-synced] reviewer:") {
		t.Errorf("not synced: %+v", r)
	}
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	r = run(t, "-C", home, "doctor")
	if r.code != exitOK || !strings.Contains(r.stdout, "ok: subagent reviewer") {
		t.Errorf("synced: %+v", r)
	}
	write(t, at(home, reviewerClaude), "mine\n")
	before := tree(t, home)
	run(t, "-C", home, "doctor").expect(t, exitFailed, "error [generated-modified] reviewer: "+reviewerClaude+" changed")
	assertSameTree(t, before, tree(t, home))
}

func TestRemovedSourceIsAStaleChoice(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	if err := os.Remove(at(home, subagent.SourceDir+"/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	list := run(t, "-C", home, "subagent", "list")
	list.expect(t, exitOK, "warning [unknown] reviewer:")
	if !strings.Contains(list.stdout, "stale") {
		t.Errorf("list:\n%s", list.stdout)
	}
	if doctor := run(t, "-C", home, "doctor"); doctor.code != exitOK || !strings.Contains(doctor.stdout, "warning: [unknown] reviewer:") {
		t.Errorf("doctor: %+v", doctor)
	}
	run(t, "-C", home, "sync").expect(t, exitOK, "warning [unknown] reviewer:")
	if exists(at(home, reviewerClaude)) {
		t.Error("stale file not removed")
	}
	run(t, "-C", home, "subagent", "disable", "reviewer").expect(t, exitOK, "")
	if strings.Contains(read(t, localFile(home)), "reviewer") {
		t.Errorf("disable kept the stale name: %s", read(t, localFile(home)))
	}
}

func TestHelpListsSubagentCommands(t *testing.T) {
	r := run(t, "help")
	for _, want := range []string{"subagent enable <name>", "subagent sync [--force]", "subagent list", "--force is for MCP and subagents"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestDoctorFailsForEditedFileOfDisabledSubagent(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "disable", "reviewer").expect(t, exitOK, "")
	write(t, at(home, reviewerClaude), read(t, at(home, reviewerClaude))+"mine\n")
	run(t, "-C", home, "doctor").expect(t, exitFailed, "error [generated-modified] reviewer: "+reviewerClaude+" changed")
	list := run(t, "-C", home, "subagent", "list")
	if !strings.Contains(list.stdout, "generated-modified") {
		t.Errorf("list:\n%s", list.stdout)
	}
	run(t, "-C", home, "sync").expect(t, exitFailed, "error [generated-modified] "+reviewerClaude+":")
}

func TestDoctorFailsForEditedFileOfOrphanRecord(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "disable", "reviewer").expect(t, exitOK, "")
	if err := os.Remove(at(home, subagent.SourceDir+"/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	write(t, at(home, reviewerClaude), "mine\n")
	doctor := run(t, "-C", home, "doctor")
	doctor.expect(t, exitFailed, "error [generated-modified] reviewer: "+reviewerClaude+" changed")
	if strings.Contains(doctor.stdout, "[unknown] reviewer") {
		t.Errorf("an orphan record is not a stale choice: %s", doctor.stdout)
	}
	run(t, "-C", home, "sync").expect(t, exitFailed, "error [generated-modified] "+reviewerClaude+":")
	run(t, "-C", home, "sync", "--force").expect(t, exitOK, "")
	if exists(at(home, reviewerClaude)) {
		t.Error("force did not remove the orphan file")
	}
	run(t, "-C", home, "doctor").expect(t, exitOK, "")
}

func TestSourceDirSymlinkOrFileIsAPathConflict(t *testing.T) {
	for _, kind := range []string{"symlink", "file"} {
		home := subagentHome(t, "claude")
		run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
		run(t, "-C", home, "sync").expect(t, exitOK, "")
		generated := read(t, at(home, reviewerClaude))
		moved := filepath.Join(t.TempDir(), "moved")
		if err := os.Rename(at(home, subagent.SourceDir), moved); err != nil {
			t.Fatal(err)
		}
		if kind == "symlink" {
			symlink(t, moved, at(home, subagent.SourceDir))
		} else {
			write(t, at(home, subagent.SourceDir), "not a directory\n")
		}
		want := "error [path-conflict] " + subagent.SourceDir
		for _, args := range [][]string{{"sync"}, {"sync", "--force"}, {"subagent", "sync", "--force"}, {"subagent", "list"}, {"doctor"}} {
			run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, want)
		}
		if !exists(at(home, reviewerClaude)) || read(t, at(home, reviewerClaude)) != generated {
			t.Errorf("%s: generated file changed or removed", kind)
		}
	}
}

func TestDoctorReportsInvalidSourceAndEditedLeftoverOfUnchosenSubagent(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	run(t, "-C", home, "sync").expect(t, exitOK, "")
	run(t, "-C", home, "subagent", "disable", "reviewer").expect(t, exitOK, "")
	write(t, at(home, reviewerClaude), "mine\n")
	write(t, at(home, subagent.SourceDir+"/reviewer.md"), brokenSource)
	doctor := run(t, "-C", home, "doctor")
	doctor.expect(t, exitFailed, "error [generated-modified] reviewer: "+reviewerClaude+" changed")
	if !strings.Contains(doctor.stdout, "warning: [subagent-invalid] reviewer: subagents/reviewer.md: description must be") {
		t.Errorf("doctor hid the invalid source:\n%s", doctor.stdout)
	}
	list := run(t, "-C", home, "subagent", "list")
	list.expect(t, exitOK, "")
	if !strings.Contains(list.stdout, "generated-modified") || !strings.Contains(list.stdout, "source: subagents/reviewer.md: description must be") {
		t.Errorf("list:\n%s", list.stdout)
	}
}

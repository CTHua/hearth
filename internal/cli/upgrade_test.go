package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
)

// p1Gitignore and p1Agents are the marked sections the first release wrote;
// NAME stands for the command name.
const (
	p1Gitignore = "# Managed by NAME; this block may be rewritten by a later NAME version.\n" +
		"# Product repos are cloned here by `NAME clone` and managed separately.\n" +
		"/products/\n" +
		"# Personal settings such as the ssh/https choice.\n" +
		"/NAME.local.toml\n"
	p1Agents = "## NAME（這一段由 NAME 管理，之後的版本會整段重寫）\n" +
		"- clone 全部產品：`NAME clone`；看每個產品的狀態：`NAME repo list`；檢查環境和設定：`NAME doctor`。\n"
	namePlaceholder = "NAME"
	teamOutside     = "team line\n"
)

func gitignorePath(home string) string {
	return filepath.Join(home, ".gitignore")
}

func agentsPath(home string) string {
	return filepath.Join(home, "AGENTS.md")
}

// replaceSection swaps the lines between begin and end for section.
func replaceSection(t *testing.T, text string, begin string, end string, section string) string {
	t.Helper()
	start := strings.Index(text, begin+"\n")
	stop := strings.Index(text, end+"\n")
	if start < 0 || stop < start {
		t.Fatalf("markers %q, %q not found", begin, end)
	}
	named := strings.ReplaceAll(section, namePlaceholder, brand.Name)
	return text[:start+len(begin)+1] + named + text[stop:]
}

// p1Home is a home repo as the first release left it, with team lines
// before and after both sections.
func p1Home(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	ignore := teamOutside + replaceSection(t, read(t, gitignorePath(home)), brand.GitignoreBegin, brand.GitignoreEnd, p1Gitignore) + teamOutside
	write(t, gitignorePath(home), ignore)
	agents := teamOutside + replaceSection(t, read(t, agentsPath(home)), brand.AgentsBegin, brand.AgentsEnd, p1Agents)
	write(t, agentsPath(home), agents)
	return home
}

func TestUpgradeCheckOnFreshHomeExitsZero(t *testing.T) {
	home := newHome(t)
	r := run(t, "-C", home, "upgrade", "--check")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "already up to date") {
		t.Errorf("stdout: %s", r.stdout)
	}
	write(t, gitignorePath(home), read(t, gitignorePath(home))+teamOutside)
	run(t, "-C", home, "upgrade", "--check").expect(t, exitOK, "")
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("upgrade --check created the personal file")
	}
}

func TestUpgradeRewritesP1SectionsOnly(t *testing.T) {
	home := p1Home(t)
	fresh := newHome(t)
	ignoreBefore := read(t, gitignorePath(home))
	agentsBefore := read(t, agentsPath(home))
	team := read(t, teamFile(home))

	r := run(t, "-C", home, "upgrade", "--check")
	r.expect(t, exitFailed, "")
	for _, want := range []string{"would update AGENTS.md (marked section)", "would update .gitignore (marked section)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("--check stdout lacks %q:\n%s", want, r.stdout)
		}
	}
	if read(t, gitignorePath(home)) != ignoreBefore || read(t, agentsPath(home)) != agentsBefore {
		t.Fatal("--check wrote a file")
	}

	r = run(t, "-C", home, "upgrade")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "updated AGENTS.md (marked section)") || !strings.Contains(r.stdout, "commit AGENTS.md, .gitignore") {
		t.Errorf("stdout: %s", r.stdout)
	}
	ignore := read(t, gitignorePath(home))
	for _, want := range []string{"/vendor/", "/" + brand.StateDir + "/", "/.claude/skills/", "/.agents/skills/", "/.mcp.json", "/.codex/", "/.claude/agents/"} {
		if !strings.Contains(ignore, "\n"+want+"\n") {
			t.Errorf(".gitignore lacks %s:\n%s", want, ignore)
		}
	}
	// Outside the sections every byte is kept: the result is the fresh
	// template with the team lines around it.
	if want := teamOutside + read(t, gitignorePath(fresh)) + teamOutside; ignore != want {
		t.Errorf(".gitignore:\n%s\nwant:\n%s", ignore, want)
	}
	if want := teamOutside + read(t, agentsPath(fresh)); read(t, agentsPath(home)) != want {
		t.Errorf("AGENTS.md:\n%s", read(t, agentsPath(home)))
	}
	if read(t, teamFile(home)) != team {
		t.Error("team file changed")
	}
	if _, err := os.Stat(localFile(home)); err == nil {
		t.Error("upgrade created the personal file")
	}

	r = run(t, "-C", home, "upgrade")
	if r.code != exitOK || !strings.Contains(r.stdout, "already up to date") {
		t.Errorf("second run: %+v", r)
	}
	run(t, "-C", home, "upgrade", "--check").expect(t, exitOK, "")
}

func TestUpgradeRewritesHandEditedSection(t *testing.T) {
	home := newHome(t)
	agents := read(t, agentsPath(home))
	edited := strings.Replace(agents, brand.AgentsEnd, "- a line someone added\n"+brand.AgentsEnd, 1)
	write(t, agentsPath(home), edited)
	run(t, "-C", home, "upgrade").expect(t, exitOK, "")
	if read(t, agentsPath(home)) != agents {
		t.Errorf("AGENTS.md:\n%s", read(t, agentsPath(home)))
	}
}

func TestUpgradeTrailingSpaceCounts(t *testing.T) {
	home := newHome(t)
	ignore := read(t, gitignorePath(home))
	write(t, gitignorePath(home), strings.Replace(ignore, "/products/\n", "/products/ \n", 1))
	run(t, "-C", home, "upgrade", "--check").expect(t, exitFailed, "")
}

func TestUpgradeBrokenMarkersWritesNothing(t *testing.T) {
	cases := map[string]func(string) string{
		"missing":  func(text string) string { return strings.Replace(text, brand.GitignoreEnd+"\n", "", 1) },
		"repeated": func(text string) string { return text + brand.GitignoreBegin + "\n" },
		"out of order": func(text string) string {
			text = strings.Replace(text, brand.GitignoreBegin+"\n", "", 1)
			return brand.GitignoreEnd + "\n" + strings.Replace(text, brand.GitignoreEnd+"\n", brand.GitignoreBegin+"\n", 1)
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			home := p1Home(t)
			broken := breakIt(read(t, gitignorePath(home)))
			write(t, gitignorePath(home), broken)
			agents := read(t, agentsPath(home))
			for _, args := range [][]string{{"upgrade"}, {"upgrade", "--check"}} {
				run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, "error [config] .gitignore")
			}
			if read(t, gitignorePath(home)) != broken || read(t, agentsPath(home)) != agents {
				t.Error("a file was written")
			}
		})
	}
}

func TestUpgradeIgnoresVersionGateOfOlderSchemaButNotNewer(t *testing.T) {
	home := newHome(t)
	text := strings.Replace(read(t, teamFile(home)), "schema_version = 2", "schema_version = 3", 1)
	write(t, teamFile(home), text)
	run(t, "-C", home, "upgrade").expect(t, exitBad, "error [version]")
	// No migration leads up from schema 0.
	write(t, teamFile(home), strings.Replace(text, "schema_version = 3", "schema_version = 0", 1))
	run(t, "-C", home, "upgrade").expect(t, exitBad, "error [config]")
}

func TestDoctorWarnsAboutOutdatedSection(t *testing.T) {
	home := p1Home(t)
	r := run(t, "-C", home, "doctor")
	r.expect(t, exitOK, "")
	for _, want := range []string{
		"warning: AGENTS.md: the marked section differs from this version of " + brand.Name + "; run `" + brand.Name + " upgrade`",
		"warning: .gitignore: the marked section differs",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
	run(t, "-C", home, "upgrade").expect(t, exitOK, "")
	if r := run(t, "-C", home, "doctor"); strings.Contains(r.stdout, "marked section differs") {
		t.Errorf("still warns after upgrade:\n%s", r.stdout)
	}
}

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gittest"
)

// clonedHome has app cloned over ssh, then the protocol switched to https.
func clonedHome(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, sshURL("app"), remote)
	gittest.Redirect(t, httpsURL("app"), remote)
	run(t, "-C", home, "repo", "add", sshURL("app"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "ssh").expect(t, exitOK, "")
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	return home
}

func TestFixRemoteChangesOnlyProtocolDifferences(t *testing.T) {
	home := clonedHome(t)
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, httpsURL("other"), remote)
	run(t, "-C", home, "repo", "add", httpsURL("other"), "--branch", "main").expect(t, exitOK, "")
	run(t, "-C", home, "clone", "other").expect(t, exitOK, "")
	gittest.Git(t, product(home, "other"), "remote", "set-url", "origin", "https://git.example.invalid/acme/elsewhere.git")
	r := run(t, "-C", home, "repo", "fix-remote")
	r.expect(t, exitFailed, "error [remote-mismatch] other:")
	if !strings.Contains(r.stdout, "fixed app") {
		t.Errorf("stdout: %s", r.stdout)
	}
	if got := origin(t, product(home, "app")); got != httpsURL("app") {
		t.Errorf("app origin %s, want %s", got, httpsURL("app"))
	}
	if got := origin(t, product(home, "other")); got != "https://git.example.invalid/acme/elsewhere.git" {
		t.Errorf("other origin changed to %s", got)
	}
	r = run(t, "-C", home, "repo", "fix-remote", "app")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "unchanged app") {
		t.Errorf("second run: %s", r.stdout)
	}
	run(t, "-C", home, "repo", "fix-remote", "nope").expect(t, exitBad, "error [usage]")
}

func TestFixRemoteLeavesMissingAndConflictsAlone(t *testing.T) {
	home := homeWith(t, simple(t, "missing", "plain")...)
	if err := os.MkdirAll(product(home, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := run(t, "-C", home, "repo", "fix-remote")
	r.expect(t, exitFailed, "error [path-conflict] plain:")
	if !strings.Contains(r.stdout, "unchanged missing") || exists(product(home, "missing")) {
		t.Errorf("stdout: %s", r.stdout)
	}
}

func TestDoctorHealthyHomeExitsZero(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	r := run(t, "-C", home, "doctor")
	r.expect(t, exitOK, "")
	for _, want := range []string{"ok: git version", "ok: protocol https", "ok: app at products/app"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestDoctorWarningsAloneExitZero(t *testing.T) {
	home := newHome(t)
	run(t, "-C", home, "repo", "add", httpsURL("app"), "--branch", "main").expect(t, exitOK, "")
	r := run(t, "-C", home, "doctor")
	r.expect(t, exitOK, "")
	if !strings.Contains(r.stdout, "warning: protocol is not set") || !strings.Contains(r.stdout, "warning: app is not cloned") {
		t.Errorf("stdout: %s", r.stdout)
	}
	if exists(localFile(home)) || exists(filepath.Join(home, "products")) {
		t.Error("doctor wrote something")
	}
}

func TestDoctorCheckoutProblemsExitOne(t *testing.T) {
	home := clonedHome(t)
	r := run(t, "-C", home, "doctor")
	r.expect(t, exitFailed, "error [remote-mismatch] app:")
	if !strings.Contains(r.stderr, "repo fix-remote app") {
		t.Errorf("no fix-remote hint: %s", r.stderr)
	}
	if got := origin(t, product(home, "app")); got != sshURL("app") {
		t.Errorf("doctor changed origin to %s", got)
	}
	if err := os.RemoveAll(product(home, "app")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(product(home, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", home, "doctor").expect(t, exitFailed, "error [path-conflict] app:")
}

func TestDoctorSetupProblemsExitTwo(t *testing.T) {
	home := newHome(t)
	agents := filepath.Join(home, "AGENTS.md")
	original := read(t, agents)
	cases := map[string]string{
		"missing":   strings.Replace(original, brand.AgentsEnd+"\n", "", 1),
		"repeated":  original + brand.AgentsBegin + "\n",
		"reordered": strings.Replace(strings.Replace(original, brand.AgentsBegin, "@@", 1), brand.AgentsEnd, brand.AgentsBegin, 1),
		"inline":    strings.Replace(original, brand.AgentsBegin, "x "+brand.AgentsBegin, 1),
	}
	for label, text := range cases {
		write(t, agents, strings.ReplaceAll(text, "@@", brand.AgentsEnd))
		r := run(t, "-C", home, "doctor")
		if r.code != exitBad || !strings.Contains(r.stderr, "error [config] AGENTS.md") {
			t.Errorf("%s: exit %d stderr %s", label, r.code, r.stderr)
		}
	}
	write(t, agents, original)
	if err := os.Remove(filepath.Join(home, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", home, "doctor").expect(t, exitBad, "error [config] .gitignore")
	write(t, teamFile(home), "schema_version = 2\n"+brand.MinVersionKey+" = \"0.1.0\"\n")
	run(t, "-C", home, "doctor").expect(t, exitBad, "error [version]")
	write(t, teamFile(home), "schema_version = 1\n")
	run(t, "-C", home, "doctor").expect(t, exitBad, "error [config]")
	run(t, "-C", t.TempDir(), "doctor").expect(t, exitBad, "error [config]")
}

func TestDoctorWithoutGitExitsTwo(t *testing.T) {
	home := newHome(t)
	t.Setenv("PATH", t.TempDir())
	r := run(t, "-C", home, "doctor")
	r.expect(t, exitBad, "error [env] git not found")
	if !strings.Contains(r.stdout, "ok: "+brand.TeamFile) {
		t.Errorf("config not checked without git: %s", r.stdout)
	}
}

func TestVersionAndHelpPrintNoDevWarning(t *testing.T) {
	home := newHome(t)
	for _, args := range [][]string{{"version"}, {"help"}} {
		r := run(t, append([]string{"-C", home}, args...)...)
		if r.code != exitOK || r.stderr != "" {
			t.Errorf("%v: exit %d stderr %q", args, r.code, r.stderr)
		}
	}
}

func TestEveryGatedCommandHonoursVersionGate(t *testing.T) {
	home := newHome(t)
	text := strings.Replace(read(t, teamFile(home)), "schema_version = 1", "schema_version = 2", 1)
	write(t, teamFile(home), text)
	gated := [][]string{{"clone"}, {"repo", "fix-remote"}, {"doctor"}, {"repo", "list"}, {"config", "protocol", "ssh"}}
	for _, args := range gated {
		run(t, append([]string{"-C", home}, args...)...).expect(t, exitBad, "error [version]")
	}
	if exists(localFile(home)) || exists(filepath.Join(home, "products")) || read(t, teamFile(home)) != text {
		t.Error("a gated command wrote a file")
	}
}

func TestCommandsWorkFromInsideACheckout(t *testing.T) {
	home := homeWith(t, simple(t, "app")...)
	run(t, "-C", home, "clone").expect(t, exitOK, "")
	inside := filepath.Join(product(home, "app"), "src")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(inside)
	for _, args := range [][]string{{"clone"}, {"repo", "list"}, {"repo", "fix-remote"}, {"doctor"}, {"config", "protocol"}} {
		run(t, args...).expect(t, exitOK, "")
	}
}

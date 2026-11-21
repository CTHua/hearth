package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/cli"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gittest"
)

const (
	httpsPersonal = "schema_version = 3\nprotocol = \"https\"\n"
	skillSummary  = core.SummaryPrefix + "0 ready"
)

// choose rewrites the personal file of a homeWith home with a repos line.
func choose(t *testing.T, home string, reposLine string) {
	t.Helper()
	write(t, localFile(home), httpsPersonal+reposLine+"\n")
}

func TestCloneWithoutNamesTakesOnlyChosenRepos(t *testing.T) {
	home := homeWith(t, simple(t, "a", "b", "c")...)
	choose(t, home, `repos = ["a", "gone"]`)
	personal := read(t, localFile(home))
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	summary := core.BatchResult{Done: []string{"a"}, DoneLabel: "cloned", SkippedLabel: "skipped"}.Summary()
	if want := "cloned a -> products/a (main)\n" + summary; r.stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, want)
	}
	if exists(product(home, "b")) || exists(product(home, "c")) {
		t.Error("a repo that is not chosen was cloned")
	}
	// A named repo is cloned even when it is not chosen, and the choice stays.
	run(t, "-C", home, "clone", "b").expect(t, exitOK, "")
	if !exists(product(home, "b")) {
		t.Error("named repo b not cloned")
	}
	if read(t, localFile(home)) != personal {
		t.Errorf("personal file changed:\n%s", read(t, localFile(home)))
	}
}

func TestCloneWithNoneChosenSaysSo(t *testing.T) {
	home := homeWith(t, simple(t, "a")...)
	choose(t, home, `repos = []`)
	r := run(t, "-C", home, "clone")
	r.expect(t, exitOK, "")
	if r.stdout != "No repos selected in "+brand.LocalFile+"\n" {
		t.Errorf("stdout %q", r.stdout)
	}
	if exists(product(home, "a")) {
		t.Error("a was cloned")
	}
}

// With nothing to clone, clone says so before it looks for git.
func TestCloneWithNothingToCloneNeedsNoGit(t *testing.T) {
	cases := map[string]struct {
		build func(t *testing.T) string
		want  string
	}{
		"no repos": {
			build: func(t *testing.T) string { return newHome(t) },
			want:  "No repos configured in " + brand.TeamFile + "\n",
		},
		"none chosen": {
			build: func(t *testing.T) string {
				home := homeWith(t, simple(t, "a")...)
				choose(t, home, `repos = []`)
				return home
			},
			want: "No repos selected in " + brand.LocalFile + "\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home := c.build(t)
			t.Setenv("PATH", t.TempDir())
			r := run(t, "-C", home, "clone")
			r.expect(t, exitOK, "")
			if r.stdout != c.want {
				t.Errorf("stdout %q, want %q", r.stdout, c.want)
			}
			if strings.Contains(r.stdout, core.SummaryPrefix) {
				t.Errorf("summary printed:\n%s", r.stdout)
			}
		})
	}
}

func TestSyncClonesChosenMissingReposFirst(t *testing.T) {
	home := homeWith(t, simple(t, "a", "b", "c")...)
	choose(t, home, `repos = ["a", "b"]`)
	// a is already cloned, so only b is cloned by sync.
	run(t, "-C", home, "clone", "a").expect(t, exitOK, "")
	r := run(t, "-C", home, "sync")
	r.expect(t, exitOK, "")
	want := "cloned b -> products/b (main)\n" + core.SummaryPrefix + "1 cloned, 0 skipped, 0 failed\n"
	if !strings.HasPrefix(r.stdout, want) {
		t.Errorf("stdout does not start with the clone step:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout[len(want):], skillSummary) {
		t.Errorf("skill sync did not follow:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, " a ") || strings.Contains(r.stdout, "cloned a") {
		t.Errorf("a was touched:\n%s", r.stdout)
	}
	if !exists(product(home, "b")) || exists(product(home, "c")) {
		t.Error("wrong repos cloned")
	}
}

func TestSyncCloneFailureLetsTheOtherPartsRun(t *testing.T) {
	home := subagentHome(t, "claude")
	run(t, "-C", home, "subagent", "enable", "reviewer").expect(t, exitOK, "")
	good := gittest.BareRemote(t, "main")
	gittest.Redirect(t, httpsURL("good"), good)
	gittest.Redirect(t, httpsURL("bad"), filepath.Join(t.TempDir(), "missing.git"))
	for _, name := range []string{"bad", "good"} {
		run(t, "-C", home, "repo", "add", httpsURL(name), "--branch", "main").expect(t, exitOK, "")
	}
	run(t, "-C", home, "config", "protocol", "https").expect(t, exitOK, "")
	r := run(t, "-C", home, "sync")
	r.expect(t, exitFailed, "bad")
	if !strings.Contains(r.stdout, "cloned good -> products/good (main)") || !strings.Contains(r.stdout, "1 cloned, 0 skipped, 1 failed") {
		t.Errorf("clone step:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, core.SummaryPrefix+"1 ready") || !exists(at(home, ".mcp.json")) || !exists(at(home, reviewerClaude)) {
		t.Errorf("skill, MCP or subagent sync did not run:\n%s", r.stdout)
	}
}

func TestSyncProtocolDetectionFailureLetsTheOtherPartsRun(t *testing.T) {
	home := newHome(t)
	missing := filepath.Join(t.TempDir(), "missing.git")
	gittest.Redirect(t, httpsURL("app"), missing)
	gittest.Redirect(t, sshURL("app"), missing)
	run(t, "-C", home, "repo", "add", httpsURL("app"), "--branch", "main").expect(t, exitOK, "")
	r := run(t, "-C", home, "sync")
	if r.code == exitOK || !strings.Contains(r.stdout, skillSummary) {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	if exists(localFile(home)) && strings.Contains(read(t, localFile(home)), "protocol") {
		t.Error("a protocol was saved")
	}
}

func TestInterruptedSyncCloneStopsTheRest(t *testing.T) {
	home := homeWith(t, simple(t, "a")...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := cli.Run(ctx, []string{"-C", home, "sync"}, &stdout, &stderr, false); code != 130 {
		t.Errorf("exit %d, want 130; stderr %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), skillSummary) {
		t.Errorf("skill sync ran after the interrupt:\n%s", stdout.String())
	}
}

// partsOutput runs the three syncs `sync` used to run, in its order, and
// keeps the dev build warning once, as one sync prints it.
func partsOutput(t *testing.T, home string) result {
	t.Helper()
	var all result
	for i, part := range []string{"skill", "mcp", "subagent"} {
		r := run(t, "-C", home, part, "sync")
		stderr := r.stderr
		if i > 0 {
			stderr = withoutDevWarning(stderr)
		}
		all.stdout += r.stdout
		all.stderr += stderr
		all.code = max(all.code, r.code)
	}
	return all
}

func withoutDevWarning(text string) string {
	var kept []string
	for _, line := range strings.SplitAfter(text, "\n") {
		if !strings.HasPrefix(line, "warning: dev build;") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

// With nothing to clone, sync prints what the three parts print, writes
// no personal file and needs no git for the clone step.
func TestSyncWithNothingToCloneIsUnchanged(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"no repos": func(t *testing.T) string { return newHome(t) },
		"none chosen": func(t *testing.T) string {
			home := newHome(t)
			run(t, "-C", home, "repo", "add", appSSH, "--branch", "main").expect(t, exitOK, "")
			write(t, localFile(home), "schema_version = 3\nrepos = []\n")
			return home
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			home := build(t)
			personal := exists(localFile(home))
			var before string
			if personal {
				before = read(t, localFile(home))
			}
			want := partsOutput(t, home)
			got := run(t, "-C", home, "sync")
			if got != want {
				t.Errorf("sync %+v\nparts %+v", got, want)
			}
			if exists(localFile(home)) != personal || (personal && read(t, localFile(home)) != before) {
				t.Error("personal file written")
			}
			// Without git the clone step still prints nothing; only the
			// parts that need git complain.
			t.Setenv("PATH", t.TempDir())
			if got, want := run(t, "-C", home, "sync"), partsOutput(t, home); got != want {
				t.Errorf("without git: sync %+v\nparts %+v", got, want)
			}
		})
	}
	t.Run("every chosen repo has something at its path", func(t *testing.T) {
		home := listHome(t)
		write(t, localFile(home), httpsPersonal+`repos = ["ok", "mismatch", "conflict"]`+"\n")
		before := read(t, localFile(home))
		want := partsOutput(t, home)
		if got := run(t, "-C", home, "sync"); got != want {
			t.Errorf("sync %+v\nparts %+v", got, want)
		}
		if read(t, localFile(home)) != before {
			t.Error("personal file written")
		}
	})
}

func TestRepoListAndDoctorShowNotSelected(t *testing.T) {
	home := listHome(t)
	// conflict is not chosen but has something at its path, so it is shown
	// and checked as before.
	write(t, localFile(home), httpsPersonal+`repos = ["ok", "mismatch"]`+"\n")
	r := run(t, "-C", home, "repo", "list")
	r.expect(t, exitOK, "")
	want := map[string]string{"ok": "ok", "mismatch": "remote-mismatch", "conflict": "path-conflict", "missing": core.NotSelectedLabel}
	for name, state := range want {
		if got := statusOf(t, r.stdout, name); got != state {
			t.Errorf("%s: %s, want %s", name, got, state)
		}
	}
	d := run(t, "-C", home, "doctor")
	d.expect(t, exitFailed, "error [path-conflict] conflict:")
	if strings.Contains(d.stdout, "missing is not cloned") || !strings.Contains(d.stdout, "ok: missing "+core.NotSelectedLabel) {
		t.Errorf("doctor stdout:\n%s", d.stdout)
	}
}

func TestDoctorPassesWithOnlyUnchosenReposMissing(t *testing.T) {
	home := homeWith(t, simple(t, "a", "b")...)
	run(t, "-C", home, "clone", "a").expect(t, exitOK, "")
	choose(t, home, `repos = ["a"]`)
	d := run(t, "-C", home, "doctor")
	d.expect(t, exitOK, "")
	if strings.Contains(d.stdout, "b is not cloned") {
		t.Errorf("doctor stdout:\n%s", d.stdout)
	}
	// Chosen but not cloned is reported as before.
	choose(t, home, `repos = ["a", "b"]`)
	d = run(t, "-C", home, "doctor")
	d.expect(t, exitOK, "")
	if !strings.Contains(d.stdout, "warning: b is not cloned at products/b") {
		t.Errorf("doctor stdout:\n%s", d.stdout)
	}
}

func TestFixRemoteStillCoversUnchosenCheckouts(t *testing.T) {
	home := listHome(t)
	write(t, localFile(home), httpsPersonal+"repos = []\n")
	r := run(t, "-C", home, "repo", "fix-remote")
	if !strings.Contains(r.stdout, "fixed mismatch: origin ") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
}

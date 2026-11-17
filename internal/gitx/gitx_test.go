package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

const remoteURL = "https://git.example.invalid/acme/app.git"

func TestDefaultBranchReadsRemoteHead(t *testing.T) {
	gittest.Redirect(t, remoteURL, gittest.BareRemote(t, "trunk"))
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	branch, found, err := g.DefaultBranch(context.Background(), remoteURL)
	if err != nil || !found || branch != "trunk" {
		t.Fatalf("DefaultBranch = %q, %v, %v; want trunk", branch, found, err)
	}
}

func TestRemoteHeadReadsHeadCommit(t *testing.T) {
	remote := gittest.BareRemote(t, "trunk")
	gittest.Redirect(t, remoteURL, remote)
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	commit, found, err := g.RemoteHead(context.Background(), remoteURL)
	want := strings.TrimSpace(gittest.Git(t, remote, "rev-parse", "HEAD"))
	if err != nil || !found || commit != want {
		t.Fatalf("RemoteHead = %q, %v, %v; want %s", commit, found, err, want)
	}
}

func TestDefaultBranchUnreachableIsRunError(t *testing.T) {
	gittest.Redirect(t, remoteURL, filepath.Join(t.TempDir(), "missing.git"))
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = g.DefaultBranch(context.Background(), remoteURL)
	var runErr *gitx.RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("err = %v, want *RunError", err)
	}
}

func TestExtTransportIsBlocked(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	gittest.Redirect(t, remoteURL, "ext::sh -c touch% "+marker)
	gittest.Git(t, "", "config", "--global", "protocol.ext.allow", "always")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "protocol.ext.allow") })
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.DefaultBranch(context.Background(), remoteURL); err == nil {
		t.Error("ls-remote through ext:: succeeded")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("ext:: command ran")
	}
}

func TestInheritedGitDirDoesNotLeak(t *testing.T) {
	other := t.TempDir()
	gittest.Git(t, other, "init", "--quiet")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := g.Run(context.Background(), dir, "rev-parse", "--show-toplevel"); err == nil {
		t.Error("git ran against the inherited GIT_DIR")
	}
	cmd := g.Cmd(context.Background(), dir, "version")
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "GIT_DIR=") {
			t.Errorf("environment still has %s", entry)
		}
	}
}

func TestRunReportsInterruption(t *testing.T) {
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Run(ctx, "", "version"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestMissingGitIsEnvError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := gitx.New(context.Background()); err == nil || !strings.Contains(err.Error(), "[env]") {
		t.Errorf("err = %v, want env error", err)
	}
}

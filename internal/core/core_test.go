package core_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/version"
)

const (
	firstSSH    = "git@git.example.invalid:acme/first.git"
	firstHTTPS  = "https://git.example.invalid/acme/first.git"
	secondSSH   = "git@git.example.invalid:acme/second.git"
	secondHTTPS = "https://git.example.invalid/acme/second.git"
	mainBranch  = "main"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

func newService(t *testing.T) *core.Service {
	t.Helper()
	ctx := context.Background()
	g, err := gitx.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "home")
	if err := scaffold.Init(ctx, g, dir); err != nil {
		t.Fatal(err)
	}
	home, err := config.Load(dir, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	return core.New(home)
}

func addRepo(t *testing.T, s *core.Service, url string) {
	t.Helper()
	branch := mainBranch
	if _, err := s.AddRepo(context.Background(), core.AddRepoParams{URL: url, Branch: &branch}); err != nil {
		t.Fatal(err)
	}
}

func TestDetectProtocolWithoutReposIsRecognizable(t *testing.T) {
	_, err := newService(t).DetectProtocol(context.Background())
	if !errors.Is(err, core.ErrNoRepos) {
		t.Errorf("err %v, want ErrNoRepos", err)
	}
}

func TestDetectProtocolTriesEveryRepoAndOverwrites(t *testing.T) {
	s := newService(t)
	addRepo(t, s, firstHTTPS)
	addRepo(t, s, secondHTTPS)
	missing := filepath.Join(t.TempDir(), "missing.git")
	gittest.Redirect(t, firstSSH, missing)
	gittest.Redirect(t, firstHTTPS, missing)
	// Only the second repo answers, and only over https.
	gittest.Redirect(t, secondSSH, missing)
	gittest.Redirect(t, secondHTTPS, gittest.BareRemote(t, mainBranch))
	if err := s.SetProtocol("ssh"); err != nil {
		t.Fatal(err)
	}
	protocol, err := s.DetectProtocol(context.Background())
	if err != nil || protocol != "https" {
		t.Fatalf("protocol %q, err %v", protocol, err)
	}
	if got := s.Home().Personal.Protocol; got != "https" {
		t.Errorf("loaded protocol %q", got)
	}
	reloaded, err := config.Load(s.Home().Root, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Personal.Protocol != "https" {
		t.Errorf("saved protocol %q", reloaded.Personal.Protocol)
	}
}

func TestCloneStreamsAndReturnsSummary(t *testing.T) {
	s := newService(t)
	addRepo(t, s, firstHTTPS)
	gittest.Redirect(t, firstHTTPS, gittest.BareRemote(t, mainBranch))
	if err := s.SetProtocol("https"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var errw bytes.Buffer
	result, err := s.Clone(context.Background(), nil, core.Output{Out: &out, Err: &errw})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Done) != 1 || result.Done[0] != "first" {
		t.Errorf("result %+v", result)
	}
	if !strings.HasSuffix(out.String(), result.Summary()) || !strings.Contains(result.Summary(), "Summary: 1 cloned, 0 skipped, 0 failed") {
		t.Errorf("stdout:\n%s", out.String())
	}
}

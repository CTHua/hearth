package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/tui"
	"github.com/CTHua/hearth/internal/version"
)

const (
	appSSH       = "git@git.example.invalid:acme/app.git"
	startTimeout = 10 * time.Second
	pollEvery    = 20 * time.Millisecond
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

// TestCancelledCloneLeavesNoTempDir uses the real core: a fake ssh that
// never answers keeps git clone running until ctrl+c cancels it.
func TestCancelledCloneLeavesNoTempDir(t *testing.T) {
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
	service := core.New(home)
	branch := "main"
	if _, err := service.AddRepo(ctx, core.AddRepoParams{URL: appSSH, Branch: &branch}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetProtocol("ssh"); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	started := filepath.Join(bin, "started")
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ntouch "+started+"\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", filepath.Join(bin, "ssh"))
	gittest.Git(t, "", "config", "--global", "protocol.ssh.allow", "always")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "protocol.ssh.allow") })

	m := tui.New(ctx, service)
	cmd := press(m, "c")
	done := make(chan tea.Msg)
	go func() { done <- cmd() }()
	deadline := time.Now().Add(startTimeout)
	for !exists(started) && time.Now().Before(deadline) {
		time.Sleep(pollEvery)
	}
	if !exists(started) {
		t.Fatal("clone never reached the remote; test is invalid")
	}
	if isQuit(press(m, "ctrl+c")) {
		t.Fatal("ctrl+c quit while the clone ran")
	}
	m.Update(<-done)
	expectScreen(t, m, "interrupted")
	products := filepath.Join(dir, config.ProductsDir)
	entries, err := os.ReadDir(products)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), brand.CloneTempPrefix) || entry.Name() == "app" {
			t.Errorf("left behind %s", entry.Name())
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

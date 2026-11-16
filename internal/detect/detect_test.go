package detect_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/detect"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

const (
	appSSH   = "git@git.example.invalid:acme/app.git"
	appHTTPS = "https://git.example.invalid/acme/app.git"
	webSSH   = "git@git.example.invalid:acme/web.git"
	webHTTPS = "https://git.example.invalid/acme/web.git"

	envSSHCommand = "GIT_SSH_COMMAND"
)

func newGit(t *testing.T) *gitx.Git {
	t.Helper()
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func missing(t *testing.T) string {
	return filepath.Join(t.TempDir(), "missing.git")
}

func TestSSHWinsWhenBothWork(t *testing.T) {
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, appSSH, remote)
	gittest.Redirect(t, appHTTPS, remote)
	got, err := detect.Protocol(context.Background(), newGit(t), []string{appHTTPS})
	if err != nil || got != giturl.SSH {
		t.Fatalf("Protocol = %q, %v; want ssh", got, err)
	}
}

func TestHTTPSWhenOnlyHTTPSWorks(t *testing.T) {
	gittest.Redirect(t, appSSH, missing(t))
	gittest.Redirect(t, appHTTPS, gittest.BareRemote(t, "main"))
	got, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH})
	if err != nil || got != giturl.HTTPS {
		t.Fatalf("Protocol = %q, %v; want https", got, err)
	}
}

func TestFirstReachableRepoDecides(t *testing.T) {
	gittest.Redirect(t, appSSH, missing(t))
	gittest.Redirect(t, appHTTPS, missing(t))
	remote := gittest.BareRemote(t, "main")
	gittest.Redirect(t, webSSH, missing(t))
	gittest.Redirect(t, webHTTPS, remote)
	got, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH, webSSH})
	if err != nil || got != giturl.HTTPS {
		t.Fatalf("Protocol = %q, %v; want https from the second repo", got, err)
	}
}

func TestNothingReachableIsEnvError(t *testing.T) {
	gittest.Redirect(t, appSSH, missing(t))
	gittest.Redirect(t, appHTTPS, missing(t))
	_, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH})
	var e *apperr.Error
	if !errors.As(err, &e) || e.Category != apperr.Env {
		t.Fatalf("err = %v, want env error", err)
	}
	if !strings.Contains(e.Message, appSSH) || !strings.Contains(e.Message, appHTTPS) {
		t.Errorf("message does not list the tried URLs: %s", e.Message)
	}
}

// fakeSSH puts a script first in PATH as ssh, recording its arguments in log.
func fakeSSH(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	gittest.Git(t, "", "config", "--global", "protocol.ssh.allow", "always")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "protocol.ssh.allow") })
	return log
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ssh was not run: %v", err)
	}
	return string(data)
}

func TestSSHRunsInBatchModeByDefault(t *testing.T) {
	log := fakeSSH(t, "exit 1")
	gittest.Unsetenv(t, envSSHCommand)
	if _, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH}); err == nil {
		t.Fatal("unreachable ssh reported as reachable")
	}
	if !strings.Contains(readLog(t, log), "BatchMode=yes") {
		t.Errorf("ssh args %q lack BatchMode=yes", readLog(t, log))
	}
}

func TestUserSSHCommandIsKept(t *testing.T) {
	log := fakeSSH(t, "exit 1")
	t.Setenv(envSSHCommand, "ssh -o UserChoice=yes")
	if _, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH}); err == nil {
		t.Fatal("unreachable ssh reported as reachable")
	}
	args := readLog(t, log)
	if !strings.Contains(args, "UserChoice=yes") || strings.Contains(args, "BatchMode") {
		t.Errorf("ssh args %q, want the user's command unchanged", args)
	}
}

func TestCoreSSHCommandIsKept(t *testing.T) {
	log := fakeSSH(t, "exit 1")
	gittest.Unsetenv(t, envSSHCommand)
	gittest.Git(t, "", "config", "--global", "core.sshCommand", "ssh -o FromConfig=yes")
	t.Cleanup(func() { gittest.Git(t, "", "config", "--global", "--unset", "core.sshCommand") })
	if _, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH}); err == nil {
		t.Fatal("unreachable ssh reported as reachable")
	}
	args := readLog(t, log)
	if !strings.Contains(args, "FromConfig=yes") || strings.Contains(args, "BatchMode") {
		t.Errorf("ssh args %q, want core.sshCommand unchanged", args)
	}
}

func TestHangingAttemptIsKilledAfterTimeout(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	// exec keeps the PID, so the recorded PID is the sleep itself.
	fakeSSH(t, "echo $$ > "+pidFile+"\nexec sleep 30")
	gittest.Unsetenv(t, envSSHCommand)
	gittest.Redirect(t, appHTTPS, missing(t))
	previous := detect.Timeout
	// Long enough for git to start ssh (the macOS git shim alone can take
	// a few hundred milliseconds), far shorter than the sleep.
	detect.Timeout = 2 * time.Second
	t.Cleanup(func() { detect.Timeout = previous })
	start := time.Now()
	_, err := detect.Protocol(context.Background(), newGit(t), []string{appSSH})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; the hanging ssh attempt was not cut off", elapsed)
	}
	var e *apperr.Error
	if !errors.As(err, &e) || e.Category != apperr.Env {
		t.Errorf("err = %v, want env error", err)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("fake ssh never started: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !processGone(pid, 3*time.Second) {
		t.Errorf("ssh child %d is still running after the timeout", pid)
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Logf("cleanup kill %d: %v", pid, err)
		}
	}
}

// processGone polls until pid no longer exists; the killed child may need a
// moment to be reaped by init.
func processGone(pid int, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

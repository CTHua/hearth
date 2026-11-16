// Package gittest isolates tests from the user's git setup and the network:
// a throwaway global config, no system config, no inherited repository
// variables, and only the local file transport allowed.
package gittest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	envGlobalConfig = "GIT_CONFIG_GLOBAL"
	envNoSystem     = "GIT_CONFIG_NOSYSTEM"
	envNoPrompt     = "GIT_TERMINAL_PROMPT"
)

// globalSettings make any URL that was not redirected to a local path fail
// instead of reaching the network.
var globalSettings = [][2]string{
	{"user.name", "test"},
	{"user.email", "test@example.invalid"},
	{"init.defaultBranch", "main"},
	{"protocol.allow", "never"},
	{"protocol.file.allow", "always"},
}

// Main prepares the environment for every test in a package and runs them.
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "gittest-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	if err := isolate(filepath.Join(dir, "gitconfig")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func isolate(globalConfig string) error {
	out, err := exec.Command("git", "rev-parse", "--local-env-vars").Output()
	if err != nil {
		return fmt.Errorf("git rev-parse --local-env-vars: %w", err)
	}
	for _, name := range strings.Fields(string(out)) {
		if err := os.Unsetenv(name); err != nil {
			return err
		}
	}
	settings := map[string]string{envGlobalConfig: globalConfig, envNoSystem: "1", envNoPrompt: "0"}
	for name, value := range settings {
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	for _, setting := range globalSettings {
		if out, err := exec.Command("git", "config", "--global", setting[0], setting[1]).CombinedOutput(); err != nil {
			return fmt.Errorf("git config %s: %v: %s", setting[0], err, out)
		}
	}
	return nil
}

// Unsetenv removes name from the environment until the test ends.
func Unsetenv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// Git runs git in dir and fails the test when it fails.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// BareRemote returns a bare repo with one commit whose HEAD is branch.
func BareRemote(t testing.TB, branch string) string {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	Git(t, base, "init", "--quiet", "--bare", "--initial-branch", branch, remote)
	Git(t, base, "init", "--quiet", "--initial-branch", branch, seed)
	Git(t, seed, "commit", "--quiet", "--allow-empty", "-m", "init")
	Git(t, seed, "push", "--quiet", remote, branch)
	return remote
}

// Redirect makes git fetch url from the local path target, until the test ends.
func Redirect(t testing.TB, url string, target string) {
	t.Helper()
	key := "url." + target + ".insteadOf"
	Git(t, "", "config", "--global", "--add", key, url)
	t.Cleanup(func() {
		cmd := exec.Command("git", "config", "--global", "--fixed-value", "--unset-all", key, url)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("git config --unset-all %s: %v: %s", key, err, out)
		}
	})
}

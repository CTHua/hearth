package dirswap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/dirswap"
)

const (
	oldText = "old\n"
	newText = "new\n"
	file    = "SKILL.md"
)

func makeDir(t *testing.T, dir string, text string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReplaceSwapsDirectories(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "skill")
	next := filepath.Join(base, "tmp", "checkout")
	stash := filepath.Join(base, "tmp", "old")
	makeDir(t, target, oldText)
	makeDir(t, next, newText)
	if err := dirswap.Replace(target, next, stash); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, target); got != newText {
		t.Errorf("target holds %q, want %q", got, newText)
	}
	if got := readText(t, stash); got != oldText {
		t.Errorf("stash holds %q, want %q", got, oldText)
	}
}

func TestFailedMoveRestoresOriginal(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "skill")
	stash := filepath.Join(base, "tmp", "old")
	makeDir(t, target, oldText)
	makeDir(t, filepath.Dir(stash), "")
	err := dirswap.Replace(target, filepath.Join(base, "tmp", "missing"), stash)
	if err == nil || !strings.Contains(err.Error(), "restored") {
		t.Fatalf("err = %v, want a restored failure", err)
	}
	if got := readText(t, target); got != oldText {
		t.Errorf("target holds %q after a failed swap, want %q", got, oldText)
	}
	if _, err := os.Lstat(stash); err == nil {
		t.Error("stash left behind after restoring")
	}
}

func TestFailedMoveAsideChangesNothing(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "skill")
	next := filepath.Join(base, "tmp", "checkout")
	makeDir(t, target, oldText)
	makeDir(t, next, newText)
	if err := dirswap.Replace(target, next, filepath.Join(base, "no-such-dir", "old")); err == nil {
		t.Fatal("Replace succeeded without a place to stash")
	}
	if got := readText(t, target); got != oldText {
		t.Errorf("target holds %q, want %q", got, oldText)
	}
	if got := readText(t, next); got != newText {
		t.Errorf("next holds %q, want %q", got, newText)
	}
}

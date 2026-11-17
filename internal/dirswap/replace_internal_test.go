package dirswap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The failure needs both moves back into target to fail, which a real file
// system cannot be made to do between two renames, so rename is injected.
func TestFailedRestoreKeepsStashAndSaysWhere(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "skill")
	stash := filepath.Join(base, "old")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("denied")
	calls := 0
	rename := func(from string, to string) error {
		calls++
		if calls == 1 {
			return os.Rename(from, to)
		}
		return denied
	}
	err := replace(rename, target, filepath.Join(base, "next"), stash)
	var stranded *StrandedError
	if !errors.As(err, &stranded) || stranded.Stash != stash || !errors.Is(err, denied) {
		t.Fatalf("err = %v, want a *StrandedError at %s", err, stash)
	}
	if data, err := os.ReadFile(filepath.Join(stash, "SKILL.md")); err != nil || string(data) != "old\n" {
		t.Errorf("stash lost the original: %q, %v", data, err)
	}
}

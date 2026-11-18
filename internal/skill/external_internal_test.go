package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/dirswap"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

// twoCommitRemote returns a bare remote whose two commits both hold a
// top-level SKILL.md, and the two shas, oldest first.
func twoCommitRemote(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	bare := filepath.Join(base, "skills.git")
	gittest.Git(t, base, "init", "--quiet", "--initial-branch", "main", work)
	var shas []string
	for _, text := range []string{"v1\n", "v2\n"} {
		if err := os.WriteFile(filepath.Join(work, File), []byte(text), filePerm); err != nil {
			t.Fatal(err)
		}
		gittest.Git(t, work, "add", File)
		gittest.Git(t, work, "commit", "--quiet", "-m", text)
		shas = append(shas, strings.TrimSpace(gittest.Git(t, work, "rev-parse", "HEAD")))
	}
	gittest.Git(t, base, "clone", "--quiet", "--bare", work, bare)
	return bare, shas[0], shas[1]
}

// A swap that cannot put either checkout back must not let the temporary
// directory cleanup delete the only copy of the old one.
func TestStrandedSwapKeepsOldCheckoutAndSaysWhere(t *testing.T) {
	ctx := context.Background()
	g, err := gitx.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bare, first, second := twoCommitRemote(t)
	root := t.TempDir()
	s := config.Skill{Name: "demo", URL: bare, Commit: first, Subdir: config.RootSubdir}
	if _, err := installExternal(ctx, g, root, s, bare, io.Discard); err != nil {
		t.Fatal(err)
	}
	original := replaceDir
	t.Cleanup(func() { replaceDir = original })
	replaceDir = func(target string, next string, stash string) error {
		if err := os.Rename(target, stash); err != nil {
			return err
		}
		return &dirswap.StrandedError{Stash: stash, Err: errors.New("injected")}
	}
	s.Commit = second
	_, err = installExternal(ctx, g, root, s, bare, io.Discard)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Category != apperr.PathConflict {
		t.Fatalf("err = %v, want path-conflict", err)
	}
	marker := " is left at "
	at := strings.Index(e.Message, marker)
	if at < 0 {
		t.Fatalf("message does not say where the old checkout is: %s", e.Message)
	}
	stash := strings.Fields(e.Message[at+len(marker):])[0]
	stash = strings.TrimSuffix(stash, ";")
	data, err := os.ReadFile(filepath.Join(stash, File))
	if err != nil || string(data) != "v1\n" {
		t.Errorf("old checkout at %s: %q, %v", stash, data, err)
	}
	if head := strings.TrimSpace(gittest.Git(t, stash, "rev-parse", "HEAD")); head != first {
		t.Errorf("stash HEAD %s, want %s", head, first)
	}
}

func TestOnlyUnsupportedSymlinkErrorsAreSymlinkUnavailable(t *testing.T) {
	// A slice, not a map keyed by errno: ENOTSUP and EOPNOTSUPP are the
	// same number on Linux, which a map literal rejects as a duplicate key.
	cases := []struct {
		errno syscall.Errno
		want  bool
	}{
		{syscall.ENOTSUP, true},
		{syscall.EOPNOTSUPP, true},
		{syscall.EPERM, true},
		{syscall.EACCES, false},
		{syscall.EEXIST, false},
		{syscall.EROFS, false},
	}
	for _, c := range cases {
		errno, want := c.errno, c.want
		err := &os.LinkError{Op: "symlink", Old: "a", New: "b", Err: errno}
		if got := symlinkUnsupported(err); got != want {
			t.Errorf("%v: %v, want %v", errno, got, want)
		}
	}
	if symlinkUnsupported(fmt.Errorf("wrapped: %w", os.ErrNotExist)) {
		t.Error("ErrNotExist counted as unsupported")
	}
}

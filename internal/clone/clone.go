// Package clone brings one product checkout into place, or confirms the one
// that is already there, without ever modifying an existing checkout.
package clone

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

// Outcome is what happened to a repo that did not fail.
type Outcome string

const (
	Cloned  Outcome = "cloned"
	Skipped Outcome = "skipped"
)

const (
	checkoutDir   = "checkout"
	headRefPrefix = "refs/heads/"
	dirPerm       = 0o755
)

// Target says what to clone and how the URL came about, for messages.
type Target struct {
	Repo config.Repo
	// URL is the URL to clone and to compare origin with.
	URL string
	// Protocol is the personal protocol URL was converted with, or empty.
	Protocol string
}

// Targets converts each repo's team-file URL with protocol; a URL that does
// not convert is a config error naming the repo.
func Targets(repos []config.Repo, protocol string) ([]Target, error) {
	targets := make([]Target, 0, len(repos))
	for _, repo := range repos {
		url, err := giturl.Expected(repo.URL, protocol)
		if err != nil {
			return nil, apperr.ForRepo(apperr.Config, repo.Name, "%v", err)
		}
		targets = append(targets, Target{Repo: repo, URL: url, Protocol: protocol})
	}
	return targets, nil
}

// One clones t into place when its path is free, or checks the checkout that
// is there. A per-repo failure is an *apperr.Error naming the repo; any other
// error, such as an interrupt, should stop the whole run. Git's progress and
// its own errors go to progress.
func One(ctx context.Context, g *gitx.Git, root string, t Target, progress io.Writer) (Outcome, error) {
	status, err := checkout.Inspect(ctx, g, root, t.Repo.Path, t.URL)
	if err != nil {
		return "", err
	}
	switch status.State {
	case checkout.OK:
		return Skipped, nil
	case checkout.Missing:
		return Cloned, cloneInto(ctx, g, root, t, progress)
	case checkout.RemoteMismatch:
		return "", apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "%s", MismatchMessage(t, status))
	}
	return "", apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "%s", status.Detail)
}

// MismatchMessage explains a remote-mismatch, naming the team-file URL and the
// protocol when the expected URL was converted.
func MismatchMessage(t Target, status checkout.Status) string {
	message := status.Detail
	if t.URL != t.Repo.URL {
		message += fmt.Sprintf(" (from %q with protocol = %s in %s)", t.Repo.URL, t.Protocol, brand.LocalFile)
	}
	return message + "; remote left unchanged"
}

func cloneInto(ctx context.Context, g *gitx.Git, root string, t Target, progress io.Writer) error {
	target := filepath.Join(root, filepath.FromSlash(t.Repo.Path))
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "cannot prepare %s: %v", filepath.Dir(t.Repo.Path), err)
	}
	// The clone is made next to the target and renamed, so a failed or
	// interrupted clone never leaves a half-made checkout at the target.
	temp, err := os.MkdirTemp(parent, brand.CloneTempPrefix+t.Repo.Name+".")
	if err != nil {
		return apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "cannot prepare %s: %v", filepath.Dir(t.Repo.Path), err)
	}
	err = cloneAndMove(ctx, g, t, filepath.Join(temp, checkoutDir), target, progress)
	if removeErr := os.RemoveAll(temp); removeErr != nil {
		fmt.Fprintf(progress, "warning: could not remove temporary directory %s: %v\n", temp, removeErr)
	}
	return err
}

func cloneAndMove(ctx context.Context, g *gitx.Git, t Target, dest string, target string, progress io.Writer) error {
	cmd := g.Cmd(ctx, "", "-c", gitx.NoExtTransport, "clone", "--branch", t.Repo.Branch, "--", t.URL, dest)
	// stdout joins stderr so the caller's stdout stays one line per repo.
	cmd.Stdout = progress
	cmd.Stderr = progress
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return apperr.ForRepo(apperr.CloneFailed, t.Repo.Name, "git clone exited with %d", exitErr.ExitCode())
	}
	if err != nil {
		return apperr.ForRepo(apperr.CloneFailed, t.Repo.Name, "cannot run git clone: %v", err)
	}
	if err := verifyBranch(ctx, g, t, dest); err != nil {
		return err
	}
	// Limit: rename replaces an empty directory created at the target during
	// the clone; nothing is lost, so that counts as cloned.
	if err := os.Rename(dest, target); err != nil {
		return apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "cannot move checkout to %s: %v", t.Repo.Path, err)
	}
	return nil
}

func verifyBranch(ctx context.Context, g *gitx.Git, t Target, dest string) error {
	out, err := g.Run(ctx, "", "-C", dest, "symbolic-ref", "HEAD")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || strings.TrimSpace(out) != headRefPrefix+t.Repo.Branch {
		return apperr.ForRepo(apperr.CloneFailed, t.Repo.Name, "clone of %s is not on branch %s (is it a tag?)", t.URL, t.Repo.Branch)
	}
	return nil
}

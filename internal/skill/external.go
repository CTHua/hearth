package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/dirswap"
	"github.com/CTHua/hearth/internal/gitx"
)

const (
	checkoutDir = "checkout"
	stashDir    = "old"
	// notSymbolic is the exit code of `git symbolic-ref -q HEAD` on a
	// detached HEAD.
	notSymbolic      = 1
	noDetachedAdvice = "advice.detachedHead=false"
	commitPeel       = "^{commit}"
)

// replaceDir is dirswap.Replace; tests swap it to make the swap fail part
// way, which a real file system cannot be made to do between two renames.
var replaceDir = dirswap.Replace

// installExternal brings the checkout of s to its pin. A checkout already
// at the pin is only checked: never fetched. One at another commit is
// replaced only when nothing local can be lost; see replaceExternal. The
// line to print is returned on success; a per-skill failure is an
// *apperr.Error, and any other error should stop the run.
func installExternal(ctx context.Context, g *gitx.Git, root string, s config.Skill, url string, progress io.Writer) (string, error) {
	rel := VendorDir + pathSeparator + s.Name
	if problem := dirProblem(root, rel); problem != "" {
		return "", apperr.ForRepo(apperr.PathConflict, s.Name, "%s", problem)
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	_, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		err := inTemp(root, s.Name, progress, func(temp string) error {
			dest := filepath.Join(temp, checkoutDir)
			if err := cloneAtPin(ctx, g, s, url, dest, progress); err != nil {
				return err
			}
			if err := os.Rename(dest, target); err != nil {
				return apperr.ForRepo(apperr.PathConflict, s.Name, "cannot move checkout to %s: %v", rel, err)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("skill %s: installed %s at %s", s.Name, rel, s.Commit), nil
	}
	if err != nil {
		return "", apperr.ForRepo(apperr.PathConflict, s.Name, "cannot inspect %s: %v", rel, err)
	}
	problem, err := checkout.RootProblem(ctx, g, target, rel)
	if err != nil {
		return "", err
	}
	if problem != "" {
		return "", apperr.ForRepo(apperr.PathConflict, s.Name, "%s", problem)
	}
	head, detached, err := headState(ctx, g, target)
	if err != nil {
		return "", err
	}
	if head == s.Commit && detached {
		if e := verifySubdir(target, s); e != nil {
			return "", e
		}
		return fmt.Sprintf("skill %s: already at %s", s.Name, s.Commit), nil
	}
	return replaceExternal(ctx, g, root, s, url, head, detached, progress)
}

// replaceExternal swaps the checkout for a fresh clone at the pin, but only
// when the old one holds nothing that exists nowhere else: no changes, no
// untracked or ignored files, a detached HEAD, and a HEAD commit that the
// fresh clone also has. Otherwise it is pin-mismatch and nothing changes.
func replaceExternal(ctx context.Context, g *gitx.Git, root string, s config.Skill, url string, head string, detached bool, progress io.Writer) (string, error) {
	rel := VendorDir + pathSeparator + s.Name
	target := filepath.Join(root, filepath.FromSlash(rel))
	mismatch := func(reason string) error {
		return apperr.ForRepo(apperr.PinMismatch, s.Name, "%s; %s, so it was left unchanged. Delete %s and run `%s skill sync` to reinstall",
			pinDetail(rel, head, detached, s.Commit), reason, rel, brand.Name)
	}
	if head == "" || !detached {
		return "", mismatch("HEAD is not detached at a commit")
	}
	// --no-optional-locks keeps status from refreshing the index, so a
	// checkout that is left alone stays byte-for-byte the same.
	status, err := g.Run(ctx, "", "--no-optional-locks", "-C", target, "status", "--porcelain", "--ignored")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", apperr.ForRepo(apperr.PathConflict, s.Name, "git status in %s failed: %v", rel, err)
	}
	if strings.TrimSpace(status) != "" {
		return "", mismatch("it has local changes, untracked or ignored files")
	}
	err = inTemp(root, s.Name, progress, func(temp string) error {
		dest := filepath.Join(temp, checkoutDir)
		if err := cloneAtPin(ctx, g, s, url, dest, progress); err != nil {
			return err
		}
		known, err := hasCommit(ctx, g, dest, head)
		if err != nil {
			return err
		}
		if !known {
			return mismatch(fmt.Sprintf("its commit %s is not in %s (a local commit?)", head, url))
		}
		err = replaceDir(target, dest, filepath.Join(temp, stashDir))
		var stranded *dirswap.StrandedError
		if errors.As(err, &stranded) {
			return err
		}
		if err != nil {
			return apperr.ForRepo(apperr.PathConflict, s.Name, "%v", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("skill %s: updated %s from %s to %s", s.Name, rel, head, s.Commit), nil
}

// inTemp runs fn with a new temporary directory inside vendor/skills/ and
// always removes it afterwards, together with whatever fn left there.
func inTemp(root string, name string, progress io.Writer, fn func(temp string) error) error {
	vendor := filepath.Join(root, filepath.FromSlash(VendorDir))
	if err := os.MkdirAll(vendor, dirPerm); err != nil {
		return apperr.ForRepo(apperr.PathConflict, name, "cannot prepare %s: %v", VendorDir, err)
	}
	temp, err := os.MkdirTemp(vendor, brand.CloneTempPrefix+name+".")
	if err != nil {
		return apperr.ForRepo(apperr.PathConflict, name, "cannot prepare %s: %v", VendorDir, err)
	}
	err = fn(temp)
	var stranded *dirswap.StrandedError
	if errors.As(err, &stranded) {
		// The only copy of the old checkout is inside temp.
		return apperr.ForRepo(apperr.PathConflict, name, "%v; move it back to %s/%s yourself", stranded, VendorDir, name)
	}
	if removeErr := os.RemoveAll(temp); removeErr != nil {
		fmt.Fprintf(progress, "warning: could not remove temporary directory %s: %v\n", temp, removeErr)
	}
	return err
}

// cloneAtPin clones url into dest without a checkout, then checks out the
// pin detached and confirms HEAD is exactly the pin.
func cloneAtPin(ctx context.Context, g *gitx.Git, s config.Skill, url string, dest string, progress io.Writer) error {
	cmd := g.Cmd(ctx, "", "-c", gitx.NoExtTransport, "clone", "--no-checkout", "--", url, dest)
	cmd.Stdout = progress
	cmd.Stderr = progress
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return apperr.ForRepo(apperr.CloneFailed, s.Name, "git clone exited with %d", exitErr.ExitCode())
	}
	if err != nil {
		return apperr.ForRepo(apperr.CloneFailed, s.Name, "cannot run git clone: %v", err)
	}
	_, err = g.Run(ctx, "", "-C", dest, "-c", gitx.NoExtTransport, "-c", noDetachedAdvice, "checkout", "--quiet", "--detach", s.Commit+commitPeel)
	var runErr *gitx.RunError
	if errors.As(err, &runErr) {
		return apperr.ForRepo(apperr.CloneFailed, s.Name, "commit %s not found in %s (git: %s)", s.Commit, url, gitx.LastLine(runErr.Stderr))
	}
	if err != nil {
		return err
	}
	head, detached, err := headState(ctx, g, dest)
	if err != nil {
		return err
	}
	if head != s.Commit || !detached {
		return apperr.ForRepo(apperr.CloneFailed, s.Name, "clone of %s is at %s, not detached at %s", url, head, s.Commit)
	}
	if e := verifySubdir(dest, s); e != nil {
		return e
	}
	return nil
}

// headState returns the commit HEAD points to, empty when there is none,
// and whether HEAD is detached.
func headState(ctx context.Context, g *gitx.Git, dir string) (string, bool, error) {
	out, err := g.Run(ctx, "", "-C", dir, "rev-parse", "--verify", "--quiet", "HEAD")
	var runErr *gitx.RunError
	if err != nil && !errors.As(err, &runErr) {
		return "", false, err
	}
	head := strings.TrimSpace(out)
	_, err = g.Run(ctx, "", "-C", dir, "symbolic-ref", "--quiet", "HEAD")
	if err == nil {
		return head, false, nil
	}
	if errors.As(err, &runErr) && runErr.ExitCode == notSymbolic {
		return head, true, nil
	}
	if errors.As(err, &runErr) {
		return head, false, nil
	}
	return "", false, err
}

func hasCommit(ctx context.Context, g *gitx.Git, dir string, commit string) (bool, error) {
	_, err := g.Run(ctx, "", "-C", dir, "cat-file", "-e", commit+commitPeel)
	var runErr *gitx.RunError
	if errors.As(err, &runErr) {
		return false, nil
	}
	return err == nil, err
}

// verifySubdir requires subdir/SKILL.md to be a regular file that, with
// every symlink resolved, stays inside the checkout.
func verifySubdir(checkoutPath string, s config.Skill) *apperr.Error {
	where := fmt.Sprintf("subdir %q at %s", s.Subdir, s.Commit)
	rootReal, err := filepath.EvalSymlinks(checkoutPath)
	if err != nil {
		return apperr.ForRepo(apperr.SkillInvalid, s.Name, "cannot resolve the checkout for %s: %v", where, err)
	}
	dir := filepath.Join(checkoutPath, filepath.FromSlash(s.Subdir))
	for _, path := range []string{dir, filepath.Join(dir, File)} {
		real, err := filepath.EvalSymlinks(path)
		if errors.Is(err, fs.ErrNotExist) {
			return apperr.ForRepo(apperr.SkillInvalid, s.Name, "no %s in %s", File, where)
		}
		if err != nil {
			return apperr.ForRepo(apperr.SkillInvalid, s.Name, "cannot resolve %s: %v", where, err)
		}
		if real != rootReal && !strings.HasPrefix(real, rootReal+string(filepath.Separator)) {
			return apperr.ForRepo(apperr.SkillInvalid, s.Name, "%s resolves outside the checkout", where)
		}
	}
	info, err := os.Stat(filepath.Join(dir, File))
	if err != nil || !info.Mode().IsRegular() {
		return apperr.ForRepo(apperr.SkillInvalid, s.Name, "no %s in %s", File, where)
	}
	return nil
}

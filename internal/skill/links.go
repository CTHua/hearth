package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

// linkTarget is the relative path a link in dir points to for item.
func linkTarget(root string, dir string, item Item) (string, error) {
	return filepath.Rel(filepath.Join(root, filepath.FromSlash(dir)), filepath.Join(root, filepath.FromSlash(item.Dir)))
}

// isManaged reports whether a link at link pointing to dest is one this
// package created: its target is inside one of the managed roots.
func isManaged(root string, link string, dest string) bool {
	// Clean first, so vendor/skills/../../x does not count as inside.
	resolved := filepath.Clean(dest)
	if !filepath.IsAbs(dest) {
		resolved = filepath.Join(filepath.Dir(link), dest)
	}
	for _, managed := range managedRoots {
		base := filepath.Join(root, filepath.FromSlash(managed))
		if strings.HasPrefix(resolved, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// linkState checks the link for item in dir without changing it.
func linkState(root string, dir string, item Item) finding {
	if problem := dirProblem(root, dir); problem != "" {
		return finding{state: PathConflict, detail: problem}
	}
	shown := dir + pathSeparator + item.Name
	link := filepath.Join(root, filepath.FromSlash(shown))
	expected, err := linkTarget(root, dir, item)
	if err != nil {
		return finding{state: PathConflict, detail: fmt.Sprintf("cannot compute the link for %s: %v", shown, err)}
	}
	info, err := os.Lstat(link)
	if errors.Is(err, fs.ErrNotExist) {
		return finding{state: LinkMissing, detail: fmt.Sprintf("%s is missing; run `%s skill sync`", shown, brand.Name)}
	}
	if err != nil {
		return finding{state: PathConflict, detail: fmt.Sprintf("cannot inspect %s: %v", shown, err)}
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return finding{state: LinkConflict, detail: notLinkDetail(shown)}
	}
	dest, err := os.Readlink(link)
	if err != nil {
		return finding{state: PathConflict, detail: fmt.Sprintf("cannot read %s: %v", shown, err)}
	}
	if dest == expected {
		return finding{state: OK}
	}
	if !isManaged(root, link, dest) {
		return finding{state: LinkConflict, detail: foreignLinkDetail(shown, dest)}
	}
	return finding{state: LinkStale, detail: fmt.Sprintf("%s points to %s, expected %s; run `%s skill sync`", shown, dest, expected, brand.Name)}
}

func notLinkDetail(shown string) string {
	return fmt.Sprintf("%s already exists and is not a symlink; left unchanged", shown)
}

func foreignLinkDetail(shown string, dest string) string {
	return fmt.Sprintf("%s points to %s, which %s does not manage; left unchanged", shown, dest, brand.Name)
}

// syncLink creates or repairs the link for item in dir and returns the line
// to print. Files and links it does not manage are never touched.
func syncLink(root string, dir string, item Item) (string, error) {
	current := linkState(root, dir, item)
	shown := dir + pathSeparator + item.Name
	link := filepath.Join(root, filepath.FromSlash(shown))
	switch current.state {
	case OK:
		return fmt.Sprintf("link %s: ok", shown), nil
	case LinkMissing:
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), dirPerm); err != nil {
			return "", apperr.ForRepo(apperr.PathConflict, item.Name, "cannot create %s: %v", dir, err)
		}
		if err := createLink(root, dir, item, link, shown); err != nil {
			return "", err
		}
		return fmt.Sprintf("link %s: created", shown), nil
	case LinkStale:
		if err := os.Remove(link); err != nil {
			return "", apperr.ForRepo(apperr.PathConflict, item.Name, "cannot remove %s: %v", shown, err)
		}
		if err := createLink(root, dir, item, link, shown); err != nil {
			return "", err
		}
		return fmt.Sprintf("link %s: updated", shown), nil
	}
	return "", apperr.ForRepo(apperr.Category(current.state), item.Name, "%s", current.detail)
}

// createLink never falls back to copying: a copy would silently go stale.
func createLink(root string, dir string, item Item, link string, shown string) error {
	expected, err := linkTarget(root, dir, item)
	if err != nil {
		return apperr.ForRepo(apperr.PathConflict, item.Name, "cannot compute the link for %s: %v", shown, err)
	}
	err = os.Symlink(expected, link)
	if err == nil {
		return nil
	}
	if symlinkUnsupported(err) {
		return apperr.ForRepo(apperr.SymlinkUnavailable, item.Name,
			"cannot create symlink %s: %v; the skill was not copied instead. Make symlinks work here and rerun `%s skill sync`",
			shown, err, brand.Name)
	}
	return apperr.ForRepo(apperr.PathConflict, item.Name, "cannot create symlink %s: %v", shown, err)
}

// symlinkUnsupported reports whether err says the file system cannot hold
// symlinks at all, as opposed to a permission or path problem.
func symlinkUnsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.EPERM)
}

// pruneLinks removes managed links in every agent's directory that keep
// does not list ("<dir>/<name>"), and returns the removed ones. A directory
// with a path problem is skipped. Only links this package manages are
// removed; failures are returned per link name.
func pruneLinks(root string, keep map[string]bool) ([]string, []*apperr.Error) {
	dirs := make([]string, 0, len(entryDirs))
	for _, dir := range entryDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	var removed []string
	var failed []*apperr.Error
	for _, dir := range dirs {
		if dirProblem(root, dir) != "" {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(dir))
		entries, err := os.ReadDir(abs)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			failed = append(failed, apperr.New(apperr.PathConflict, "cannot read %s: %v", dir, err))
			continue
		}
		for _, entry := range entries {
			shown := dir + pathSeparator + entry.Name()
			link := filepath.Join(abs, entry.Name())
			if keep[shown] || entry.Type()&fs.ModeSymlink == 0 {
				continue
			}
			dest, err := os.Readlink(link)
			if err != nil {
				failed = append(failed, apperr.ForRepo(apperr.PathConflict, entry.Name(), "cannot read %s: %v", shown, err))
				continue
			}
			if !isManaged(root, link, dest) {
				continue
			}
			if err := os.Remove(link); err != nil {
				failed = append(failed, apperr.ForRepo(apperr.PathConflict, entry.Name(), "cannot remove %s: %v", shown, err))
				continue
			}
			removed = append(removed, shown)
		}
	}
	return removed, failed
}

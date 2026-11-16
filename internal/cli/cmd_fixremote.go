package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/clone"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

const (
	fixedLabel     = "fixed"
	unchangedLabel = "unchanged"
)

func runFixRemote(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo fix-remote")
	names, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo fix-remote [<name>...]\n", brand.Name)
		return nil
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	git, err := gitx.New(ctx)
	if err != nil {
		return err
	}
	selected, err := selectRepos(home.Team, names)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintf(s.out, "No repos configured in %s\n", brand.TeamFile)
		return nil
	}
	targets, err := cloneTargets(selected, home.Personal.Protocol)
	if err != nil {
		return err
	}
	var batch batchResult
	for _, target := range targets {
		fixed, err := fixOne(ctx, s, git, home.Root, target)
		if err := batch.record(s, target.Repo.Name, err); err != nil {
			return err
		}
		if err != nil {
			continue
		}
		if fixed {
			batch.done = append(batch.done, target.Repo.Name)
			continue
		}
		batch.skipped = append(batch.skipped, target.Repo.Name)
	}
	batch.summary(s, fixedLabel, unchangedLabel)
	return batch.exit()
}

// fixOne changes origin only when it differs from the expected URL by
// protocol alone; every other difference is reported and left as it is.
func fixOne(ctx context.Context, s streams, git *gitx.Git, root string, t clone.Target) (bool, error) {
	status, err := checkout.Inspect(ctx, git, root, t.Repo.Path, t.URL)
	if err != nil {
		return false, err
	}
	switch status.State {
	case checkout.OK:
		fmt.Fprintf(s.out, "unchanged %s: origin is already %s\n", t.Repo.Name, t.URL)
		return false, nil
	case checkout.Missing:
		fmt.Fprintf(s.out, "unchanged %s: not cloned at %s\n", t.Repo.Name, t.Repo.Path)
		return false, nil
	case checkout.PathConflict:
		return false, apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "%s", status.Detail)
	}
	if !status.HasOrigin || !giturl.OnlyProtocolDiffers(status.Origin, t.URL) {
		message := clone.MismatchMessage(t, status) + "; it differs by more than the protocol, so fix it yourself"
		return false, apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "%s", message)
	}
	target := filepath.Join(root, filepath.FromSlash(t.Repo.Path))
	if _, err := git.Run(ctx, "", "-C", target, "remote", "set-url", "origin", t.URL); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "git remote set-url failed: %v", err)
	}
	fmt.Fprintf(s.out, "fixed %s: origin %s -> %s\n", t.Repo.Name, status.Origin, t.URL)
	return true, nil
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/clone"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

const failedLabel = "failed"

func runClone(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("clone")
	names, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s clone [<name>...]\n", brand.Name)
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
	protocol, err := ensureProtocol(ctx, s, home, git, repoURLs(selected))
	if err != nil {
		return err
	}
	targets, err := cloneTargets(selected, protocol)
	if err != nil {
		return err
	}
	var batch batchResult
	for _, target := range targets {
		outcome, err := clone.One(ctx, git, home.Root, target, s.err)
		if err := batch.record(s, target.Repo.Name, err); err != nil {
			return err
		}
		if err != nil {
			continue
		}
		if outcome == clone.Cloned {
			fmt.Fprintf(s.out, "cloned %s -> %s (%s)\n", target.Repo.Name, target.Repo.Path, target.Repo.Branch)
			batch.done = append(batch.done, target.Repo.Name)
			continue
		}
		fmt.Fprintf(s.out, "skipped %s: already cloned at %s\n", target.Repo.Name, target.Repo.Path)
		batch.skipped = append(batch.skipped, target.Repo.Name)
	}
	batch.summary(s, string(clone.Cloned), string(clone.Skipped))
	return batch.exit()
}

// selectRepos returns every repo when names is empty, otherwise the named
// ones in team-file order, each once. Unknown names are a usage error.
func selectRepos(team config.Team, names []string) ([]config.Repo, error) {
	if len(names) == 0 {
		return team.Repos, nil
	}
	var unknown []string
	for _, name := range names {
		_, found := team.Find(name)
		if !found && !slices.Contains(unknown, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return nil, unknownNames(unknown, team)
	}
	var selected []config.Repo
	for _, repo := range team.Repos {
		if slices.Contains(names, repo.Name) {
			selected = append(selected, repo)
		}
	}
	return selected, nil
}

func repoURLs(repos []config.Repo) []string {
	urls := make([]string, 0, len(repos))
	for _, repo := range repos {
		urls = append(urls, repo.URL)
	}
	return urls
}

func cloneTargets(repos []config.Repo, protocol string) ([]clone.Target, error) {
	targets := make([]clone.Target, 0, len(repos))
	for _, repo := range repos {
		url, err := giturl.Expected(repo.URL, protocol)
		if err != nil {
			return nil, apperr.ForRepo(apperr.Config, repo.Name, "%v", err)
		}
		targets = append(targets, clone.Target{Repo: repo, URL: url, Protocol: protocol})
	}
	return targets, nil
}

// batchResult collects a run over several repos where one failure does not
// stop the others.
type batchResult struct {
	done    []string
	skipped []string
	failed  []string
}

// record prints a per-repo failure and keeps going; any other error, such as
// an interrupt, is returned so the run stops.
func (b *batchResult) record(s streams, name string, err error) error {
	if err == nil {
		return nil
	}
	var repoErr *apperr.Error
	if !errors.As(err, &repoErr) {
		return err
	}
	apperr.Print(s.err, repoErr)
	b.failed = append(b.failed, fmt.Sprintf("%s (%s)", name, repoErr.Category))
	return nil
}

func (b *batchResult) summary(s streams, doneLabel string, skippedLabel string) {
	fmt.Fprintf(s.out, "Summary: %d %s, %d %s, %d %s\n",
		len(b.done), doneLabel, len(b.skipped), skippedLabel, len(b.failed), failedLabel)
	groups := []struct {
		label string
		names []string
	}{{doneLabel, b.done}, {skippedLabel, b.skipped}, {failedLabel, b.failed}}
	for _, group := range groups {
		names := strings.Join(group.names, ", ")
		if names == "" {
			names = noneLabel
		}
		fmt.Fprintf(s.out, "  %s: %s\n", group.label, names)
	}
}

func (b *batchResult) exit() error {
	if len(b.failed) > 0 {
		return apperr.Exit(apperr.ExitFailed)
	}
	return nil
}

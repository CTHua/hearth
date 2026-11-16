package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/detect"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

const (
	flagName   = "name"
	flagPath   = "path"
	flagBranch = "branch"
	noneLabel  = "(none)"
)

func runRepo(ctx context.Context, s streams, g globals, args []string) error {
	subcommands := map[string]command{
		"add":        runRepoAdd,
		"remove":     runRepoRemove,
		"list":       runRepoList,
		"fix-remote": runFixRemote,
	}
	if len(args) == 0 {
		return apperr.New(apperr.Usage, "repo needs a subcommand: add, remove, list or fix-remote")
	}
	run, found := subcommands[args[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown repo subcommand %q; choose add, remove, list or fix-remote", args[0])
	}
	return run(ctx, s, g, args[1:])
}

func runRepoAdd(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo add")
	name := fs.String(flagName, "", "")
	path := fs.String(flagPath, "", "")
	branch := fs.String(flagBranch, "", "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo add <url> [--name <name>] [--path <path>] [--branch <branch>]\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "repo add takes exactly one URL")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	repo, err := newRepo(positional[0], setFlags(fs), *name, *path, *branch)
	if err != nil {
		return err
	}
	branchGiven := setFlags(fs)[flagBranch]
	if !branchGiven {
		// Everything known locally is checked before the remote is contacted.
		if err := home.Team.CheckNew(repo, config.FieldBranch); err != nil {
			return addProblems(err)
		}
		repo.Branch, err = remoteDefaultBranch(ctx, s, home, repo.URL)
		if err != nil {
			return err
		}
	}
	team, err := home.Team.WithRepo(repo)
	if err != nil {
		return addProblems(err)
	}
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "added %s -> %s (%s)\n", repo.Name, repo.Path, repo.Branch)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

// newRepo fills the defaults: name from the URL, path from the name.
func newRepo(rawURL string, given map[string]bool, name string, path string, branch string) (config.Repo, error) {
	u, err := giturl.Parse(rawURL)
	if err != nil {
		return config.Repo{}, apperr.New(apperr.Usage, "url %q: %v", rawURL, err)
	}
	if !given[flagName] {
		name = u.DefaultName()
	}
	if !given[flagPath] {
		path = config.ProductsDir + "/" + name
	}
	return config.Repo{Name: name, URL: rawURL, Path: path, Branch: branch}, nil
}

// addProblems turns validation problems of the new entry into one usage
// error that names the flag to fix.
func addProblems(err error) error {
	var problems config.FieldErrors
	if !errors.As(err, &problems) {
		return err
	}
	parts := make([]string, 0, len(problems))
	for _, problem := range problems {
		parts = append(parts, fmt.Sprintf("%s %s%s", problem.Field, problem.Message, flagHint(problem.Field)))
	}
	return apperr.New(apperr.Usage, "cannot add repo: %s; %s is unchanged", strings.Join(parts, "; "), brand.TeamFile)
}

func flagHint(field string) string {
	switch field {
	case config.FieldName, config.FieldPath, config.FieldBranch:
		return fmt.Sprintf(" (choose another with --%s)", field)
	}
	return ""
}

// remoteDefaultBranch asks the remote for its HEAD branch, using the URL the
// user will clone with; that needs a protocol, which may first be detected.
func remoteDefaultBranch(ctx context.Context, s streams, home *config.Home, rawURL string) (string, error) {
	git, err := gitx.New(ctx)
	if err != nil {
		return "", err
	}
	protocol, err := ensureProtocol(ctx, s, home, git, []string{rawURL})
	if err != nil {
		return "", err
	}
	url, err := giturl.Expected(rawURL, protocol)
	if err != nil {
		return "", apperr.New(apperr.Usage, "url %q: %v", rawURL, err)
	}
	branch, found, err := git.DefaultBranch(ctx, url)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", apperr.New(apperr.Env, "cannot ask %s for its default branch: %v; pass --branch", url, err)
	}
	if !found {
		return "", apperr.New(apperr.Usage, "%s did not report a default branch; pass --branch", url)
	}
	return branch, nil
}

// ensureProtocol returns the personal protocol, detecting and saving it first
// when it is not set yet.
func ensureProtocol(ctx context.Context, s streams, home *config.Home, git *gitx.Git, urls []string) (string, error) {
	if home.Personal.Protocol != "" {
		return home.Personal.Protocol, nil
	}
	protocol, err := detect.Protocol(ctx, git, urls)
	if err != nil {
		return "", err
	}
	personal := home.Personal.WithProtocol(protocol)
	if err := config.WritePersonal(home.Root, personal); err != nil {
		return "", err
	}
	home.Personal = personal
	fmt.Fprintf(s.out, "protocol: detected %s and saved it to %s; change it with `%s config protocol <ssh|https>`\n",
		protocol, brand.LocalFile, brand.Name)
	return protocol, nil
}

func runRepoRemove(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo remove")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo remove <name>\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "repo remove takes exactly one name")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	name := positional[0]
	repo, found := home.Team.Find(name)
	if !found {
		return unknownNames([]string{name}, home.Team)
	}
	team, _ := home.Team.Without(name)
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed %s from %s\n", name, brand.TeamFile)
	if exists(filepath.Join(home.Root, filepath.FromSlash(repo.Path))) {
		fmt.Fprintf(s.out, "the checkout at %s was not touched; delete it yourself if you no longer need it\n", repo.Path)
	}
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func unknownNames(names []string, team config.Team) error {
	known := strings.Join(team.Names(), ", ")
	if known == "" {
		known = noneLabel
	}
	return apperr.New(apperr.Usage, "unknown repo name(s): %s; available: %s", strings.Join(names, ", "), known)
}

func runRepoList(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo list")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo list\n", brand.Name)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "repo list takes no arguments")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	if len(home.Team.Repos) == 0 {
		fmt.Fprintf(s.out, "No repos configured in %s\n", brand.TeamFile)
		return nil
	}
	git, err := gitx.New(ctx)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tPATH\tBRANCH\tSTATUS\tDETAIL")
	for _, repo := range home.Team.Repos {
		expected, err := giturl.Expected(repo.URL, home.Personal.Protocol)
		if err != nil {
			return apperr.ForRepo(apperr.Config, repo.Name, "%v", err)
		}
		status, err := checkout.Inspect(ctx, git, home.Root, repo.Path, expected)
		if err != nil {
			return err
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", repo.Name, repo.Path, repo.Branch, status.State, status.Detail)
	}
	return table.Flush()
}

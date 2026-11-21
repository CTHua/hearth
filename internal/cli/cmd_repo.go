package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/core"
)

const (
	flagName        = "name"
	flagPath        = "path"
	flagBranch      = "branch"
	flagDescription = "description"
)

func runRepo(ctx context.Context, s streams, g globals, args []string) error {
	subcommands := map[string]command{
		"add":        runRepoAdd,
		"remove":     runRepoRemove,
		"describe":   runRepoDescribe,
		"list":       runRepoList,
		"fix-remote": runFixRemote,
	}
	if len(args) == 0 {
		return apperr.New(apperr.Usage, "repo needs a subcommand: add, remove, describe, list or fix-remote")
	}
	run, found := subcommands[args[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown repo subcommand %q; choose add, remove, describe, list or fix-remote", args[0])
	}
	return run(ctx, s, g, args[1:])
}

func runRepoAdd(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo add")
	name := fs.String(flagName, "", "")
	path := fs.String(flagPath, "", "")
	branch := fs.String(flagBranch, "", "")
	description := fs.String(flagDescription, "", "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo add <url> [--name <name>] [--path <path>] [--branch <branch>] [--description <text>]\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "repo add takes exactly one URL")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	given := setFlags(fs)
	params := core.AddRepoParams{
		URL:         positional[0],
		Name:        givenValue(given, flagName, name),
		Path:        givenValue(given, flagPath, path),
		Branch:      givenValue(given, flagBranch, branch),
		Description: givenValue(given, flagDescription, description),
	}
	result, err := core.New(home).AddRepo(ctx, params)
	printDetected(s, result.DetectedProtocol)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "added %s -> %s (%s)\n", result.Repo.Name, result.Repo.Path, result.Repo.Branch)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

// printDetected tells the user a protocol was detected and saved.
func printDetected(s streams, protocol string) {
	if protocol != "" {
		fmt.Fprintln(s.out, core.ProtocolNotice(protocol))
	}
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
	result, err := core.New(home).RemoveRepo(positional[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed %s from %s\n", result.Repo.Name, brand.TeamFile)
	if result.CheckoutKept {
		fmt.Fprintf(s.out, "the checkout at %s was not touched; delete it yourself if you no longer need it\n", result.Repo.Path)
	}
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runRepoDescribe(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("repo describe")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s repo describe <name> <text>\n", brand.Name)
		fmt.Fprintln(s.out, "an empty <text> removes the description")
		return nil
	}
	if len(positional) != 2 {
		return apperr.New(apperr.Usage, "repo describe takes exactly a name and a text; pass \"\" to remove the description")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	result, err := core.New(home).DescribeRepo(positional[0], positional[1])
	if err != nil {
		return err
	}
	repo := result.Repo
	switch {
	case !result.Changed && repo.Description == "":
		fmt.Fprintf(s.out, "unchanged %s: it has no description\n", repo.Name)
		return nil
	case !result.Changed:
		fmt.Fprintf(s.out, "unchanged %s: the description is already %q\n", repo.Name, repo.Description)
		return nil
	case repo.Description == "":
		fmt.Fprintf(s.out, "removed the description of %s\n", repo.Name)
	default:
		fmt.Fprintf(s.out, "described %s: %s\n", repo.Name, repo.Description)
	}
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
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
	rows, err := core.New(home).ListRepos(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintf(s.out, "No repos configured in %s\n", brand.TeamFile)
		return nil
	}
	table := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tPATH\tBRANCH\tSTATUS\tDETAIL\tDESCRIPTION")
	for _, row := range rows {
		repo := row.Repo
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", repo.Name, repo.Path, repo.Branch, row.StatusLabel(), row.Status.Detail, repo.Description)
	}
	return table.Flush()
}

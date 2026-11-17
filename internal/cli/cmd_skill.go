package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	flagCommit = "commit"
	flagSubdir = "subdir"
	yesLabel   = "yes"
	noLabel    = "no"
	noSource   = "-"
	listSep    = ", "
	skillGroup = "skill"
)

func runSkill(ctx context.Context, s streams, g globals, args []string) error {
	subcommands := map[string]command{
		"add":     runSkillAdd,
		"remove":  runSkillRemove,
		"update":  runSkillUpdate,
		"enable":  runSkillEnable,
		"disable": runSkillDisable,
		"sync":    runSkillSync,
		"list":    runSkillList,
	}
	const choices = "add, remove, update, enable, disable, sync or list"
	if len(args) == 0 {
		return apperr.New(apperr.Usage, "skill needs a subcommand: %s", choices)
	}
	run, found := subcommands[args[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown skill subcommand %q; choose %s", args[0], choices)
	}
	return run(ctx, s, g, args[1:])
}

func runSkillAdd(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("skill add")
	name := fs.String(flagName, "", "")
	commit := fs.String(flagCommit, "", "")
	subdir := fs.String(flagSubdir, config.RootSubdir, "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s skill add <url> [--name <name>] [--commit <sha>] [--subdir <dir>]\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "skill add takes exactly one URL")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	given := setFlags(fs)
	params := core.AddSkillParams{
		URL:    positional[0],
		Name:   givenValue(given, flagName, name),
		Commit: givenValue(given, flagCommit, commit),
		Subdir: givenValue(given, flagSubdir, subdir),
	}
	result, err := core.New(home).AddSkill(ctx, params)
	printDetected(s, result.DetectedProtocol)
	if err != nil {
		return err
	}
	entry := result.Skill
	fmt.Fprintf(s.out, "added skill %s at %s (subdir %s)\n", entry.Name, entry.Commit, entry.Subdir)
	fmt.Fprintf(s.out, "to use it: `%s skill enable %s`, then `%s skill sync`\n", brand.Name, entry.Name, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runSkillRemove(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("skill remove")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s skill remove <name>\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "skill remove takes exactly one name")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	result, err := core.New(home).RemoveSkill(positional[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed skill %s from %s\n", result.Name, brand.TeamFile)
	fmt.Fprintf(s.out, "the next `%s skill sync` removes its links", brand.Name)
	if result.VendorKept {
		fmt.Fprintf(s.out, "; %s was not touched, delete it yourself if you no longer need it", result.Vendor)
	}
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runSkillUpdate(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("skill update")
	commit := fs.String(flagCommit, "", "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s skill update <name> [--commit <sha>]\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "skill update takes exactly one name")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	result, err := core.New(home).UpdateSkill(ctx, positional[0], givenValue(setFlags(fs), flagCommit, commit))
	printDetected(s, result.DetectedProtocol)
	if err != nil {
		return err
	}
	if result.Unchanged {
		fmt.Fprintf(s.out, "skill %s is already pinned at %s; %s is unchanged\n", result.Name, result.To, brand.TeamFile)
		return nil
	}
	fmt.Fprintf(s.out, "updated skill %s: %s -> %s; run `%s skill sync` to move its checkout\n", result.Name, result.From, result.To, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runSkillEnable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, skillGroup, "enable")
	if err != nil || home == nil {
		return err
	}
	added, err := core.New(home).EnableSkills(names)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "enabled: %s (saved to %s); run `%s skill sync`\n", core.LabelList(added), brand.LocalFile, brand.Name)
	return nil
}

func runSkillDisable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, skillGroup, "disable")
	if err != nil || home == nil {
		return err
	}
	removed, err := core.New(home).DisableSkills(names)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "disabled: %s (saved to %s); run `%s skill sync` to remove their links\n", strings.Join(removed, listSep), brand.LocalFile, brand.Name)
	return nil
}

// personalNameArgs parses `<group> enable|disable <name>...`, such as
// `skill enable pdf`, and loads the home repo. home is nil when only help
// was printed.
func personalNameArgs(s streams, g globals, args []string, group string, verb string) ([]string, *config.Home, error) {
	command := group + " " + verb
	fs := newFlagSet(command)
	names, help, err := parseArgs(fs, args)
	if err != nil {
		return nil, nil, err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s %s <name>...\n", brand.Name, command)
		return nil, nil, nil
	}
	if len(names) == 0 {
		return nil, nil, apperr.New(apperr.Usage, "%s takes at least one name", command)
	}
	home, err := loadHome(s, g)
	if err != nil {
		return nil, nil, err
	}
	return names, home, nil
}

func runSkillSync(ctx context.Context, s streams, g globals, args []string) error {
	home, err := skillCommand(s, g, args, "sync")
	if err != nil || home == nil {
		return err
	}
	_, err = core.New(home).SyncSkills(ctx, s.output())
	return err
}

// runSkillList only reads: it never connects to a remote or writes a file.
func runSkillList(ctx context.Context, s streams, g globals, args []string) error {
	home, err := skillCommand(s, g, args, "list")
	if err != nil || home == nil {
		return err
	}
	rows, err := core.New(home).ListSkills(ctx)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tSOURCE\tENABLED\tSTATUS\tDETAIL")
	for _, row := range rows {
		source := string(row.Source)
		if source == "" {
			source = noSource
		}
		enabled := noLabel
		if row.Enabled {
			enabled = yesLabel
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", row.Name, source, enabled, row.State, row.Detail)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	for _, row := range rows {
		if row.State == skill.Unknown {
			apperr.Warn(s.err, apperr.ForRepo(apperr.Category(row.State), row.Name, "%s", row.Detail))
		}
	}
	return nil
}

// skillCommand parses a `skill <verb>` that takes no arguments and loads
// the home repo. home is nil when only help was printed.
func skillCommand(s streams, g globals, args []string, verb string) (*config.Home, error) {
	fs := newFlagSet("skill " + verb)
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return nil, err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s skill %s\n", brand.Name, verb)
		return nil, nil
	}
	if len(positional) > 0 {
		return nil, apperr.New(apperr.Usage, "skill %s takes no arguments", verb)
	}
	return loadHome(s, g)
}

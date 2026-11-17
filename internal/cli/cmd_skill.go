package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	flagCommit  = "commit"
	flagSubdir  = "subdir"
	yesLabel    = "yes"
	noLabel     = "no"
	noSource    = "-"
	readyLabel  = "ready"
	warnedLabel = "warned"
	listSep     = ", "
	pathSep     = "/"
	skillGroup  = "skill"
)

// skillFlags are the skill add flags that set a field of the same name.
var skillFlags = []string{config.FieldName, config.FieldCommit, config.FieldSubdir}

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
	entry, err := newSkill(positional[0], given, *name, *commit, *subdir)
	if err != nil {
		return err
	}
	var pending []string
	if !given[flagCommit] {
		pending = append(pending, config.FieldCommit)
	}
	// Everything known locally is checked before the remote is contacted.
	if err := home.Team.CheckNewSkill(entry, pending...); err != nil {
		return entryProblems("add skill", err, skillFlags)
	}
	if err := sourceConflict(home, "add skill", entry.Name); err != nil {
		return err
	}
	if !given[flagCommit] {
		entry.Commit, err = remoteHeadCommit(ctx, s, home, entry.URL)
		if err != nil {
			return err
		}
	}
	team, err := home.Team.WithSkill(entry)
	if err != nil {
		return entryProblems("add skill", err, skillFlags)
	}
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "added skill %s at %s (subdir %s)\n", entry.Name, entry.Commit, entry.Subdir)
	fmt.Fprintf(s.out, "to use it: `%s skill enable %s`, then `%s skill sync`\n", brand.Name, entry.Name, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

// newSkill fills the defaults: name from the last subdir segment, or from
// the URL when the skill is at the top of its repo.
func newSkill(rawURL string, given map[string]bool, name string, commit string, subdir string) (config.Skill, error) {
	u, err := giturl.Parse(rawURL)
	if err != nil {
		return config.Skill{}, apperr.New(apperr.Usage, "url %q: %v", rawURL, err)
	}
	if !given[flagName] {
		segments := strings.Split(subdir, pathSep)
		name = segments[len(segments)-1]
		if subdir == config.RootSubdir {
			name = u.DefaultName()
		}
	}
	return config.Skill{Name: name, URL: rawURL, Commit: commit, Subdir: subdir}, nil
}

// sourceConflict rejects a name a team skill directory or the built-in skill
// already uses; the team file itself is checked by config.
// It looks past external entries, since the one being updated is itself
// in the catalog.
func sourceConflict(home *config.Home, action string, name string) error {
	cat, err := skill.Discover(home)
	if err != nil {
		return err
	}
	for _, item := range cat.Items() {
		if item.Source == skill.External || !strings.EqualFold(item.Name, name) {
			continue
		}
		return apperr.New(apperr.Usage, "cannot %s: name %q is already used by the %s skill at %s (names are compared case-insensitively); %s is unchanged",
			action, name, item.Source, item.Dir, brand.TeamFile)
	}
	return nil
}

// remoteHeadCommit asks the remote which commit its HEAD points to.
func remoteHeadCommit(ctx context.Context, s streams, home *config.Home, rawURL string) (string, error) {
	git, url, err := remoteURL(ctx, s, home, rawURL)
	if err != nil {
		return "", err
	}
	commit, found, err := git.RemoteHead(ctx, url)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", apperr.New(apperr.Env, "cannot ask %s for its HEAD commit: %v; pass --%s", url, err, flagCommit)
	}
	if !found {
		return "", apperr.New(apperr.Usage, "%s did not report a HEAD commit; pass --%s", url, flagCommit)
	}
	return commit, nil
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
	entry, found := home.Team.FindSkill(positional[0])
	if !found {
		return unknownSkill(positional[0], home.Team)
	}
	name := entry.Name
	team, _ := home.Team.WithoutSkill(name)
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed skill %s from %s\n", name, brand.TeamFile)
	fmt.Fprintf(s.out, "the next `%s skill sync` removes its links", brand.Name)
	vendor := skill.VendorDir + pathSep + name
	if exists(filepath.Join(home.Root, filepath.FromSlash(vendor))) {
		fmt.Fprintf(s.out, "; %s was not touched, delete it yourself if you no longer need it", vendor)
	}
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func unknownSkill(name string, team config.Team) error {
	known := strings.Join(team.SkillNames(), listSep)
	if known == "" {
		known = noneLabel
	}
	return apperr.New(apperr.Usage, "unknown skill %q in %s; external skills: %s", name, brand.TeamFile, known)
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
	entry, found := home.Team.FindSkill(positional[0])
	if !found {
		return unknownSkill(positional[0], home.Team)
	}
	name := entry.Name
	// Checked before the remote is contacted and before anything is written.
	if err := sourceConflict(home, "update skill", name); err != nil {
		return err
	}
	next := *commit
	if !setFlags(fs)[flagCommit] {
		next, err = remoteHeadCommit(ctx, s, home, entry.URL)
		if err != nil {
			return err
		}
	}
	if next == entry.Commit {
		fmt.Fprintf(s.out, "skill %s is already pinned at %s; %s is unchanged\n", name, next, brand.TeamFile)
		return nil
	}
	team, err := home.Team.WithSkillCommit(name, next)
	if err != nil {
		return entryProblems("update skill", err, skillFlags)
	}
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "updated skill %s: %s -> %s; run `%s skill sync` to move its checkout\n", name, entry.Commit, next, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runSkillEnable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, skillGroup, "enable")
	if err != nil || home == nil {
		return err
	}
	cat, err := skill.Discover(home)
	if err != nil {
		return err
	}
	chosen := slices.Clone(home.Personal.Skills)
	var refused []string
	var added []string
	for _, name := range names {
		item, found := cat.Find(name)
		switch {
		case !found:
			refused = append(refused, fmt.Sprintf("%s (no such skill)", name))
		case item.Source == skill.Builtin:
			refused = append(refused, fmt.Sprintf("%s (built in; installed whenever an agent is chosen)", name))
		case item.Invalid != "":
			refused = append(refused, fmt.Sprintf("%s (%s: %s)", name, apperr.SkillInvalid, item.Invalid))
		case !containsFold(chosen, item.Name):
			chosen = append(chosen, item.Name)
			added = append(added, item.Name)
		}
	}
	if len(refused) > 0 {
		available := strings.Join(cat.Choosable(), listSep)
		if available == "" {
			available = noneLabel
		}
		return apperr.New(apperr.Usage, "cannot enable %s; available: %s; %s is unchanged", strings.Join(refused, listSep), available, brand.LocalFile)
	}
	if err := config.WritePersonal(home.Root, home.Personal.WithSkills(chosen)); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "enabled: %s (saved to %s); run `%s skill sync`\n", labelList(added), brand.LocalFile, brand.Name)
	return nil
}

// runSkillDisable also removes stale choices, names no skill has any more,
// which is why it does not look at the catalog.
func runSkillDisable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, skillGroup, "disable")
	if err != nil || home == nil {
		return err
	}
	chosen := slices.Clone(home.Personal.Skills)
	var missing []string
	var removed []string
	for _, name := range names {
		index := slices.IndexFunc(chosen, func(c string) bool { return strings.EqualFold(c, name) })
		if index < 0 {
			if !containsFold(removed, name) {
				missing = append(missing, name)
			}
			continue
		}
		removed = append(removed, chosen[index])
		chosen = slices.Delete(chosen, index, index+1)
	}
	if len(missing) > 0 {
		return apperr.New(apperr.Usage, "not enabled: %s; enabled: %s; %s is unchanged",
			strings.Join(missing, listSep), labelList(home.Personal.Skills), brand.LocalFile)
	}
	if err := config.WritePersonal(home.Root, home.Personal.WithSkills(chosen)); err != nil {
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

func containsFold(list []string, name string) bool {
	return slices.ContainsFunc(list, func(item string) bool { return strings.EqualFold(item, name) })
}

func labelList(names []string) string {
	if len(names) == 0 {
		return noneLabel
	}
	return strings.Join(names, listSep)
}

func runSkillSync(ctx context.Context, s streams, g globals, args []string) error {
	home, git, cat, err := skillCommand(ctx, s, g, args, "sync")
	if err != nil || home == nil {
		return err
	}
	return syncSkills(ctx, s, home, git, cat)
}

// syncSkills settles the protocol when a clone needs it, then installs and
// links the chosen skills and prints the summary.
func syncSkills(ctx context.Context, s streams, home *config.Home, git *gitx.Git, cat skill.Catalog) error {
	urls, err := skill.CloneURLs(ctx, git, home, cat)
	if err != nil {
		return err
	}
	if len(urls) > 0 {
		if _, err := ensureProtocol(ctx, s, home, git, urls); err != nil {
			return err
		}
	}
	result, err := skill.Sync(ctx, git, home, cat, s.out, s.err)
	if err != nil {
		return err
	}
	batch := batchResult{done: result.Ready, skipped: result.Warned, failed: result.Failed}
	batch.summary(s, readyLabel, warnedLabel)
	return batch.exit()
}

// runSkillList only reads: it never connects to a remote or writes a file.
func runSkillList(ctx context.Context, s streams, g globals, args []string) error {
	home, git, cat, err := skillCommand(ctx, s, g, args, "list")
	if err != nil || home == nil {
		return err
	}
	rows, err := skill.Inspect(ctx, git, home, cat)
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

// skillCommand parses a `skill <verb>` that takes no arguments and prepares
// what it needs. home is nil when only help was printed.
func skillCommand(ctx context.Context, s streams, g globals, args []string, verb string) (*config.Home, *gitx.Git, skill.Catalog, error) {
	fs := newFlagSet("skill " + verb)
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return nil, nil, skill.Catalog{}, err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s skill %s\n", brand.Name, verb)
		return nil, nil, skill.Catalog{}, nil
	}
	if len(positional) > 0 {
		return nil, nil, skill.Catalog{}, apperr.New(apperr.Usage, "skill %s takes no arguments", verb)
	}
	home, err := loadHome(s, g)
	if err != nil {
		return nil, nil, skill.Catalog{}, err
	}
	git, cat, err := skillTools(ctx, home)
	if err != nil {
		return nil, nil, skill.Catalog{}, err
	}
	return home, git, cat, nil
}

// skillTools finds git and the skills home offers.
func skillTools(ctx context.Context, home *config.Home) (*gitx.Git, skill.Catalog, error) {
	git, err := gitx.New(ctx)
	if err != nil {
		return nil, skill.Catalog{}, err
	}
	cat, err := skill.Discover(home)
	if err != nil {
		return nil, skill.Catalog{}, err
	}
	return git, cat, nil
}

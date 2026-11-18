package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/subagent"
)

const subagentGroup = "subagent"

func runSubagent(ctx context.Context, s streams, g globals, args []string) error {
	subcommands := map[string]command{
		"enable":  runSubagentEnable,
		"disable": runSubagentDisable,
		"sync":    runSubagentSync,
		"list":    runSubagentList,
	}
	const choices = "enable, disable, sync or list"
	if len(args) == 0 {
		return apperr.New(apperr.Usage, "subagent needs a subcommand: %s", choices)
	}
	run, found := subcommands[args[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown subagent subcommand %q; choose %s", args[0], choices)
	}
	return run(ctx, s, g, args[1:])
}

func runSubagentEnable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, subagentGroup, "enable")
	if err != nil || home == nil {
		return err
	}
	added, err := core.New(home).EnableSubagents(names)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "enabled: %s (saved to %s); run `%s sync`\n", core.LabelList(added), brand.LocalFile, brand.Name)
	return nil
}

func runSubagentDisable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, subagentGroup, "disable")
	if err != nil || home == nil {
		return err
	}
	removed, err := core.New(home).DisableSubagents(names)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "disabled: %s (saved to %s); run `%s sync` to remove their generated files\n", strings.Join(removed, listSep), brand.LocalFile, brand.Name)
	return nil
}

// runSubagentList only reads.
func runSubagentList(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("subagent list")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s subagent list\n", brand.Name)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "subagent list takes no arguments")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	rows, err := core.New(home).ListSubagents()
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tENABLED\tSTATUS\tDETAIL")
	for _, row := range rows {
		enabled := noLabel
		if row.Enabled {
			enabled = yesLabel
		}
		// A stale choice is treated as disabled, so it is neither yes nor no.
		if row.Stale {
			enabled = noValue
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", row.Name, enabled, row.State, row.FullDetail())
	}
	if err := table.Flush(); err != nil {
		return err
	}
	for _, row := range rows {
		if row.Stale {
			apperr.Warn(s.err, apperr.ForRepo(apperr.Unknown, row.Name, "%s", subagent.StaleDetail(row.Name)))
		}
	}
	return nil
}

func runSubagentSync(_ context.Context, s streams, g globals, args []string) error {
	force, help, err := syncArgs(s, args, "subagent sync")
	if err != nil || help {
		return err
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	_, err = core.New(home).SyncSubagents(force, s.output())
	return err
}

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/upgrade"
)

const flagCheck = "check"

// runUpgrade does not load the home repo through config.Load, which refuses
// the older schemas it is there to migrate.
func runUpgrade(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("upgrade")
	check := fs.Bool(flagCheck, false, "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s upgrade [--%s]\n", brand.Name, flagCheck)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "upgrade takes no arguments")
	}
	root, err := findRoot(g)
	if err != nil {
		return err
	}
	changes, err := upgrade.Run(root, upgrade.Default(), *check)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Fprintln(s.out, "already up to date")
		return nil
	}
	verb := "updated"
	if *check {
		verb = "would update"
	}
	var shared []string
	for _, change := range changes {
		fmt.Fprintf(s.out, "%s %s (%s)\n", verb, change.File, change.Detail)
		if change.File != brand.LocalFile {
			shared = append(shared, change.File)
		}
	}
	if *check {
		return apperr.Exit(apperr.ExitFailed)
	}
	if len(shared) > 0 {
		fmt.Fprintf(s.out, "commit %s and open a PR\n", strings.Join(shared, listSep))
	}
	return nil
}

package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/tui"
)

// runDefault runs when no command is given: the terminal UI in a terminal
// inside a home repo, otherwise the help on stdout, as `help` prints it.
func runDefault(ctx context.Context, s streams, g globals) error {
	root, err := findRoot(g)
	if isNoRoot(err) {
		printUsage(s.out)
		fmt.Fprintf(s.out, "\nno %s here or in any parent directory; run `%s init <dir>` first\n", brand.TeamFile, brand.Name)
		return nil
	}
	if err != nil {
		return err
	}
	if !g.terminal {
		printUsage(s.out)
		return nil
	}
	// The version gate runs before the UI opens.
	home, err := loadRoot(s, root)
	if err != nil {
		return err
	}
	return openUI(ctx, home)
}

// isNoRoot reports whether the root search ended without a team file; a
// -C that names no directory is a usage error and is reported as one.
func isNoRoot(err error) bool {
	var noRoot *config.NoRootError
	return errors.As(err, &noRoot)
}

func runUI(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("ui")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s ui\n", brand.Name)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "ui takes no arguments")
	}
	if !g.terminal {
		return apperr.New(apperr.Usage, "ui needs a terminal: stdin and stdout must both be terminals")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	return openUI(ctx, home)
}

// openUI runs the terminal UI on a home repo that passed the version gate.
// Tests swap it through export_test.go, since the real UI needs a terminal.
var openUI = func(ctx context.Context, home *config.Home) error {
	return tui.Run(ctx, core.New(home))
}

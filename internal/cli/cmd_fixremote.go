package cli

import (
	"context"
	"fmt"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/core"
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
	_, err = core.New(home).FixRemote(ctx, names, s.output())
	return err
}

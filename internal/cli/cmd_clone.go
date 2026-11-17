package cli

import (
	"context"
	"fmt"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/core"
)

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
	_, err = core.New(home).Clone(ctx, names, s.output())
	return err
}

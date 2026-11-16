package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
)

func runInit(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("init")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s init <dir>\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "init takes exactly one directory")
	}
	dir := positional[0]
	if g.dir != "" && !filepath.IsAbs(dir) {
		dir = filepath.Join(g.dir, dir)
	}
	git, err := gitx.New(ctx)
	if err != nil {
		return err
	}
	if err := scaffold.Init(ctx, git, dir); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "initialized a home repo in %s (not committed)\n", dir)
	fmt.Fprintf(s.out, "next: cd %s, then `%s repo add <url>` for each product\n", dir, brand.Name)
	return nil
}

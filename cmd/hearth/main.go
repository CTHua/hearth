// Command hearth manages a home repo and the product checkouts under it.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/charmbracelet/x/term"

	"github.com/CTHua/hearth/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// isatty, not a char-device check: /dev/null is a char device too.
	terminal := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, terminal)
	stop()
	os.Exit(code)
}

// Command hearth manages a home repo and the product checkouts under it.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/CTHua/hearth/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

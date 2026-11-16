package cli

import (
	"context"
	"fmt"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
)

const (
	notSetLabel    = "not set"
	protocolSubcmd = "protocol"
)

func runConfig(ctx context.Context, s streams, g globals, args []string) error {
	if len(args) == 0 || args[0] != protocolSubcmd {
		return apperr.New(apperr.Usage, "usage: %s config protocol [%s|%s]", brand.Name, giturl.SSH, giturl.HTTPS)
	}
	return runConfigProtocol(ctx, s, g, args[1:])
}

func runConfigProtocol(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("config protocol")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s config protocol [%s|%s]\n", brand.Name, giturl.SSH, giturl.HTTPS)
		return nil
	}
	if len(positional) > 1 {
		return apperr.New(apperr.Usage, "config protocol takes at most one value")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	if len(positional) == 0 {
		current := home.Personal.Protocol
		if current == "" {
			current = notSetLabel
		}
		fmt.Fprintf(s.out, "protocol: %s\n", current)
		return nil
	}
	protocol := positional[0]
	if !config.ValidProtocol(protocol) {
		return apperr.New(apperr.Usage, "protocol must be %q or %q, not %q", giturl.SSH, giturl.HTTPS, protocol)
	}
	personal := home.Personal.WithProtocol(protocol)
	if err := config.WritePersonal(home.Root, personal); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "protocol: %s (saved to %s)\n", protocol, brand.LocalFile)
	return nil
}

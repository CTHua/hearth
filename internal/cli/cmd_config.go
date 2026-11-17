package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
)

const (
	notSetLabel    = "not set"
	protocolSubcmd = "protocol"
	agentsSubcmd   = "agents"
	// noAgents clears the agents list.
	noAgents       = "none"
	agentSeparator = ","
)

func runConfig(ctx context.Context, s streams, g globals, args []string) error {
	if len(args) > 0 && args[0] == protocolSubcmd {
		return runConfigProtocol(ctx, s, g, args[1:])
	}
	if len(args) > 0 && args[0] == agentsSubcmd {
		return runConfigAgents(ctx, s, g, args[1:])
	}
	return apperr.New(apperr.Usage, "usage: %s config protocol [%s|%s] or %s config agents [%s|%s]",
		brand.Name, giturl.SSH, giturl.HTTPS, brand.Name, agentsUsage(), noAgents)
}

func agentsUsage() string {
	return strings.Join(config.Agents(), agentSeparator)
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

func runConfigAgents(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("config agents")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s config agents [%s|%s]\n", brand.Name, agentsUsage(), noAgents)
		return nil
	}
	if len(positional) > 1 {
		return apperr.New(apperr.Usage, "config agents takes at most one comma-separated value")
	}
	var agents []string
	if len(positional) == 1 {
		agents, err = parseAgents(positional[0])
		if err != nil {
			return err
		}
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	if len(positional) == 0 {
		fmt.Fprintf(s.out, "agents: %s\n", agentsLabel(home.Personal.Agents))
		return nil
	}
	if err := config.WritePersonal(home.Root, home.Personal.WithAgents(agents)); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "agents: %s (saved to %s); run `%s sync`\n", agentsLabel(agents), brand.LocalFile, brand.Name)
	return nil
}

// parseAgents reads "claude,codex" or "none"; repeated agents count once.
func parseAgents(value string) ([]string, error) {
	if value == noAgents {
		return nil, nil
	}
	var agents []string
	for _, part := range strings.Split(value, agentSeparator) {
		agent := strings.TrimSpace(part)
		if !config.ValidAgent(agent) {
			return nil, apperr.New(apperr.Usage, "agent must be one of %s, or %q alone; not %q",
				strings.Join(config.Agents(), listSep), noAgents, agent)
		}
		if !slices.Contains(agents, agent) {
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

func agentsLabel(agents []string) string {
	if len(agents) == 0 {
		return noAgents
	}
	return strings.Join(agents, listSep)
}

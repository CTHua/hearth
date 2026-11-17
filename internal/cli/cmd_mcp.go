package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/mcp"
)

const (
	flagEnvVar         = "env-var"
	flagURL            = "url"
	flagBearerTokenEnv = "bearer-token-env"
	flagHeaderEnv      = "header-env"
	flagForce          = "force"
	headerEnvSep       = "="
	mcpGroup           = "mcp"
	noValue            = "-"
)

func runMCP(ctx context.Context, s streams, g globals, args []string) error {
	subcommands := map[string]command{
		"add":     runMCPAdd,
		"remove":  runMCPRemove,
		"enable":  runMCPEnable,
		"disable": runMCPDisable,
		"sync":    runMCPSync,
		"list":    runMCPList,
	}
	const choices = "add, remove, enable, disable, sync or list"
	if len(args) == 0 {
		return apperr.New(apperr.Usage, "mcp needs a subcommand: %s", choices)
	}
	run, found := subcommands[args[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown mcp subcommand %q; choose %s", args[0], choices)
	}
	return run(ctx, s, g, args[1:])
}

func runMCPAdd(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("mcp add")
	var envVars listFlag
	var headerEnvs listFlag
	fs.Var(&envVars, flagEnvVar, "")
	rawURL := fs.String(flagURL, "", "")
	bearer := fs.String(flagBearerTokenEnv, "", "")
	fs.Var(&headerEnvs, flagHeaderEnv, "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s mcp add <name> [--%s VAR]... -- <command> [args...]\n", brand.Name, flagEnvVar)
		fmt.Fprintf(s.out, "       %s mcp add <name> --%s <url> [--%s VAR] [--%s 'Header=VAR']...\n", brand.Name, flagURL, flagBearerTokenEnv, flagHeaderEnv)
		return nil
	}
	if len(positional) == 0 {
		return apperr.New(apperr.Usage, "mcp add takes a name, then `-- <command> [args...]` or --%s <url>", flagURL)
	}
	headers, err := parseHeaderEnvs(headerEnvs)
	if err != nil {
		return err
	}
	server := config.MCP{Name: positional[0], EnvVars: envVars, BearerTokenEnv: *bearer, HeaderEnvs: headers}
	if len(positional) > 1 {
		server.Command = positional[1]
		server.Args = positional[2:]
	}
	// A flag of the other transport is refused when given at all, even
	// empty: config only sees values and would let '' through.
	given := setFlags(fs)
	server.Transport = config.TransportStdio
	if given[flagURL] {
		server.Transport = config.TransportHTTP
		server.URL = *rawURL
	}
	if err := mixedFlags(server, given, len(positional) > 1); err != nil {
		return err
	}
	if server.Transport == config.TransportStdio && server.Command == "" {
		return apperr.New(apperr.Usage, "mcp add %s: give `-- <command> [args...]` for a stdio server, or --%s <url> for an http one", server.Name, flagURL)
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	team, err := home.Team.WithMCP(server)
	if err != nil {
		return entryProblems("add MCP server", err, nil)
	}
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "added MCP server %s (%s)\n", server.Name, server.Transport)
	fmt.Fprintf(s.out, "to use it: `%s mcp enable %s`, then `%s sync`\n", brand.Name, server.Name, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

// mixedFlags rejects what the other transport takes: for stdio the http
// flags, for http --env-var and a command after the name.
func mixedFlags(server config.MCP, given map[string]bool, hasCommand bool) error {
	var mixed []string
	if server.Transport == config.TransportHTTP {
		if given[flagEnvVar] {
			mixed = append(mixed, "--"+flagEnvVar)
		}
		if hasCommand {
			mixed = append(mixed, "a command")
		}
	} else {
		for _, flag := range []string{flagBearerTokenEnv, flagHeaderEnv} {
			if given[flag] {
				mixed = append(mixed, "--"+flag)
			}
		}
	}
	if len(mixed) == 0 {
		return nil
	}
	return apperr.New(apperr.Usage, "cannot add MCP server %s: %s cannot be used with transport %q; %s is unchanged",
		server.Name, strings.Join(mixed, listSep), server.Transport, brand.TeamFile)
}

// parseHeaderEnvs splits each 'Header=VAR' at the first '='.
func parseHeaderEnvs(values []string) ([]config.HeaderEnv, error) {
	var headers []config.HeaderEnv
	for _, value := range values {
		header, variable, found := strings.Cut(value, headerEnvSep)
		if !found {
			return nil, apperr.New(apperr.Usage, "--%s %q must be 'Header=VAR'", flagHeaderEnv, value)
		}
		headers = append(headers, config.HeaderEnv{Header: header, Var: variable})
	}
	return headers, nil
}

func runMCPRemove(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("mcp remove")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s mcp remove <name>\n", brand.Name)
		return nil
	}
	if len(positional) != 1 {
		return apperr.New(apperr.Usage, "mcp remove takes exactly one name")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	server, found := home.Team.FindMCP(positional[0])
	if !found {
		return apperr.New(apperr.Usage, "unknown MCP server %q in %s; servers: %s", positional[0], brand.TeamFile, labelList(home.Team.MCPNames()))
	}
	team, _ := home.Team.WithoutMCP(server.Name)
	if err := config.WriteTeam(home.Root, team); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed MCP server %s from %s; the next `%s sync` drops it from the generated files\n", server.Name, brand.TeamFile, brand.Name)
	fmt.Fprintln(s.out, brand.TeamFileChangedHint)
	return nil
}

func runMCPEnable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, mcpGroup, "enable")
	if err != nil || home == nil {
		return err
	}
	chosen := slices.Clone(home.Personal.MCP)
	var refused []string
	var added []string
	for _, name := range names {
		server, found := home.Team.FindMCP(name)
		switch {
		case !found:
			refused = append(refused, name)
		case !containsFold(chosen, server.Name):
			chosen = append(chosen, server.Name)
			added = append(added, server.Name)
		}
	}
	if len(refused) > 0 {
		return apperr.New(apperr.Usage, "cannot enable %s (no such MCP server); available: %s; %s is unchanged",
			strings.Join(refused, listSep), labelList(home.Team.MCPNames()), brand.LocalFile)
	}
	if err := config.WritePersonal(home.Root, home.Personal.WithMCPChoices(chosen)); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "enabled: %s (saved to %s); run `%s sync`\n", labelList(added), brand.LocalFile, brand.Name)
	return nil
}

// runMCPDisable also removes stale choices, names no server has any more,
// which is why it does not look at the team file.
func runMCPDisable(_ context.Context, s streams, g globals, args []string) error {
	names, home, err := personalNameArgs(s, g, args, mcpGroup, "disable")
	if err != nil || home == nil {
		return err
	}
	chosen := slices.Clone(home.Personal.MCP)
	var missing []string
	var removed []string
	for _, name := range names {
		index := slices.IndexFunc(chosen, func(c string) bool { return strings.EqualFold(c, name) })
		if index < 0 {
			if !containsFold(removed, name) {
				missing = append(missing, name)
			}
			continue
		}
		removed = append(removed, chosen[index])
		chosen = slices.Delete(chosen, index, index+1)
	}
	if len(missing) > 0 {
		return apperr.New(apperr.Usage, "not enabled: %s; enabled: %s; %s is unchanged",
			strings.Join(missing, listSep), labelList(home.Personal.MCP), brand.LocalFile)
	}
	if err := config.WritePersonal(home.Root, home.Personal.WithMCPChoices(chosen)); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "disabled: %s (saved to %s); run `%s sync`\n", strings.Join(removed, listSep), brand.LocalFile, brand.Name)
	return nil
}

// runMCPList only reads: it starts no process, connects to nothing and
// writes no file. Variables are listed by name, never by value.
func runMCPList(_ context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("mcp list")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s mcp list\n", brand.Name)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "mcp list takes no arguments")
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	rows := mcp.Inspect(home)
	table := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tTRANSPORT\tENABLED\tMISSING")
	for _, row := range rows {
		transport := row.Transport
		enabled := noLabel
		if row.Enabled {
			enabled = yesLabel
		}
		// A stale choice is treated as disabled, so it is neither yes nor no.
		if row.Stale {
			transport = noValue
			enabled = noValue
		}
		missing := strings.Join(row.MissingEnv, listSep)
		if missing == "" {
			missing = noValue
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", row.Name, transport, enabled, missing)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	for _, row := range rows {
		if row.Stale {
			apperr.Warn(s.err, apperr.ForRepo(apperr.Unknown, row.Name, "%s", mcp.StaleDetail(row.Name)))
		}
	}
	return nil
}

func runMCPSync(_ context.Context, s streams, g globals, args []string) error {
	force, help, err := syncArgs(s, args, "mcp sync")
	if err != nil || help {
		return err
	}
	return syncMCP(s, g, force)
}

// syncArgs parses `[--force]` and nothing else.
func syncArgs(s streams, args []string, name string) (bool, bool, error) {
	fs := newFlagSet(name)
	force := fs.Bool(flagForce, false, "")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return false, false, err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s %s [--%s]\n", brand.Name, name, flagForce)
		return false, true, nil
	}
	if len(positional) > 0 {
		return false, false, apperr.New(apperr.Usage, "%s takes no arguments", name)
	}
	return *force, false, nil
}

func syncMCP(s streams, g globals, force bool) error {
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	return syncMCPHome(s, home, force)
}

func syncMCPHome(s streams, home *config.Home, force bool) error {
	result, err := mcp.Sync(home, force, s.out, s.err)
	if err != nil {
		return err
	}
	if len(result.Failed) > 0 {
		return apperr.Exit(apperr.ExitFailed)
	}
	return nil
}

// runSync loads the home repo once, runs the skill sync, then the MCP sync
// whatever the skill sync's result, and exits with the worse code. A config
// that does not load stops both, so its errors print once. --force is only
// for MCP; the skill sync never sees it.
func runSync(ctx context.Context, s streams, g globals, args []string) error {
	force, help, err := syncArgs(s, args, "sync")
	if err != nil || help {
		return err
	}
	home, err := loadHome(s, g)
	if err != nil {
		return err
	}
	git, cat, skillErr := skillTools(ctx, home)
	if skillErr == nil {
		skillErr = syncSkills(ctx, s, home, git, cat)
	}
	// An interrupt stops the run instead of going on to MCP.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	skillCode := apperr.Report(s.err, skillErr)
	mcpCode := apperr.Report(s.err, syncMCPHome(s, home, force))
	code := max(skillCode, mcpCode)
	if code == apperr.ExitOK {
		return nil
	}
	return apperr.Exit(code)
}

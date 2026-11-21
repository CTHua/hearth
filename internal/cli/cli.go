// Package cli parses arguments and runs the subcommands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/version"
)

const usageText = `usage: %[1]s [-C <dir>] [<command> [<args>]]

Manage a home repo: the team's list of product repos and skills in %[2]s,
the product checkouts under products/, and the skills each person chooses.
With no command, in a terminal inside a home repo, the terminal UI opens;
otherwise this help is printed.

commands:
  ui                         open the terminal UI; stdin and stdout must be
                             terminals
  init <dir>                 create a new home repo in a missing or empty <dir>
  clone [<name>...]          clone the listed repos that are not cloned yet;
                             with no names, the repos chosen in
                             %[3]s (every repo when it has no
                             repos key)
  repo add <url> [--name <name>] [--path <path>] [--branch <branch>] [--description <text>]
                             add a repo to %[2]s
  repo remove <name>         remove a repo from %[2]s; the checkout is kept
  repo describe <name> <text>
                             set what a repo is for in %[2]s; an empty
                             <text> removes it
  repo list                  show every repo and the state of its checkout
  repo fix-remote [<name>...]
                             point origin at the expected URL when only the
                             protocol differs
  skill add <url> [--name <name>] [--commit <sha>] [--subdir <dir>]...
                             pin an external skill in %[2]s; without
                             --commit, the remote's HEAD commit. Without
                             --subdir the remote's SKILL.md files are
                             listed: one is added alone, several as a
                             group sharing one commit; several --subdir
                             make a group; --name <group> --subdir <dir>
                             adds to an existing group
  skill remove <name>|<group>|<group>/<name>
                             remove an external skill or a whole group
                             from %[2]s; a group's skill leaves its group;
                             <group>/<name> picks the group when several
                             have a skill of that name
  skill update <name>|<group> [--commit <sha>]
                             move the pin of a skill or a whole group
  skill enable <name>|<group>...
                             choose skills in %[3]s; a group stands for
                             all of its skills
  skill disable <name>|<group>...
                             stop using skills, including removed ones
  skill sync                 install the chosen skills into vendor/skills/
                             and link them for each chosen agent
  skill list                 show every skill and its state
  mcp add <name> [--env-var VAR]... -- <command> [args...]
  mcp add <name> --url <url> [--bearer-token-env VAR] [--header-env 'Header=VAR']...
                             add a stdio or http MCP server to %[2]s;
                             secrets are named by environment variable only
  mcp remove <name>          remove an MCP server from %[2]s
  mcp enable <name>...       choose MCP servers in %[3]s
  mcp disable <name>...      stop using MCP servers, including removed ones
  mcp sync [--force]         write .mcp.json (claude) and .codex/config.toml
                             (codex) for the chosen agents; --force
                             overwrites files edited by hand
  mcp list                   show every MCP server and missing variables
  subagent enable <name>...  choose subagents from subagents/ in %[3]s
  subagent disable <name>... stop using subagents, including removed ones
  subagent sync [--force]    write .claude/agents/<name>.md (claude) and
                             .codex/agents/<name>.toml (codex) for the
                             chosen agents; --force overwrites files
                             edited by hand
  subagent list              show every subagent and its state
  sync [--force]             clone the chosen repos that are not cloned
                             yet, then skill sync, mcp sync and subagent
                             sync; --force is for MCP and subagents
  config protocol [ssh|https]
                             show or set your protocol in %[3]s
  config agents [claude,codex|none]
                             show or set your agents in %[3]s
  doctor                     check git, both config files, every checkout,
                             every chosen skill, MCP server and subagent
  upgrade [--check]          rewrite the marked sections of AGENTS.md and
                             .gitignore and migrate both config files to
                             this version; --check only lists what would
                             change and exits 1 if anything would
  version                    print the version
  help                       show this help

options:
  -C <dir>  run as if started in <dir>; the home repo is the first directory
            at or above it that holds %[2]s

exit codes: 0 ok, 1 some repo, skill or generated file failed, or upgrade
--check found changes, 2 config/usage/env/version error, 130 interrupted
`

// streams bundles what a command writes to.
type streams struct {
	out io.Writer
	err io.Writer
}

// output is where core's batch actions stream their transcript.
func (s streams) output() core.Output {
	return core.Output{Out: s.out, Err: s.err}
}

// globals are the options given before the command, and whether the
// process runs in a terminal.
type globals struct {
	dir      string
	terminal bool
}

type command func(ctx context.Context, s streams, g globals, args []string) error

// Run executes one command line and returns the process exit code.
// terminal reports whether stdin and stdout are both terminals; only then
// may the terminal UI open.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, terminal bool) int {
	s := streams{out: stdout, err: stderr}
	err := dispatch(ctx, s, args, terminal)
	// An interrupt wins over whatever error the killed git child produced.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return apperr.Report(stderr, err)
}

func dispatch(ctx context.Context, s streams, args []string, terminal bool) error {
	fs := newFlagSet("")
	dir := fs.String("C", "", "")
	help, err := parseGlobal(fs, args)
	if err != nil {
		return err
	}
	if help {
		printUsage(s.out)
		return nil
	}
	g := globals{dir: *dir, terminal: terminal}
	rest := fs.Args()
	if len(rest) == 0 {
		return runDefault(ctx, s, g)
	}
	commands := map[string]command{
		"ui":       runUI,
		"upgrade":  runUpgrade,
		"init":     runInit,
		"clone":    runClone,
		"repo":     runRepo,
		"skill":    runSkill,
		"mcp":      runMCP,
		"subagent": runSubagent,
		"sync":     runSync,
		"config":   runConfig,
		"doctor":   runDoctor,
		"version":  runVersion,
		"help":     runHelp,
	}
	run, found := commands[rest[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown command %q; see `%s help`", rest[0], brand.Name)
	}
	return run(ctx, s, g, rest[1:])
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, usageText, brand.Name, brand.TeamFile, brand.LocalFile)
}

func runHelp(_ context.Context, s streams, _ globals, _ []string) error {
	printUsage(s.out)
	return nil
}

func runVersion(_ context.Context, s streams, _ globals, args []string) error {
	if len(args) > 0 {
		return apperr.New(apperr.Usage, "version takes no arguments")
	}
	fmt.Fprintln(s.out, version.Current())
	return nil
}

// startDir is where the root search begins: -C when given, else the cwd.
func startDir(g globals) (string, error) {
	if g.dir != "" {
		return g.dir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", apperr.New(apperr.Env, "cannot read the current directory: %v", err)
	}
	return cwd, nil
}

// findRoot is the home repo the command runs in.
func findRoot(g globals) (string, error) {
	start, err := startDir(g)
	if err != nil {
		return "", err
	}
	return config.FindRoot(start)
}

// loadHome finds the home repo and loads it through the version gate. Every
// command except version, help, init and upgrade goes through here before
// writing.
func loadHome(s streams, g globals) (*config.Home, error) {
	root, err := findRoot(g)
	if err != nil {
		return nil, err
	}
	return loadRoot(s, root)
}

func loadRoot(s streams, root string) (*config.Home, error) {
	home, err := config.Load(root, version.Current())
	if err != nil {
		return nil, err
	}
	for _, warning := range home.Warnings {
		fmt.Fprintln(s.err, warning)
	}
	return home, nil
}

func parseGlobal(fs *flag.FlagSet, args []string) (bool, error) {
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	if err != nil {
		return false, apperr.New(apperr.Usage, "%v; see `%s help`", err, brand.Name)
	}
	return false, nil
}

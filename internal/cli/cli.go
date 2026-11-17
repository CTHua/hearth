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
	"github.com/CTHua/hearth/internal/version"
)

const usageText = `usage: %[1]s [-C <dir>] <command> [<args>]

Manage a home repo: the team's list of product repos and skills in %[2]s,
the product checkouts under products/, and the skills each person chooses.

commands:
  init <dir>                 create a new home repo in a missing or empty <dir>
  clone [<name>...]          clone the listed repos that are not cloned yet
  repo add <url> [--name <name>] [--path <path>] [--branch <branch>]
                             add a repo to %[2]s
  repo remove <name>         remove a repo from %[2]s; the checkout is kept
  repo list                  show every repo and the state of its checkout
  repo fix-remote [<name>...]
                             point origin at the expected URL when only the
                             protocol differs
  skill add <url> [--name <name>] [--commit <sha>] [--subdir <dir>]
                             pin an external skill in %[2]s; without
                             --commit, the remote's HEAD commit
  skill remove <name>        remove an external skill from %[2]s
  skill update <name> [--commit <sha>]
                             move a skill's pin in %[2]s
  skill enable <name>...     choose skills in %[3]s
  skill disable <name>...    stop using skills, including removed ones
  skill sync                 install the chosen skills into vendor/skills/
                             and link them for each chosen agent
  skill list                 show every skill and its state
  config protocol [ssh|https]
                             show or set your protocol in %[3]s
  config agents [claude,codex|none]
                             show or set your agents in %[3]s
  doctor                     check git, both config files, every checkout
                             and every chosen skill
  version                    print the version
  help                       show this help

options:
  -C <dir>  run as if started in <dir>; the home repo is the first directory
            at or above it that holds %[2]s

exit codes: 0 ok, 1 some repo or skill failed, 2 config/usage/env/version
error, 130 interrupted
`

// streams bundles what a command writes to.
type streams struct {
	out io.Writer
	err io.Writer
}

// globals are the options given before the command.
type globals struct {
	dir string
}

type command func(ctx context.Context, s streams, g globals, args []string) error

// Run executes one command line and returns the process exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	s := streams{out: stdout, err: stderr}
	err := dispatch(ctx, s, args)
	// An interrupt wins over whatever error the killed git child produced.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return apperr.Report(stderr, err)
}

func dispatch(ctx context.Context, s streams, args []string) error {
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
	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(s.err)
		return apperr.New(apperr.Usage, "no command given")
	}
	commands := map[string]command{
		"init":    runInit,
		"clone":   runClone,
		"repo":    runRepo,
		"skill":   runSkill,
		"config":  runConfig,
		"doctor":  runDoctor,
		"version": runVersion,
		"help":    runHelp,
	}
	run, found := commands[rest[0]]
	if !found {
		return apperr.New(apperr.Usage, "unknown command %q; see `%s help`", rest[0], brand.Name)
	}
	return run(ctx, s, globals{dir: *dir}, rest[1:])
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

// loadHome finds the home repo and loads it through the version gate. Every
// command except version, help and init goes through here before writing.
func loadHome(s streams, g globals) (*config.Home, error) {
	start, err := startDir(g)
	if err != nil {
		return nil, err
	}
	root, err := config.FindRoot(start)
	if err != nil {
		return nil, err
	}
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

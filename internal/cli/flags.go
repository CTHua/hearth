package cli

import (
	"errors"
	"flag"
	"io"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

const endOfFlags = "--"

// newFlagSet returns a flag set that reports errors through apperr instead
// of printing its own usage.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parseArgs parses flags anywhere among the arguments, unlike flag.Parse,
// which stops at the first positional one; `repo add <url> --branch main`
// is the documented form. Everything after "--" is positional. help is true
// when -h or --help was given.
func parseArgs(fs *flag.FlagSet, args []string) (positional []string, help bool, err error) {
	for {
		before := len(args)
		err := fs.Parse(args)
		if errors.Is(err, flag.ErrHelp) {
			return nil, true, nil
		}
		if err != nil {
			return nil, false, apperr.New(apperr.Usage, "%s: %v; see `%s help`", fs.Name(), err, brand.Name)
		}
		rest := fs.Args()
		consumed := before - len(rest)
		if consumed > 0 && args[consumed-1] == endOfFlags {
			return append(positional, rest...), false, nil
		}
		if len(rest) == 0 {
			return positional, false, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// setFlags reports which flags were given on the command line, so an empty
// value given on purpose is told apart from an omitted flag.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

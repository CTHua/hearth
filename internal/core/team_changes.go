package core

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gitx"
)

// TeamTable is the team-file table an entry belongs to; names are unique
// only within one table.
type TeamTable string

const (
	TableRepos  TeamTable = "repos"
	TableSkills TeamTable = "skills"
	TableMCP    TeamTable = "mcp"
)

// ChangeKind is how an entry differs from the committed team file.
type ChangeKind string

const (
	Added    ChangeKind = "added"
	Removed  ChangeKind = "removed"
	Modified ChangeKind = "modified"
)

// TeamChange is one entry that differs from the team file in HEAD.
type TeamChange struct {
	Table TeamTable
	Name  string
	Kind  ChangeKind
}

const (
	headRevision = "HEAD:"
	changesSep   = "; "
)

// TeamChanges compares the team file with the one committed in HEAD, entry
// by entry, table by table (repos, skills, MCP). With nothing to compare
// with (not in a git repo, no commit yet, or no team file in HEAD) there are
// no changes and no error. A HEAD version this build cannot read is an
// error of one line.
func (s *Service) TeamChanges(ctx context.Context) ([]TeamChange, error) {
	git, err := gitx.New(ctx)
	if err != nil {
		return nil, err
	}
	// The home root may be a subdirectory of the repo; HEAD paths are
	// relative to the repo's top.
	prefix, err := git.Run(ctx, s.home.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, notComparable(err)
	}
	committed, err := git.Run(ctx, s.home.Root, "show", headRevision+strings.TrimSuffix(prefix, "\n")+brand.TeamFile)
	if err != nil {
		return nil, notComparable(err)
	}
	head, err := config.ParseTeam([]byte(committed))
	if err != nil {
		return nil, apperr.New(apperr.Config, "cannot compare %s with HEAD: %s", brand.TeamFile, strings.ReplaceAll(err.Error(), "\n", changesSep))
	}
	team := s.home.Team
	changes := diffEntries(TableRepos, head.Repos, team.Repos, func(r config.Repo) string { return r.Name }, func(a, b config.Repo) bool { return a == b })
	changes = append(changes, diffEntries(TableSkills, head.Skills, team.Skills, func(e config.Skill) string { return e.Name }, func(a, b config.Skill) bool { return a == b })...)
	changes = append(changes, diffEntries(TableMCP, head.MCP, team.MCP, func(m config.MCP) string { return m.Name }, sameMCP)...)
	return changes, nil
}

// notComparable keeps an interrupt and turns git's refusal (no repo, no
// HEAD, no such file) into "nothing to compare with".
func notComparable(err error) error {
	var runErr *gitx.RunError
	if errors.As(err, &runErr) {
		return nil
	}
	return err
}

// diffEntries lists the added and modified entries in current order, then
// the removed ones in committed order.
func diffEntries[T any](table TeamTable, committed []T, current []T, name func(T) string, same func(T, T) bool) []TeamChange {
	var changes []TeamChange
	for _, entry := range current {
		i := slices.IndexFunc(committed, func(old T) bool { return name(old) == name(entry) })
		switch {
		case i < 0:
			changes = append(changes, TeamChange{Table: table, Name: name(entry), Kind: Added})
		case !same(committed[i], entry):
			changes = append(changes, TeamChange{Table: table, Name: name(entry), Kind: Modified})
		}
	}
	for _, old := range committed {
		if !slices.ContainsFunc(current, func(entry T) bool { return name(entry) == name(old) }) {
			changes = append(changes, TeamChange{Table: table, Name: name(old), Kind: Removed})
		}
	}
	return changes
}

// sameMCP compares by value; an empty list equals a missing one, since the
// file cannot tell them apart.
func sameMCP(a config.MCP, b config.MCP) bool {
	return a.Name == b.Name && a.Transport == b.Transport && a.Command == b.Command && a.URL == b.URL &&
		a.BearerTokenEnv == b.BearerTokenEnv && slices.Equal(a.Args, b.Args) && slices.Equal(a.EnvVars, b.EnvVars) &&
		slices.Equal(a.HeaderEnvs, b.HeaderEnvs)
}

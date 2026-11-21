package config

import (
	"slices"
	"strings"
)

// RepoChosen reports whether p chooses the repo called name: every repo
// when the personal file has no repos key, otherwise only the listed ones,
// compared case-insensitively like the team file's names.
func (p Personal) RepoChosen(name string) bool {
	if p.Repos == nil {
		return true
	}
	return slices.ContainsFunc(p.Repos, func(chosen string) bool { return strings.EqualFold(chosen, name) })
}

// WithRepos returns p with the chosen repo names replaced. An empty, non-nil
// names is written as `repos = []` and chooses none; nil removes the key,
// which chooses every repo.
func (p Personal) WithRepos(names []string) Personal {
	next := p.started()
	next.Repos = slices.Clone(names)
	return next
}

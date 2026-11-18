// Package config reads and writes the team file and the personal file of a
// home repo, including the version gate that runs before strict parsing.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/giturl"
)

// SchemaVersion is the newest schema_version this build reads and the one it writes.
const SchemaVersion = 1

// MinVersion is the lowest release that reads SchemaVersion; init writes it
// regardless of which build runs init.
const MinVersion = "0.1.0"

// Repo is one [[repos]] entry of the team file. Description is optional; it
// is empty when the entry has none, and the key is then not written.
type Repo struct {
	Name        string `toml:"name"`
	URL         string `toml:"url"`
	Path        string `toml:"path"`
	Branch      string `toml:"branch"`
	Description string `toml:"description,omitempty"`
}

func (r Repo) field(name string) string {
	switch name {
	case FieldName:
		return r.Name
	case FieldURL:
		return r.URL
	case FieldPath:
		return r.Path
	case FieldBranch:
		return r.Branch
	case FieldDescription:
		return r.Description
	}
	return ""
}

// Team is the content of the team file.
type Team struct {
	SchemaVersion int64
	MinVersion    string
	Repos         []Repo
	Skills        []Skill
	MCP           []MCP
}

// Personal is the content of the personal file. Protocol is empty and the
// lists are nil when the file is missing or does not set them.
type Personal struct {
	SchemaVersion int64
	Protocol      string
	Agents        []string
	// Skills are the chosen skill names. A name may no longer exist in any
	// source; callers treat such a stale choice as not chosen.
	Skills []string
	// MCP are the chosen MCP server names; stale ones are treated the same
	// way as stale skills.
	MCP []string
	// Subagents are the chosen subagent names; stale ones are treated the
	// same way as stale skills.
	Subagents []string
}

// Home is a loaded home repo.
type Home struct {
	Root     string
	Team     Team
	Personal Personal
	// Warnings are non-fatal notes, such as a skipped version check.
	Warnings []string
}

// NewTeam is the team file init writes: no repos.
func NewTeam() Team {
	return Team{SchemaVersion: SchemaVersion, MinVersion: MinVersion}
}

// Find returns the repo called name.
func (t Team) Find(name string) (Repo, bool) {
	for _, repo := range t.Repos {
		if repo.Name == name {
			return repo, true
		}
	}
	return Repo{}, false
}

// Names lists the repo names in file order.
func (t Team) Names() []string {
	names := make([]string, 0, len(t.Repos))
	for _, repo := range t.Repos {
		names = append(names, repo.Name)
	}
	return names
}

// WithRepo returns t with repo appended. The error is FieldErrors when repo
// breaks a rule or conflicts with an existing entry.
func (t Team) WithRepo(repo Repo) (Team, error) {
	next := t
	next.Repos = append(slices.Clone(t.Repos), repo)
	if problems := checkRepos(next.Repos, nil, nil); len(problems) > 0 {
		return t, problems
	}
	return next, nil
}

// CheckNew reports, as FieldErrors, what WithRepo would reject about repo,
// leaving out the pending fields that are not known yet, such as a branch
// still to be asked from the remote. Conflicts with existing entries are
// checked in full.
func (t Team) CheckNew(repo Repo, pending ...string) error {
	repos := append(slices.Clone(t.Repos), repo)
	skip := map[fieldKey]bool{}
	for _, field := range pending {
		skip[fieldKey{index: len(repos) - 1, field: field}] = true
	}
	if problems := checkRepos(repos, nil, skip); len(problems) > 0 {
		return problems
	}
	return nil
}

// Without returns t without the repo called name, and whether it was there.
func (t Team) Without(name string) (Team, bool) {
	next := t
	next.Repos = slices.DeleteFunc(slices.Clone(t.Repos), func(r Repo) bool { return r.Name == name })
	return next, len(next.Repos) != len(t.Repos)
}

// WithDescription returns t with the description of the repo called name
// replaced, and whether that repo exists; an empty description removes it.
// The error is FieldErrors when description breaks a rule.
func (t Team) WithDescription(name string, description string) (Team, bool, error) {
	index := slices.IndexFunc(t.Repos, func(r Repo) bool { return r.Name == name })
	if index < 0 {
		return t, false, nil
	}
	next := t
	next.Repos = slices.Clone(t.Repos)
	next.Repos[index].Description = description
	if problems := checkRepos(next.Repos, nil, nil); len(problems) > 0 {
		return t, true, problems
	}
	return next, true, nil
}

// WithProtocol returns p with protocol set, starting a new personal file at
// the current schema when none was loaded.
func (p Personal) WithProtocol(protocol string) Personal {
	next := p.started()
	next.Protocol = protocol
	return next
}

// ValidProtocol reports whether p may be stored as the protocol.
func ValidProtocol(p string) bool {
	return p == giturl.SSH || p == giturl.HTTPS
}

// Load reads both files under root. running is the version of this build.
// Version keys of both files are checked before anything else, so a file from
// a newer release fails with a version error rather than with unknown keys.
func Load(root string, running string) (*Home, error) {
	teamData, err := os.ReadFile(filepath.Join(root, brand.TeamFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, apperr.New(apperr.Config, "%s not found in %s", brand.TeamFile, root)
	}
	if err != nil {
		return nil, apperr.New(apperr.Config, "cannot read %s: %v", brand.TeamFile, err)
	}
	personalData, hasPersonal, err := readOptional(filepath.Join(root, brand.LocalFile))
	if err != nil {
		return nil, apperr.New(apperr.Config, "cannot read %s: %v", brand.LocalFile, err)
	}
	teamDoc, err := decode(brand.TeamFile, teamData)
	if err != nil {
		return nil, err
	}
	var personalDoc document
	if hasPersonal {
		personalDoc, err = decode(brand.LocalFile, personalData)
		if err != nil {
			return nil, err
		}
	}
	home := &Home{Root: root}
	problems, warning := checkTeamVersions(teamDoc, running)
	if hasPersonal {
		problems = append(problems, checkSchema(personalDoc)...)
	}
	if warning != "" {
		home.Warnings = append(home.Warnings, warning)
	}
	if len(problems) > 0 {
		return nil, problems
	}
	team, problems := parseTeam(teamDoc)
	if hasPersonal {
		personal, personalProblems := parsePersonal(personalDoc)
		problems = append(problems, personalProblems...)
		home.Personal = personal
	}
	if len(problems) > 0 {
		return nil, problems
	}
	home.Team = team
	return home, nil
}

func readOptional(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func configError(file string, format string, args ...any) *apperr.Error {
	return apperr.New(apperr.Config, "%s: %s", file, fmt.Sprintf(format, args...))
}

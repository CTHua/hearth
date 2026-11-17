package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/brand"
)

// Field names of a [[skills]] entry that a [[repos]] entry does not have.
const (
	FieldCommit = "commit"
	FieldSubdir = "subdir"
)

// RootSubdir is the subdir of a skill that sits at the top of its repo.
const RootSubdir = "."

// Agents a person may choose.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

var (
	skillFields = []string{FieldName, FieldURL, FieldCommit, FieldSubdir}
	knownAgents = []string{AgentClaude, AgentCodex}
	// Full SHA-1 or SHA-256 object names; branches and tags would move.
	commitRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	// The same characters as a URL path segment in internal/giturl.
	subdirSegmentRE   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	forbiddenSegments = []string{".", "..", ".git"}
)

// Skill is one [[skills]] entry of the team file: an external skill pinned
// to a commit. Its checkout is always vendor/skills/<Name>.
type Skill struct {
	Name   string `toml:"name"`
	URL    string `toml:"url"`
	Commit string `toml:"commit"`
	Subdir string `toml:"subdir"`
}

func (s Skill) field(name string) string {
	switch name {
	case FieldName:
		return s.Name
	case FieldURL:
		return s.URL
	case FieldCommit:
		return s.Commit
	case FieldSubdir:
		return s.Subdir
	}
	return ""
}

// Agents lists the agents a person may choose.
func Agents() []string {
	return slices.Clone(knownAgents)
}

// ValidAgent reports whether a may be stored in the agents list.
func ValidAgent(a string) bool {
	return slices.Contains(knownAgents, a)
}

// IsReserved reports whether name is taken by the built-in skill; names are
// compared case-insensitively.
func IsReserved(name string) bool {
	return strings.EqualFold(name, brand.BuiltinSkill)
}

func reservedProblem() string {
	return fmt.Sprintf("%q is reserved for the built-in skill (names are compared case-insensitively)", brand.BuiltinSkill)
}

func skillValueProblem(field string, value string) string {
	switch field {
	case FieldName:
		if problem := segmentProblem(value); problem != "" {
			return problem
		}
		if IsReserved(value) {
			return reservedProblem()
		}
	case FieldCommit:
		if !commitRE.MatchString(value) {
			return fmt.Sprintf("%q must be a full lowercase commit SHA (40 or 64 hex characters), not a branch or tag", value)
		}
	case FieldSubdir:
		return subdirProblem(value)
	default:
		return valueProblem(field, value)
	}
	return ""
}

func subdirProblem(value string) string {
	if value == RootSubdir {
		return ""
	}
	for _, segment := range strings.Split(value, pathSeparator) {
		if !subdirSegmentRE.MatchString(segment) || slices.Contains(forbiddenSegments, segment) {
			return fmt.Sprintf("%q must be %q or a relative path of letters, digits, '.', '_' and '-' without '.', '..' or '.git' segments", value, RootSubdir)
		}
	}
	return ""
}

// checkSkills validates every value, then names across entries.
func checkSkills(skills []Skill, reported FieldErrors, pending map[fieldKey]bool) FieldErrors {
	table := entryTable{
		name:    keySkills,
		fields:  skillFields,
		value:   func(i int, field string) string { return skills[i].field(field) },
		problem: skillValueProblem,
	}
	valid, problems := checkEntries(table, len(skills), reported, pending)
	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.Name
	}
	return append(problems, duplicateNames(keySkills, names, valid)...)
}

// FindSkill returns the external skill called name. Like every skill name
// lookup it ignores case; the entry keeps the case it was written with.
func (t Team) FindSkill(name string) (Skill, bool) {
	for _, skill := range t.Skills {
		if strings.EqualFold(skill.Name, name) {
			return skill, true
		}
	}
	return Skill{}, false
}

// SkillNames lists the external skill names in file order.
func (t Team) SkillNames() []string {
	names := make([]string, 0, len(t.Skills))
	for _, skill := range t.Skills {
		names = append(names, skill.Name)
	}
	return names
}

// WithSkill returns t with skill appended. The error is FieldErrors when
// skill breaks a rule or conflicts with an existing entry.
func (t Team) WithSkill(skill Skill) (Team, error) {
	next := t
	next.Skills = append(slices.Clone(t.Skills), skill)
	if problems := checkSkills(next.Skills, nil, nil); len(problems) > 0 {
		return t, problems
	}
	return next, nil
}

// CheckNewSkill reports, as FieldErrors, what WithSkill would reject about
// skill, leaving out the pending fields that are not known yet, such as a
// commit still to be asked from the remote.
func (t Team) CheckNewSkill(skill Skill, pending ...string) error {
	skills := append(slices.Clone(t.Skills), skill)
	skip := map[fieldKey]bool{}
	for _, field := range pending {
		skip[fieldKey{index: len(skills) - 1, field: field}] = true
	}
	if problems := checkSkills(skills, nil, skip); len(problems) > 0 {
		return problems
	}
	return nil
}

// WithSkillCommit returns t with the skill called name pinned to commit,
// keeping its place in the file. The error is FieldErrors when commit is not
// a full SHA; name must exist.
func (t Team) WithSkillCommit(name string, commit string) (Team, error) {
	next := t
	next.Skills = slices.Clone(t.Skills)
	for i := range next.Skills {
		if strings.EqualFold(next.Skills[i].Name, name) {
			next.Skills[i].Commit = commit
		}
	}
	if problems := checkSkills(next.Skills, nil, nil); len(problems) > 0 {
		return t, problems
	}
	return next, nil
}

// WithoutSkill returns t without the skill called name, and whether it was there.
func (t Team) WithoutSkill(name string) (Team, bool) {
	next := t
	next.Skills = slices.DeleteFunc(slices.Clone(t.Skills), func(s Skill) bool { return strings.EqualFold(s.Name, name) })
	return next, len(next.Skills) != len(t.Skills)
}

// WithAgents returns p with the agents list replaced.
func (p Personal) WithAgents(agents []string) Personal {
	next := p.started()
	next.Agents = slices.Clone(agents)
	return next
}

// WithSkills returns p with the chosen skill names replaced.
func (p Personal) WithSkills(names []string) Personal {
	next := p.started()
	next.Skills = slices.Clone(names)
	return next
}

// started returns p at the current schema when no personal file was loaded.
func (p Personal) started() Personal {
	next := p
	if next.SchemaVersion == 0 {
		next.SchemaVersion = SchemaVersion
	}
	return next
}

// listProblems checks a personal string array: each value with check, then
// duplicates, compared case-insensitively.
func listProblems(key string, values []string, check func(string) string) []string {
	var problems []string
	seen := map[string]int{}
	for i, value := range values {
		if message := check(value); message != "" {
			problems = append(problems, fmt.Sprintf("%s[%d]: %s", key, i, message))
			continue
		}
		lower := strings.ToLower(value)
		if first, dup := seen[lower]; dup {
			problems = append(problems, fmt.Sprintf("%s[%d]: duplicate %q (also %s[%d], compared case-insensitively)", key, i, value, key, first))
			continue
		}
		seen[lower] = i
	}
	return problems
}

func agentProblem(value string) string {
	if !ValidAgent(value) {
		return fmt.Sprintf("%q must be one of %s", value, strings.Join(knownAgents, ", "))
	}
	return ""
}

// chosenSkillProblem checks only the form of a chosen name: a name that no
// skill has is a stale choice the commands warn about, not a config error,
// so a teammate removing a skill cannot break everyone's commands.
func chosenSkillProblem(value string) string {
	if problem := NameProblem(value); problem != "" {
		return problem
	}
	if IsReserved(value) {
		return reservedProblem() + "; it is installed whenever an agent is chosen"
	}
	return ""
}

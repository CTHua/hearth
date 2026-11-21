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
	// FieldSubdirs makes an entry a group: one checkout, one skill per subdir.
	FieldSubdirs = "subdirs"
)

// RootSubdir is the subdir of a skill that sits at the top of its repo.
const RootSubdir = "."

// Agents a person may choose.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

var (
	// skillFields are the string fields every [[skills]] entry sets; it sets
	// exactly one of subdir and subdirs as well.
	skillFields = []string{FieldName, FieldURL, FieldCommit}
	knownAgents = []string{AgentClaude, AgentCodex}
	// Full SHA-1 or SHA-256 object names; branches and tags would move.
	commitRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	// The same characters as a URL path segment in internal/giturl.
	subdirSegmentRE   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	forbiddenSegments = []string{".", "..", ".git"}
)

// Skill is one [[skills]] entry of the team file: an external skill pinned
// to a commit, or a group of skills from one repo sharing that commit. Its
// checkout is always vendor/skills/<Name>. Exactly one of Subdir and Subdirs
// is set; with Subdirs, Name is the group's and each member is named after
// the last segment of its subdir.
type Skill struct {
	Name   string `toml:"name"`
	URL    string `toml:"url"`
	Commit string `toml:"commit"`
	Subdir string `toml:"subdir,omitempty"`
	// Subdirs is non-nil, even when empty, whenever the file sets the key.
	Subdirs []string `toml:"subdirs,omitempty"`
}

// IsGroup reports whether the entry is a group of skills.
func (s Skill) IsGroup() bool {
	return s.Subdirs != nil
}

// Members returns the subdir of every skill of the entry in file order: the
// one subdir of a single skill, or every subdir of a group.
func (s Skill) Members() []string {
	if s.IsGroup() {
		return slices.Clone(s.Subdirs)
	}
	return []string{s.Subdir}
}

// MemberName is the name of the group member at subdir: its last segment.
func MemberName(subdir string) string {
	segments := strings.Split(subdir, pathSeparator)
	return segments[len(segments)-1]
}

// Member is one skill of a group: the group's entry and the member's subdir.
type Member struct {
	Group  Skill
	Subdir string
}

// FindMembers returns every group member called name, compared
// case-insensitively, in file order. Two groups may have members of the
// same name; the catalog then marks both invalid.
func (t Team) FindMembers(name string) []Member {
	var members []Member
	for _, entry := range t.Skills {
		for _, subdir := range entry.Subdirs {
			if strings.EqualFold(MemberName(subdir), name) {
				members = append(members, Member{Group: entry, Subdir: subdir})
			}
		}
	}
	return members
}

// FindMember returns the first group that has a member called name,
// compared case-insensitively, and that member's subdir.
func (t Team) FindMember(name string) (Skill, string, bool) {
	members := t.FindMembers(name)
	if len(members) == 0 {
		return Skill{}, "", false
	}
	return members[0].Group, members[0].Subdir, true
}

func (s Skill) field(name string) string {
	switch name {
	case FieldName:
		return s.Name
	case FieldURL:
		return s.URL
	case FieldCommit:
		return s.Commit
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
	for i, skill := range skills {
		if hasReported(reported, i, FieldSubdir) || hasReported(reported, i, FieldSubdirs) {
			continue
		}
		subdirProblems := memberProblems(i, skill)
		if len(subdirProblems) > 0 {
			valid[i] = false
			problems = append(problems, subdirProblems...)
		}
	}
	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.Name
	}
	problems = append(problems, duplicateNames(keySkills, names, valid)...)
	return append(problems, groupNameConflicts(skills, valid)...)
}

func hasReported(reported FieldErrors, index int, field string) bool {
	return slices.ContainsFunc(reported, func(e FieldError) bool { return e.Index == index && e.Field == field })
}

// bothSubdirsProblem is the message for an entry that sets both subdir and
// subdirs; the parser reports it whenever both keys are written, even with
// an empty subdir.
func bothSubdirsProblem() string {
	return fmt.Sprintf("set either %s or %s, not both", FieldSubdir, FieldSubdirs)
}

// memberProblems checks that the entry at index sets exactly one of subdir
// and subdirs, and every subdir and member name it sets.
func memberProblems(index int, s Skill) FieldErrors {
	problem := func(field string, message string) FieldError {
		return FieldError{Table: keySkills, Index: index, Field: field, Message: message}
	}
	switch {
	case s.Subdir != "" && s.IsGroup():
		return FieldErrors{problem(FieldSubdirs, bothSubdirsProblem())}
	case !s.IsGroup():
		if s.Subdir == "" {
			return FieldErrors{problem(FieldSubdir, fmt.Sprintf("missing; set %s, or %s for a group", FieldSubdir, FieldSubdirs))}
		}
		if message := subdirProblem(s.Subdir); message != "" {
			return FieldErrors{problem(FieldSubdir, message)}
		}
		return nil
	case len(s.Subdirs) == 0:
		return FieldErrors{problem(FieldSubdirs, "must be a non-empty array of strings")}
	}
	var problems FieldErrors
	seenSubdirs := map[string]int{}
	seenNames := map[string]int{}
	for i, subdir := range s.Subdirs {
		field := fmt.Sprintf("%s[%d]", FieldSubdirs, i)
		if message := groupSubdirProblem(subdir); message != "" {
			problems = append(problems, problem(field, message))
			continue
		}
		if first, dup := seenSubdirs[subdir]; dup {
			problems = append(problems, problem(field, fmt.Sprintf("duplicate subdir %q (also %s[%d])", subdir, FieldSubdirs, first)))
			continue
		}
		seenSubdirs[subdir] = i
		name := MemberName(subdir)
		if message := MemberNameProblem(name); message != "" {
			problems = append(problems, problem(field, fmt.Sprintf("member name %s", message)))
			continue
		}
		lower := strings.ToLower(name)
		if first, dup := seenNames[lower]; dup {
			message := fmt.Sprintf("member name %q is also the name of %s[%d] (names are compared case-insensitively)", name, FieldSubdirs, first)
			problems = append(problems, problem(field, message))
			continue
		}
		seenNames[lower] = i
	}
	return problems
}

// groupSubdirProblem is subdirProblem without the repo's top: a group
// member names its own directory.
func groupSubdirProblem(value string) string {
	if value == "" {
		return "must be a non-empty string"
	}
	if value == RootSubdir {
		return fmt.Sprintf("%q cannot be a group member; use %s for a skill at the top of its repo", RootSubdir, FieldSubdir)
	}
	return subdirProblem(value)
}

// MemberNameProblem explains why name, the last segment of a subdir, cannot
// be a group member's name; it is empty when it can.
func MemberNameProblem(name string) string {
	if problem := segmentProblem(name); problem != "" {
		return problem
	}
	if IsReserved(name) {
		return reservedProblem()
	}
	return ""
}

// groupNameConflicts rejects a group named like a member of any group,
// itself included; equal entry names are left to duplicateNames. O(n*m)
// over the members of every group; fine for the dozens a team lists.
func groupNameConflicts(skills []Skill, valid []bool) FieldErrors {
	var problems FieldErrors
	for i, group := range skills {
		if !valid[i] || !group.IsGroup() {
			continue
		}
		for j, other := range skills {
			if !valid[j] || !other.IsGroup() {
				continue
			}
			index := slices.IndexFunc(other.Subdirs, func(subdir string) bool { return strings.EqualFold(MemberName(subdir), group.Name) })
			if index < 0 {
				continue
			}
			message := fmt.Sprintf("group name %q is also the name of the member %s[%d].%s[%d] (names are compared case-insensitively)",
				group.Name, keySkills, j, FieldSubdirs, index)
			problems = append(problems, FieldError{Table: keySkills, Index: i, Field: FieldName, Message: message})
			break
		}
	}
	return problems
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

// WithSkillSubdirs returns t with the subdirs of the group called name
// replaced, keeping its place in the file. The error is FieldErrors when the
// new subdirs break a rule or conflict with another entry; name must exist.
func (t Team) WithSkillSubdirs(name string, subdirs []string) (Team, error) {
	next := t
	next.Skills = slices.Clone(t.Skills)
	for i := range next.Skills {
		if strings.EqualFold(next.Skills[i].Name, name) {
			next.Skills[i].Subdirs = slices.Clone(subdirs)
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
		next.SchemaVersion = PersonalSchemaVersion
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

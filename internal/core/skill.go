package core

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	readyLabel  = "ready"
	warnedLabel = "warned"
	// groupMemberSep joins a group and one of its skills in
	// `skill remove <group>/<skill>`.
	groupMemberSep = pathSep
)

// skillFlags are the skill add flags that set a field of the same name.
var skillFlags = []string{config.FieldName, config.FieldCommit, config.FieldSubdir}

// Reasons a skill found on the remote was not added to the new group.
const (
	notInPlugin    = "not listed in " + skill.PluginFile
	duplicateLast  = "another skill found has the same last path segment %q (names are compared case-insensitively)"
	teamSkillTaken = "name %q is already used by the %s skill at %s"
)

// AddSkillParams describes a new [[skills]] entry. A nil field was not
// given. With no Subdirs the remote is listed: one skill becomes a single
// entry, several a group. One subdir is a single skill, several a group.
// Name defaults to the last subdir segment for a single skill, and to the
// plugin manifest's name or the URL's last segment for a group; Commit is
// the remote's HEAD. When Name is an existing group of the same repo and
// Subdirs are given, they are added to that group instead.
type AddSkillParams struct {
	URL     string
	Name    *string
	Commit  *string
	Subdirs []string
}

// SkippedSkill is a skill found on the remote that was not added, and why.
type SkippedSkill struct {
	Dir    string
	Reason string
}

// AddSkillResult is the entry that was added, or the group that grew.
// DetectedProtocol is set when the protocol was detected and saved, even
// when adding then failed.
type AddSkillResult struct {
	Skill config.Skill
	// Added are the subdirs this call added: the one subdir of a single
	// skill, or every new member of a group.
	Added []string
	// Extended is set when Added went into an existing group.
	Extended bool
	// Listed is set when the remote was listed; Found then counts the
	// skills found there and Skipped are those not added.
	Listed  bool
	Found   int
	Skipped []SkippedSkill
	// DetectedProtocol is set when the protocol was detected and saved.
	DetectedProtocol string
}

// Lines are what the CLI and the terminal UI say about an added skill: the
// entry, how many skills the remote had, and each one left out with why.
func (r AddSkillResult) Lines() []string {
	entry := r.Skill
	var lines []string
	switch {
	case r.Extended:
		lines = append(lines, fmt.Sprintf("added %d skill(s) to group %s at %s: %s", len(r.Added), entry.Name, entry.Commit, LabelList(memberNames(r.Added))))
	case entry.IsGroup():
		lines = append(lines, fmt.Sprintf("added group %s at %s: %d skill(s) added", entry.Name, entry.Commit, len(r.Added)))
	default:
		lines = append(lines, fmt.Sprintf("added skill %s at %s (subdir %s)", entry.Name, entry.Commit, entry.Subdir))
	}
	if r.Listed {
		lines = append(lines, fmt.Sprintf("found %d skill(s) in %s", r.Found, entry.URL))
	}
	if len(r.Skipped) > 0 {
		lines = append(lines, fmt.Sprintf("not added (%d):", len(r.Skipped)))
		for _, skipped := range r.Skipped {
			lines = append(lines, fmt.Sprintf("  %s: %s", skipped.Dir, skipped.Reason))
		}
		lines = append(lines, fmt.Sprintf("add one with `%s skill add %s --name %s --subdir <path>`", brand.Name, entry.URL, entry.Name))
	}
	return lines
}

// EnableHint is the line telling how to start using what was added.
func (r AddSkillResult) EnableHint() string {
	enable := r.Skill.Name
	if r.Extended {
		enable = strings.Join(memberNames(r.Added), " ")
	}
	return fmt.Sprintf("to use it: `%s skill enable %s`, then `%s skill sync`", brand.Name, enable, brand.Name)
}

func memberNames(subdirs []string) []string {
	names := make([]string, 0, len(subdirs))
	for _, subdir := range subdirs {
		names = append(names, config.MemberName(subdir))
	}
	return names
}

// AddSkill pins an external skill, or a group of them, in the team file.
// Everything known locally is checked before the remote is contacted.
func (s *Service) AddSkill(ctx context.Context, p AddSkillParams) (AddSkillResult, error) {
	u, err := giturl.Parse(p.URL)
	if err != nil {
		return AddSkillResult{}, apperr.New(apperr.Usage, "url %q: %v", p.URL, err)
	}
	if p.Name != nil && len(p.Subdirs) > 0 {
		if existing, found := s.home.Team.FindSkill(*p.Name); found && sameRepo(existing.URL, p.URL) {
			return s.extendGroup(existing, p)
		}
	}
	switch len(p.Subdirs) {
	case 0:
		return s.addListed(ctx, u, p)
	case 1:
		name := config.MemberName(p.Subdirs[0])
		if p.Subdirs[0] == config.RootSubdir {
			name = u.DefaultName()
		}
		entry := config.Skill{Name: valueOr(p.Name, name), URL: p.URL, Commit: valueOr(p.Commit, ""), Subdir: p.Subdirs[0]}
		return s.addEntry(ctx, entry, p.Commit == nil, AddSkillResult{Added: entry.Members()})
	}
	entry := config.Skill{Name: valueOr(p.Name, u.DefaultName()), URL: p.URL, Commit: valueOr(p.Commit, ""), Subdirs: slices.Clone(p.Subdirs)}
	for _, member := range entry.Subdirs {
		if err := s.sourceConflict("add skill", config.MemberName(member)); err != nil {
			return AddSkillResult{}, err
		}
	}
	return s.addEntry(ctx, entry, p.Commit == nil, AddSkillResult{Added: entry.Members()})
}

func valueOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

// sameRepo reports whether two team-file URLs name the same repo, whatever
// the protocol they are written in.
func sameRepo(a string, b string) bool {
	first, err := giturl.Parse(a)
	if err != nil {
		return false
	}
	second, err := giturl.Parse(b)
	return err == nil && first == second
}

// addEntry checks entry, pins it to the remote's HEAD when askCommit is set,
// and writes it.
func (s *Service) addEntry(ctx context.Context, entry config.Skill, askCommit bool, result AddSkillResult) (AddSkillResult, error) {
	var pending []string
	if askCommit {
		pending = append(pending, config.FieldCommit)
	}
	if err := s.home.Team.CheckNewSkill(entry, pending...); err != nil {
		return result, entryProblems("add skill", err, skillFlags)
	}
	if err := s.sourceConflict("add skill", entry.Name); err != nil {
		return result, err
	}
	if askCommit {
		commit, detected, err := s.remoteHeadCommit(ctx, entry.URL)
		result.DetectedProtocol = detected
		if err != nil {
			return result, err
		}
		entry.Commit = commit
	}
	return s.writeNewSkill(entry, result)
}

func (s *Service) writeNewSkill(entry config.Skill, result AddSkillResult) (AddSkillResult, error) {
	team, err := s.home.Team.WithSkill(entry)
	if err != nil {
		return result, entryProblems("add skill", err, skillFlags)
	}
	if err := s.saveTeam(team); err != nil {
		return result, err
	}
	result.Skill = entry
	return result, nil
}

// extendGroup adds the given subdirs to the existing entry of the same repo
// called p.Name, keeping its pin.
func (s *Service) extendGroup(group config.Skill, p AddSkillParams) (AddSkillResult, error) {
	switch {
	case !group.IsGroup():
		return AddSkillResult{}, apperr.New(apperr.Usage, "cannot add skill: %q is a single skill (%s %q), not a group; %s is unchanged",
			group.Name, config.FieldSubdir, group.Subdir, brand.TeamFile)
	case p.Commit != nil:
		return AddSkillResult{}, apperr.New(apperr.Usage, "cannot add skill: skills added to group %q keep its commit %s; drop --%s, or move the group with `%s skill update %s`; %s is unchanged",
			group.Name, group.Commit, config.FieldCommit, brand.Name, group.Name, brand.TeamFile)
	}
	for _, subdir := range p.Subdirs {
		if slices.Contains(group.Subdirs, subdir) {
			return AddSkillResult{}, apperr.New(apperr.Usage, "cannot add skill: %s is already in group %q; %s is unchanged", subdir, group.Name, brand.TeamFile)
		}
		if err := s.sourceConflict("add skill", config.MemberName(subdir)); err != nil {
			return AddSkillResult{}, err
		}
	}
	team, err := s.home.Team.WithSkillSubdirs(group.Name, append(slices.Clone(group.Subdirs), p.Subdirs...))
	if err != nil {
		return AddSkillResult{}, entryProblems("add skill", err, skillFlags)
	}
	if err := s.saveTeam(team); err != nil {
		return AddSkillResult{}, err
	}
	updated, _ := team.FindSkill(group.Name)
	return AddSkillResult{Skill: updated, Added: slices.Clone(p.Subdirs), Extended: true}, nil
}

// addListed lists the remote's skills at the pin and adds them: one as a
// single skill, several as a group.
func (s *Service) addListed(ctx context.Context, u giturl.URL, p AddSkillParams) (AddSkillResult, error) {
	result := AddSkillResult{Listed: true}
	// The name may come from the remote, so only a given one is checked now.
	probe := config.Skill{Name: valueOr(p.Name, ""), URL: p.URL, Commit: valueOr(p.Commit, ""), Subdir: config.RootSubdir}
	pending := []string{}
	if p.Name == nil {
		pending = append(pending, config.FieldName)
	}
	if p.Commit == nil {
		pending = append(pending, config.FieldCommit)
	}
	if err := s.home.Team.CheckNewSkill(probe, pending...); err != nil {
		return result, entryProblems("add skill", err, skillFlags)
	}
	if p.Name != nil {
		if err := s.sourceConflict("add skill", *p.Name); err != nil {
			return result, err
		}
	}
	git, url, detected, err := s.remoteURL(ctx, p.URL)
	result.DetectedProtocol = detected
	if err != nil {
		return result, err
	}
	listing, err := skill.ListRemote(ctx, git, url, valueOr(p.Commit, ""))
	if err != nil {
		return result, err
	}
	result.Found = len(listing.Dirs)
	switch {
	case len(listing.Dirs) == 0:
		return result, apperr.New(apperr.Usage, "cannot add skill: no %s found in %s at %s; %s is unchanged", skill.File, url, listing.Commit, brand.TeamFile)
	case slices.Contains(listing.Dirs, config.RootSubdir) || len(listing.Dirs) == 1:
		subdir := config.RootSubdir
		name := u.DefaultName()
		if !slices.Contains(listing.Dirs, config.RootSubdir) {
			subdir = listing.Dirs[0]
			name = config.MemberName(subdir)
		}
		entry := config.Skill{Name: valueOr(p.Name, name), URL: p.URL, Commit: listing.Commit, Subdir: subdir}
		result.Added = []string{subdir}
		return s.addFound(entry, result)
	}
	cat, err := skill.Discover(s.home)
	if err != nil {
		return result, err
	}
	members, skipped := pickMembers(cat, listing)
	result.Skipped = skipped
	if len(members) == 0 {
		return result, apperr.New(apperr.Usage, "cannot add skill: none of the %d skills found in %s can be added; %s is unchanged", len(listing.Dirs), url, brand.TeamFile)
	}
	name := u.DefaultName()
	if listing.PluginName != "" && config.NameProblem(listing.PluginName) == "" && !config.IsReserved(listing.PluginName) {
		name = listing.PluginName
	}
	entry := config.Skill{Name: valueOr(p.Name, name), URL: p.URL, Commit: listing.Commit, Subdirs: members}
	result.Added = slices.Clone(members)
	return s.addFound(entry, result)
}

// addFound checks and writes an entry made from a listing; a name a team
// or the built-in skill has asks for --name.
func (s *Service) addFound(entry config.Skill, result AddSkillResult) (AddSkillResult, error) {
	cat, err := skill.Discover(s.home)
	if err != nil {
		return result, err
	}
	if item, taken := otherSource(cat, entry.Name); taken {
		return result, apperr.New(apperr.Usage, "cannot add skill: name %q is already used by the %s skill at %s (names are compared case-insensitively); choose another with --%s; %s is unchanged",
			entry.Name, item.Source, item.Dir, config.FieldName, brand.TeamFile)
	}
	return s.writeNewSkill(entry, result)
}

// pickMembers keeps the listed skills the plugin manifest selects whose
// name can be used, and says why each other one was left out. O(n^2) over
// the skills of one repo; fine for the dozens a repo holds.
func pickMembers(cat skill.Catalog, listing skill.Listing) ([]string, []SkippedSkill) {
	var members []string
	var skipped []SkippedSkill
	for _, dir := range listing.Dirs {
		name := config.MemberName(dir)
		reason := ""
		item, taken := otherSource(cat, name)
		switch {
		case !slices.Contains(listing.Selected, dir):
			reason = notInPlugin
		case config.MemberNameProblem(name) != "":
			reason = "name " + config.MemberNameProblem(name)
		// Only selected dirs can join, so a dir the manifest left out cannot
		// clash with a selected one.
		case slices.ContainsFunc(listing.Selected, func(other string) bool { return other != dir && strings.EqualFold(config.MemberName(other), name) }):
			reason = fmt.Sprintf(duplicateLast, name)
		case taken:
			reason = fmt.Sprintf(teamSkillTaken, name, item.Source, item.Dir)
		}
		if reason != "" {
			skipped = append(skipped, SkippedSkill{Dir: dir, Reason: reason})
			continue
		}
		members = append(members, dir)
	}
	return members, skipped
}

// otherSource returns the team or built-in skill of cat called name,
// compared case-insensitively.
func otherSource(cat skill.Catalog, name string) (skill.Item, bool) {
	for _, item := range cat.Items() {
		if item.Source != skill.External && strings.EqualFold(item.Name, name) {
			return item, true
		}
	}
	return skill.Item{}, false
}

// sourceConflict rejects a name a team skill directory or the built-in skill
// already uses; the team file itself is checked by config.
// It looks past external entries, since the one being updated is itself
// in the catalog.
func (s *Service) sourceConflict(action string, name string) error {
	cat, err := skill.Discover(s.home)
	if err != nil {
		return err
	}
	if item, taken := otherSource(cat, name); taken {
		return apperr.New(apperr.Usage, "cannot %s: name %q is already used by the %s skill at %s (names are compared case-insensitively); %s is unchanged",
			action, name, item.Source, item.Dir, brand.TeamFile)
	}
	return nil
}

// remoteHeadCommit asks the remote which commit its HEAD points to. The
// second result is the protocol it detected, if any.
func (s *Service) remoteHeadCommit(ctx context.Context, rawURL string) (string, string, error) {
	git, url, detected, err := s.remoteURL(ctx, rawURL)
	if err != nil {
		return "", detected, err
	}
	commit, found, err := git.RemoteHead(ctx, url)
	if err != nil {
		if ctx.Err() != nil {
			return "", detected, ctx.Err()
		}
		return "", detected, apperr.New(apperr.Env, "cannot ask %s for its HEAD commit: %v; pass --%s", url, err, config.FieldCommit)
	}
	if !found {
		return "", detected, apperr.New(apperr.Usage, "%s did not report a HEAD commit; pass --%s", url, config.FieldCommit)
	}
	return commit, detected, nil
}

// RemoveSkillResult is the removed skill and its vendor checkout, which is
// never deleted. Group is set when only one member left its group; the
// shared checkout then stays in use.
type RemoveSkillResult struct {
	Name       string
	Group      string
	Vendor     string
	VendorKept bool
}

// RemoveSkill removes an external skill or a whole group from the team
// file, or one member from its group, named "<member>" or, when several
// groups have a member of that name, "<group>/<member>". Removing a group's
// last member is a usage error: remove the group instead.
func (s *Service) RemoveSkill(name string) (RemoveSkillResult, error) {
	entry, found := s.home.Team.FindSkill(name)
	if !found {
		return s.removeMember(name)
	}
	team, _ := s.home.Team.WithoutSkill(entry.Name)
	if err := s.saveTeam(team); err != nil {
		return RemoveSkillResult{}, err
	}
	vendor := skill.VendorDir + pathSep + entry.Name
	kept := exists(filepath.Join(s.home.Root, filepath.FromSlash(vendor)))
	return RemoveSkillResult{Name: entry.Name, Vendor: vendor, VendorKept: kept}, nil
}

// removeMember takes one member out of its group. name is "<member>", or
// "<group>/<member>" when several groups have a member of that name; names
// cannot hold "/", so the two forms cannot be confused.
func (s *Service) removeMember(name string) (RemoveSkillResult, error) {
	var target config.Member
	if groupName, memberName, qualified := strings.Cut(name, groupMemberSep); qualified {
		found, err := s.qualifiedMember(groupName, memberName)
		if err != nil {
			return RemoveSkillResult{}, err
		}
		target = found
	} else {
		members := s.home.Team.FindMembers(name)
		switch len(members) {
		case 0:
			return RemoveSkillResult{}, unknownSkill(name, s.home.Team)
		case 1:
			target = members[0]
		default:
			forms := make([]string, 0, len(members))
			for _, m := range members {
				forms = append(forms, m.Group.Name+groupMemberSep+config.MemberName(m.Subdir))
			}
			return RemoveSkillResult{}, apperr.New(apperr.Usage, "cannot remove skill %s: %d groups have a skill of that name; name one as <group>%s<skill>: %s; %s is unchanged",
				name, len(members), groupMemberSep, strings.Join(forms, listSep), brand.TeamFile)
		}
	}
	group := target.Group
	subdir := target.Subdir
	member := config.MemberName(subdir)
	if len(group.Subdirs) == 1 {
		return RemoveSkillResult{}, apperr.New(apperr.Usage, "cannot remove skill %s: it is the last member of group %s; remove the group with `%s skill remove %s`; %s is unchanged",
			member, group.Name, brand.Name, group.Name, brand.TeamFile)
	}
	rest := slices.DeleteFunc(slices.Clone(group.Subdirs), func(other string) bool { return other == subdir })
	team, err := s.home.Team.WithSkillSubdirs(group.Name, rest)
	if err != nil {
		return RemoveSkillResult{}, entryProblems("remove skill", err, nil)
	}
	if err := s.saveTeam(team); err != nil {
		return RemoveSkillResult{}, err
	}
	return RemoveSkillResult{Name: member, Group: group.Name}, nil
}

// qualifiedMember finds the member called memberName in the group called
// groupName; a missing group, a single skill or a missing member is a usage
// error.
func (s *Service) qualifiedMember(groupName string, memberName string) (config.Member, error) {
	group, found := s.home.Team.FindSkill(groupName)
	switch {
	case !found:
		return config.Member{}, apperr.New(apperr.Usage, "cannot remove skill %s%s%s: no group %q in %s; %s is unchanged",
			groupName, groupMemberSep, memberName, groupName, brand.TeamFile, brand.TeamFile)
	case !group.IsGroup():
		return config.Member{}, apperr.New(apperr.Usage, "cannot remove skill %s%s%s: %q is a single skill, not a group; %s is unchanged",
			groupName, groupMemberSep, memberName, group.Name, brand.TeamFile)
	}
	for _, subdir := range group.Subdirs {
		if strings.EqualFold(config.MemberName(subdir), memberName) {
			return config.Member{Group: group, Subdir: subdir}, nil
		}
	}
	return config.Member{}, apperr.New(apperr.Usage, "cannot remove skill %s%s%s: group %s has no skill %q; its skills: %s; %s is unchanged",
		groupName, groupMemberSep, memberName, group.Name, memberName, LabelList(memberNames(group.Subdirs)), brand.TeamFile)
}

func unknownSkill(name string, team config.Team) error {
	return apperr.New(apperr.Usage, "unknown skill %q in %s; external skills and groups: %s", name, brand.TeamFile, LabelList(team.SkillNames()))
}

// UpdateSkillResult is a pin move. Unchanged is set when the skill was
// already at To and nothing was written. DetectedProtocol is set when the
// protocol was detected and saved, even when updating then failed.
type UpdateSkillResult struct {
	Name             string
	From             string
	To               string
	Unchanged        bool
	DetectedProtocol string
}

// UpdateSkill moves the pin of a skill or a whole group to commit, or to
// the remote's HEAD when commit is nil. A group member has no pin of its
// own, so naming one is a usage error.
func (s *Service) UpdateSkill(ctx context.Context, name string, commit *string) (UpdateSkillResult, error) {
	var result UpdateSkillResult
	entry, found := s.home.Team.FindSkill(name)
	if !found {
		if group, _, member := s.home.Team.FindMember(name); member {
			return result, apperr.New(apperr.Usage, "cannot update skill %s: it is pinned with its group %s; run `%s skill update %s`; %s is unchanged",
				name, group.Name, brand.Name, group.Name, brand.TeamFile)
		}
		return result, unknownSkill(name, s.home.Team)
	}
	result.Name = entry.Name
	result.From = entry.Commit
	// Checked before the remote is contacted and before anything is written.
	// A group's name is not a skill's, so a team skill may share it.
	if !entry.IsGroup() {
		if err := s.sourceConflict("update skill", entry.Name); err != nil {
			return result, err
		}
	}
	if commit != nil {
		result.To = *commit
	} else {
		head, detected, err := s.remoteHeadCommit(ctx, entry.URL)
		result.DetectedProtocol = detected
		if err != nil {
			return result, err
		}
		result.To = head
	}
	if result.To == entry.Commit {
		result.Unchanged = true
		return result, nil
	}
	team, err := s.home.Team.WithSkillCommit(entry.Name, result.To)
	if err != nil {
		return result, entryProblems("update skill", err, skillFlags)
	}
	if err := s.saveTeam(team); err != nil {
		return result, err
	}
	return result, nil
}

// EnableSkills adds names to the chosen skills and returns the ones that
// were not chosen yet. A name no skill has but a group does stands for every
// valid member of that group. Built-in and invalid skills are refused, and
// then nothing is written.
func (s *Service) EnableSkills(names []string) ([]string, error) {
	cat, err := skill.Discover(s.home)
	if err != nil {
		return nil, err
	}
	chosen := slices.Clone(s.home.Personal.Skills)
	var refused []string
	var added []string
	choose := func(name string) {
		if !containsFold(chosen, name) {
			chosen = append(chosen, name)
			added = append(added, name)
		}
	}
	for _, name := range names {
		item, found := cat.Find(name)
		members := cat.Members(name)
		switch {
		case !found && len(members) > 0:
			valid := 0
			for _, member := range members {
				if member.Invalid == "" {
					valid++
					choose(member.Name)
				}
			}
			if valid == 0 {
				refused = append(refused, fmt.Sprintf("%s (group with no valid skills)", name))
			}
		case !found:
			refused = append(refused, fmt.Sprintf("%s (no such skill)", name))
		case item.Source == skill.Builtin:
			refused = append(refused, fmt.Sprintf("%s (built in; installed whenever an agent is chosen)", name))
		case item.Invalid != "":
			refused = append(refused, fmt.Sprintf("%s (%s: %s)", name, apperr.SkillInvalid, item.Invalid))
		default:
			choose(item.Name)
		}
	}
	if len(refused) > 0 {
		return nil, apperr.New(apperr.Usage, "cannot enable %s; available: %s; %s is unchanged",
			strings.Join(refused, listSep), LabelList(cat.Choosable()), brand.LocalFile)
	}
	if err := s.savePersonal(s.home.Personal.WithSkills(chosen)); err != nil {
		return nil, err
	}
	return added, nil
}

// DisableSkills removes names from the chosen skills and returns them as
// they were written there. A name that is not chosen but names a group
// stands for that group's chosen members; a group with none chosen is a
// usage error. It also removes stale choices, names no skill has any more,
// which is why the catalog is read only for names that are not chosen.
func (s *Service) DisableSkills(names []string) ([]string, error) {
	var expanded []string
	for _, name := range names {
		if containsFold(s.home.Personal.Skills, name) {
			expanded = append(expanded, name)
			continue
		}
		// Only a name that is not chosen needs the catalog, so a stale
		// choice can be removed even when the skills cannot be listed.
		cat, err := skill.Discover(s.home)
		if err != nil {
			return nil, err
		}
		members := cat.Members(name)
		if len(members) == 0 {
			expanded = append(expanded, name)
			continue
		}
		var on []string
		for _, member := range members {
			if containsFold(s.home.Personal.Skills, member.Name) {
				on = append(on, member.Name)
			}
		}
		if len(on) == 0 {
			return nil, apperr.New(apperr.Usage, "not enabled: no skill of group %s; enabled: %s; %s is unchanged",
				name, LabelList(s.home.Personal.Skills), brand.LocalFile)
		}
		expanded = append(expanded, on...)
	}
	chosen, removed, err := withoutChoices(s.home.Personal.Skills, expanded)
	if err != nil {
		return nil, err
	}
	if err := s.savePersonal(s.home.Personal.WithSkills(chosen)); err != nil {
		return nil, err
	}
	return removed, nil
}

// withoutChoices removes names, compared case-insensitively, from chosen.
// A name that is not there is a usage error.
func withoutChoices(current []string, names []string) ([]string, []string, error) {
	chosen := slices.Clone(current)
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
		return nil, nil, apperr.New(apperr.Usage, "not enabled: %s; enabled: %s; %s is unchanged",
			strings.Join(missing, listSep), LabelList(current), brand.LocalFile)
	}
	return chosen, removed, nil
}

func containsFold(list []string, name string) bool {
	return slices.ContainsFunc(list, func(item string) bool { return strings.EqualFold(item, name) })
}

// ListSkills inspects every skill; it never connects to a remote or writes
// a file.
func (s *Service) ListSkills(ctx context.Context) ([]skill.Row, error) {
	git, cat, err := s.skillTools(ctx)
	if err != nil {
		return nil, err
	}
	return skill.Inspect(ctx, git, s.home, cat)
}

// SyncSkills settles the protocol when a clone needs it, then installs and
// links the chosen skills.
func (s *Service) SyncSkills(ctx context.Context, out Output) (BatchResult, error) {
	result := newBatch(readyLabel, warnedLabel)
	git, cat, err := s.skillTools(ctx)
	if err != nil {
		return result, err
	}
	urls, err := skill.CloneURLs(ctx, git, s.home, cat)
	if err != nil {
		return result, err
	}
	if len(urls) > 0 {
		if _, err := s.ensureProtocolTo(ctx, git, urls, out, &result); err != nil {
			return result, err
		}
	}
	synced, err := skill.Sync(ctx, git, s.home, cat, out.Out, out.Err)
	if err != nil {
		return result, err
	}
	result.Done = synced.Ready
	result.Skipped = synced.Warned
	result.Failed = synced.Failed
	return result.finish(out)
}

// skillTools finds git and the skills the home repo offers.
func (s *Service) skillTools(ctx context.Context) (*gitx.Git, skill.Catalog, error) {
	git, err := gitx.New(ctx)
	if err != nil {
		return nil, skill.Catalog{}, err
	}
	cat, err := skill.Discover(s.home)
	if err != nil {
		return nil, skill.Catalog{}, err
	}
	return git, cat, nil
}

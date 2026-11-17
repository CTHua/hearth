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
)

// skillFlags are the skill add flags that set a field of the same name.
var skillFlags = []string{config.FieldName, config.FieldCommit, config.FieldSubdir}

// AddSkillParams describes a new [[skills]] entry. A nil field was not
// given: Subdir defaults to the repo's top, Name to the last Subdir segment
// or the URL's last segment, and Commit is asked from the remote's HEAD.
type AddSkillParams struct {
	URL    string
	Name   *string
	Commit *string
	Subdir *string
}

// AddSkillResult is the entry that was added. DetectedProtocol is set when
// the protocol was detected and saved, even when adding then failed.
type AddSkillResult struct {
	Skill            config.Skill
	DetectedProtocol string
}

// AddSkill pins an external skill in the team file. Everything known
// locally is checked before the remote is contacted.
func (s *Service) AddSkill(ctx context.Context, p AddSkillParams) (AddSkillResult, error) {
	var result AddSkillResult
	entry, err := newSkill(p)
	if err != nil {
		return result, err
	}
	var pending []string
	if p.Commit == nil {
		pending = append(pending, config.FieldCommit)
	}
	if err := s.home.Team.CheckNewSkill(entry, pending...); err != nil {
		return result, entryProblems("add skill", err, skillFlags)
	}
	if err := s.sourceConflict("add skill", entry.Name); err != nil {
		return result, err
	}
	if p.Commit == nil {
		commit, detected, err := s.remoteHeadCommit(ctx, entry.URL)
		result.DetectedProtocol = detected
		if err != nil {
			return result, err
		}
		entry.Commit = commit
	}
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

// newSkill fills the defaults: name from the last subdir segment, or from
// the URL when the skill is at the top of its repo.
func newSkill(p AddSkillParams) (config.Skill, error) {
	u, err := giturl.Parse(p.URL)
	if err != nil {
		return config.Skill{}, apperr.New(apperr.Usage, "url %q: %v", p.URL, err)
	}
	subdir := config.RootSubdir
	if p.Subdir != nil {
		subdir = *p.Subdir
	}
	var name string
	switch {
	case p.Name != nil:
		name = *p.Name
	case subdir == config.RootSubdir:
		name = u.DefaultName()
	default:
		segments := strings.Split(subdir, pathSep)
		name = segments[len(segments)-1]
	}
	commit := ""
	if p.Commit != nil {
		commit = *p.Commit
	}
	return config.Skill{Name: name, URL: p.URL, Commit: commit, Subdir: subdir}, nil
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
	for _, item := range cat.Items() {
		if item.Source == skill.External || !strings.EqualFold(item.Name, name) {
			continue
		}
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
// never deleted.
type RemoveSkillResult struct {
	Name       string
	Vendor     string
	VendorKept bool
}

// RemoveSkill removes an external skill from the team file.
func (s *Service) RemoveSkill(name string) (RemoveSkillResult, error) {
	entry, found := s.home.Team.FindSkill(name)
	if !found {
		return RemoveSkillResult{}, unknownSkill(name, s.home.Team)
	}
	team, _ := s.home.Team.WithoutSkill(entry.Name)
	if err := s.saveTeam(team); err != nil {
		return RemoveSkillResult{}, err
	}
	vendor := skill.VendorDir + pathSep + entry.Name
	kept := exists(filepath.Join(s.home.Root, filepath.FromSlash(vendor)))
	return RemoveSkillResult{Name: entry.Name, Vendor: vendor, VendorKept: kept}, nil
}

func unknownSkill(name string, team config.Team) error {
	return apperr.New(apperr.Usage, "unknown skill %q in %s; external skills: %s", name, brand.TeamFile, LabelList(team.SkillNames()))
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

// UpdateSkill moves a skill's pin to commit, or to the remote's HEAD when
// commit is nil.
func (s *Service) UpdateSkill(ctx context.Context, name string, commit *string) (UpdateSkillResult, error) {
	var result UpdateSkillResult
	entry, found := s.home.Team.FindSkill(name)
	if !found {
		return result, unknownSkill(name, s.home.Team)
	}
	result.Name = entry.Name
	result.From = entry.Commit
	// Checked before the remote is contacted and before anything is written.
	if err := s.sourceConflict("update skill", entry.Name); err != nil {
		return result, err
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
// were not chosen yet. Built-in and invalid skills are refused, and then
// nothing is written.
func (s *Service) EnableSkills(names []string) ([]string, error) {
	cat, err := skill.Discover(s.home)
	if err != nil {
		return nil, err
	}
	chosen := slices.Clone(s.home.Personal.Skills)
	var refused []string
	var added []string
	for _, name := range names {
		item, found := cat.Find(name)
		switch {
		case !found:
			refused = append(refused, fmt.Sprintf("%s (no such skill)", name))
		case item.Source == skill.Builtin:
			refused = append(refused, fmt.Sprintf("%s (built in; installed whenever an agent is chosen)", name))
		case item.Invalid != "":
			refused = append(refused, fmt.Sprintf("%s (%s: %s)", name, apperr.SkillInvalid, item.Invalid))
		case !containsFold(chosen, item.Name):
			chosen = append(chosen, item.Name)
			added = append(added, item.Name)
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
// they were written there. It also removes stale choices, names no skill has
// any more, which is why it does not look at the catalog.
func (s *Service) DisableSkills(names []string) ([]string, error) {
	chosen, removed, err := withoutChoices(s.home.Personal.Skills, names)
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

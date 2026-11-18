package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/subagent"
)

// EnableSubagents adds names to the chosen subagents and returns the ones
// that were not chosen yet. A name with no source file or an invalid one is
// refused, and then nothing is written.
func (s *Service) EnableSubagents(names []string) ([]string, error) {
	cat, err := subagent.Discover(s.home.Root)
	if err != nil {
		return nil, err
	}
	chosen := slices.Clone(s.home.Personal.Subagents)
	var refused []string
	var added []string
	for _, name := range names {
		item, found := cat.Find(name)
		switch {
		case !found:
			refused = append(refused, fmt.Sprintf("%s (no such subagent)", name))
		case item.Invalid != "":
			refused = append(refused, fmt.Sprintf("%s (%s: %s)", name, subagent.InvalidCategory, item.Invalid))
		case !containsFold(chosen, item.Name):
			chosen = append(chosen, item.Name)
			added = append(added, item.Name)
		}
	}
	if len(refused) > 0 {
		return nil, apperr.New(apperr.Usage, "cannot enable %s; available: %s; %s is unchanged",
			strings.Join(refused, listSep), LabelList(cat.Choosable()), brand.LocalFile)
	}
	if err := s.savePersonal(s.home.Personal.WithSubagents(chosen)); err != nil {
		return nil, err
	}
	return added, nil
}

// DisableSubagents removes names from the chosen subagents and returns them
// as they were written there. It also removes stale choices, names no
// source file has any more, which is why it does not look at the sources.
func (s *Service) DisableSubagents(names []string) ([]string, error) {
	chosen, removed, err := withoutChoices(s.home.Personal.Subagents, names)
	if err != nil {
		return nil, err
	}
	if err := s.savePersonal(s.home.Personal.WithSubagents(chosen)); err != nil {
		return nil, err
	}
	return removed, nil
}

// ListSubagents reports every subagent source and stale choice; it only
// reads.
func (s *Service) ListSubagents() ([]subagent.Row, error) {
	return subagent.Inspect(s.home)
}

// SyncSubagents writes or removes the generated subagent files of the
// chosen agents. The error carries exit code 1 when a file failed.
func (s *Service) SyncSubagents(force bool, out Output) (subagent.Result, error) {
	result, err := subagent.Sync(s.home, force, out.Out, out.Err)
	if err != nil {
		return result, err
	}
	return result, failedExit(len(result.Failed))
}

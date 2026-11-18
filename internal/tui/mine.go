package tui

import (
	"context"
	"fmt"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/skill"
)

// section is a group of rows on Mine, in screen order.
type section int

const (
	sectionProtocol section = iota
	sectionAgents
	sectionSkills
	sectionServers
	sectionSubagents
)

// mineRow says which group a Mine row is in and its index there.
func (m *Model) mineRow(row int) (section, int) {
	skillsStart := optionCount + agentRows
	serversStart := skillsStart + len(m.snap.skills)
	subagentsStart := serversStart + len(m.snap.servers)
	switch {
	case row < optionCount:
		return sectionProtocol, row
	case row < skillsStart:
		return sectionAgents, row - optionCount
	case row < serversStart:
		return sectionSkills, row - skillsStart
	case row < subagentsStart:
		return sectionServers, row - serversStart
	}
	return sectionSubagents, row - subagentsStart
}

// mineKey handles Mine: every change here goes to the personal file, and
// s syncs the generated files.
func (m *Model) mineKey(msg tea.KeyPressMsg) tea.Cmd {
	if key.Matches(msg, keys.Sync) {
		return m.start(jobSync, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.Sync(ctx, false, out)
			return nil, false, err
		})
	}
	group, index := m.mineRow(m.cursor[tabMine])
	if group == sectionProtocol {
		if !key.Matches(msg, keys.Choose) {
			return nil
		}
		return m.chooseProtocol(index)
	}
	if !key.Matches(msg, keys.Toggle) {
		return nil
	}
	switch group {
	case sectionAgents:
		if m.busy() {
			return nil
		}
		agents := toggled(m.snap.home.Personal.Agents, config.Agents()[index])
		err := m.actions.SetAgents(agents)
		m.finishNow(onSuccess([]string{fmt.Sprintf("agents: %s (saved to %s)", core.LabelList(agents), brand.LocalFile)}, err), false, err)
	case sectionSkills:
		item := m.snap.skills[index]
		if item.Source == skill.Builtin {
			m.messages = []string{msgBuiltin}
			return nil
		}
		if m.busy() {
			return nil
		}
		m.toggleChoice(item.Name, item.Enabled, m.actions.EnableSkills, m.actions.DisableSkills)
	case sectionSubagents:
		row := m.snap.subagents[index]
		// Like the built-in skill, an invalid one that is not chosen cannot
		// be turned on; core would only refuse it.
		if row.Invalid != "" && !row.Enabled {
			m.messages = []string{fmt.Sprintf(msgSubagentInvalid, row.Invalid)}
			return nil
		}
		if m.busy() {
			return nil
		}
		m.toggleChoice(row.Name, row.Enabled, m.actions.EnableSubagents, m.actions.DisableSubagents)
	default:
		if m.busy() {
			return nil
		}
		server := m.snap.servers[index]
		// A stale choice counts as enabled: toggling it removes it.
		m.toggleChoice(server.Name, server.Enabled || server.Stale, m.actions.EnableMCP, m.actions.DisableMCP)
	}
	return nil
}

func (m *Model) chooseProtocol(option int) tea.Cmd {
	if option == optionDetect {
		if len(m.snap.home.Team.Repos) == 0 {
			m.messages = []string{core.ErrNoRepos.Message}
			return nil
		}
		return m.start(jobDetect, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
			protocol, err := m.actions.DetectProtocol(ctx)
			return detectedLines(protocol), false, err
		})
	}
	if m.busy() {
		return nil
	}
	protocol := giturl.SSH
	if option == optionHTTPS {
		protocol = giturl.HTTPS
	}
	err := m.actions.SetProtocol(protocol)
	m.finishNow(onSuccess([]string{fmt.Sprintf("protocol: %s (saved to %s)", protocol, brand.LocalFile)}, err), false, err)
	return nil
}

// toggleChoice enables or disables one personal choice.
func (m *Model) toggleChoice(name string, enabled bool, enable func([]string) ([]string, error), disable func([]string) ([]string, error)) {
	if enabled {
		removed, err := disable([]string{name})
		m.finishNow(onSuccess([]string{fmt.Sprintf("disabled: %s (saved to %s)", core.LabelList(removed), brand.LocalFile)}, err), false, err)
		return
	}
	added, err := enable([]string{name})
	m.finishNow(onSuccess([]string{fmt.Sprintf("enabled: %s (saved to %s)", core.LabelList(added), brand.LocalFile)}, err), false, err)
}

// toggled returns the known agents in their usual order, with agent flipped.
func toggled(current []string, agent string) []string {
	var next []string
	for _, known := range config.Agents() {
		on := slices.Contains(current, known)
		if known == agent {
			on = !on
		}
		if on {
			next = append(next, known)
		}
	}
	return next
}

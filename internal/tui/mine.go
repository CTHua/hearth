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
	sectionRepos
	sectionSkills
	sectionServers
	sectionSubagents
)

// skillLine is one row of Mine's skill section: the title row of a group,
// or one skill, which may be a member of the group above it.
type skillLine struct {
	// group is set on a title row.
	group string
	// members are the indexes in snap.skills of a group's skills.
	members []int
	// skill is the index in snap.skills of a skill row; -1 on a title row.
	skill int
}

// skillLines puts a title row before the members of each group; the rows
// of one group are next to each other in catalog order.
func (m *Model) skillLines() []skillLine {
	var lines []skillLine
	// title is the index in lines of the current group's title row.
	title := -1
	for i, row := range m.snap.skills {
		switch {
		case row.Group == "":
			title = -1
		case title < 0 || lines[title].group != row.Group:
			lines = append(lines, skillLine{group: row.Group, skill: -1})
			title = len(lines) - 1
		}
		if title >= 0 {
			lines[title].members = append(lines[title].members, i)
		}
		lines = append(lines, skillLine{skill: i})
	}
	return lines
}

// groupCounts are how many members of a title row's group are enabled, and
// how many it has.
func (m *Model) groupCounts(line skillLine) (int, int) {
	on := 0
	for _, i := range line.members {
		if m.snap.skills[i].Enabled {
			on++
		}
	}
	return on, len(line.members)
}

// toggleGroup turns on every member of the group that is off and can be
// chosen; when none is left to turn on, it turns every enabled member off.
func (m *Model) toggleGroup(line skillLine) {
	var off []string
	var on []string
	for _, i := range line.members {
		row := m.snap.skills[i]
		switch {
		case row.Enabled:
			on = append(on, row.Name)
		case row.Invalid == "":
			off = append(off, row.Name)
		}
	}
	if len(off) > 0 {
		added, err := m.actions.EnableSkills(off)
		m.finishChoice(onSuccess([]string{fmt.Sprintf("enabled: %s (saved to %s)", core.LabelList(added), brand.LocalFile)}, err), err)
		return
	}
	if len(on) == 0 {
		m.messages = []string{fmt.Sprintf(msgGroupInvalid, line.group)}
		return
	}
	removed, err := m.actions.DisableSkills(on)
	m.finishChoice(onSuccess([]string{fmt.Sprintf("disabled: %s (saved to %s)", core.LabelList(removed), brand.LocalFile)}, err), err)
}

// mineRow says which group a Mine row is in and its index there; a skill
// row's index is in skillLines.
func (m *Model) mineRow(row int) (section, int) {
	reposStart := optionCount + agentRows
	skillsStart := reposStart + len(m.snap.repos)
	serversStart := skillsStart + len(m.skillLines())
	subagentsStart := serversStart + len(m.snap.servers)
	switch {
	case row < optionCount:
		return sectionProtocol, row
	case row < reposStart:
		return sectionAgents, row - optionCount
	case row < skillsStart:
		return sectionRepos, row - reposStart
	case row < serversStart:
		return sectionSkills, row - skillsStart
	case row < subagentsStart:
		return sectionServers, row - serversStart
	}
	return sectionSubagents, row - subagentsStart
}

// mineKey handles Mine: every change here goes to the personal file, and
// s clones the chosen repos and syncs the generated files.
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
		m.finishChoice(onSuccess([]string{fmt.Sprintf("agents: %s (saved to %s)", core.LabelList(agents), brand.LocalFile)}, err), err)
	case sectionRepos:
		if m.busy() {
			return nil
		}
		row := m.snap.repos[index]
		m.toggleChoice(row.Repo.Name, row.Chosen, m.actions.EnableRepos, m.actions.DisableRepos)
	case sectionSkills:
		line := m.skillLines()[index]
		if line.skill < 0 {
			if m.busy() {
				return nil
			}
			m.toggleGroup(line)
			return nil
		}
		item := m.snap.skills[line.skill]
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
		m.finishChoice(onSuccess([]string{fmt.Sprintf("disabled: %s (saved to %s)", core.LabelList(removed), brand.LocalFile)}, err), err)
		return
	}
	added, err := enable([]string{name})
	m.finishChoice(onSuccess([]string{fmt.Sprintf("enabled: %s (saved to %s)", core.LabelList(added), brand.LocalFile)}, err), err)
}

// finishChoice is finishNow for a change of what to sync: agents, repos,
// skills, MCP servers or subagents. Once one is saved, quitting with q syncs
// until a sync succeeds.
func (m *Model) finishChoice(lines []string, err error) {
	if err == nil {
		m.unsynced = true
	}
	m.finishNow(lines, false, err)
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

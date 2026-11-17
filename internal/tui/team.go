package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
)

// The MCP form follows `mcp add`: the command field splits at spaces into
// the command and its args, and env vars are separated by commas.
const (
	envSep         = ","
	msgMCPBoth     = "fill command for a stdio server or URL for an http one, not both"
	msgMCPNeither  = "fill command for a stdio server or URL for an http one"
	msgMCPEnvOnURL = "env vars are for stdio servers; give an http server's token with `%s mcp add --url`"
)

func (m *Model) reposKey(msg tea.KeyPressMsg, confirm string) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Add):
		return m.openForm(repoForm(), m.submitRepo)
	case key.Matches(msg, keys.Clone):
		return m.start(jobClone, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.Clone(ctx, nil, out)
			return nil, false, err
		})
	}
	fix := key.Matches(msg, keys.Fix)
	if !fix && !key.Matches(msg, keys.Remove) {
		return nil
	}
	names := make([]string, 0, len(m.snap.repos))
	for _, row := range m.snap.repos {
		names = append(names, row.Repo.Name)
	}
	name, ok := m.selectedName(names)
	if !ok {
		return nil
	}
	if fix {
		return m.start(jobFixRemote, func(ctx context.Context, out core.Output) ([]string, bool, error) {
			_, err := m.actions.FixRemote(ctx, []string{name}, out)
			return nil, false, err
		})
	}
	if !m.confirmRemove(name, confirm) {
		return nil
	}
	result, err := m.actions.RemoveRepo(name)
	lines := []string{fmt.Sprintf("removed %s from %s", name, brand.TeamFile)}
	if result.CheckoutKept {
		lines = append(lines, fmt.Sprintf("the checkout at %s was not touched", result.Repo.Path))
	}
	m.finishNow(onSuccess(lines, err), true, err)
	return nil
}

func (m *Model) submitRepo(values []string) (tea.Cmd, error) {
	url := strings.TrimSpace(values[0])
	if url == "" {
		return nil, apperr.New(apperr.Usage, msgURLRequired)
	}
	params := core.AddRepoParams{URL: url, Name: optional(values[1]), Path: optional(values[2]), Branch: optional(values[3])}
	add := func(ctx context.Context) ([]string, error) {
		result, err := m.actions.AddRepo(ctx, params)
		lines := detectedLines(result.DetectedProtocol)
		if err == nil {
			lines = append(lines, fmt.Sprintf("added %s -> %s (%s)", result.Repo.Name, result.Repo.Path, result.Repo.Branch))
		}
		return lines, err
	}
	// Without a branch the remote is asked for one, which may take a while.
	if params.Branch == nil {
		return m.start(jobRepoAdd, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
			lines, err := add(ctx)
			return lines, true, err
		}), nil
	}
	lines, err := add(m.ctx)
	m.finishNow(lines, true, err)
	return nil, nil
}

// skillsKey handles the team Skills tab: the external skills of the team
// file. Turning a skill on or off for oneself is on Mine.
func (m *Model) skillsKey(msg tea.KeyPressMsg, confirm string) tea.Cmd {
	if key.Matches(msg, keys.Add) {
		return m.openForm(skillForm(), m.submitSkill)
	}
	update := key.Matches(msg, keys.Update)
	if !update && !key.Matches(msg, keys.Remove) {
		return nil
	}
	name, ok := m.selectedName(m.snap.home.Team.SkillNames())
	if !ok {
		return nil
	}
	if update {
		return m.openForm(updateForm(fmt.Sprintf(formTitleUpdate, name)), m.submitUpdate(name))
	}
	if !m.confirmRemove(name, confirm) {
		return nil
	}
	result, err := m.actions.RemoveSkill(name)
	lines := []string{fmt.Sprintf("removed skill %s from %s; the next sync removes its links", name, brand.TeamFile)}
	if result.VendorKept {
		lines = append(lines, fmt.Sprintf("%s was not touched; delete it yourself if you no longer need it", result.Vendor))
	}
	m.finishNow(onSuccess(lines, err), true, err)
	return nil
}

func (m *Model) submitSkill(values []string) (tea.Cmd, error) {
	url := strings.TrimSpace(values[0])
	if url == "" {
		return nil, apperr.New(apperr.Usage, msgURLRequired)
	}
	params := core.AddSkillParams{URL: url, Name: optional(values[1]), Subdir: optional(values[2]), Commit: optional(values[3])}
	return m.start(jobSkillAdd, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
		result, err := m.actions.AddSkill(ctx, params)
		lines := detectedLines(result.DetectedProtocol)
		if err == nil {
			entry := result.Skill
			lines = append(lines, fmt.Sprintf("added skill %s at %s (subdir %s)", entry.Name, entry.Commit, entry.Subdir))
		}
		return lines, true, err
	}), nil
}

// submitUpdate moves name's pin. A blank commit asks the remote for its
// HEAD, so it runs in the background like skill add.
func (m *Model) submitUpdate(name string) func([]string) (tea.Cmd, error) {
	return func(values []string) (tea.Cmd, error) {
		commit := optional(values[0])
		update := func(ctx context.Context) ([]string, bool, error) {
			result, err := m.actions.UpdateSkill(ctx, name, commit)
			lines := detectedLines(result.DetectedProtocol)
			switch {
			case err != nil:
				return lines, false, err
			case result.Unchanged:
				return append(lines, fmt.Sprintf("skill %s is already pinned at %s; %s is unchanged", result.Name, result.To, brand.TeamFile)), false, nil
			}
			return append(lines, fmt.Sprintf("updated skill %s: %s -> %s; the next sync moves its checkout", result.Name, result.From, result.To)), true, nil
		}
		if commit == nil {
			return m.start(jobSkillUpdate, func(ctx context.Context, _ core.Output) ([]string, bool, error) {
				return update(ctx)
			}), nil
		}
		lines, changed, err := update(m.ctx)
		m.finishNow(lines, changed, err)
		return nil, nil
	}
}

// mcpKey handles the team MCP tab: adding and removing servers. Turning a
// server on or off for oneself is on Mine.
func (m *Model) mcpKey(msg tea.KeyPressMsg, confirm string) tea.Cmd {
	if key.Matches(msg, keys.Add) {
		return m.openForm(mcpForm(), m.submitMCP)
	}
	if !key.Matches(msg, keys.Remove) {
		return nil
	}
	name, ok := m.selectedName(m.snap.home.Team.MCPNames())
	if !ok || !m.confirmRemove(name, confirm) {
		return nil
	}
	_, err := m.actions.RemoveMCP(name)
	lines := []string{fmt.Sprintf("removed MCP server %s from %s; the next sync removes it from the agents' files", name, brand.TeamFile)}
	m.finishNow(onSuccess(lines, err), true, err)
	return nil
}

// submitMCP adds the server; a refused one keeps the form open with the
// reason, and the team file is not written.
func (m *Model) submitMCP(values []string) (tea.Cmd, error) {
	server, err := newServer(values)
	if err != nil {
		return nil, err
	}
	if err := m.actions.AddMCP(server); err != nil {
		return nil, err
	}
	m.finishNow([]string{fmt.Sprintf("added MCP server %s (%s); turn it on in %s", server.Name, server.Transport, tabNames[tabMine])}, true, nil)
	return nil, nil
}

// newServer turns the MCP form into an entry. Quoting is not supported: an
// argument with a space needs the CLI. Names are left for core to check.
func newServer(values []string) (config.MCP, error) {
	command := strings.Fields(values[1])
	url := strings.TrimSpace(values[2])
	var envVars []string
	for _, name := range strings.Split(values[3], envSep) {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			envVars = append(envVars, trimmed)
		}
	}
	switch {
	case len(command) > 0 && url != "":
		return config.MCP{}, apperr.New(apperr.Usage, msgMCPBoth)
	case len(command) == 0 && url == "":
		return config.MCP{}, apperr.New(apperr.Usage, msgMCPNeither)
	case url != "" && len(envVars) > 0:
		return config.MCP{}, apperr.New(apperr.Usage, msgMCPEnvOnURL, brand.Name)
	}
	server := config.MCP{Name: strings.TrimSpace(values[0])}
	if url != "" {
		server.Transport = config.TransportHTTP
		server.URL = url
		return server, nil
	}
	server.Transport = config.TransportStdio
	server.Command = command[0]
	if len(command) > 1 {
		server.Args = command[1:]
	}
	server.EnvVars = envVars
	return server, nil
}

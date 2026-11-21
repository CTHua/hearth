package core

import (
	"context"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/subagent"
)

// AddMCP adds a server to the team file. Mixing the transports' flags is
// the CLI's to check; this checks the entry itself.
func (s *Service) AddMCP(server config.MCP) error {
	team, err := s.home.Team.WithMCP(server)
	if err != nil {
		return entryProblems("add MCP server", err, nil)
	}
	return s.saveTeam(team)
}

// RemoveMCP removes a server from the team file and returns it.
func (s *Service) RemoveMCP(name string) (config.MCP, error) {
	server, found := s.home.Team.FindMCP(name)
	if !found {
		return config.MCP{}, apperr.New(apperr.Usage, "unknown MCP server %q in %s; servers: %s", name, brand.TeamFile, LabelList(s.home.Team.MCPNames()))
	}
	team, _ := s.home.Team.WithoutMCP(server.Name)
	if err := s.saveTeam(team); err != nil {
		return config.MCP{}, err
	}
	return server, nil
}

// EnableMCP adds names to the chosen servers and returns the ones that were
// not chosen yet. An unknown name is refused, and then nothing is written.
func (s *Service) EnableMCP(names []string) ([]string, error) {
	chosen := slices.Clone(s.home.Personal.MCP)
	var refused []string
	var added []string
	for _, name := range names {
		server, found := s.home.Team.FindMCP(name)
		switch {
		case !found:
			refused = append(refused, name)
		case !containsFold(chosen, server.Name):
			chosen = append(chosen, server.Name)
			added = append(added, server.Name)
		}
	}
	if len(refused) > 0 {
		return nil, apperr.New(apperr.Usage, "cannot enable %s (no such MCP server); available: %s; %s is unchanged",
			strings.Join(refused, listSep), LabelList(s.home.Team.MCPNames()), brand.LocalFile)
	}
	if err := s.savePersonal(s.home.Personal.WithMCPChoices(chosen)); err != nil {
		return nil, err
	}
	return added, nil
}

// DisableMCP removes names from the chosen servers and returns them as they
// were written there. It also removes stale choices, names no server has
// any more, which is why it does not look at the team file.
func (s *Service) DisableMCP(names []string) ([]string, error) {
	chosen, removed, err := withoutChoices(s.home.Personal.MCP, names)
	if err != nil {
		return nil, err
	}
	if err := s.savePersonal(s.home.Personal.WithMCPChoices(chosen)); err != nil {
		return nil, err
	}
	return removed, nil
}

// ListMCP reports every server; it starts no process, connects to nothing
// and writes no file.
func (s *Service) ListMCP() []mcp.Row {
	return mcp.Inspect(s.home)
}

// SyncMCP writes or removes the generated MCP files of the chosen agents.
// The error carries exit code 1 when a file failed.
func (s *Service) SyncMCP(force bool, out Output) (mcp.Result, error) {
	result, err := mcp.Sync(s.home, force, out.Out, out.Err)
	if err != nil {
		return result, err
	}
	return result, failedExit(len(result.Failed))
}

// SyncResult is what the top-level sync did in each part. Clone is Empty
// when there was nothing to clone.
type SyncResult struct {
	Clone     BatchResult
	Skills    BatchResult
	MCP       mcp.Result
	Subagents subagent.Result
}

// Sync clones the chosen repos that have no checkout yet, then runs the
// skill sync, the MCP sync and the subagent sync whatever the earlier parts'
// results, and ends with the worst exit code. Each part's error is reported
// to out.Err as it ends, so the transcript reads in order. force is for MCP
// and subagents. An interrupt stops the run before the next part.
func (s *Service) Sync(ctx context.Context, force bool, out Output) (SyncResult, error) {
	var result SyncResult
	cloned, cloneErr := s.cloneMissing(ctx, out)
	result.Clone = cloned
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	cloneCode := apperr.Report(out.Err, cloneErr)
	skills, skillErr := s.SyncSkills(ctx, out)
	result.Skills = skills
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	skillCode := apperr.Report(out.Err, skillErr)
	servers, mcpErr := s.SyncMCP(force, out)
	result.MCP = servers
	mcpCode := apperr.Report(out.Err, mcpErr)
	subagents, subagentErr := s.SyncSubagents(force, out)
	result.Subagents = subagents
	subagentCode := apperr.Report(out.Err, subagentErr)
	code := max(cloneCode, skillCode, mcpCode, subagentCode)
	if code == apperr.ExitOK {
		return result, nil
	}
	return result, apperr.Exit(code)
}

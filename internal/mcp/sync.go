package mcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/genfile"
)

// rootDir is what filepath.Dir returns for a file at the top of the root.
const rootDir = "."

// target is one generated file and the agent that reads it.
type target struct {
	rel    string
	agent  string
	render func([]config.MCP) ([]byte, error)
}

var targets = []target{
	{rel: ClaudeFile, agent: config.AgentClaude, render: renderClaude},
	{rel: CodexFile, agent: config.AgentCodex, render: func(servers []config.MCP) ([]byte, error) { return renderCodex(servers), nil }},
	{rel: CopilotFile, agent: config.AgentCopilot, render: renderCopilot},
}

// Result is what a sync did: warnings that do not change the exit code,
// and failures as "<file> (<category>)".
type Result struct {
	Warned []string
	Failed []string
}

// tally prints problems as they are found and records them.
type tally struct {
	result Result
	errw   io.Writer
}

func (t *tally) fail(e *apperr.Error) {
	apperr.Print(t.errw, e)
	t.result.Failed = append(t.result.Failed, fmt.Sprintf("%s (%s)", e.Name, e.Category))
}

func (t *tally) warn(e *apperr.Error) {
	apperr.Warn(t.errw, e)
	t.result.Warned = append(t.result.Warned, fmt.Sprintf("%s (%s)", e.Name, e.Category))
}

// Sync writes the file of each chosen agent from the chosen servers and
// removes the files of agents no longer chosen. Each file is decided on
// its own: a file is written only when it is missing or still exactly what
// sync last wrote, and removed only when it is still what sync last wrote;
// otherwise it is left alone and reported as generated-modified. force
// overwrites and removes those anyway. A file sync never wrote, for an
// agent that is not chosen, is not touched. A symlink or a file of the
// wrong type on any path is refused even with force. Progress lines go to
// out, problems to errw. The error is set only for a state file that
// cannot be parsed.
func Sync(home *config.Home, force bool, out io.Writer, errw io.Writer) (Result, error) {
	t := &tally{errw: errw}
	sel := choose(home)
	for _, name := range sel.stale {
		t.warn(apperr.ForRepo(apperr.Unknown, name, "%s", StaleDetail(name)))
	}
	for _, row := range Inspect(home) {
		if !row.Enabled || row.Stale {
			continue
		}
		for _, warning := range row.Warnings() {
			t.warn(apperr.ForRepo(apperr.EnvMissing, row.Name, "%s", warning))
		}
	}
	if len(sel.servers) > 0 && len(home.Personal.Agents) == 0 {
		fmt.Fprintf(out, "mcp: servers are enabled but no agent is chosen; choose one with `%s config agents`\n", brand.Name)
	}
	if problem := genfile.FileProblem(home.Root, brand.StateFile); problem != "" {
		t.fail(apperr.ForRepo(apperr.PathConflict, brand.StateFile, "%s; no MCP file was written", problem))
		return t.result, nil
	}
	st, err := genfile.Load(home.Root)
	if err != nil {
		return Result{}, err
	}
	changed := false
	for _, tg := range targets {
		wanted := slices.Contains(home.Personal.Agents, tg.agent)
		line, fileChanged, e := syncFile(home.Root, tg, wanted, sel.servers, st, force)
		changed = changed || fileChanged
		if e != nil {
			t.fail(e)
			continue
		}
		if line != "" {
			fmt.Fprintln(out, line)
		}
	}
	if changed {
		if err := genfile.Save(home.Root, st); err != nil {
			t.fail(apperr.ForRepo(apperr.PathConflict, brand.StateFile, "cannot write: %v; the next sync reports the files it wrote as generated-modified", err))
		}
	}
	return t.result, nil
}

// syncFile brings one generated file in line and updates st. changed
// reports whether st was changed.
func syncFile(root string, tg target, wanted bool, servers []config.MCP, st genfile.State, force bool) (string, bool, *apperr.Error) {
	recorded, managed := st.Files[tg.rel]
	if !wanted && !managed {
		return "", false, nil
	}
	if problem := genfile.FileProblem(root, tg.rel); problem != "" {
		return "", false, apperr.ForRepo(apperr.PathConflict, tg.rel, "%s; left unchanged, even with --force", problem)
	}
	current, exists, err := genfile.ReadHash(root, tg.rel)
	if err != nil {
		return "", false, apperr.ForRepo(apperr.PathConflict, tg.rel, "cannot read: %v", err)
	}
	untouched := exists && managed && current == recorded
	path := filepath.Join(root, filepath.FromSlash(tg.rel))
	if !wanted {
		if !exists {
			delete(st.Files, tg.rel)
			return "", true, nil
		}
		if !untouched && !force {
			return "", false, modified(tg.rel, "not removed")
		}
		if err := os.Remove(path); err != nil {
			return "", false, apperr.ForRepo(apperr.PathConflict, tg.rel, "cannot remove: %v", err)
		}
		delete(st.Files, tg.rel)
		// .codex/ goes too once it is empty; the root itself never does.
		if filepath.Dir(tg.rel) != rootDir {
			if err := genfile.RemoveIfEmpty(filepath.Dir(path)); err != nil {
				return "", true, apperr.ForRepo(apperr.PathConflict, tg.rel, "removed, but cannot remove its empty directory: %v", err)
			}
		}
		return fmt.Sprintf("mcp %s: removed (%s is not chosen)", tg.rel, tg.agent), true, nil
	}
	if exists && !untouched && !force {
		return "", false, modified(tg.rel, "not overwritten")
	}
	content, err := tg.render(servers)
	if err != nil {
		return "", false, apperr.ForRepo(apperr.Env, tg.rel, "%v", err)
	}
	sum := genfile.Digest(content)
	if untouched && current == sum {
		return fmt.Sprintf("mcp %s: up to date", tg.rel), false, nil
	}
	if err := genfile.WriteFile(path, content); err != nil {
		return "", false, apperr.ForRepo(apperr.PathConflict, tg.rel, "cannot write: %v", err)
	}
	st.Files[tg.rel] = sum
	return fmt.Sprintf("mcp %s: wrote %d server(s)", tg.rel, len(servers)), true, nil
}

func modified(rel string, action string) *apperr.Error {
	return apperr.ForRepo(apperr.GeneratedModified, rel,
		"changed since `%s sync` last wrote it, or not written by it; %s. Move your changes into %s, then run `%s sync --force`",
		brand.Name, action, brand.TeamFile, brand.Name)
}

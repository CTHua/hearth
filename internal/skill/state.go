package skill

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gitx"
)

// State is the condition of one skill as `skill list` shows it.
type State string

const (
	OK           State = "ok"
	Disabled     State = "disabled"
	Unknown            = State(apperr.Unknown)
	NotInstalled       = State(apperr.NotInstalled)
	PinMismatch        = State(apperr.PinMismatch)
	SkillInvalid       = State(apperr.SkillInvalid)
	LinkMissing        = State(apperr.LinkMissing)
	LinkStale          = State(apperr.LinkStale)
	LinkConflict       = State(apperr.LinkConflict)
	PathConflict       = State(apperr.PathConflict)
)

// severity orders states from the worst; a skill with several findings,
// such as one link per agent, shows the worst.
var severity = []State{PathConflict, LinkConflict, SkillInvalid, PinMismatch, LinkStale, NotInstalled, LinkMissing, Unknown, OK}

// warnings are states that need attention but are not failures.
var warnings = []State{NotInstalled, LinkMissing, Unknown}

// Warning reports whether s needs attention without being a failure.
func (s State) Warning() bool {
	return slices.Contains(warnings, s)
}

// Failure reports whether s is a problem that fails doctor.
func (s State) Failure() bool {
	return s != OK && s != Disabled && !s.Warning()
}

// Row is one line of `skill list`. Source is empty for a stale choice.
type Row struct {
	Name    string
	Source  Source
	Enabled bool
	State   State
	Detail  string
}

// finding is one state with the reason for it.
type finding struct {
	state  State
	detail string
}

func worse(a finding, b finding) finding {
	if slices.Index(severity, b.state) < slices.Index(severity, a.state) {
		return b
	}
	return a
}

// selection is what the personal file asks for: the chosen skills in
// catalog order, and chosen names no skill has.
type selection struct {
	wanted []Item
	stale  []string
	dirs   []string
}

func choose(home *config.Home, cat Catalog) selection {
	chosen := map[string]bool{}
	var sel selection
	for _, name := range home.Personal.Skills {
		chosen[strings.ToLower(name)] = true
		if _, found := cat.Find(name); !found {
			sel.stale = append(sel.stale, name)
		}
	}
	agents := len(home.Personal.Agents) > 0
	for _, item := range cat.items {
		if (item.Source == Builtin && agents) || (item.Source != Builtin && chosen[strings.ToLower(item.Name)]) {
			sel.wanted = append(sel.wanted, item)
		}
	}
	for _, agent := range home.Personal.Agents {
		sel.dirs = append(sel.dirs, entryDirs[agent])
	}
	return sel
}

func (s selection) wants(item Item) bool {
	return slices.ContainsFunc(s.wanted, func(w Item) bool { return w.Name == item.Name && w.Source == item.Source })
}

// StaleDetail explains a chosen name that no skill has.
func StaleDetail(name string) string {
	return fmt.Sprintf("enabled in %s, but no skill is called %q any more; treated as disabled. Remove it with `%s skill disable %s`",
		brand.LocalFile, name, brand.Name, name)
}

// MissingAgents lists the chosen agents whose CLI is not on PATH.
func MissingAgents(agents []string) []string {
	var missing []string
	for _, agent := range agents {
		if _, err := exec.LookPath(agent); err != nil {
			missing = append(missing, agent)
		}
	}
	return missing
}

// Inspect reports every skill of the catalog, then every stale choice. Only
// chosen skills, and the built-in one when an agent is chosen, get a state
// other than Disabled. It only reads: it never fetches or writes. The error
// is set only when the run was interrupted or git could not be started.
func Inspect(ctx context.Context, g *gitx.Git, home *config.Home, cat Catalog) ([]Row, error) {
	sel := choose(home, cat)
	var rows []Row
	for _, item := range cat.items {
		row := Row{Name: item.Name, Source: item.Source, Enabled: sel.wants(item), State: Disabled, Detail: item.Invalid}
		if row.Enabled {
			result, err := inspectItem(ctx, g, home.Root, item, sel.dirs)
			if err != nil {
				return nil, err
			}
			row.State = result.state
			row.Detail = result.detail
		}
		rows = append(rows, row)
	}
	for _, name := range sel.stale {
		rows = append(rows, Row{Name: name, Enabled: true, State: Unknown, Detail: StaleDetail(name)})
	}
	return rows, nil
}

func inspectItem(ctx context.Context, g *gitx.Git, root string, item Item, dirs []string) (finding, error) {
	if item.Invalid != "" {
		return finding{state: SkillInvalid, detail: item.Invalid}, nil
	}
	result := finding{state: OK}
	switch item.Source {
	case External:
		installed, err := externalState(ctx, g, root, item.Skill)
		if err != nil {
			return finding{}, err
		}
		result = installed
	case Builtin:
		result = builtinState(root)
	}
	for _, dir := range dirs {
		result = worse(result, linkState(root, dir, item))
	}
	return result, nil
}

// externalState checks the checkout of s without fetching.
func externalState(ctx context.Context, g *gitx.Git, root string, s config.Skill) (finding, error) {
	rel := VendorDir + pathSeparator + s.Name
	if problem := dirProblem(root, rel); problem != "" {
		return finding{state: PathConflict, detail: problem}, nil
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
		return finding{state: NotInstalled, detail: fmt.Sprintf("%s is missing; run `%s skill sync`", rel, brand.Name)}, nil
	}
	problem, err := checkout.RootProblem(ctx, g, target, rel)
	if err != nil || problem != "" {
		return finding{state: PathConflict, detail: problem}, err
	}
	head, detached, err := headState(ctx, g, target)
	if err != nil {
		return finding{}, err
	}
	if head != s.Commit || !detached {
		return finding{state: PinMismatch, detail: pinDetail(rel, head, detached, s.Commit) + fmt.Sprintf("; run `%s skill sync`", brand.Name)}, nil
	}
	if e := verifySubdir(target, s); e != nil {
		return finding{state: SkillInvalid, detail: e.Message}, nil
	}
	return finding{state: OK}, nil
}

func pinDetail(rel string, head string, detached bool, pin string) string {
	where := head
	if head == "" {
		where = "no commit"
	}
	if !detached {
		where += " on a branch"
	}
	return fmt.Sprintf("%s is at %s, but %s pins %s", rel, where, brand.TeamFile, pin)
}

func builtinState(root string) finding {
	if problem := dirProblem(root, builtinSkillDir); problem != "" {
		return finding{state: PathConflict, detail: problem}
	}
	rel := builtinSkillDir + pathSeparator + File
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return finding{state: NotInstalled, detail: fmt.Sprintf("%s is missing; run `%s skill sync`", rel, brand.Name)}
	}
	if err != nil {
		return finding{state: PathConflict, detail: fmt.Sprintf("cannot read %s: %v", rel, err)}
	}
	if string(data) != string(builtinContent) {
		return finding{state: NotInstalled, detail: fmt.Sprintf("%s differs from this build's; run `%s skill sync`", rel, brand.Name)}
	}
	return finding{state: OK}
}

// dirProblem rejects a symlink anywhere on rel, and any part of it that
// exists but is not a directory.
func dirProblem(root string, rel string) string {
	if problem := checkout.PathProblem(root, rel); problem != "" {
		return problem
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("cannot inspect %s: %v", rel, err)
	}
	if !info.IsDir() {
		return fmt.Sprintf("%s is not a directory", rel)
	}
	return ""
}

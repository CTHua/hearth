package subagent

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/genfile"
)

// target is where one agent reads its subagents.
type target struct {
	agent string
	dir   string
	ext   string
	// parentGoes is set when the directory above dir is removed too once
	// it is empty: .codex/ holds nothing else that must stay, while
	// .claude/ holds the team's settings.json.
	parentGoes bool
}

var targets = []target{
	{agent: config.AgentClaude, dir: ClaudeDir, ext: ".md"},
	{agent: config.AgentCodex, dir: CodexDir, ext: ".toml", parentGoes: true},
}

func (tg target) rel(name string) string {
	return tg.dir + pathSeparator + name + tg.ext
}

// owns reports whether a state record belongs to this target.
func (tg target) owns(rel string) bool {
	return strings.HasPrefix(rel, tg.dir+pathSeparator)
}

// ownerOf is the target of a recorded path; the path is one of ours.
func ownerOf(rel string) target {
	for _, tg := range targets {
		if tg.owns(rel) {
			return tg
		}
	}
	return target{}
}

// Result is what a sync did: warnings that do not change the exit code,
// and failures as "<name or file> (<category>)".
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

// selection is what the personal file asks for: the chosen valid items,
// chosen names whose file is invalid, and chosen names no file has.
type selection struct {
	items   []Item
	invalid []string
	stale   []string
}

func choose(home *config.Home, cat Catalog) selection {
	var sel selection
	for _, name := range home.Personal.Subagents {
		item, found := cat.Find(name)
		switch {
		case !found:
			sel.stale = append(sel.stale, name)
		case item.Invalid != "":
			sel.invalid = append(sel.invalid, item.Name)
		default:
			sel.items = append(sel.items, item)
		}
	}
	return sel
}

// Sync writes the file of each chosen subagent for each chosen agent, and
// removes the files it wrote that are no longer wanted. It owns only the
// state records under ClaudeDir and CodexDir; other records, such as the
// MCP files', are kept as they are. Each file is decided on its own, like
// the MCP files: written only when missing or still exactly what sync last
// wrote, removed only when still what sync last wrote, otherwise reported
// as generated-modified unless force. A symlink on any path is refused even
// with force. A file that is neither recorded nor wanted is never touched.
// A chosen subagent whose source became invalid fails, and its generated
// files are left as they are, so a broken change does not quietly take it
// away. The error is set only when the sources cannot be read or the state
// file cannot be parsed.
func Sync(home *config.Home, force bool, out io.Writer, errw io.Writer) (Result, error) {
	cat, err := Discover(home.Root)
	if err != nil {
		return Result{}, err
	}
	t := &tally{errw: errw}
	sel := choose(home, cat)
	for _, name := range sel.stale {
		t.warn(apperr.ForRepo(apperr.Unknown, name, "%s", StaleDetail(name)))
	}
	for _, name := range sel.invalid {
		item, _ := cat.Find(name)
		t.fail(apperr.ForRepo(InvalidCategory, name, "%s; its generated files are left unchanged", item.Invalid))
	}
	if len(sel.items) > 0 && len(home.Personal.Agents) == 0 {
		fmt.Fprintf(out, "subagent: subagents are enabled but no agent is chosen; choose one with `%s config agents`\n", brand.Name)
	}
	if problem := genfile.FileProblem(home.Root, brand.StateFile); problem != "" {
		t.fail(apperr.ForRepo(apperr.PathConflict, brand.StateFile, "%s; no subagent file was written", problem))
		return t.result, nil
	}
	st, err := genfile.Load(home.Root)
	if err != nil {
		return Result{}, err
	}
	wanted := wantedFiles(home, sel)
	kept := keptFiles(sel)
	changed := false
	emptied := map[string]bool{}
	for _, rel := range paths(wanted, st, kept) {
		content, want := wanted[rel]
		line, fileChanged, removed, e := syncFile(home.Root, rel, content, want, st, force)
		changed = changed || fileChanged
		if removed {
			emptied[ownerOf(rel).dir] = true
		}
		if e != nil {
			t.fail(e)
			continue
		}
		if line != "" {
			fmt.Fprintln(out, line)
		}
	}
	for _, tg := range targets {
		if !emptied[tg.dir] {
			continue
		}
		if err := removeEmptyDirs(home.Root, tg); err != nil {
			t.fail(apperr.ForRepo(apperr.PathConflict, tg.dir, "files removed, but cannot remove the empty directory: %v", err))
		}
	}
	if changed {
		if err := genfile.Save(home.Root, st); err != nil {
			t.fail(apperr.ForRepo(apperr.PathConflict, brand.StateFile, "cannot write: %v; the next sync reports the files it wrote as generated-modified", err))
		}
	}
	return t.result, nil
}

// wantedFiles is the content of every file this sync should leave, by
// path: each chosen valid subagent for each chosen agent.
func wantedFiles(home *config.Home, sel selection) map[string][]byte {
	wanted := map[string][]byte{}
	for _, item := range sel.items {
		for _, tg := range targets {
			if slices.Contains(home.Personal.Agents, tg.agent) {
				wanted[tg.rel(item.Name)] = item.files[tg.agent]
			}
		}
	}
	return wanted
}

// keptFiles are the paths of chosen subagents whose source is invalid:
// sync leaves them as they are.
func keptFiles(sel selection) map[string]bool {
	kept := map[string]bool{}
	for _, name := range sel.invalid {
		for _, tg := range targets {
			kept[tg.rel(name)] = true
		}
	}
	return kept
}

// nameOf is the subagent a generated path belongs to.
func nameOf(rel string) string {
	return strings.TrimSuffix(path.Base(rel), ownerOf(rel).ext)
}

// paths is every path to bring in line, sorted: the wanted ones and the
// recorded ones this package owns, except those kept as they are.
func paths(wanted map[string][]byte, st genfile.State, kept map[string]bool) []string {
	set := map[string]bool{}
	for rel := range wanted {
		set[rel] = true
	}
	for rel := range st.Files {
		if ownerOf(rel).dir != "" && !kept[rel] {
			set[rel] = true
		}
	}
	list := make([]string, 0, len(set))
	for rel := range set {
		list = append(list, rel)
	}
	sort.Strings(list)
	return list
}

// syncFile brings one generated file in line and updates st. changed
// reports whether st was changed, removed whether the file was removed.
func syncFile(root string, rel string, content []byte, wanted bool, st genfile.State, force bool) (string, bool, bool, *apperr.Error) {
	recorded, managed := st.Files[rel]
	if problem := genfile.FileProblem(root, rel); problem != "" {
		return "", false, false, apperr.ForRepo(apperr.PathConflict, rel, "%s; left unchanged, even with --force", problem)
	}
	current, exists, err := genfile.ReadHash(root, rel)
	if err != nil {
		return "", false, false, apperr.ForRepo(apperr.PathConflict, rel, "cannot read: %v", err)
	}
	untouched := exists && managed && current == recorded
	file := filepath.Join(root, filepath.FromSlash(rel))
	if !wanted {
		if !exists {
			delete(st.Files, rel)
			return "", true, false, nil
		}
		if !untouched && !force {
			return "", false, false, modified(rel, "not removed")
		}
		if err := os.Remove(file); err != nil {
			return "", false, false, apperr.ForRepo(apperr.PathConflict, rel, "cannot remove: %v", err)
		}
		delete(st.Files, rel)
		return fmt.Sprintf("subagent %s: removed", rel), true, true, nil
	}
	if exists && !untouched && !force {
		return "", false, false, modified(rel, "not overwritten")
	}
	sum := genfile.Digest(content)
	if untouched && current == sum {
		return fmt.Sprintf("subagent %s: up to date", rel), false, false, nil
	}
	if err := genfile.WriteFile(file, content); err != nil {
		return "", false, false, apperr.ForRepo(apperr.PathConflict, rel, "cannot write: %v", err)
	}
	st.Files[rel] = sum
	return fmt.Sprintf("subagent %s: wrote", rel), true, false, nil
}

// removeEmptyDirs removes the target's directory once it is empty, and the
// one above it when the target allows.
func removeEmptyDirs(root string, tg target) error {
	dir := filepath.Join(root, filepath.FromSlash(tg.dir))
	if err := genfile.RemoveIfEmpty(dir); err != nil {
		return err
	}
	if !tg.parentGoes {
		return nil
	}
	return genfile.RemoveIfEmpty(filepath.Dir(dir))
}

func modified(rel string, action string) *apperr.Error {
	return apperr.ForRepo(apperr.GeneratedModified, rel,
		"changed since `%s sync` last wrote it, or not written by it; %s. Move your changes into %s/%s%s, then run `%s sync --force`",
		brand.Name, action, SourceDir, nameOf(rel), sourceExt, brand.Name)
}

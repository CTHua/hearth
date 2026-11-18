package subagent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/genfile"
)

// State is what `subagent list` shows for one subagent.
type State string

const (
	// OK is chosen, with every generated file as sync would write it.
	OK State = "ok"
	// Off is a valid subagent nobody chose.
	Off State = "off"
	// NotSynced is chosen, with a generated file missing or outdated.
	NotSynced State = "not-synced"
	// Stale is a chosen name no source file has any more.
	Stale        State = "stale"
	Invalid      State = State(InvalidCategory)
	Modified     State = State(apperr.GeneratedModified)
	PathConflict State = State(apperr.PathConflict)
)

// InvalidCategory is the error category of a source file that breaks a
// rule. It lives here rather than in apperr because only this package and
// its callers report it.
const InvalidCategory apperr.Category = "subagent-invalid"

// rank orders the states of one subagent's files: the worst one is shown.
var rank = []State{OK, NotSynced, Modified, PathConflict}

// Row is one subagent as `subagent list` and doctor show it.
type Row struct {
	Name    string
	Enabled bool
	// Stale is a chosen name no source file has any more; it is treated as
	// not chosen. Its State is Stale unless a file it left is a problem.
	Stale bool
	State State
	// Detail is the problem when there is one, else the description.
	Detail string
	// Invalid is why the source file breaks a rule, kept apart from State
	// so a problem with a generated file does not hide it; empty when the
	// source is valid.
	Invalid string
}

// invalidSep joins a file problem and the source's reason in FullDetail.
const invalidSep = "; source: "

// FullDetail is Detail, followed by the source's reason when State is a
// file problem that would otherwise hide it.
func (r Row) FullDetail() string {
	if r.Invalid == "" || r.State == Invalid {
		return r.Detail
	}
	return r.Detail + invalidSep + r.Invalid
}

// report is the worst state found among one name's files.
type report struct {
	state  State
	detail string
}

func (r *report) add(state State, detail string) {
	if slices.Index(rank, state) > slices.Index(rank, r.state) {
		r.state = state
		r.detail = detail
	}
}

// Inspect reports every source file, then every stale choice, then every
// name that has only a recorded generated file left. It checks each file
// sync would act on: a wanted one for being missing, outdated, edited or
// in the way, and a recorded one sync would remove for being edited or in
// the way. It only reads. The error is set when the sources cannot be read
// or the state file cannot be parsed.
func Inspect(home *config.Home) ([]Row, error) {
	cat, err := Discover(home.Root)
	if err != nil {
		return nil, err
	}
	stateProblem := genfile.FileProblem(home.Root, brand.StateFile)
	st := genfile.State{Files: map[string]string{}}
	if stateProblem == "" {
		st, err = genfile.Load(home.Root)
		if err != nil {
			return nil, err
		}
	}
	sel := choose(home, cat)
	wanted := wantedFiles(home, sel)
	files := map[string]*report{}
	var orphans []string
	// Without a state file to trust, every file would look edited.
	checked := paths(wanted, st, keptFiles(sel))
	if stateProblem != "" {
		checked = nil
	}
	for _, rel := range checked {
		name := nameOf(rel)
		if files[name] == nil {
			files[name] = &report{state: OK}
			orphans = append(orphans, name)
		}
		content, want := wanted[rel]
		if want {
			files[name].add(fileState(home.Root, rel, content, st))
			continue
		}
		files[name].add(leftoverState(home.Root, rel, st.Files[rel]))
	}
	enabled := map[string]bool{}
	for _, name := range home.Personal.Subagents {
		enabled[strings.ToLower(name)] = true
	}
	listed := map[string]bool{}
	var rows []Row
	for _, item := range cat.Items() {
		row := Row{Name: item.Name, Enabled: enabled[strings.ToLower(item.Name)], State: Off, Detail: item.Description, Invalid: item.Invalid}
		switch {
		case item.Invalid != "" && row.Enabled:
			row.State = Invalid
			row.Detail = item.Invalid
		case row.Enabled && stateProblem != "":
			row.State = PathConflict
			row.Detail = stateProblem
		case row.Enabled && len(home.Personal.Agents) == 0:
			row.State = OK
			row.Detail = fmt.Sprintf("no agent is chosen, so nothing is generated; choose one with `%s config agents`", brand.Name)
		case row.Enabled:
			row.State = OK
		case item.Invalid != "":
			row.State = Invalid
			row.Detail = item.Invalid
		}
		rows = append(rows, withFiles(row, files[item.Name]))
		listed[item.Name] = true
	}
	for _, name := range sel.stale {
		rows = append(rows, withFiles(Row{Name: name, Enabled: true, Stale: true, State: Stale, Detail: StaleDetail(name)}, files[name]))
		listed[name] = true
	}
	for _, name := range orphans {
		if listed[name] || files[name].state == OK {
			continue
		}
		rows = append(rows, Row{Name: name, State: files[name].state, Detail: files[name].detail})
	}
	return rows, nil
}

// withFiles replaces the row's state with the worst state of its files when
// that is a problem. An invalid chosen subagent keeps its state: sync leaves
// its files alone.
func withFiles(row Row, files *report) Row {
	if files == nil || files.state == OK || (row.State == Invalid && row.Enabled) {
		return row
	}
	row.State = files.state
	row.Detail = files.detail
	return row
}

// leftoverState is the state of a recorded file sync would remove: a
// problem only when it was edited or is in the way.
func leftoverState(root string, rel string, recorded string) (State, string) {
	if problem := genfile.FileProblem(root, rel); problem != "" {
		return PathConflict, problem
	}
	current, exists, err := genfile.ReadHash(root, rel)
	if err != nil {
		return PathConflict, fmt.Sprintf("cannot read %s: %v", rel, err)
	}
	if exists && current != recorded {
		return Modified, fmt.Sprintf("%s changed since `%s sync` last wrote it; sync will not remove it without --force", rel, brand.Name)
	}
	return OK, ""
}

func fileState(root string, rel string, content []byte, st genfile.State) (State, string) {
	if problem := genfile.FileProblem(root, rel); problem != "" {
		return PathConflict, problem
	}
	current, exists, err := genfile.ReadHash(root, rel)
	if err != nil {
		return PathConflict, fmt.Sprintf("cannot read %s: %v", rel, err)
	}
	recorded, managed := st.Files[rel]
	switch {
	case exists && (!managed || current != recorded):
		return Modified, fmt.Sprintf("%s changed since `%s sync` last wrote it, or not written by it", rel, brand.Name)
	case !exists || current != genfile.Digest(content):
		return NotSynced, fmt.Sprintf("%s is missing or outdated; run `%s sync`", rel, brand.Name)
	}
	return OK, ""
}

// StaleDetail explains a chosen name that no source file has.
func StaleDetail(name string) string {
	return fmt.Sprintf("enabled in %s, but there is no %s/%s%s any more; treated as disabled, and sync removes its generated files. Remove it with `%s subagent disable %s`",
		brand.LocalFile, SourceDir, name, sourceExt, brand.Name, name)
}

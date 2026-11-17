package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

// Result is what a sync did: skills that are in place with every link,
// warnings that do not change the exit code, and failures as
// "<name> (<category>)".
type Result struct {
	Ready  []string
	Warned []string
	Failed []string
}

// tally records per-skill outcomes, keeping only the first failure of each.
type tally struct {
	result Result
	errw   io.Writer
}

func (t *tally) fail(e *apperr.Error) {
	apperr.Print(t.errw, e)
	label := fmt.Sprintf("%s (%s)", e.Name, e.Category)
	if e.Name == "" {
		label = string(e.Category)
	}
	if !slices.Contains(t.result.Failed, label) {
		t.result.Failed = append(t.result.Failed, label)
	}
}

func (t *tally) warn(e *apperr.Error) {
	apperr.Warn(t.errw, e)
	t.result.Warned = append(t.result.Warned, fmt.Sprintf("%s (%s)", e.Name, e.Category))
}

// record prints a per-skill failure and reports whether it was one; any
// other error, such as an interrupt, is returned to stop the run.
func (t *tally) record(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	var e *apperr.Error
	if !errors.As(err, &e) {
		return false, err
	}
	t.fail(e)
	return true, nil
}

// CloneURLs returns the team-file URLs of the chosen external skills whose
// checkout is not at its pin, so the caller can settle the protocol before
// Sync needs the network. It only reads.
func CloneURLs(ctx context.Context, g *gitx.Git, home *config.Home, cat Catalog) ([]string, error) {
	var urls []string
	for _, item := range choose(home, cat).wanted {
		if item.Source != External || item.Invalid != "" {
			continue
		}
		state, err := externalState(ctx, g, home.Root, item.Skill)
		if err != nil {
			return nil, err
		}
		if state.state == NotInstalled || state.state == PinMismatch {
			urls = append(urls, item.Skill.URL)
		}
	}
	return urls, nil
}

// Sync checks the chosen agents' CLIs, installs or moves each chosen
// external skill to its pin, writes or removes the built-in skill, creates
// or repairs each link, then removes the managed links nothing chosen needs.
// One skill failing does not stop the others. Progress lines go to out,
// problems and git's own output to errw. The error is set only when the
// run must stop, such as on an interrupt.
func Sync(ctx context.Context, g *gitx.Git, home *config.Home, cat Catalog, out io.Writer, errw io.Writer) (Result, error) {
	t := &tally{errw: errw}
	sel := choose(home, cat)
	for _, agent := range MissingAgents(home.Personal.Agents) {
		t.warn(apperr.ForRepo(apperr.AgentMissing, agent, "%s not found in PATH; links are created anyway, and %s does not install agents", agent, brand.Name))
	}
	for _, name := range sel.stale {
		t.warn(apperr.ForRepo(apperr.Category(Unknown), name, "%s", StaleDetail(name)))
	}
	ready, err := installAll(ctx, g, home, sel, t, out)
	if err != nil {
		return Result{}, err
	}
	keep := map[string]bool{}
	for _, item := range sel.wanted {
		if item.Invalid != "" {
			continue
		}
		for _, dir := range sel.dirs {
			keep[dir+pathSeparator+item.Name] = true
		}
	}
	for _, item := range ready {
		linked := true
		for _, dir := range sel.dirs {
			line, err := syncLink(home.Root, dir, item)
			failed, err := t.record(err)
			if err != nil {
				return Result{}, err
			}
			if failed {
				linked = false
				continue
			}
			fmt.Fprintln(out, line)
		}
		if linked {
			t.result.Ready = append(t.result.Ready, item.Name)
		}
	}
	removed, problems := pruneLinks(home.Root, keep)
	for _, shown := range removed {
		fmt.Fprintf(out, "link %s: removed\n", shown)
	}
	for _, problem := range problems {
		t.fail(problem)
	}
	if len(home.Personal.Agents) == 0 && len(home.Personal.Skills) == 0 {
		fmt.Fprintf(out, "Nothing enabled in %s\n", brand.LocalFile)
	}
	return t.result, nil
}

// installAll brings every chosen skill into place and returns the ones
// ready to be linked; the built-in skill is removed when no agent is chosen.
func installAll(ctx context.Context, g *gitx.Git, home *config.Home, sel selection, t *tally, out io.Writer) ([]Item, error) {
	var ready []Item
	builtinWanted := false
	for _, item := range sel.wanted {
		if item.Invalid != "" {
			t.fail(apperr.ForRepo(apperr.SkillInvalid, item.Name, "%s; not linked", item.Invalid))
			continue
		}
		line, err := installItem(ctx, g, home, item, t.errw)
		failed, err := t.record(err)
		if err != nil {
			return nil, err
		}
		builtinWanted = builtinWanted || item.Source == Builtin
		if failed {
			continue
		}
		if line != "" {
			fmt.Fprintln(out, line)
		}
		ready = append(ready, item)
	}
	if builtinWanted {
		return ready, nil
	}
	line, err := removeBuiltin(home.Root)
	if _, err := t.record(err); err != nil {
		return nil, err
	}
	if line != "" {
		fmt.Fprintln(out, line)
	}
	return ready, nil
}

func installItem(ctx context.Context, g *gitx.Git, home *config.Home, item Item, progress io.Writer) (string, error) {
	switch item.Source {
	case External:
		url, err := giturl.Expected(item.Skill.URL, home.Personal.Protocol)
		if err != nil {
			return "", apperr.ForRepo(apperr.Config, item.Name, "%v", err)
		}
		return installExternal(ctx, g, home.Root, item.Skill, url, progress)
	case Builtin:
		return writeBuiltin(home.Root)
	}
	return "", nil
}

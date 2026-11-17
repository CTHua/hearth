package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/clone"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/skill"
	"github.com/CTHua/hearth/internal/version"
)

// doctorReport tracks the worst result: setup problems exit 2, checkout
// problems exit 1, warnings alone exit 0.
type doctorReport struct {
	s           streams
	setupBad    bool
	checkoutBad bool
}

func (r *doctorReport) ok(format string, args ...any) {
	fmt.Fprintf(r.s.out, "ok: "+format+"\n", args...)
}

func (r *doctorReport) warn(format string, args ...any) {
	fmt.Fprintf(r.s.out, "warning: "+format+"\n", args...)
}

func (r *doctorReport) setup(err error) {
	r.setupBad = true
	var list apperr.List
	if errors.As(err, &list) {
		for _, e := range list {
			apperr.Print(r.s.err, e)
		}
		return
	}
	var single *apperr.Error
	if errors.As(err, &single) {
		apperr.Print(r.s.err, single)
		return
	}
	apperr.Print(r.s.err, apperr.New(apperr.Env, "%v", err))
}

func (r *doctorReport) checkoutFailed(e *apperr.Error) {
	r.checkoutBad = true
	apperr.Print(r.s.err, e)
}

func (r *doctorReport) exit() error {
	if r.setupBad {
		return apperr.Exit(apperr.ExitUsage)
	}
	if r.checkoutBad {
		return apperr.Exit(apperr.ExitFailed)
	}
	return nil
}

// runDoctor only reads: it never connects to a remote or writes a file.
func runDoctor(ctx context.Context, s streams, g globals, args []string) error {
	fs := newFlagSet("doctor")
	positional, help, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if help {
		fmt.Fprintf(s.out, "usage: %s doctor\n", brand.Name)
		return nil
	}
	if len(positional) > 0 {
		return apperr.New(apperr.Usage, "doctor takes no arguments")
	}
	report := &doctorReport{s: s}
	git := checkGit(ctx, report)
	report.ok("%s %s", brand.Name, version.Current())
	home := checkConfig(report, g)
	if home == nil {
		return report.exit()
	}
	checkMarkers(report, home.Root)
	if home.Personal.Protocol == "" {
		report.warn("protocol is not set in %s; it is detected on the first `%s clone`, or set it with `%s config protocol <ssh|https>`",
			brand.LocalFile, brand.Name, brand.Name)
	} else {
		report.ok("protocol %s", home.Personal.Protocol)
	}
	// Before the git check returns: these need no git.
	checkMCP(report, home)
	if git == nil {
		report.warn("checkouts not checked because git is missing")
		return report.exit()
	}
	if err := checkRepos(ctx, report, git, home); err != nil {
		return err
	}
	if err := checkSkills(ctx, report, git, home); err != nil {
		return err
	}
	return report.exit()
}

// checkSkills reports only what the personal file asks for; disabled
// skills are not mentioned. A missing agent CLI is only a warning, since CI
// and sandboxes often have none.
func checkSkills(ctx context.Context, report *doctorReport, git *gitx.Git, home *config.Home) error {
	cat, err := skill.Discover(home)
	if err != nil {
		report.setup(err)
		return nil
	}
	for _, agent := range skill.MissingAgents(home.Personal.Agents) {
		report.warn("[%s] %s: %s not found in PATH; %s does not install agents", apperr.AgentMissing, agent, agent, brand.Name)
	}
	rows, err := skill.Inspect(ctx, git, home, cat)
	if err != nil {
		return err
	}
	for _, row := range rows {
		switch {
		case row.State == skill.Disabled:
			continue
		case row.State == skill.OK:
			report.ok("skill %s (%s)", row.Name, row.Source)
		case row.State.Warning():
			report.warn("[%s] %s: %s", row.State, row.Name, row.Detail)
		default:
			report.checkoutFailed(apperr.ForRepo(apperr.Category(row.State), row.Name, "%s", row.Detail))
		}
	}
	return nil
}

// checkMCP reports only the chosen servers. A missing variable or command
// is only a warning: many variables are set only in some shells. Names are
// printed, never values.
func checkMCP(report *doctorReport, home *config.Home) {
	for _, row := range mcp.Inspect(home) {
		switch {
		case !row.Enabled:
			continue
		case row.Stale:
			report.warn("[%s] %s: %s", apperr.Unknown, row.Name, mcp.StaleDetail(row.Name))
		case len(row.Warnings()) == 0:
			report.ok("mcp server %s (%s)", row.Name, row.Transport)
		default:
			for _, warning := range row.Warnings() {
				report.warn("[%s] %s: %s", apperr.EnvMissing, row.Name, warning)
			}
		}
	}
}

func checkGit(ctx context.Context, report *doctorReport) *gitx.Git {
	git, err := gitx.New(ctx)
	if err != nil {
		report.setup(err)
		return nil
	}
	out, err := git.Run(ctx, "", "--version")
	if err != nil {
		report.setup(apperr.New(apperr.Env, "git --version failed: %v", err))
		return nil
	}
	report.ok("%s", strings.TrimSpace(out))
	return git
}

func checkConfig(report *doctorReport, g globals) *config.Home {
	home, err := loadHome(report.s, g)
	if err != nil {
		report.setup(err)
		return nil
	}
	personal := brand.LocalFile + " not present"
	if home.Personal.SchemaVersion != 0 {
		personal = brand.LocalFile
	}
	report.ok("%s (%d repos), %s", brand.TeamFile, len(home.Team.Repos), personal)
	return home
}

func checkMarkers(report *doctorReport, root string) {
	changes, problems := scaffold.SectionChanges(root)
	if len(problems) > 0 {
		report.setup(problems)
		return
	}
	report.ok("marked sections in AGENTS.md and .gitignore")
	// An outdated section is only a warning: the old one still works.
	for _, change := range changes {
		report.warn("%s: the marked section differs from this version of %s; run `%s upgrade`", change.File, brand.Name, brand.Name)
	}
}

func checkRepos(ctx context.Context, report *doctorReport, git *gitx.Git, home *config.Home) error {
	targets, err := clone.Targets(home.Team.Repos, home.Personal.Protocol)
	if err != nil {
		report.setup(err)
		return nil
	}
	for _, t := range targets {
		status, err := checkout.Inspect(ctx, git, home.Root, t.Repo.Path, t.URL)
		if err != nil {
			return err
		}
		switch status.State {
		case checkout.OK:
			report.ok("%s at %s", t.Repo.Name, t.Repo.Path)
		case checkout.Missing:
			report.warn("%s is not cloned at %s; run `%s clone %s`", t.Repo.Name, t.Repo.Path, brand.Name, t.Repo.Name)
		case checkout.RemoteMismatch:
			message := clone.MismatchMessage(t, status)
			if status.HasOrigin && giturl.OnlyProtocolDiffers(status.Origin, t.URL) {
				message += fmt.Sprintf("; only the protocol differs, run `%s repo fix-remote %s`", brand.Name, t.Repo.Name)
			}
			report.checkoutFailed(apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "%s", message))
		default:
			report.checkoutFailed(apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "%s", status.Detail))
		}
	}
	return nil
}

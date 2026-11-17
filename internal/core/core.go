// Package core holds every user action on a loaded home repo, shared by the
// CLI and the terminal UI. Actions take structured parameters and return
// structured results; they parse no flags and pick no output stream.
//
// Batch actions (Clone, FixRemote, SyncSkills, SyncMCP, Sync) are the one
// exception: they stream per-item lines, git's progress and the summary to
// the Output the caller passes, as they happen, because a run can take
// minutes. Their result carries the same outcome in structured form.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/mcp"
	"github.com/CTHua/hearth/internal/skill"
)

const (
	noneLabel   = "(none)"
	failedLabel = "failed"
	listSep     = ", "
	pathSep     = "/"
)

// Actions is what the terminal UI calls; Service implements it, and UI tests
// pass a fake. Methods are not safe for concurrent use: run one at a time.
type Actions interface {
	// Home returns a copy of the home repo as the last action left it.
	Home() config.Home

	AddRepo(ctx context.Context, p AddRepoParams) (AddRepoResult, error)
	RemoveRepo(name string) (RemoveRepoResult, error)
	ListRepos(ctx context.Context) ([]RepoRow, error)
	Clone(ctx context.Context, names []string, out Output) (BatchResult, error)
	FixRemote(ctx context.Context, names []string, out Output) (BatchResult, error)

	SetProtocol(protocol string) error
	DetectProtocol(ctx context.Context) (string, error)

	SetAgents(agents []string) error
	EnableSkills(names []string) ([]string, error)
	DisableSkills(names []string) ([]string, error)
	AddSkill(ctx context.Context, p AddSkillParams) (AddSkillResult, error)
	RemoveSkill(name string) (RemoveSkillResult, error)
	UpdateSkill(ctx context.Context, name string, commit *string) (UpdateSkillResult, error)
	ListSkills(ctx context.Context) ([]skill.Row, error)
	SyncSkills(ctx context.Context, out Output) (BatchResult, error)

	AddMCP(server config.MCP) error
	RemoveMCP(name string) (config.MCP, error)
	EnableMCP(names []string) ([]string, error)
	DisableMCP(names []string) ([]string, error)
	ListMCP() []mcp.Row
	Sync(ctx context.Context, force bool, out Output) (SyncResult, error)

	TeamChanges(ctx context.Context) ([]TeamChange, error)
}

var _ Actions = (*Service)(nil)

// Service runs actions on one loaded home repo. Each action that writes a
// file also updates the loaded copy, so later actions see the change.
type Service struct {
	home *config.Home
}

// New returns the actions for home, which must come from config.Load.
func New(home *config.Home) *Service {
	return &Service{home: home}
}

// Home returns a copy of the loaded home repo.
func (s *Service) Home() config.Home {
	return *s.home
}

// Output is where a batch action streams its transcript: per-item lines and
// the summary to Out, problems and git's own output to Err.
type Output struct {
	Out io.Writer
	Err io.Writer
}

// BatchResult is a run over several items where one failure does not stop
// the others. Failed entries read "<name> (<category>)".
type BatchResult struct {
	Done    []string
	Skipped []string
	Failed  []string
	// DoneLabel and SkippedLabel name the groups in the summary, such as
	// "cloned" and "skipped".
	DoneLabel    string
	SkippedLabel string
	// Empty is set when there was nothing to run on, such as no repos.
	Empty bool
	// DetectedProtocol is set when the run detected and saved the protocol.
	DetectedProtocol string
}

func newBatch(doneLabel string, skippedLabel string) BatchResult {
	return BatchResult{DoneLabel: doneLabel, SkippedLabel: skippedLabel}
}

// Summary is the closing block the CLI prints, one group per line.
// SummaryPrefix starts the first line of a Summary; the group lines under it
// are indented.
const SummaryPrefix = "Summary: "

func (b BatchResult) Summary() string {
	var text strings.Builder
	fmt.Fprintf(&text, SummaryPrefix+"%d %s, %d %s, %d %s\n",
		len(b.Done), b.DoneLabel, len(b.Skipped), b.SkippedLabel, len(b.Failed), failedLabel)
	groups := []struct {
		label string
		names []string
	}{{b.DoneLabel, b.Done}, {b.SkippedLabel, b.Skipped}, {failedLabel, b.Failed}}
	for _, group := range groups {
		fmt.Fprintf(&text, "  %s: %s\n", group.label, LabelList(group.names))
	}
	return text.String()
}

// record prints a per-item failure and keeps going; any other error, such
// as an interrupt, is returned so the run stops.
func (b *BatchResult) record(out Output, name string, err error) error {
	if err == nil {
		return nil
	}
	var itemErr *apperr.Error
	if !errors.As(err, &itemErr) {
		return err
	}
	apperr.Print(out.Err, itemErr)
	b.Failed = append(b.Failed, fmt.Sprintf("%s (%s)", name, itemErr.Category))
	return nil
}

// finish writes the summary and returns the error a batch with failures
// ends with: already printed, it only carries the exit code.
func (b BatchResult) finish(out Output) (BatchResult, error) {
	fmt.Fprint(out.Out, b.Summary())
	return b, failedExit(len(b.Failed))
}

func failedExit(failed int) error {
	if failed > 0 {
		return apperr.Exit(apperr.ExitFailed)
	}
	return nil
}

// LabelList joins names for a message, or says "(none)".
func LabelList(names []string) string {
	if len(names) == 0 {
		return noneLabel
	}
	return strings.Join(names, listSep)
}

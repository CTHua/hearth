package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/clone"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

const (
	fixedLabel     = "fixed"
	unchangedLabel = "unchanged"
)

// AddRepoParams describes a new [[repos]] entry. A nil field was not given:
// Name defaults to the URL's last segment, Path to products/<name>, and
// Branch is asked from the remote, which may first detect the protocol.
type AddRepoParams struct {
	URL    string
	Name   *string
	Path   *string
	Branch *string
}

// AddRepoResult is the entry that was added. DetectedProtocol is set when
// the protocol was detected and saved, even when adding then failed.
type AddRepoResult struct {
	Repo             config.Repo
	DetectedProtocol string
}

// AddRepo adds a repo to the team file. Everything known locally is checked
// before the remote is contacted.
func (s *Service) AddRepo(ctx context.Context, p AddRepoParams) (AddRepoResult, error) {
	var result AddRepoResult
	repo, err := newRepo(p)
	if err != nil {
		return result, err
	}
	if p.Branch == nil {
		if err := s.home.Team.CheckNew(repo, config.FieldBranch); err != nil {
			return result, addProblems(err)
		}
		branch, detected, err := s.remoteDefaultBranch(ctx, repo.URL)
		result.DetectedProtocol = detected
		if err != nil {
			return result, err
		}
		repo.Branch = branch
	}
	team, err := s.home.Team.WithRepo(repo)
	if err != nil {
		return result, addProblems(err)
	}
	if err := s.saveTeam(team); err != nil {
		return result, err
	}
	result.Repo = repo
	return result, nil
}

// newRepo fills the defaults: name from the URL, path from the name.
func newRepo(p AddRepoParams) (config.Repo, error) {
	u, err := giturl.Parse(p.URL)
	if err != nil {
		return config.Repo{}, apperr.New(apperr.Usage, "url %q: %v", p.URL, err)
	}
	name := u.DefaultName()
	if p.Name != nil {
		name = *p.Name
	}
	path := config.ProductsDir + pathSep + name
	if p.Path != nil {
		path = *p.Path
	}
	branch := ""
	if p.Branch != nil {
		branch = *p.Branch
	}
	return config.Repo{Name: name, URL: p.URL, Path: path, Branch: branch}, nil
}

// repoFlags are the repo add flags that set a field of the same name.
var repoFlags = []string{config.FieldName, config.FieldPath, config.FieldBranch}

// addProblems turns validation problems of the new entry into one usage
// error that names the flag to fix.
func addProblems(err error) error {
	return entryProblems("add repo", err, repoFlags)
}

// entryProblems turns validation problems of a new or changed team-file
// entry into one usage error; fields in flagged get a hint naming the flag.
func entryProblems(action string, err error, flagged []string) error {
	var problems config.FieldErrors
	if !errors.As(err, &problems) {
		return err
	}
	parts := make([]string, 0, len(problems))
	for _, problem := range problems {
		parts = append(parts, fmt.Sprintf("%s %s%s", problem.Field, problem.Message, flagHint(problem.Field, flagged)))
	}
	return apperr.New(apperr.Usage, "cannot %s: %s; %s is unchanged", action, strings.Join(parts, "; "), brand.TeamFile)
}

func flagHint(field string, flagged []string) string {
	if slices.Contains(flagged, field) {
		return fmt.Sprintf(" (choose another with --%s)", field)
	}
	return ""
}

// remoteURL returns git and the URL the user will clone rawURL with; that
// needs a protocol, which may first be detected.
func (s *Service) remoteURL(ctx context.Context, rawURL string) (*gitx.Git, string, string, error) {
	git, err := gitx.New(ctx)
	if err != nil {
		return nil, "", "", err
	}
	protocol, detected, err := s.ensureProtocol(ctx, git, []string{rawURL})
	if err != nil {
		return nil, "", "", err
	}
	url, err := giturl.Expected(rawURL, protocol)
	if err != nil {
		return nil, "", detected, apperr.New(apperr.Usage, "url %q: %v", rawURL, err)
	}
	return git, url, detected, nil
}

// remoteDefaultBranch asks the remote for its HEAD branch. The second
// result is the protocol it detected, if any.
func (s *Service) remoteDefaultBranch(ctx context.Context, rawURL string) (string, string, error) {
	git, url, detected, err := s.remoteURL(ctx, rawURL)
	if err != nil {
		return "", detected, err
	}
	branch, found, err := git.DefaultBranch(ctx, url)
	if err != nil {
		if ctx.Err() != nil {
			return "", detected, ctx.Err()
		}
		return "", detected, apperr.New(apperr.Env, "cannot ask %s for its default branch: %v; pass --branch", url, err)
	}
	if !found {
		return "", detected, apperr.New(apperr.Usage, "%s did not report a default branch; pass --branch", url)
	}
	return branch, detected, nil
}

// ensureProtocol returns the personal protocol, detecting and saving it
// first when it is not set yet. The second result is the detected protocol,
// empty when it was already set.
func (s *Service) ensureProtocol(ctx context.Context, git *gitx.Git, urls []string) (string, string, error) {
	if s.home.Personal.Protocol != "" {
		return s.home.Personal.Protocol, "", nil
	}
	protocol, err := s.detectAndSave(ctx, git, urls)
	if err != nil {
		return "", "", err
	}
	return protocol, protocol, nil
}

// ensureProtocolTo is ensureProtocol for a batch action, which reports the
// detection in its transcript.
func (s *Service) ensureProtocolTo(ctx context.Context, git *gitx.Git, urls []string, out Output, result *BatchResult) (string, error) {
	protocol, detected, err := s.ensureProtocol(ctx, git, urls)
	if err != nil {
		return "", err
	}
	if detected != "" {
		result.DetectedProtocol = detected
		fmt.Fprintln(out.Out, ProtocolNotice(detected))
	}
	return protocol, nil
}

// ProtocolNotice is the line telling the user a protocol was detected and saved.
func ProtocolNotice(protocol string) string {
	return fmt.Sprintf("protocol: detected %s and saved it to %s; change it with `%s config protocol <ssh|https>`",
		protocol, brand.LocalFile, brand.Name)
}

// RemoveRepoResult is the entry that was removed and whether its checkout
// is still on disk; it is never deleted.
type RemoveRepoResult struct {
	Repo         config.Repo
	CheckoutKept bool
}

// RemoveRepo removes a repo from the team file and leaves its checkout.
func (s *Service) RemoveRepo(name string) (RemoveRepoResult, error) {
	repo, found := s.home.Team.Find(name)
	if !found {
		return RemoveRepoResult{}, unknownNames([]string{name}, s.home.Team)
	}
	team, _ := s.home.Team.Without(name)
	if err := s.saveTeam(team); err != nil {
		return RemoveRepoResult{}, err
	}
	kept := exists(filepath.Join(s.home.Root, filepath.FromSlash(repo.Path)))
	return RemoveRepoResult{Repo: repo, CheckoutKept: kept}, nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func unknownNames(names []string, team config.Team) error {
	return apperr.New(apperr.Usage, "unknown repo name(s): %s; available: %s", strings.Join(names, ", "), LabelList(team.Names()))
}

// RepoRow is one repo and the state of its checkout.
type RepoRow struct {
	Repo   config.Repo
	Status checkout.Status
}

// ListRepos inspects every checkout; it only reads. With no repos it needs
// no git and returns no rows.
func (s *Service) ListRepos(ctx context.Context) ([]RepoRow, error) {
	if len(s.home.Team.Repos) == 0 {
		return nil, nil
	}
	git, err := gitx.New(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]RepoRow, 0, len(s.home.Team.Repos))
	for _, repo := range s.home.Team.Repos {
		expected, err := giturl.Expected(repo.URL, s.home.Personal.Protocol)
		if err != nil {
			return nil, apperr.ForRepo(apperr.Config, repo.Name, "%v", err)
		}
		status, err := checkout.Inspect(ctx, git, s.home.Root, repo.Path, expected)
		if err != nil {
			return nil, err
		}
		rows = append(rows, RepoRow{Repo: repo, Status: status})
	}
	return rows, nil
}

// Clone clones the named repos, or every repo when names is empty, that
// are not cloned yet. One repo failing does not stop the others.
func (s *Service) Clone(ctx context.Context, names []string, out Output) (BatchResult, error) {
	result := newBatch(string(clone.Cloned), string(clone.Skipped))
	git, err := gitx.New(ctx)
	if err != nil {
		return result, err
	}
	selected, err := selectRepos(s.home.Team, names)
	if err != nil {
		return result, err
	}
	if len(selected) == 0 {
		return noRepos(out, result)
	}
	protocol, err := s.ensureProtocolTo(ctx, git, repoURLs(selected), out, &result)
	if err != nil {
		return result, err
	}
	targets, err := clone.Targets(selected, protocol)
	if err != nil {
		return result, err
	}
	for _, target := range targets {
		outcome, err := clone.One(ctx, git, s.home.Root, target, out.Err)
		if err := result.record(out, target.Repo.Name, err); err != nil {
			return result, err
		}
		if err != nil {
			continue
		}
		if outcome == clone.Cloned {
			fmt.Fprintf(out.Out, "cloned %s -> %s (%s)\n", target.Repo.Name, target.Repo.Path, target.Repo.Branch)
			result.Done = append(result.Done, target.Repo.Name)
			continue
		}
		fmt.Fprintf(out.Out, "skipped %s: already cloned at %s\n", target.Repo.Name, target.Repo.Path)
		result.Skipped = append(result.Skipped, target.Repo.Name)
	}
	return result.finish(out)
}

func noRepos(out Output, result BatchResult) (BatchResult, error) {
	fmt.Fprintf(out.Out, "No repos configured in %s\n", brand.TeamFile)
	result.Empty = true
	return result, nil
}

// selectRepos returns every repo when names is empty, otherwise the named
// ones in team-file order, each once. Unknown names are a usage error.
func selectRepos(team config.Team, names []string) ([]config.Repo, error) {
	if len(names) == 0 {
		return team.Repos, nil
	}
	var unknown []string
	for _, name := range names {
		_, found := team.Find(name)
		if !found && !slices.Contains(unknown, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return nil, unknownNames(unknown, team)
	}
	var selected []config.Repo
	for _, repo := range team.Repos {
		if slices.Contains(names, repo.Name) {
			selected = append(selected, repo)
		}
	}
	return selected, nil
}

func repoURLs(repos []config.Repo) []string {
	urls := make([]string, 0, len(repos))
	for _, repo := range repos {
		urls = append(urls, repo.URL)
	}
	return urls
}

// FixRemote points origin at the expected URL for the named repos, or every
// repo when names is empty, when only the protocol differs.
func (s *Service) FixRemote(ctx context.Context, names []string, out Output) (BatchResult, error) {
	result := newBatch(fixedLabel, unchangedLabel)
	git, err := gitx.New(ctx)
	if err != nil {
		return result, err
	}
	selected, err := selectRepos(s.home.Team, names)
	if err != nil {
		return result, err
	}
	if len(selected) == 0 {
		return noRepos(out, result)
	}
	targets, err := clone.Targets(selected, s.home.Personal.Protocol)
	if err != nil {
		return result, err
	}
	for _, target := range targets {
		fixed, err := fixOne(ctx, out, git, s.home.Root, target)
		if err := result.record(out, target.Repo.Name, err); err != nil {
			return result, err
		}
		if err != nil {
			continue
		}
		if fixed {
			result.Done = append(result.Done, target.Repo.Name)
			continue
		}
		result.Skipped = append(result.Skipped, target.Repo.Name)
	}
	return result.finish(out)
}

// fixOne changes origin only when it differs from the expected URL by
// protocol alone; every other difference is reported and left as it is.
func fixOne(ctx context.Context, out Output, git *gitx.Git, root string, t clone.Target) (bool, error) {
	status, err := checkout.Inspect(ctx, git, root, t.Repo.Path, t.URL)
	if err != nil {
		return false, err
	}
	switch status.State {
	case checkout.OK:
		fmt.Fprintf(out.Out, "unchanged %s: origin is already %s\n", t.Repo.Name, t.URL)
		return false, nil
	case checkout.Missing:
		fmt.Fprintf(out.Out, "unchanged %s: not cloned at %s\n", t.Repo.Name, t.Repo.Path)
		return false, nil
	case checkout.PathConflict:
		return false, apperr.ForRepo(apperr.PathConflict, t.Repo.Name, "%s", status.Detail)
	}
	if !status.HasOrigin || !giturl.OnlyProtocolDiffers(status.Origin, t.URL) {
		message := clone.MismatchMessage(t, status) + "; it differs by more than the protocol, so fix it yourself"
		return false, apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "%s", message)
	}
	target := filepath.Join(root, filepath.FromSlash(t.Repo.Path))
	if _, err := git.Run(ctx, "", "-C", target, "remote", "set-url", "origin", t.URL); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, apperr.ForRepo(apperr.RemoteMismatch, t.Repo.Name, "git remote set-url failed: %v", err)
	}
	fmt.Fprintf(out.Out, "fixed %s: origin %s -> %s\n", t.Repo.Name, status.Origin, t.URL)
	return true, nil
}

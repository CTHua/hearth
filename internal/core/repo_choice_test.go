package core_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/version"
)

const thirdSSH = "git@git.example.invalid:acme/third.git"

// threeRepos is a service whose team file lists first, second and third,
// and whose personal file sets only the protocol.
func threeRepos(t *testing.T) *core.Service {
	t.Helper()
	s := newService(t)
	for _, url := range []string{firstSSH, secondSSH, thirdSSH} {
		addRepo(t, s, url)
	}
	if err := s.SetProtocol("ssh"); err != nil {
		t.Fatal(err)
	}
	return s
}

func personalText(t *testing.T, s *core.Service) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.Home().Root, brand.LocalFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// savedRepos reloads the home and returns the saved choice.
func savedRepos(t *testing.T, s *core.Service) []string {
	t.Helper()
	home, err := config.Load(s.Home().Root, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	return home.Personal.Repos
}

func TestDisableReposStartsFromEveryRepoAndKeepsTheKey(t *testing.T) {
	s := threeRepos(t)
	removed, err := s.DisableRepos([]string{"second"})
	if err != nil || !slices.Equal(removed, []string{"second"}) {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if got := savedRepos(t, s); !slices.Equal(got, []string{"first", "third"}) {
		t.Errorf("saved %v", got)
	}
	if _, err := s.DisableRepos([]string{"first", "third"}); err != nil {
		t.Fatal(err)
	}
	got := savedRepos(t, s)
	if got == nil || len(got) != 0 || !strings.Contains(personalText(t, s), "repos = []\n") {
		t.Errorf("saved %#v:\n%s", got, personalText(t, s))
	}
	added, err := s.EnableRepos([]string{"third"})
	if err != nil || !slices.Equal(added, []string{"third"}) {
		t.Fatalf("added %v, err %v", added, err)
	}
	if got := savedRepos(t, s); !slices.Equal(got, []string{"third"}) {
		t.Errorf("saved %v", got)
	}
}

func TestEnableReposWithoutKeyChangesNothing(t *testing.T) {
	s := threeRepos(t)
	before := personalText(t, s)
	added, err := s.EnableRepos([]string{"first"})
	if err != nil || len(added) != 0 {
		t.Fatalf("added %v, err %v", added, err)
	}
	if personalText(t, s) != before {
		t.Errorf("personal file changed:\n%s", personalText(t, s))
	}
}

func TestRepoChoiceKeepsStaleNames(t *testing.T) {
	s := threeRepos(t)
	path := filepath.Join(s.Home().Root, brand.LocalFile)
	if err := os.WriteFile(path, []byte("schema_version = 3\nrepos = [\"gone\", \"first\", \"second\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := config.Load(s.Home().Root, version.Current())
	if err != nil {
		t.Fatal(err)
	}
	s = core.New(home)
	if _, err := s.DisableRepos([]string{"second"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnableRepos([]string{"third"}); err != nil {
		t.Fatal(err)
	}
	if got := savedRepos(t, s); !slices.Equal(got, []string{"gone", "first", "third"}) {
		t.Errorf("saved %v", got)
	}
}

func TestRepoChoiceRefusesUnknownNames(t *testing.T) {
	s := threeRepos(t)
	before := personalText(t, s)
	for name, action := range map[string]func([]string) ([]string, error){"enable": s.EnableRepos, "disable": s.DisableRepos} {
		_, err := action([]string{"first", "nope"})
		if err == nil || !strings.Contains(err.Error(), "error [usage]") || !strings.Contains(err.Error(), "nope") {
			t.Errorf("%s: err %v", name, err)
		}
	}
	if personalText(t, s) != before {
		t.Errorf("personal file changed:\n%s", personalText(t, s))
	}
}

func TestListReposMarksUnchosenMissingRepos(t *testing.T) {
	s := threeRepos(t)
	if _, err := s.DisableRepos([]string{"second"}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListRepos(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, row := range rows {
		labels = append(labels, row.Repo.Name+"="+row.StatusLabel())
	}
	if want := "first=missing second=" + core.NotSelectedLabel + " third=missing"; strings.Join(labels, " ") != want {
		t.Errorf("rows %v, want %s", labels, want)
	}
}

// cancelAfter is a writer that cancels its context once the text written to
// it contains marker.
type cancelAfter struct {
	text   bytes.Buffer
	marker string
	cancel context.CancelFunc
}

func (w *cancelAfter) Write(p []byte) (int, error) {
	n, err := w.text.Write(p)
	if strings.Contains(w.text.String(), w.marker) {
		w.cancel()
	}
	return n, err
}

func TestSyncInterruptedDuringCloneStopsTheRest(t *testing.T) {
	s := newService(t)
	for _, url := range []string{firstHTTPS, secondHTTPS} {
		addRepo(t, s, url)
		gittest.Redirect(t, url, gittest.BareRemote(t, mainBranch))
	}
	if err := s.SetProtocol("https"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &cancelAfter{marker: "cloned first ", cancel: cancel}
	var errw bytes.Buffer
	_, err := s.Sync(ctx, false, core.Output{Out: out, Err: &errw})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
	if want := "cloned first -> products/first (main)\n"; out.text.String() != want {
		t.Errorf("stdout:\n%s\nwant only the first clone:\n%s", out.text.String(), want)
	}
	// Sync returns the interrupt for its caller to report, so the clone
	// step's error is not reported a second time on the way out.
	var interrupted bytes.Buffer
	apperr.Report(&interrupted, context.Canceled)
	if strings.Contains(errw.String(), interrupted.String()) {
		t.Errorf("interrupt reported by Sync:\n%s", errw.String())
	}
	for _, path := range []string{".mcp.json", brand.StateDir} {
		if _, err := os.Lstat(filepath.Join(s.Home().Root, path)); err == nil {
			t.Errorf("%s written after the interrupt", path)
		}
	}
}

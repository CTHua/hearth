package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/cli"
	"github.com/CTHua/hearth/internal/version"
)

func TestNoCommandOutsideTerminalPrintsHelp(t *testing.T) {
	home := newHome(t)
	for _, r := range []result{run(t, "-C", home), runInTerminal(t, "-C", t.TempDir())} {
		if r.code != exitOK || !strings.HasPrefix(r.stdout, "usage: "+brand.Name) || r.stderr != "" {
			t.Errorf("%+v", r)
		}
	}
	if r := run(t, "-C", home); strings.Contains(r.stdout, brand.Name+" init <dir>` first") {
		t.Errorf("hint printed inside a home repo:\n%s", r.stdout)
	}
	outside := runInTerminal(t, "-C", t.TempDir())
	if !strings.Contains(outside.stdout, "run `"+brand.Name+" init <dir>` first") {
		t.Errorf("no init hint outside a home repo:\n%s", outside.stdout)
	}
	if r := run(t, "-C", t.TempDir()); !strings.Contains(r.stdout, "init <dir>` first") {
		t.Errorf("no init hint outside a home repo without a terminal:\n%s", r.stdout)
	}
}

func TestNoCommandWithMissingDirectoryIsUsageError(t *testing.T) {
	run(t, "-C", filepath.Join(t.TempDir(), "missing")).expect(t, exitBad, "error [usage]")
}

func TestNoCommandInTerminalOpensUIUnderC(t *testing.T) {
	home := newHome(t)
	nested := filepath.Join(home, "products", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := cli.StubUI(t)
	runInTerminal(t, "-C", nested).expect(t, exitOK, "")
	runInTerminal(t, "-C", home, "ui").expect(t, exitOK, "")
	if want := []string{home, home}; !slices.Equal(*roots, want) {
		t.Errorf("UI opened on %v, want %v", *roots, want)
	}
}

func TestUIOutsideTerminalIsUsageError(t *testing.T) {
	run(t, "-C", newHome(t), "ui").expect(t, exitBad, "error [usage]")
	runInTerminal(t, "-C", newHome(t), "ui", "extra").expect(t, exitBad, "error [usage]")
}

func TestUIVersionGate(t *testing.T) {
	home := newHome(t)
	text := strings.Replace(read(t, teamFile(home)), "schema_version = 2", "schema_version = 3", 1)
	write(t, teamFile(home), text)
	roots := cli.StubUI(t)
	runInTerminal(t, "-C", home).expect(t, exitBad, "error [version]")
	runInTerminal(t, "-C", home, "ui").expect(t, exitBad, "error [version]")
	if len(*roots) > 0 {
		t.Errorf("UI opened past the version gate: %v", *roots)
	}
}

func TestUIMinVersionGate(t *testing.T) {
	home := newHome(t)
	saved := version.Version
	version.Version = "0.0.9"
	t.Cleanup(func() { version.Version = saved })
	roots := cli.StubUI(t)
	runInTerminal(t, "-C", home).expect(t, exitBad, "error [version]")
	runInTerminal(t, "-C", home, "ui").expect(t, exitBad, "error [version]")
	if len(*roots) > 0 {
		t.Errorf("UI opened past the version gate: %v", *roots)
	}
}

func TestNoCommandReportsAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0600 directory, so the search cannot fail")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Error(err)
		}
	})
	run(t, "-C", locked).expect(t, exitBad, "cannot check")
}

func TestHelpDescriptionsLineUp(t *testing.T) {
	r := run(t, "help")
	_, commands, _ := strings.Cut(r.stdout, "commands:\n")
	commands, _, _ = strings.Cut(commands, "\n\n")
	columns := map[int][]string{}
	for _, line := range strings.Split(commands, "\n") {
		rest := strings.TrimPrefix(line, "  ")
		gap := strings.Index(rest, "  ")
		if strings.HasPrefix(rest, " ") {
			gap = 0
		}
		if gap < 0 {
			continue
		}
		column := len(line) - len(strings.TrimLeft(line[2+gap:], " "))
		columns[column] = append(columns[column], line)
	}
	if len(columns) != 1 {
		t.Errorf("descriptions start at %d columns: %v", len(columns), columns)
	}
}

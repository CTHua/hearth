package checkout_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

const expectedURL = "https://git.example.invalid/acme/app.git"

func setup(t *testing.T) (*gitx.Git, string) {
	t.Helper()
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	gittest.Git(t, root, "init", "--quiet")
	return g, root
}

func inspect(t *testing.T, g *gitx.Git, root string, path string) checkout.Status {
	t.Helper()
	status, err := checkout.Inspect(context.Background(), g, root, path, expectedURL)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func cloneTo(t *testing.T, root string, path string, origin string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, "", "clone", "--quiet", gittest.BareRemote(t, "main"), target)
	gittest.Git(t, target, "remote", "set-url", "origin", origin)
}

func TestStates(t *testing.T) {
	g, root := setup(t)
	if got := inspect(t, g, root, "products/app").State; got != checkout.Missing {
		t.Errorf("no checkout: %s, want missing", got)
	}
	cloneTo(t, root, "products/app", expectedURL)
	status := inspect(t, g, root, "products/app")
	if status.State != checkout.OK || status.Origin != expectedURL {
		t.Errorf("matching origin: %+v, want ok", status)
	}
	cloneTo(t, root, "products/ssh", "git@git.example.invalid:acme/app.git")
	if got := inspect(t, g, root, "products/ssh"); got.State != checkout.RemoteMismatch || got.Origin == "" {
		t.Errorf("other protocol: %+v, want remote-mismatch with origin", got)
	}
	cloneTo(t, root, "products/noorigin", expectedURL)
	gittest.Git(t, filepath.Join(root, "products", "noorigin"), "remote", "remove", "origin")
	if got := inspect(t, g, root, "products/noorigin"); got.State != checkout.RemoteMismatch || got.HasOrigin {
		t.Errorf("no origin: %+v, want remote-mismatch without origin", got)
	}
}

func TestPathConflicts(t *testing.T) {
	g, root := setup(t)
	products := filepath.Join(root, "products")
	if err := os.MkdirAll(filepath.Join(products, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(products, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	gittest.Git(t, outside, "init", "--quiet")
	if err := os.Symlink(outside, filepath.Join(products, "link")); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"products/plain",       // a directory inside the home repo, not its own checkout
		"products/file",        // a regular file
		"products/file/nested", // a file as a parent
		"products/link",        // a symlink to a checkout elsewhere
		"products/link/sub",    // a symlink in the middle of the path
	}
	for _, path := range cases {
		if got := inspect(t, g, root, path); got.State != checkout.PathConflict || got.Detail == "" {
			t.Errorf("%s: %+v, want path-conflict with detail", path, got)
		}
	}
}

func TestSymlinkedProductsDirIsRejected(t *testing.T) {
	g, root := setup(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "products")); err != nil {
		t.Fatal(err)
	}
	if got := inspect(t, g, root, "products/app"); got.State != checkout.PathConflict {
		t.Errorf("products symlink: %+v, want path-conflict", got)
	}
	broken := t.TempDir()
	if err := os.Symlink(filepath.Join(broken, "gone"), filepath.Join(broken, "products")); err != nil {
		t.Fatal(err)
	}
	if got := inspect(t, g, broken, "products/app"); got.State != checkout.PathConflict {
		t.Errorf("broken products symlink: %+v, want path-conflict", got)
	}
}

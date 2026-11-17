// Package checkout inspects the product checkout a repo entry points to,
// without changing it.
package checkout

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CTHua/hearth/internal/gitx"
)

// State is the condition of one repo's checkout.
type State string

const (
	Missing        State = "missing"
	OK             State = "ok"
	RemoteMismatch State = "remote-mismatch"
	PathConflict   State = "path-conflict"
)

const pathSeparator = "/"

// Status is the result of Inspect. Detail explains every state but OK and
// Missing. Origin is set when the checkout has an origin remote.
type Status struct {
	State     State
	Detail    string
	Origin    string
	HasOrigin bool
}

// Inspect reports the state of the checkout at relPath under root, comparing
// its origin with expectedURL exactly. It only reads: it never fetches or
// writes. The error is set only when the run was interrupted or git could
// not be started.
func Inspect(ctx context.Context, g *gitx.Git, root string, relPath string, expectedURL string) (Status, error) {
	if detail := PathProblem(root, relPath); detail != "" {
		return Status{State: PathConflict, Detail: detail}, nil
	}
	target := filepath.Join(root, filepath.FromSlash(relPath))
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return Status{State: Missing}, nil
	}
	if err != nil {
		return Status{State: PathConflict, Detail: fmt.Sprintf("cannot inspect %s: %v", relPath, err)}, nil
	}
	if !info.IsDir() {
		return Status{State: PathConflict, Detail: fmt.Sprintf("%s is not a directory", relPath)}, nil
	}
	detail, err := RootProblem(ctx, g, target, relPath)
	if err != nil {
		return Status{}, err
	}
	if detail != "" {
		return Status{State: PathConflict, Detail: detail}, nil
	}
	return checkOrigin(ctx, g, target, relPath, expectedURL)
}

// PathProblem rejects a symlink anywhere on relPath below root, and a parent
// that exists but is not a directory. relPath uses "/" separators. It is
// empty when nothing is wrong, including when part of the path is missing.
func PathProblem(root string, relPath string) string {
	segments := strings.Split(relPath, pathSeparator)
	current := root
	for depth, segment := range segments {
		current = filepath.Join(current, segment)
		shown := strings.Join(segments[:depth+1], pathSeparator)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return ""
		}
		if err != nil {
			return fmt.Sprintf("cannot inspect %s: %v", shown, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Sprintf("%s is a symlink", shown)
		}
		isParent := depth < len(segments)-1
		if isParent && !info.IsDir() {
			return fmt.Sprintf("%s is not a directory", shown)
		}
	}
	return ""
}

// RootProblem explains why the existing directory target is not the root of
// its own git checkout; it is empty when it is. relPath names target in the
// message. The error is set only when the run was interrupted or git could
// not be started.
func RootProblem(ctx context.Context, g *gitx.Git, target string, relPath string) (string, error) {
	out, err := g.Run(ctx, "", "-C", target, "rev-parse", "--show-toplevel")
	var runErr *gitx.RunError
	if errors.As(err, &runErr) {
		// git's own reason matters: "not a git repository" and "dubious
		// ownership" need different fixes.
		return fmt.Sprintf("%s is not a git checkout (git: %s)", relPath, gitx.LastLine(runErr.Stderr)), nil
	}
	if err != nil {
		return "", err
	}
	toplevel := strings.TrimRight(out, "\n")
	same, err := sameFile(toplevel, target)
	if err != nil {
		return fmt.Sprintf("cannot compare %s with %s: %v", relPath, toplevel, err), nil
	}
	if !same {
		return fmt.Sprintf("%s is not a checkout root; git resolves it to %s", relPath, toplevel), nil
	}
	return "", nil
}

func sameFile(a string, b string) (bool, error) {
	infoA, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(infoA, infoB), nil
}

func checkOrigin(ctx context.Context, g *gitx.Git, target string, relPath string, expectedURL string) (Status, error) {
	out, err := g.Run(ctx, "", "-C", target, "config", "--local", "--get", "remote.origin.url")
	var runErr *gitx.RunError
	if errors.As(err, &runErr) {
		detail := fmt.Sprintf("%s has no origin, expected %q", relPath, expectedURL)
		return Status{State: RemoteMismatch, Detail: detail}, nil
	}
	if err != nil {
		return Status{}, err
	}
	origin := strings.TrimRight(out, "\n")
	status := Status{State: OK, Origin: origin, HasOrigin: true}
	if origin != expectedURL {
		status.State = RemoteMismatch
		status.Detail = fmt.Sprintf("origin of %s is %q, expected %q", relPath, origin, expectedURL)
	}
	return status, nil
}

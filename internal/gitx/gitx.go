// Package gitx runs git with an environment that cannot be redirected to
// another repository by inherited variables such as GIT_DIR.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/CTHua/hearth/internal/apperr"
)

const (
	binary = "git"
	// NoExtTransport blocks the ext:: transport, which runs arbitrary commands.
	NoExtTransport = "protocol.ext.allow=never"
	symrefPrefix   = "ref: refs/heads/"
	headRef        = "HEAD"
	waitDelay      = 2 * time.Second
)

// Git runs git commands with inherited repository-local variables removed.
type Git struct {
	env []string
}

// RunError is a git command that exited with a non-zero status.
type RunError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *RunError) Error() string {
	return fmt.Sprintf("git %s exited with %d: %s", strings.Join(e.Args, " "), e.ExitCode, LastLine(e.Stderr))
}

// New finds git and builds its environment. It fails with an env error when
// git is not on PATH.
func New(ctx context.Context) (*Git, error) {
	if _, err := exec.LookPath(binary); err != nil {
		return nil, apperr.New(apperr.Env, "git not found in PATH")
	}
	out, err := (&Git{env: os.Environ()}).Run(ctx, "", "rev-parse", "--local-env-vars")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, apperr.New(apperr.Env, "git rev-parse --local-env-vars failed: %v", err)
	}
	return &Git{env: withoutVars(os.Environ(), strings.Fields(out))}, nil
}

func withoutVars(environ []string, names []string) []string {
	drop := make(map[string]bool, len(names))
	for _, name := range names {
		drop[name] = true
	}
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !drop[name] {
			kept = append(kept, entry)
		}
	}
	return kept
}

// Cmd prepares git with the cleaned environment and no stdin. dir may be
// empty for the current directory. Callers may add to Env or set Stdout and
// Stderr before running it.
func (g *Git) Cmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = append([]string(nil), g.env...)
	// Without it, a child git started (such as ssh) that outlives a killed
	// git keeps the output pipes open and Wait blocks until that child ends.
	cmd.WaitDelay = waitDelay
	return cmd
}

// Run runs git and returns its stdout. A non-zero exit is a *RunError; an
// interrupted run returns the context's error.
func (g *Git) Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := g.Cmd(ctx, dir, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", &RunError{Args: args, ExitCode: exitErr.ExitCode(), Stderr: stderr.String()}
	}
	if err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// DefaultBranch asks the remote at url which branch its HEAD points to.
// ok is false when the remote answered without a branch symref.
func (g *Git) DefaultBranch(ctx context.Context, url string) (string, bool, error) {
	out, err := g.Run(ctx, "", "-c", NoExtTransport, "ls-remote", "--symref", url, headRef)
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(out, "\n") {
		ref, target, found := strings.Cut(line, "\t")
		if found && target == headRef && strings.HasPrefix(ref, symrefPrefix) {
			return strings.TrimPrefix(ref, symrefPrefix), true, nil
		}
	}
	return "", false, nil
}

// LastLine is the last non-empty line of git's output, used to show git's own
// reason in a one-line error.
func LastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

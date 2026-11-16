// Package apperr defines the error categories, the one-line error format and
// the process exit codes shared by every command.
package apperr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Category is the bracketed word in "error [<category>] ...".
type Category string

const (
	Config         Category = "config"
	Usage          Category = "usage"
	Env            Category = "env"
	PathConflict   Category = "path-conflict"
	RemoteMismatch Category = "remote-mismatch"
	CloneFailed    Category = "clone-failed"
	Version        Category = "version"
)

// Exit codes, the same as the Python bootstrap script.
const (
	ExitOK          = 0
	ExitFailed      = 1
	ExitUsage       = 2
	ExitInterrupted = 130
)

const interruptedMessage = "interrupted"

// Error is one reportable problem. Name is set when it concerns a single repo.
type Error struct {
	Category Category
	Name     string
	Message  string
}

// New returns an error that is not tied to a single repo.
func New(category Category, format string, args ...any) *Error {
	return &Error{Category: category, Message: fmt.Sprintf(format, args...)}
}

// ForRepo returns an error about the repo called name.
func ForRepo(category Category, name string, format string, args ...any) *Error {
	return &Error{Category: category, Name: name, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("error [%s] %s", e.Category, e.Message)
	}
	return fmt.Sprintf("error [%s] %s: %s", e.Category, e.Name, e.Message)
}

// List is several problems found together, such as every mistake in a config
// file; each is reported on its own line.
type List []*Error

func (l List) Error() string {
	lines := make([]string, 0, len(l))
	for _, e := range l {
		lines = append(lines, e.Error())
	}
	return strings.Join(lines, "\n")
}

// exitError carries an exit code for a failure that has already been printed,
// such as a batch where some repos failed.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}

// Exit returns an error that Report turns into code without printing anything.
func Exit(code int) error {
	return &exitError{code: code}
}

// Print writes e as one line, for failures that do not stop a batch.
func Print(w io.Writer, e *Error) {
	fmt.Fprintln(w, e.Error())
}

// Report prints err to w in the shared format and returns the exit code.
// Errors that are not *Error or List are unexpected, so they are reported as
// environment errors rather than dropped.
func Report(w io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, interruptedMessage)
		return ExitInterrupted
	}
	var exit *exitError
	if errors.As(err, &exit) {
		return exit.code
	}
	var list List
	if errors.As(err, &list) {
		fmt.Fprintln(w, list.Error())
		return ExitUsage
	}
	var single *Error
	if errors.As(err, &single) {
		Print(w, single)
		return ExitUsage
	}
	Print(w, New(Env, "%v", err))
	return ExitUsage
}

// Package dirswap replaces a non-empty directory with another one so that a
// failure part way leaves the original in place.
package dirswap

import (
	"errors"
	"fmt"
	"os"
)

// StrandedError means the original could not be moved back: its only copy
// is at Stash, which the caller must not delete.
type StrandedError struct {
	Stash string
	Err   error
}

func (e *StrandedError) Error() string {
	return fmt.Sprintf("%v; the original is left at %s", e.Err, e.Stash)
}

func (e *StrandedError) Unwrap() error {
	return e.Err
}

// Replace moves target to stash, then next to target. When the second move
// fails, target is moved back from stash; when that fails too, the error is
// a *StrandedError. stash must not exist and should be on the same file
// system, for example inside a temporary directory next to target; the
// caller deletes it afterwards, which is when the old content is gone.
func Replace(target string, next string, stash string) error {
	return replace(os.Rename, target, next, stash)
}

// replace takes the rename function so tests can make each move fail.
func replace(rename func(string, string) error, target string, next string, stash string) error {
	if err := rename(target, stash); err != nil {
		return fmt.Errorf("cannot move %s aside: %w", target, err)
	}
	moveErr := rename(next, target)
	if moveErr == nil {
		return nil
	}
	if restoreErr := rename(stash, target); restoreErr != nil {
		err := fmt.Errorf("cannot move %s into place: %w", next, errors.Join(moveErr, restoreErr))
		return &StrandedError{Stash: stash, Err: err}
	}
	return fmt.Errorf("cannot move %s into place: %w; the original was restored", next, moveErr)
}

package cli

import (
	"context"
	"testing"

	"github.com/CTHua/hearth/internal/core"
)

// StubUI replaces the terminal UI until the test ends and returns the roots
// of the home repos it was opened on; the stub quits without asking for a
// sync.
func StubUI(t *testing.T) *[]string {
	t.Helper()
	return StubUIQuit(t, false)
}

// StubUIQuit is StubUI whose UI quits reporting syncOnQuit, as tui.Run does
// after q with unsynced choices.
func StubUIQuit(t *testing.T, syncOnQuit bool) *[]string {
	t.Helper()
	saved := openUI
	var roots []string
	openUI = func(_ context.Context, actions core.Actions) (bool, error) {
		roots = append(roots, actions.Home().Root)
		return syncOnQuit, nil
	}
	t.Cleanup(func() { openUI = saved })
	return &roots
}

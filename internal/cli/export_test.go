package cli

import (
	"context"
	"testing"

	"github.com/CTHua/hearth/internal/config"
)

// StubUI replaces the terminal UI until the test ends and returns the roots
// of the home repos it was opened on.
func StubUI(t *testing.T) *[]string {
	t.Helper()
	saved := openUI
	var roots []string
	openUI = func(_ context.Context, home *config.Home) error {
		roots = append(roots, home.Root)
		return nil
	}
	t.Cleanup(func() { openUI = saved })
	return &roots
}

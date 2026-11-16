package apperr_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/CTHua/hearth/internal/apperr"
)

func TestErrorFormats(t *testing.T) {
	plain := apperr.New(apperr.Config, "bad %s", "value")
	if got, want := plain.Error(), "error [config] bad value"; got != want {
		t.Errorf("New: %q, want %q", got, want)
	}
	repo := apperr.ForRepo(apperr.RemoteMismatch, "web-app", "origin is %q", "x")
	if got, want := repo.Error(), `error [remote-mismatch] web-app: origin is "x"`; got != want {
		t.Errorf("ForRepo: %q, want %q", got, want)
	}
}

func TestReportExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   int
		output string
	}{
		{"nil", nil, apperr.ExitOK, ""},
		{"single", apperr.New(apperr.Usage, "no"), apperr.ExitUsage, "error [usage] no\n"},
		{"wrapped", fmt.Errorf("ctx: %w", apperr.New(apperr.Env, "git")), apperr.ExitUsage, "error [env] git\n"},
		{"list", apperr.List{apperr.New(apperr.Config, "a"), apperr.New(apperr.Config, "b")}, apperr.ExitUsage, "error [config] a\nerror [config] b\n"},
		{"batch", apperr.Exit(apperr.ExitFailed), apperr.ExitFailed, ""},
		{"interrupted", context.Canceled, apperr.ExitInterrupted, "interrupted\n"},
		{"unexpected", errors.New("disk full"), apperr.ExitUsage, "error [env] disk full\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if code := apperr.Report(&buf, c.err); code != c.code {
			t.Errorf("%s: code %d, want %d", c.name, code, c.code)
		}
		if buf.String() != c.output {
			t.Errorf("%s: output %q, want %q", c.name, buf.String(), c.output)
		}
	}
}

func TestExitCodesMatchPythonVersion(t *testing.T) {
	codes := []int{apperr.ExitOK, apperr.ExitFailed, apperr.ExitUsage, apperr.ExitInterrupted}
	want := []int{0, 1, 2, 130}
	for i := range codes {
		if codes[i] != want[i] {
			t.Errorf("exit code %d = %d, want %d", i, codes[i], want[i])
		}
	}
}

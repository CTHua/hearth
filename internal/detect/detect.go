// Package detect chooses ssh or https by trying the remotes for real.
package detect

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/gitx"
)

// Timeout bounds each ls-remote attempt; tests shorten it.
var Timeout = 10 * time.Second

const (
	envSSHCommand    = "GIT_SSH_COMMAND"
	envNoPrompt      = "GIT_TERMINAL_PROMPT=0"
	batchSSHCommand  = envSSHCommand + "=ssh -o BatchMode=yes"
	sshCommandConfig = "core.sshCommand"
	headRef          = "HEAD"
)

// order is the order forms are tried in; ssh wins when both work.
var order = []string{giturl.SSH, giturl.HTTPS}

// Protocol tries each URL in order, ssh form first, and returns the protocol
// of the first URL that answers in either form. urls must already be valid.
// When none answers it is an env error and nothing has been written.
func Protocol(ctx context.Context, g *gitx.Git, urls []string) (string, error) {
	env, err := attemptEnv(ctx, g)
	if err != nil {
		return "", err
	}
	var tried []string
	for _, raw := range urls {
		u, err := giturl.Parse(raw)
		if err != nil {
			return "", apperr.New(apperr.Config, "url %q: %v", raw, err)
		}
		for _, protocol := range order {
			url := u.Format(protocol)
			if reachable(ctx, g, env, url) {
				return protocol, nil
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			tried = append(tried, url)
		}
	}
	return "", apperr.New(apperr.Env,
		"could not reach any repo over ssh or https (tried %s); check your access, or set it with `%s config protocol <ssh|https>`",
		strings.Join(tried, ", "), brand.Name)
}

// attemptEnv never lets git prompt, and makes ssh fail instead of asking,
// unless the user chose their own ssh command.
func attemptEnv(ctx context.Context, g *gitx.Git) ([]string, error) {
	env := []string{envNoPrompt}
	if _, set := os.LookupEnv(envSSHCommand); set {
		return env, nil
	}
	_, err := g.Run(ctx, "", "config", "--get", sshCommandConfig)
	if err == nil {
		return env, nil
	}
	var runErr *gitx.RunError
	if !errors.As(err, &runErr) {
		return nil, err
	}
	return append(env, batchSSHCommand), nil
}

func reachable(ctx context.Context, g *gitx.Git, env []string, url string) bool {
	attemptCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := g.Cmd(attemptCtx, "", "-c", gitx.NoExtTransport, "ls-remote", url, headRef)
	cmd.Env = append(cmd.Env, env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	// Its own process group, so a timeout also kills the ssh git started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd.Run() == nil
}

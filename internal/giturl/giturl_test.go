package giturl_test

import (
	"testing"

	"github.com/CTHua/hearth/internal/giturl"
)

func TestParseAcceptsThreeFormsOnAnyHost(t *testing.T) {
	cases := map[string]giturl.URL{
		"git@git.example.invalid:acme/web-app.git":          {Host: "git.example.invalid", Path: "acme/web-app.git"},
		"ssh://git@git.example.invalid/acme/web-app.git":    {Host: "git.example.invalid", Path: "acme/web-app.git"},
		"https://git.example.invalid/acme/web-app":          {Host: "git.example.invalid", Path: "acme/web-app"},
		"https://gitlab.example.invalid/group/sub/repo.git": {Host: "gitlab.example.invalid", Path: "group/sub/repo.git"},
		"git@localhost:a/b":                                 {Host: "localhost", Path: "a/b"},
	}
	for raw, want := range cases {
		got, err := giturl.Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
}

func TestParseRejectsUnconvertibleURLs(t *testing.T) {
	rejected := []string{
		"",
		"/srv/git/app.git",
		"../app.git",
		"file:///srv/git/app.git",
		"http://git.example.invalid/acme/app.git",
		"https://git.example.invalid:8443/acme/app.git",
		"ssh://git@git.example.invalid:2222/acme/app.git",
		"https://user@git.example.invalid/acme/app.git",
		"ssh://deploy@git.example.invalid/acme/app.git",
		"deploy@git.example.invalid:acme/app.git",
		"git@[::1]:acme/app.git",
		"git@-bad.example.invalid:acme/app.git",
		"git@bad-.example.invalid:acme/app.git",
		"git@git..example.invalid:acme/app.git",
		"git@git.example.invalid:/acme/app.git",
		"git@git.example.invalid:app.git",
		"https://git.example.invalid/app.git",
		"https://git.example.invalid/acme/app.git/",
		"https://git.example.invalid/acme//app.git",
		"https://git.example.invalid/acme/../app.git",
		"https://git.example.invalid/acme/./app.git",
		"https://git.example.invalid/acme/app.git?x=1",
		"https://git.example.invalid/acme/app.git#frag",
		"https://git.example.invalid/acme/my app.git",
		"ext::sh -c touch% /tmp/pwned",
		"https://git.example.invalid/acme/missing;touch",
		"https://git.example.invalid/acme/$(touch)",
		"git@git.example.invalid:acme/a`id`",
		"-uhttps://git.example.invalid/acme/app.git",
	}
	for _, raw := range rejected {
		if u, err := giturl.Parse(raw); err == nil {
			t.Errorf("Parse(%q) = %+v, want error", raw, u)
		}
	}
}

func TestExpectedConvertsAndKeepsHostAndPath(t *testing.T) {
	cases := []struct {
		raw, protocol, want string
	}{
		{"git@git.example.invalid:acme/app.git", giturl.HTTPS, "https://git.example.invalid/acme/app.git"},
		{"git@git.example.invalid:acme/app.git", giturl.SSH, "git@git.example.invalid:acme/app.git"},
		{"ssh://git@git.example.invalid/acme/app.git", giturl.SSH, "git@git.example.invalid:acme/app.git"},
		{"ssh://git@git.example.invalid/acme/app.git", giturl.HTTPS, "https://git.example.invalid/acme/app.git"},
		{"https://git.example.invalid/acme/app", giturl.SSH, "git@git.example.invalid:acme/app"},
		{"https://git.example.invalid/acme/app", giturl.HTTPS, "https://git.example.invalid/acme/app"},
		{"ssh://git@git.example.invalid/acme/app.git", "", "ssh://git@git.example.invalid/acme/app.git"},
	}
	for _, c := range cases {
		got, err := giturl.Expected(c.raw, c.protocol)
		if err != nil || got != c.want {
			t.Errorf("Expected(%q, %q) = %q, %v; want %q", c.raw, c.protocol, got, err, c.want)
		}
	}
}

func TestOnlyProtocolDiffers(t *testing.T) {
	cases := []struct {
		origin, expected string
		want             bool
	}{
		{"git@git.example.invalid:acme/app.git", "https://git.example.invalid/acme/app.git", true},
		{"ssh://git@git.example.invalid/acme/app.git", "git@git.example.invalid:acme/app.git", true},
		{"https://git.example.invalid/acme/app.git", "https://git.example.invalid/acme/app.git", false},
		{"git@git.example.invalid:acme/app", "https://git.example.invalid/acme/app.git", false},
		{"git@other.example.invalid:acme/app.git", "https://git.example.invalid/acme/app.git", false},
		{"https://git.example.invalid:8443/acme/app.git", "https://git.example.invalid/acme/app.git", false},
		{"/srv/git/app.git", "https://git.example.invalid/acme/app.git", false},
	}
	for _, c := range cases {
		if got := giturl.OnlyProtocolDiffers(c.origin, c.expected); got != c.want {
			t.Errorf("OnlyProtocolDiffers(%q, %q) = %v, want %v", c.origin, c.expected, got, c.want)
		}
	}
}

func TestDefaultNameDropsGitSuffix(t *testing.T) {
	cases := map[string]string{
		"git@git.example.invalid:acme/web-app.git":    "web-app",
		"https://git.example.invalid/acme/api":        "api",
		"https://git.example.invalid/a/b/tool.v2.git": "tool.v2",
	}
	for raw, want := range cases {
		u, err := giturl.Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if got := u.DefaultName(); got != want {
			t.Errorf("DefaultName(%q) = %q, want %q", raw, got, want)
		}
	}
}

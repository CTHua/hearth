package scaffold_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/gittest"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/subagent"
)

func TestMain(m *testing.M) {
	gittest.Main(m)
}

// Strings that would point a new home repo at the Python scripts, which this
// template no longer ships. vendor/skills is allowed since P2: it is where
// `skill sync` installs external skills, so the docs name it.
var forbiddenMentions = []string{
	"scripts/bootstrap", "scripts/setup", "repos.json", "repos.example.json", "skills.json",
	"config.example.toml", "tests/", "python3",
}

// pythonConfigRE is the Python reference version's config.toml as a file
// name of its own. Since P3 the docs name `.codex/config.toml`, which `sync`
// writes, so a config.toml right after "/" is allowed.
var pythonConfigRE = regexp.MustCompile(`(?:^|[^/])config\.toml`)

// init never writes agent auto-load settings or the directories that only
// `sync` creates; since P6 that includes the generated subagent files.
var forbiddenPaths = []string{".mcp.json", ".codex/config.toml", ".claude/skills", ".agents/skills", "vendor/skills", "vendor", brand.StateDir, ".claude/agents", ".codex"}

var (
	machinePathRE = regexp.MustCompile(`(?:/Users|/home)/[A-Za-z0-9]|[A-Za-z]:\\Users\\`)
	githubRepoRE  = regexp.MustCompile(`(?:^|[^\w.-])github\.com[:/]([A-Za-z0-9][\w.-]*/[\w.-]+)`)
	emailRE       = regexp.MustCompile(`([A-Za-z0-9._%+-]+)@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+)`)
	ownRepo       = "CTHua/" + brand.Name
	// An address domain is allowed when it equals allowedEmailDomain or ends
	// with one of allowedEmailSuffixes; a plain suffix test would let
	// a@notexample.com through.
	allowedEmailDomain   = "example.com"
	allowedEmailSuffixes = []string{".example.com", ".invalid"}
)

// sshRemoteUser is the user part of git@host:path remotes, not an address.
const sshRemoteUser = "git"

func initHome(t *testing.T) string {
	t.Helper()
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "home")
	if err := scaffold.Init(context.Background(), g, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// generatedFiles lists every file init wrote, relative to dir, without .git.
func generatedFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(rel)] = readFile(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestInitWritesTemplateTeamFileAndGitRepo(t *testing.T) {
	dir := initHome(t)
	files := generatedFiles(t, dir)
	want := []string{
		"AGENTS.md", "CLAUDE.md", "README.md", ".gitignore", brand.TeamFile,
		".claude/settings.json", "subagents/subagent.md",
		"agents/README.md", "agents/_task-brief.md", "agents/models.json",
		"memory/README.md", "memory/conventions/_template.md", "memory/decisions/_template.md",
		"skills/README.md", "skills/_template/SKILL.md",
		"workflows/README.md", "workflows/code-review.md", "workflows/cross-repo-change.md", "workflows/development.md",
	}
	for _, path := range want {
		if _, ok := files[path]; !ok {
			t.Errorf("missing %s", path)
		}
	}
	if len(files) != len(want) {
		t.Errorf("init wrote %d files, want %d: %v", len(files), len(want), files)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("no git repo: %v", err)
	}
	cmd := gittest.Git(t, dir, "rev-list", "--all", "--count")
	if strings.TrimSpace(cmd) != "0" {
		t.Errorf("init made commits: %s", cmd)
	}
}

func TestInitTeamFileHasNoRepos(t *testing.T) {
	dir := initHome(t)
	want := brand.TeamFileHeader + "\nschema_version = 1\n" + brand.MinVersionKey + " = \"0.1.0\"\n"
	if got := readFile(t, filepath.Join(dir, brand.TeamFile)); got != want {
		t.Errorf("team file:\n%s\nwant:\n%s", got, want)
	}
}

func TestInitRefusesNonEmptyDirectory(t *testing.T) {
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Init(context.Background(), g, dir); err == nil || !strings.Contains(err.Error(), "[path-conflict]") {
		t.Fatalf("err = %v, want path-conflict", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("directory changed: %v, %v", entries, err)
	}
	file := filepath.Join(dir, "keep")
	if err := scaffold.Init(context.Background(), g, file); err == nil {
		t.Error("init into a regular file succeeded")
	}
}

func TestInitIntoExistingEmptyDirectory(t *testing.T) {
	g, err := gitx.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := scaffold.Init(context.Background(), g, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, brand.TeamFile)); err != nil {
		t.Error(err)
	}
}

func markerBlock(t *testing.T, name string, text string, begin string, end string) []string {
	t.Helper()
	lines := strings.Split(text, "\n")
	beginAt, endAt := -1, -1
	for i, line := range lines {
		if strings.Contains(line, begin) || strings.Contains(line, end) {
			if line != begin && line != end {
				t.Errorf("%s: marker not on its own line: %q", name, line)
			}
		}
		if line == begin {
			if beginAt >= 0 {
				t.Errorf("%s: begin marker twice", name)
			}
			beginAt = i
		}
		if line == end {
			if endAt >= 0 {
				t.Errorf("%s: end marker twice", name)
			}
			endAt = i
		}
	}
	if beginAt < 0 || endAt < 0 || endAt < beginAt {
		t.Fatalf("%s: markers missing or out of order (begin %d, end %d)", name, beginAt, endAt)
	}
	return lines[beginAt+1 : endAt]
}

func TestAgentsBlock(t *testing.T) {
	dir := initHome(t)
	block := markerBlock(t, "AGENTS.md", readFile(t, filepath.Join(dir, "AGENTS.md")), brand.AgentsBegin, brand.AgentsEnd)
	if len(block) < 4 || len(block) > 7 {
		t.Errorf("block has %d lines, want about 5:\n%s", len(block), strings.Join(block, "\n"))
	}
	text := strings.Join(block, "\n")
	wants := []string{
		"github.com/" + ownRepo,
		brand.Name + " clone", brand.Name + " repo list", brand.Name + " doctor",
		brand.Name + " repo add", brand.Name + " repo remove", brand.TeamFile,
		brand.Name + " sync", brand.Name + " mcp add", brand.Name + " mcp enable",
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("block lacks %q", want)
		}
	}
}

func TestGitignoreBlockIgnoresProductsAndPersonalFile(t *testing.T) {
	dir := initHome(t)
	block := markerBlock(t, ".gitignore", readFile(t, filepath.Join(dir, ".gitignore")), brand.GitignoreBegin, brand.GitignoreEnd)
	for _, want := range []string{"/products/", "/" + brand.LocalFile} {
		found := false
		for _, line := range block {
			found = found || line == want
		}
		if !found {
			t.Errorf("block lacks %q", want)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "products", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"products/app/README", brand.LocalFile} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(path)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignored := gittest.Git(t, dir, "check-ignore", "products/app/README", brand.LocalFile)
	if strings.Count(ignored, "\n") != 2 {
		t.Errorf("check-ignore: %q", ignored)
	}
	for _, shared := range []string{brand.TeamFile, ".claude/settings.json", "agents/models.json"} {
		cmd := gittest.Git(t, dir, "ls-files", "--others", "--exclude-standard", shared)
		if strings.TrimSpace(cmd) != shared {
			t.Errorf("%s should stay trackable, ls-files says %q", shared, cmd)
		}
	}
}

func TestGitignoreBlockIgnoresSkillOutputs(t *testing.T) {
	dir := initHome(t)
	block := markerBlock(t, ".gitignore", readFile(t, filepath.Join(dir, ".gitignore")), brand.GitignoreBegin, brand.GitignoreEnd)
	for _, want := range []string{"/vendor/", "/" + brand.StateDir + "/", "/.claude/skills/", "/.agents/skills/"} {
		if !strings.Contains(strings.Join(block, "\n")+"\n", want+"\n") {
			t.Errorf("block lacks %q", want)
		}
	}
	paths := []string{"vendor/skills/x/SKILL.md", brand.StateDir + "/skills/x/SKILL.md", ".claude/skills/x", ".agents/skills/x"}
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignored := gittest.Git(t, dir, append([]string{"check-ignore"}, paths...)...)
	if strings.Count(ignored, "\n") != len(paths) {
		t.Errorf("check-ignore: %q", ignored)
	}
	shared := ".claude/settings.json"
	if got := strings.TrimSpace(gittest.Git(t, dir, "ls-files", "--others", "--exclude-standard", shared)); got != shared {
		t.Errorf("%s should stay trackable, ls-files says %q", shared, got)
	}
}

func TestGeneratedRepoDropsPythonAndSkillInstaller(t *testing.T) {
	dir := initHome(t)
	for path, text := range generatedFiles(t, dir) {
		for _, mention := range forbiddenMentions {
			if strings.Contains(text, mention) {
				t.Errorf("%s mentions %q", path, mention)
			}
		}
		if match := pythonConfigRE.FindString(text); match != "" {
			t.Errorf("%s mentions %q", path, match)
		}
	}
	for _, path := range forbiddenPaths {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path))); err == nil {
			t.Errorf("%s exists", path)
		}
	}
}

func TestPythonConfigCheckAllowsOnlyCodexPath(t *testing.T) {
	for _, sample := range []string{"config.toml", "edit `config.toml`", "see ./config.toml.bak and config.toml"} {
		if !pythonConfigRE.MatchString(sample) {
			t.Errorf("missed %q", sample)
		}
	}
	if pythonConfigRE.MatchString("writes `.codex/config.toml` for codex") {
		t.Error("flagged .codex/config.toml")
	}
}

func TestGitignoreBlockIgnoresMCPOutputs(t *testing.T) {
	dir := initHome(t)
	block := markerBlock(t, ".gitignore", readFile(t, filepath.Join(dir, ".gitignore")), brand.GitignoreBegin, brand.GitignoreEnd)
	for _, want := range []string{"/.mcp.json", "/.codex/"} {
		if !strings.Contains(strings.Join(block, "\n")+"\n", want+"\n") {
			t.Errorf("block lacks %q", want)
		}
	}
	paths := []string{".mcp.json", ".codex/config.toml", brand.StateFile}
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignored := gittest.Git(t, dir, append([]string{"check-ignore"}, paths...)...)
	if strings.Count(ignored, "\n") != len(paths) {
		t.Errorf("check-ignore: %q", ignored)
	}
}

func TestTemplateSubagentIsValidAndOutputsAreIgnored(t *testing.T) {
	dir := initHome(t)
	cat, err := subagent.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	items := cat.Items()
	if len(items) != 1 || items[0].Name != "subagent" || items[0].Invalid != "" {
		t.Errorf("template subagents: %+v", items)
	}
	block := markerBlock(t, ".gitignore", readFile(t, filepath.Join(dir, ".gitignore")), brand.GitignoreBegin, brand.GitignoreEnd)
	if !strings.Contains(strings.Join(block, "\n")+"\n", "/.claude/agents/\n") {
		t.Errorf("block lacks /.claude/agents/:\n%s", strings.Join(block, "\n"))
	}
	paths := []string{subagent.ClaudeDir + "/subagent.md", subagent.CodexDir + "/subagent.toml"}
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignored := gittest.Git(t, dir, append([]string{"check-ignore"}, paths...)...)
	if strings.Count(ignored, "\n") != len(paths) {
		t.Errorf("check-ignore: %q", ignored)
	}
	for _, shared := range []string{"subagents/subagent.md", ".claude/settings.json"} {
		if got := strings.TrimSpace(gittest.Git(t, dir, "ls-files", "--others", "--exclude-standard", shared)); got != shared {
			t.Errorf("%s should stay trackable, ls-files says %q", shared, got)
		}
	}
}

func TestTemplateHasNoMachineOrPersonalDetails(t *testing.T) {
	err := fs.WalkDir(scaffold.Files(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(scaffold.Files(), path)
		if err != nil {
			return err
		}
		for _, finding := range audit(string(data)) {
			t.Errorf("%s: %s", path, finding)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// audit returns every machine path, foreign GitHub repo URL and real-looking
// email address in text.
func audit(text string) []string {
	var findings []string
	if match := machinePathRE.FindString(text); match != "" {
		findings = append(findings, "machine path "+match)
	}
	for _, match := range githubRepoRE.FindAllStringSubmatch(text, -1) {
		if !strings.HasPrefix(match[1], ownRepo) {
			findings = append(findings, "GitHub repo URL "+match[0])
		}
	}
	for _, match := range emailRE.FindAllStringSubmatch(text, -1) {
		if match[1] == sshRemoteUser {
			continue
		}
		domain := strings.ToLower(match[2])
		allowed := domain == allowedEmailDomain
		for _, suffix := range allowedEmailSuffixes {
			allowed = allowed || strings.HasSuffix(domain, suffix)
		}
		if !allowed {
			findings = append(findings, "email "+match[0])
		}
	}
	return findings
}

func TestAuditCatchesForbiddenDetails(t *testing.T) {
	samples := []string{
		"see /Users/someone/notes",
		"clone https://github.com/acme-team/home-repo.git",
		"git@github.com:acme-team/app.git",
		"mail platform-team@acme-team.io",
		"mail someone@example.org",
		"mail someone@example.net",
		"mail a@notexample.com",
		"mail a@example.com.evil.io",
	}
	for _, sample := range samples {
		if len(audit(sample)) == 0 {
			t.Errorf("audit missed %q", sample)
		}
	}
	allowed := "go install github.com/" + ownRepo + "/cmd/x@latest; git@git.example.invalid:acme/app.git; a@example.com; b@team.example.com; c@host.invalid; docs.github.com/en"
	if findings := audit(allowed); len(findings) != 0 {
		t.Errorf("audit flagged allowed text: %v", findings)
	}
}

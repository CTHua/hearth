# hearth development rules

For engineers and AI coding agents working in this repo. `CLAUDE.md` loads only this file.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/hearth/main.go` | Entry point: checks whether stdin and stdout are TTYs and hands off to `internal/cli` |
| `internal/cli` | Subcommands and argument parsing (standard library `flag`): parse flags → call `internal/core` → print the result; with no arguments, opens the TUI (then syncs when the TUI asks for it) or prints help |
| `internal/core` | One method per user action (repo, repo choice, protocol, agent, skill, MCP, subagent, sync); takes the loaded home and returns structured results; `Clone` without names takes only the chosen repos; `Sync` first clones the chosen repos that have no checkout, then syncs skills, MCP, and subagents; `RepoRow` carries the choice and shows `not-selected` for a repo neither chosen nor cloned; `TeamChanges` compares `hearth.toml` with HEAD; `core.Actions` is the interface the TUI uses, and tests swap in a fake implementation |
| `internal/tui` | Bubble Tea v2 TUI: four tabs, Mine (personal file, including the repo, skill, MCP, and subagent switches; `s` clones and syncs) and the team group's Repos, Skills, MCP (team file), centered forms, a `?` key table, mouse (clicks are matched against regions recorded at render time) and `y` clipboard; acts only through `core.Actions`; background actions run one at a time, and each can be cancelled with `ctrl+c`; `q` after a saved agent, repo, skill, MCP, or subagent choice that no successful sync followed makes `tui.Run` report a sync, which `internal/cli` then runs once the UI has closed (`ctrl+c` never does) |
| `internal/upgrade` | `hearth upgrade`: rewrites the marked sections and upgrades each config file's `schema_version` along its own migration chain to its own target (team file 2, personal file 3); migrations are text replacements on the file that change only the values of `schema_version` and `min_hearth_version`, leaving every other byte as is; the chains and the target versions are parameters |
| `internal/config` | Reading, writing, validation, version gate, and root search for `hearth.toml` (`[[repos]]`, `[[skills]]` (`subdir` for a single skill, `subdirs` for a group), `[[mcp]]`) and `hearth.local.toml` (`protocol`, `agents`, `repos` (no key means every repo is chosen, `[]` none), `skills`, `mcp`, `subagents`); the team and personal files have separate schema versions (2 and 3); `Load` accepts only the current ones, and `ParseTeam` (reads HEAD) also accepts 1 |
| `internal/mcp` | Generates `.mcp.json` and `.codex/config.toml` (the formats live only here, golden files in `testdata/`), and checks environment variables and PATH |
| `internal/subagent` | Reads `subagents/<name>.md` (YAML frontmatter through `yaml.Node` to keep order), validates it, converts it to `.claude/agents/<name>.md` and `.codex/agents/<name>.toml` (the formats live only here, golden files in `testdata/`; the Codex side is not yet confirmed to take effect), status, sync; owns the state records that start with `.claude/agents/` or `.codex/agents/` |
| `internal/genfile` | Reading and writing `.hearth/state.json`, hashing, and the file and symlink checks used to detect hand edits, shared by `internal/mcp` and `internal/subagent`; each changes only its own records |
| `internal/tomlstr` | Hand-written TOML values and keys (strings, arrays, inline tables, dotted keys), for places where the encoder cannot keep order |
| `internal/giturl` | Parsing of the three URL forms and ssh/https conversion |
| `internal/gitx` | Runs git with the inherited git local env vars removed |
| `internal/checkout` | Read-only status check of one checkout; the symlink check on the path and the checkout root check are also used by `internal/skill` |
| `internal/clone` | Clones one product, or confirms an existing checkout |
| `internal/skill` | The three skill sources, expansion of group members (one checkout per entry, each member checks its own `SKILL.md`), status, detached-commit install and pin changes, per-agent links, and `skill add` listing remote skills (`git clone --no-checkout` into the system temporary directory plus `git ls-tree`, reading only one blob, `.claude-plugin/plugin.json`); the built-in skills' `SKILL.md` files are in `internal/skill/builtin/` (go:embed) |
| `internal/dirswap` | Replaces a non-empty directory, moving the original back if it fails midway |
| `internal/detect` | Detects the protocol with `git ls-remote` |
| `internal/scaffold` | `hearth init`, marked section checks, and the byte-for-byte comparison of marked sections against the template (shared by `upgrade` and `doctor`) |
| `internal/apperr` | Error categories, error format, exit codes |
| `internal/brand` | Every string that contains the package name |
| `template/home/` | The Home Repo template that `hearth init` writes (go:embed) |
| `install.sh` | The `curl \| sh` install script (POSIX sh): picks the platform, downloads the archive and `SHA256SUMS` from the Release, verifies the checksum, and test-runs `version` in the install directory before replacing the file |
| `scripts/` | `build-release.sh` builds the archives for 4 platforms and `SHA256SUMS`; `test-install.sh` tests `install.sh` offline with `file://` (run by both CI and the release workflow) |
| `.github/workflows/release.yml` | Runs the checks and the build; on a tag, makes the build provenance attestation and creates a draft Release; PRs and manual runs only build and save the result as a workflow artifact |

## Rules

- Verification: work is done only when `gofmt -l .` prints nothing and `go vet ./...` and `go test ./...` both pass.
- String constants that contain the package name live only in `internal/brand`. This line must print nothing:

  ```sh
  grep -rn '"[^"]*hearth' --include='*.go' . | grep -v '^./internal/brand/' | grep -v '"github.com/CTHua/hearth/'
  ```

- Tests touch only public interfaces and never the network. Tests that use git call `gittest.Main` in `TestMain`, and use `gittest.BareRemote` and `gittest.Redirect` for remotes.
- The only dependencies are `github.com/BurntSushi/toml`, `go.yaml.in/yaml/v3` for reading subagent frontmatter, the Charm packages for the TUI (`charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`), and `github.com/charmbracelet/x/term` for TTY detection. Discuss before adding or upgrading a dependency.
- Config file formats, URL rules, and marker strings are an external contract; when you change one, bump the `schema_version` of the affected file by one.
- The template (`template/home/`) must not mention Python scripts and must not contain machine paths, real GitHub URLs, or real email addresses; the `internal/scaffold` tests check this.
- The agents' MCP config (`.mcp.json`, `.codex/config.toml`) and `.hearth/state.json` are written to the user's Home Repo only by `hearth sync` / `hearth mcp sync`; `hearth init` and the template do not create them. Files edited by hand are neither overwritten nor deleted unless `--force` is given; nothing is written when a symlink sits in the path. Secrets are written only as environment variable names.
- Subagent output files (`.claude/agents/<name>.md`, `.codex/agents/<name>.toml`) are written from `subagents/` to the user's Home Repo only by `hearth sync` / `hearth subagent sync`; `hearth init` and the template do not create them. The rules are the same as for MCP: files edited by hand are neither overwritten nor deleted unless `--force` is given; nothing is written when a symlink sits in the path; files with no record that are not targets of this run are left alone.
- Skill directories (`vendor/skills/`, `.hearth/`, `.claude/skills/`, `.agents/skills/`) are written to the user's Home Repo only by `hearth sync` / `hearth skill sync`; `hearth init` and the template do not create them, and `sync` deletes only links it created.
- Releases: the archive names `hearth_<os>_<arch>.tar.gz`, the `SHA256SUMS` format (`<64 hex chars><two spaces><file name>`, sorted by file name), and the three `install.sh` environment variables (`HEARTH_VERSION`, `HEARTH_INSTALL_DIR`, `HEARTH_BASE_URL`) are an external contract; changing them after the first Release breaks other people's installs. Release steps: trigger `release` manually in Actions as a dry run → create and push a `v<MAJOR.MINOR.PATCH>` tag → check the draft Release's artifacts → click Publish → confirm the published Release has 5 artifacts, then run `install.sh` once more. A change to `install.sh` on `main` affects everyone who installs from then on.
- Do not commit, push, tag, or publish a Release unless asked.

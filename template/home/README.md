# Home Repo

The shared starting point for work, used by the engineering team and AI coding agents. Team memory, skills, workflows, and the list of product repos are maintained here.

Product code is not in this repo. Each product is its own Git repo, cloned into `products/` by `hearth clone`, with its own version control and its own commits.

The shared rules are in [`AGENTS.md`](AGENTS.md) and apply to both engineers and agents.

## Requirements

- Git.
- `hearth`: `go install github.com/CTHua/hearth/cmd/hearth@latest`; for other ways to install, see https://github.com/CTHua/hearth . Supports macOS and Linux.
- Read access to the repos listed in `hearth.toml`. For SSH, set up your SSH key first.

## Quick start

```sh
git clone <your Home Repo URL> home-repo
cd home-repo
hearth config protocol ssh   # or https; if unset, the first clone connects to detect it
hearth clone                 # clones every chosen product (all of them until you choose)
hearth repo list             # each product's status
hearth doctor                # checks git, the config files, and every checkout
```

`hearth` finds the Home Repo root by looking upward from the current directory for `hearth.toml`, so it also runs inside `products/<name>/`; you can also set it with `-C <dir>`.

## The two config files

| File | Who changes it | Version control |
| --- | --- | --- |
| `hearth.toml` | The team, through `hearth repo add` / `remove`, `hearth skill add` / `remove` / `update`, `hearth mcp add` / `remove`, then a PR | In version control |
| `hearth.local.toml` | Each person, through `hearth config protocol`, `hearth config agents`, `hearth skill enable` / `disable`, `hearth mcp enable` / `disable`, `hearth subagent enable` / `disable`, and the repo switches on the Mine tab of the interactive UI | Ignored by `.gitignore` |

The format of `hearth.toml`:

```toml
# Managed by hearth. Edit with `hearth repo add` / `hearth repo remove`; comments are not kept.
schema_version = 1
min_hearth_version = "0.1.0"

[[repos]]
name = "web-app"
url = "git@git.example.invalid:acme/web-app.git"
path = "products/web-app"
branch = "main"
description = "Customer-facing web app"
```

| Field | Rules |
| --- | --- |
| `name` | The product's identifying name, also the name used by `hearth clone <name>`. Only letters, digits, `.`, `_`, `-`, starting with a letter or digit. Must not repeat, ignoring case |
| `url` | Clone URL; only three forms are accepted: `git@<host>:<path>`, `ssh://git@<host>/<path>`, `https://<host>/<path>` |
| `path` | Path relative to the Home Repo; must be under `products/`, and each segment follows the same rule as `name`. Must not repeat, ignoring case, and must not be a parent or child of another `path` |
| `branch` | The branch checked out on clone. Must be a branch, not a tag |
| `description` | Optional. What this product does; agents use it to decide which product a task belongs to. One line of text, Chinese is fine; no newlines, tabs, or other control characters, no leading or trailing whitespace, at most 200 characters. Missing or an empty string means none |

- The four fields `name`, `url`, `path`, `branch` are required, `description` is optional, and no other fields are allowed. `hearth doctor` reminds you about products without a `description`.
- When `hearth` changes this file it rewrites the whole file, keeping only the comment on the first line; comments you add will disappear, so put explanations in the PR or in this README.
- When `min_hearth_version` is newer than your `hearth`, or `schema_version` is newer than your `hearth` supports, commands report `version`, exit 2, and do nothing. Upgrade `hearth` to fix it.

Which products each person clones is in `hearth.local.toml` (`schema_version = 3`):

```toml
schema_version = 3
protocol = "ssh"
repos = ["web-app"]
```

- Without a `repos` key, every product in `hearth.toml` is chosen. With the key, only the listed ones are, so a product the team adds later is not chosen until you add it; `repos = []` chooses none. A name that is no longer in `hearth.toml` counts as not chosen.
- `hearth clone` with no names clones only the chosen products; `hearth clone <name>` clones that product whether or not it is chosen, and leaves the choice as it is.
- `hearth sync` first clones each chosen product that has no checkout yet, then installs skills and writes the MCP and subagent files; a failed clone does not stop the rest, but the exit code is `1`. Products that are already cloned are left alone.
- `hearth repo list` shows `not-selected` for a product that is neither chosen nor cloned; `hearth doctor` does not count it as a problem. Turning a product off does not delete its checkout.
- In the interactive UI, `q` after changing a choice of agents, repos, skills, MCP servers, or subagents runs `hearth sync` once the UI closes; `ctrl+c` closes it without syncing.

## Adding and removing products

```sh
hearth repo add git@git.example.invalid:acme/web-app.git            # name=web-app, path=products/web-app
hearth repo add https://git.example.invalid/acme/api.git --name api-service --branch develop --description "Orders API"
hearth repo describe web-app "Customer-facing web app"              # adds or changes the description
hearth repo describe web-app ""                                     # clears the description
hearth repo remove web-app                                          # changes only hearth.toml; the checkout stays
```

- Without `--name`, the name is the last segment of the URL with `.git` removed; without `--path`, the path is `products/<name>`; without `--branch`, the remote is asked for its default branch. The text of `--description` and `repo describe` has leading and trailing whitespace removed first.
- When something breaks the rules or conflicts with an existing entry, it exits 2, `hearth.toml` stays unchanged, and the message says which flag to set it with.
- When you are done, remember to commit `hearth.toml` and open a PR.

## Skills

```sh
hearth skill add https://git.example.invalid/acme/skills.git --subdir pdf   # team file; pinned to the remote HEAD's commit at that moment
hearth skill add https://git.example.invalid/acme/toolkit.git              # no --subdir: lists the skills in the repo; several become one group
hearth config agents claude,codex                                          # personal file; none clears it
hearth skill enable pdf                                                    # personal file; a group's name turns on the whole group
hearth skill sync                                                          # installs and creates links
hearth skill list                                                          # read-only, no network
```

External skills in `hearth.toml`:

```toml
[[skills]]
name = "pdf"
url = "https://git.example.invalid/acme/skills.git"
commit = "0123456789abcdef0123456789abcdef01234567"
subdir = "pdf"
```

| Field | Rules |
| --- | --- |
| `name` | Same rule as `name` in `[[repos]]`; must not be `hearth` (ignoring case), and must not share a name with a team skill under `skills/` |
| `url` | The same three forms as `[[repos]]`, converted according to `protocol` on clone |
| `commit` | A full 40 or 64 lowercase hex characters; must not be a branch or tag |
| `subdir` | `.` or a relative path inside the repo, with no `.`, `..`, or `.git` segments; must contain `SKILL.md` |

When a repo has several skills, write them as one group entry with `subdirs`; the whole group shares one commit and one checkout:

```toml
[[skills]]
name = "toolkit"
url = "https://git.example.invalid/acme/toolkit.git"
commit = "0123456789abcdef0123456789abcdef01234567"
subdirs = ["skills/grilling", "skills/grill-me"]
```

- An entry may have only one of `subdir` or `subdirs`. `subdirs` must not be empty; each value follows the same rule as `subdir`, but must not be `.` and must not repeat.
- Each skill's name is the last segment of its subdir (for example `grilling`), follows the same rule as `name`, and must not repeat within a group; the `skills` list in `hearth.local.toml` uses this name.
- A group's name must not match any entry's `name` or any skill in any group. When different groups have skills with the same name, both are `skill-invalid`.
- Without `--subdir`, `skill add <url>` clones a copy into the system temporary directory and lists every `SKILL.md` (no checkout, nothing inside is run, and the copy is deleted afterward): if there is a top-level `SKILL.md` or only one, it is added as a single skill; with two or more, it creates a group. When the repo root has `.claude-plugin/plugin.json`, the members are the skills under the paths listed in its `skills` field, and the group's name is its `name`; skills left out are listed one by one, and you can add them with `skill add <url> --name <group> --subdir <path>`. Given several `--subdir` flags, it creates the group directly without listing files.
- `skill enable <group>` turns on the whole group and `skill disable <group>` turns off the group's enabled skills; `skill remove <group>` removes the whole entry, and `skill remove <skill>` only takes that skill out of its group; when several groups have a skill of that name, write `skill remove <group>/<skill>` to say which group; `skill update <group>` changes the whole group's pin.
- Config files from older versions (`schema_version = 1`) need `hearth upgrade` first; it changes only two values, `schema_version` and `min_hearth_version`.

Personal choices in `hearth.local.toml`: `agents = ["claude", "codex"]`, `skills = ["pdf"]` (MCP and subagents use `mcp` and `subagents`); a missing key means empty.

- Without `--commit`, `skill add` and `skill update` ask the remote for `HEAD` with `git ls-remote`; given a full SHA, they do not use the network.
- `skill sync` clones external skills into `vendor/skills/<name>` (one copy per group entry, cloned only when at least one member is enabled), detached at the pin, writes the built-in `hearth` skill to `.hearth/skills/hearth/`, and then symlinks them into `.claude/skills/` (`claude`) and `.agents/skills/` (`codex`). Skills already at the pin are not cloned or fetched.
- `skill remove` changes only `hearth.toml`: the next `sync` clears the link, and you delete `vendor/skills/<name>` yourself.
- `doctor` also checks the chosen skills, MCP servers, and subagents, and invalid subagent definitions. `not-installed`, `link-missing`, `unknown`, `agent-missing` are only warnings.

| Category | Meaning | What to do |
| --- | --- | --- |
| `agent-missing` | A chosen agent CLI is not on PATH. Only a warning; links are created anyway | Install that agent, or remove it with `hearth config agents` |
| `not-installed` | Chosen but not installed yet | `hearth skill sync` |
| `pin-mismatch` | `vendor/skills/<name>` is not at the pin and has local changes, ignored files, local commits, or is not detached, so it was not switched | Check the contents yourself, delete that directory, then `sync` |
| `skill-invalid` | No `SKILL.md` under `subdir`, the resolved path leaves the checkout, or the name conflicts with another source | Fix `hearth.toml` or rename the team skill's directory |
| `link-missing`, `link-stale` | A link made by `hearth` is gone or points to the wrong place | `hearth skill sync` fixes it |
| `link-conflict` | That location holds your own file or link, and `hearth` leaves it alone | Move it away yourself, then `sync` |
| `symlink-unavailable` | This file system cannot create symlinks; `hearth` does not fall back to copying | Use a location that supports symlinks |
| `unknown` | A skill you have chosen has been removed; it is treated as not chosen | `hearth skill disable <name>` |

`hearth sync` first clones each chosen product that has no checkout yet, then runs `hearth skill sync`, `hearth mcp sync`, `hearth subagent sync` in order; it runs the later steps even when an earlier one fails, the clone included, and takes the most severe exit code; `--force` goes to MCP and subagents.

## MCP servers

```sh
hearth mcp add docs --env-var DOCS_TOKEN -- npx -y @acme/docs-mcp           # stdio, team file
hearth mcp add tracker --url https://mcp.example.invalid/mcp --bearer-token-env TRACKER_TOKEN \
  --header-env 'X-Workspace=TRACKER_WORKSPACE'                              # http, team file
hearth mcp enable docs tracker                                              # personal file
hearth sync                                                                 # generates each agent's MCP config
hearth mcp list                                                             # read-only; lists missing environment variable names
```

MCP servers in `hearth.toml`:

```toml
[[mcp]]
name = "docs"
transport = "stdio"
command = "npx"
args = ["-y", "@acme/docs-mcp"]
env_vars = ["DOCS_TOKEN"]

[[mcp]]
name = "tracker"
transport = "http"
url = "https://mcp.example.invalid/mcp"
bearer_token_env = "TRACKER_TOKEN"
header_envs = { "X-Workspace" = "TRACKER_WORKSPACE" }
```

| Field | Rules |
| --- | --- |
| `name` | Letters, digits, `_`, `-`, starting with a letter or digit; no `.`. Must not repeat, ignoring case |
| `transport` | `stdio` or `http` |
| `command`, `args`, `env_vars` | Only for `stdio`. `command` is required; `env_vars` are the names of environment variables to pass through to the server |
| `url`, `bearer_token_env`, `header_envs` | Only for `http`. `url` must be `https://`, or `http://localhost` or `http://127.0.0.1` (with optional port and path), with no username or password; `header_envs` maps "header name → environment variable name" and must not include `Authorization` (use `bearer_token_env` for tokens) |

- Secrets are written only as environment variable names (uppercase letters, digits, and `_`), never as values; this file has no place for token or header values.
- Personal choices in `hearth.local.toml`: `mcp = ["docs"]`. A server the team removed becomes an `unknown` warning; clear it with `hearth mcp disable <name>`.
- `sync` generates whole files for the agents you chose: `claude` writes `.mcp.json` and `codex` writes `.codex/config.toml`, both ignored by `.gitignore`. After you drop an agent, `sync` deletes the file it generated for it.
- Claude Code asks you to approve the servers in `.mcp.json` the first time it reads them; Codex reads `.codex/config.toml` only in a trusted project.
- Export the environment variables in the shell that starts the agent; `hearth doctor` can only check the shell it runs in.
- `sync` records the hash of each generated file in `.hearth/state.json`. When a file has been edited by hand, `sync` neither overwrites nor deletes it, reports `generated-modified`, exits 1, and still updates the other files; only `hearth sync --force` overwrites it.
- When `.mcp.json`, `.codex`, `.hearth`, or the files under them are symlinks or the wrong type, it reports `path-conflict` and writes nothing, even with `--force`.

| Category | Meaning | What to do |
| --- | --- | --- |
| `generated-modified` | A generated file was changed after the last `sync`, or the file already exists but was not written by `hearth` | Move the settings you want into `hearth.toml`, then run `hearth sync --force` |
| `env-missing` | An environment variable a chosen server needs is not set, or `command` is not on PATH. Only a warning, and only names are printed | Export that variable in the shell that starts the agent, or install that command |

## Subagents

The team writes subagent definitions in `subagents/<name>.md`, and changes go through PRs; one file serves both Claude Code and Codex. Each person picks which ones to use; all are off by default:

```sh
hearth subagent enable subagent      # personal file
hearth sync                          # generates .claude/agents/ and .codex/agents/ for the chosen agents
hearth subagent list                 # read-only; lists each definition's status and why it is invalid
hearth subagent disable subagent     # the next sync deletes the files it generated
```

A definition is Markdown with YAML frontmatter:

```markdown
---
name: reviewer
description: Reviews a diff for correctness first, then style.
claude:
  tools: Read, Grep, Glob
  model: sonnet
codex:
  model_reasoning_effort: high
  features:
    shell_tool: false
---

You review diffs. Report each finding with its file and line.
```

| Part | Rules |
| --- | --- |
| File name | `<name>.md` directly under `subagents/`; files starting with `_` are templates and do not appear in the list |
| `name` | Required; equals the file name without `.md`; starts with a lowercase letter and contains only lowercase letters, digits, `_`, `-` |
| `description` | Required; must not be empty |
| `claude:`, `codex:` | Optional mappings holding each tool's own fields, passed through as is; `hearth` does not check the field names; must not contain `name`, `description`, `developer_instructions` |
| Body | The content after the frontmatter; must not be empty; it is the body of the Claude file and also the `developer_instructions` of the Codex file |

- The frontmatter accepts only four keys, `name`, `description`, `claude`, `codex`, and YAML anchors (`&x`) and aliases (`*x`) are not allowed anywhere.
- `claude` generates `.claude/agents/<name>.md`: `name`, `description`, the `claude:` fields in their original order, and the body last. Values are carried over; comments, quoting style, and flow-list style are not kept.
- `codex` generates `.codex/agents/<name>.toml`: `name`, `description`, `developer_instructions`, followed by the `codex:` fields. Values may only be strings, integers, floats, booleans, and lists of these; nested mappings are written as dotted keys like `features.shell_tool = false`. `null`, dates and times, non-string keys, and mappings inside lists are all invalid.
- **The Codex side is not yet confirmed to take effect**: the files follow the official Codex docs, but in a 2026-11-18 test with Codex 0.162.1, the role named at spawn did not get the definition applied. Subagents whose names contain `-` may not be callable in Codex.
- Invalid definitions are not generated; `subagent list` and `doctor` show the reason, and other definitions are generated as usual; `enable` with an invalid or nonexistent name reports `usage`, and `hearth.local.toml` stays unchanged.
- For one that was already chosen and became invalid later (for example, a PR broke its frontmatter), the files it generated stay as they are, and `sync` reports `subagent-invalid` and exits 1.
- A name still in your personal list whose definition was deleted or renamed counts as `unknown`: `sync` deletes the files it generated; clear it with `hearth subagent disable <name>`.
- Like the MCP config, the generated files are recorded in `.hearth/state.json`: files edited by hand are neither overwritten nor deleted, `generated-modified` is reported, and only `hearth sync --force` overwrites them; when a symlink sits in the path, it reports `path-conflict` and writes nothing, even with `--force`.
- When `subagents/` is a symlink or not a directory, it reports `path-conflict`, exits 2, and writes or deletes nothing; if it does not exist, there are taken to be no subagents.
- `doctor` and `subagent list` also check generated files left behind by agents that were dropped, subagents that are not chosen, or subagents whose source was deleted: files edited by hand report `generated-modified`, consistent with `sync`.
- Other files you put in `.claude/agents/` or `.codex/agents/` yourself are left alone by `hearth`; `.claude/agents/` is ignored by `.gitignore` and does not enter version control.

| Category | Meaning | What to do |
| --- | --- | --- |
| `subagent-invalid` | `subagents/<name>.md` does not meet the rules above | Fix that file as the message says, and open a PR |
| `not-synced` | Chosen, but the generated file is missing or out of date. Only a warning | `hearth sync` |

## ssh or https

`hearth.toml` holds one URL per product, in either ssh or https form. On clone it is converted according to the `protocol` in each person's `hearth.local.toml`; `hearth.toml` itself is not changed:

| Form in `hearth.toml` | `protocol = "ssh"` | `protocol = "https"` |
| --- | --- | --- |
| `git@<host>:<path>` | Unchanged | `https://<host>/<path>` |
| `ssh://git@<host>/<path>` | `git@<host>:<path>` | `https://<host>/<path>` |
| `https://<host>/<path>` | `git@<host>:<path>` | Unchanged |

- `<host>` and `<path>` are kept as is, including whether they end in `.git`.
- URLs with a port, another user, `http://`, or a local path are always rejected, because they cannot be converted.
- When `protocol` is not set, `hearth clone` connects to try ssh and then https, writes the result into `hearth.local.toml`, and prints it. To change it: `hearth config protocol <ssh|https>`.

## What clone does and does not do

| State of the target path | Result |
| --- | --- |
| Does not exist | Clones with `branch` and prints `cloned` |
| Already a Git checkout, and origin matches the expected URL | Prints `skipped` and changes nothing |
| Anything else | Reports an error and changes nothing |

- It first validates the whole `hearth.toml` and the given `name`; if anything is wrong, it clones nothing at all.
- One product failing does not affect the others. At the end it lists which were cloned, skipped, and failed.
- The clone is made first in a temporary directory at the same level, and renamed to the target path only after it succeeds. On failure, nothing half-done is left at the target path, and the next run clones again.
- For a checkout that already exists, it does not pull, fetch, switch branches, reset, stash, or clean, and does not change the remote.

So **`hearth clone` does not guarantee an existing checkout is up to date**, nor that it is on the branch `branch` names. To update one, work in the product repo yourself.

### origin is compared exactly

An existing checkout's `remote.origin.url` must be exactly the same string as the expected URL. SSH and HTTPS are not treated as the same, and neither is the presence or absence of a trailing `.git`.

After `protocol` changes, `hearth clone` and `hearth doctor` report `remote-mismatch`. `hearth repo fix-remote` changes origin only when the difference is purely the protocol; other differences are only reported, and you confirm yourself which side is right.

### Exit code

| exit code | Meaning |
| --- | --- |
| 0 | Success; also 0 when `hearth.toml` has no products |
| 1 | At least one product failed; the others were handled as usual |
| 2 | Config, usage, version, or environment error. Nothing was cloned in this case |
| 130 | Interrupted with Ctrl-C while running. Half-cloned temporary directories are cleaned up; just run it again |

### Error categories

Error messages have the format `error [<category>] <message>`, and those about a single product are `error [<category>] <name>: <message>`.

| Category | Meaning | What to do |
| --- | --- | --- |
| `config` | The format or content of `hearth.toml` or `hearth.local.toml` breaks the rules | Fix that file as the message says; the message gives the location |
| `usage` | Wrong command or arguments, for example a `name` that is not in `hearth.toml` | The message lists the available choices |
| `env` | `git` was not found, or no product could be reached | Install git; check the network and permissions |
| `version` | The config file needs a newer `hearth` than yours | Upgrade `hearth` |
| `path-conflict` | The target path is taken or unusable: not a directory, not a separate Git checkout, a symlink in the path, or the directory cannot be created | Check that path's contents yourself, then decide whether to move or delete it. `hearth` does not touch it |
| `remote-mismatch` | An existing checkout's origin is not the same string as the expected URL | If only the protocol differs, run `hearth repo fix-remote`; otherwise confirm which side is right |
| `clone-failed` | `git clone` failed, or `branch` is not a branch | Read the error git printed: permissions, network, URL, branch name |

### Known limitations

- When `products/` itself, or any level in the middle of a `path`, is a symlink, `hearth` refuses, even if the symlink points somewhere safe.
- There is a time gap between the path checks and the clone. `hearth` is a local development tool and does not guard against someone on the same machine maliciously changing directories at the same time.
- When the process is terminated by `kill` (Ctrl-C does not count), temporary directories starting with `.hearth-` may be left under `products/`. They are not treated as checkouts and are not deleted automatically; delete them by hand.
- Claude Code started from `products/<name>/` cannot read `.claude/skills/` at the Home Repo root; to use skills, start it from the root.
- Under Codex's workspace-write sandbox, `.agents/` is read-only, so `hearth skill sync` run by the agent itself fails; ask the user to run it.
- `products/` is ignored by `.gitignore`, so worktrees that Claude Code creates do not include `products/`, and the `@` menu does not list the files inside it.

## Directories

| Path | Contents |
| --- | --- |
| `AGENTS.md` | The shared rules, the single source |
| `CLAUDE.md` | Only makes Claude Code load `AGENTS.md` |
| `hearth.toml` | The product repo list |
| `products/` | Checkouts of the product repos. Created by `hearth clone`, not in version control |
| `memory/` | Team memory: cross-product decisions and team habits, not system specifications; see [`memory/README.md`](memory/README.md) |
| `skills/` | Skills the team wrote, and the template; see [`skills/README.md`](skills/README.md) |
| `vendor/`, `.hearth/`, `.claude/skills/`, `.agents/skills/` | External skills, built-in skills, and links generated by `hearth sync`; not in version control |
| `.mcp.json`, `.codex/` | MCP config and Codex subagent files generated by `hearth sync`; not in version control |
| `subagents/` | The team's subagent definitions; see "Subagents" above |
| `agents/` | Prompt templates for handing work to agents, and `models.json`, which records the default models; see [`agents/README.md`](agents/README.md) |
| `.claude/` | Shared Claude Code settings (`settings.json`); the `agents/` and `skills/` that `hearth sync` generates are not in version control |
| `workflows/` | Workflows for development, code review, and cross-repo changes; see [`workflows/README.md`](workflows/README.md) |

## How to work

Start your agent or editor from the Home Repo root so that it reads `AGENTS.md`.

### Single-product tasks

1. Confirm the product is cloned: `hearth clone <name>`.
2. Tell the agent which product this task changes.
3. Follow [`workflows/development.md`](workflows/development.md).

### Cross-product tasks

1. Confirm all related products are cloned: `hearth clone <name> <name>`.
2. List explicitly which repos may be changed this time. An agent does not change unlisted repos just because the task involves several products.
3. Follow [`workflows/cross-repo-change.md`](workflows/cross-repo-change.md).

### Handing off to another agent

Copy [`agents/_task-brief.md`](agents/_task-brief.md), fill it in, and paste it to the other agent. No tool reads `agents/` automatically.

### Model settings

This covers only Claude Code connecting directly to the Anthropic API. The main conversation's model is not hard-coded; pick it yourself at startup:

```sh
claude --model opus --effort medium
```

The general-purpose subagent explicitly sets `model: sonnet` under `claude:` in `subagents/subagent.md` (to use it, first run `hearth subagent enable subagent` and `hearth sync`), so it does not switch along with the main conversation's `/model`. `agents/models.json` is a record for people; no program reads it. Codex, Grok, and other tools need their own settings, which this config does not control. For the precedence order and how to pin a version, see [`agents/README.md`](agents/README.md#model-settings-claude-code-anthropic-api).

### Does Claude Code read the shared rules

`CLAUDE.md` loads the shared rules with `@AGENTS.md`. Claude Code looks for `CLAUDE.md` in parent directories, so it reads them when started from the Home Repo root or from under `products/<name>/`; started from a directory outside the Home Repo, it does not.

When you are not sure whether another tool reads `AGENTS.md` automatically, ask it to read `AGENTS.md` at the Home Repo root before starting work.

### Searching product code

`products/` is ignored by `.gitignore`, so search tools that filter by `.gitignore` do not find product code when run at the Home Repo root without a path. Point the path at the product's checkout and they will:

```sh
rg <pattern> products/<name>
```

## The Home Repo and product repos are managed separately

- The Home Repo and each product repo are separate Git repos, with separate commits and separate PRs.
- Commit product changes in the product repo. Running `git` in a directory outside `products/<name>/` works on the Home Repo.
- A Home Repo commit records only the URLs and branch names written in `hearth.toml`; **it does not represent a fixed combination of product versions**. With the same Home Repo commit, different people cloning at different times may get different product contents.
- When you need to reproduce a cross-product verification, also record each repo's commit SHA at the time. Run this at the Home Repo root:

  ```sh
  for d in products/*/; do
    [ -e "${d}.git" ] && printf '%s  %s\n' "$(git -C "$d" rev-parse HEAD)" "$d"
  done
  ```

## What to change after setup

1. This README's title and first paragraph: add the team name and this Home Repo's clone URL.
2. `AGENTS.md`: outside the section hearth manages, add the rules that are shared by the team but do not belong to any single product.
3. Add products with `hearth repo add`.
4. License: no LICENSE is included. Before making it public or sharing it outside, choose a license yourself and add the file.

## Security and limitations

### .gitignore is not a security boundary

`.gitignore` ignores `/products/`, `/hearth.local.toml`, the directories and MCP config that `hearth sync` generates, common credential files, and Claude Code's local settings (`.claude/settings.local.json`, `CLAUDE.local.md`), which only means the Home Repo does not track those files. It is not a read/write permission setting for agents, and not a security isolation mechanism:

- Agents can read and change every file in `products/`, including credentials and local settings in the product repos.
- To limit what an agent can read and write, use your agent tool's own permission settings or sandbox.
- Do not write secrets, credentials, customer personal data, or raw sensitive logs into any file in this repo, including `memory/`.

The content between `hearth:begin` and `hearth:end` in `.gitignore` and `AGENTS.md` is managed by `hearth`, and later versions rewrite the whole section; content outside the markers belongs to the team, and `hearth` does not change it.

### Not included in this version

- Installing, signing in to, or configuring agent tools.
- Wiring `agents/` to any tool's auto-load location. Definitions in `subagents/` are turned by `hearth sync` into files in `.claude/agents/` and `.codex/agents/`; they are not links to `agents/`. Skills are wired into `.claude/skills/` and `.agents/skills/` only after each person picks them and runs `hearth skill sync`.
- The choice of quality gate (which lint, type checks, and test thresholds to run). Each product follows its own repo's rules.
- LICENSE.
- Windows support.

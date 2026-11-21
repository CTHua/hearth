# Home Repo shared rules

This file holds the shared rules for engineers and AI coding agents working in this repo, and it is the single source for them.
`CLAUDE.md` only loads this file; other tools' entry files also just point here and do not copy the rules.

<!-- hearth:begin -->
## hearth (hearth manages this section; later versions rewrite all of it)
- Install: `go install github.com/CTHua/hearth/cmd/hearth@latest`; for other ways see https://github.com/CTHua/hearth . After upgrading hearth, run `hearth upgrade` to update this section and the marked section of `.gitignore`.
- Clone the products chosen in `hearth.local.toml` (every product when it has no `repos` key): `hearth clone`; clone one product: `hearth clone <name>`; see each product's status: `hearth repo list`; check the environment and config: `hearth doctor`; running `hearth` with no arguments in a terminal opens the interactive UI for the user; agents must not run it.
- The product list is in `hearth.toml`, and each product's `description` says what it does. To add or remove a product or change a `description`, ask the user to run `hearth repo add <url> [--description <text>]` / `hearth repo remove <name>` / `hearth repo describe <name> <text>` themselves; do not edit `hearth.toml` by hand; the agent's sandbox may block the write.
- Each person's ssh/https choice is in `hearth.local.toml` (not in version control); view or set it with `hearth config protocol`.
- Skills, MCP servers, and subagents: the team list uses `hearth skill add <url>` / `remove <name>` and `hearth mcp add <name>` / `remove <name>`; subagents are `subagents/<name>.md` files, changed through PRs; personal choices use `hearth skill enable` / `disable <name>`, `hearth mcp enable` / `disable <name>`, `hearth subagent enable` / `disable <name>`, `hearth config agents`, and then the user runs `hearth sync`; `hearth skill list`, `hearth mcp list`, `hearth subagent list` show the status.
<!-- hearth:end -->

## What this repo is

The Home Repo is the engineering team's shared starting point for work; it holds the team's memory, skills, workflows, and the list of product repos.

Product code is not here. Each product is its own Git repo, cloned into `products/` by `hearth clone`, with its own version control and its own commits.

## Directory guide

| Path | Contents | When to read |
| --- | --- | --- |
| `README.md` | Requirements, quick start, `hearth` commands, and error handling | First-time setup, or when `hearth` fails |
| `hearth.toml` | The product repo list (`[[repos]]`, with `description` saying what each product does) and external skills pinned to a commit (`[[skills]]`) | To decide which product a task belongs to, or to find a product repo or external skill |
| `hearth.local.toml` | Personal settings: `protocol` (`ssh` or `https`), `agents`, `repos`, `skills`, `mcp`, `subagents`. Not in version control | To know which protocol the user clones with and which repos, skills, MCP servers, and subagents they picked |
| `products/` | Checkouts of the product repos, at each product's `path` in `hearth.toml`. Not in the Home Repo's version control | The product the task names |
| `memory/decisions/` | Confirmed cross-product decisions | The task involves cross-product design or trade-offs |
| `memory/conventions/` | The team's habits and conventions | Before starting, to confirm the way the team agreed on |
| `skills/<name>/SKILL.md` | Reusable procedures the team wrote | The task fits a skill's use case |
| `vendor/skills/`, `.hearth/`, `.claude/skills/`, `.agents/skills/` | External skill checkouts, built-in skills, and per-agent links created by `hearth sync`. Not in version control; do not edit by hand | No need to read; agents load them from their own skills directory |
| `.mcp.json`, `.codex/config.toml` | MCP config that `hearth sync` generates from the `[[mcp]]` entries in `hearth.toml` and personal choices; it holds only environment variable names. Not in version control; do not edit by hand | No need to read; to change an MCP server, change `hearth.toml` |
| `subagents/<name>.md` | The team's subagent definitions, one file for both Claude Code and Codex; `claude:` and `codex:` hold each tool's own fields | When adding or changing a subagent |
| `.claude/agents/`, `.codex/agents/` | Subagent files that `hearth sync` generates from `subagents/` and personal choices. Not in version control; do not edit by hand | No need to read; to change a subagent, change `subagents/<name>.md` |
| `agents/` | Prompt templates for handing work to another agent; `agents/models.json` records each role's default model | When handing a task to another agent |
| `.claude/settings.json` | Shared Claude Code settings | When changing Claude Code's model settings |
| `workflows/` | Minimal workflows for development, code review, and cross-repo changes | Before starting a task of the matching kind |

## Load by task

- Read only the documents this task needs. Do not read all memory, skills, or product code up front.
- For memory and skills, read the summaries first, then decide whether to read the full text:
  - `grep -H '^summary:' memory/decisions/*.md memory/conventions/*.md`
  - `grep -H '^description:' skills/*/SKILL.md`
- Memory records only decisions and habits. For how the system works (APIs, data formats, config values, behavior), the product repo's code and tests are the only standard; when memory and code disagree, trust the code and report which entry needs updating.
- Files or directories whose names start with `_` are templates, not content.
- Do not assume tools discover these directories automatically. When needed, read them yourself at the paths above.

## Working in product repos

1. `hearth.toml` is the authority for the product list. Read each entry's `description` to decide which product the task belongs to (if there is none, look at `name`; if you cannot decide, ask); the checkout's location is the entry's `path`. When the checkout does not exist, run `hearth clone <name>`. It does not update a checkout that already exists.
2. Before starting, read the product repo's applicable `AGENTS.md` or `CLAUDE.md`: the one at the repo root, and the ones in the subdirectories you will change.
3. Before running Git, tests, or builds, confirm you are working in the right repo:
   - `git -C <path> rev-parse --show-toplevel`
   - `git -C <path> remote get-url origin`
   - Running `git` in a directory outside a product checkout works on the Home Repo.
4. Change only the repos the task names. A task that involves several products does not mean you may also change other repos along the way; if you find another repo needs changes too, report it first.
5. Commit the Home Repo and product repos separately. Commit product changes in the product repo; do not copy them into the Home Repo.

## Searching product code

`.gitignore` ignores `products/`. Search tools that filter by `.gitignore` (ripgrep, Claude Code's Grep) do not find product code when run at the Home Repo root without a path.

- Point the search path at the product's checkout, for example `rg <pattern> products/<name>`; or go into that checkout first and search there.
- `git grep` and `git ls-files` at the Home Repo root see only the Home Repo's own files; to look in a product, use `git -C <path>`.
- Before reporting "not found", confirm the search scope includes that product's checkout.

## Rule precedence

- This file is the team-wide baseline.
- Product implementation details (language, framework, tests, naming, release process) follow the product repo's own rules.
- When the two conflict and you cannot tell which rule applies, point out which two rules conflict, then stop and ask. Do not guess.

## Not without being asked

- Do not create commits or push.
- Do not run deployments.
- Do not run destructive operations, such as `git reset --hard`, `git clean`, force-push, deleting branches, or overwriting or discarding uncommitted changes.

## Model settings

For details, see "Model settings" in `agents/README.md`.

- This covers only Claude Code connecting directly to the Anthropic API. Codex, Grok, and other tools have their own settings, which this config does not control.
- The main conversation's model is not written into the shared settings; whoever starts the session picks it with `claude --model` or `/model`.
- Subagent definitions always state `model` explicitly and do not rely on inheriting the main conversation's model.
- `agents/models.json` is only a record; no program reads it. When you change a model, change it, `.claude/settings.json`, and the definitions in `subagents/` together.
- Do not write API keys, tokens, or base URLs into `.claude/settings.json`.

## Personal settings

- `hearth.local.toml` is each person's own settings. Do not create or change it for the user; when the protocol, agents, or skill, MCP, or subagent choices need to change, ask the user to run `hearth config protocol <ssh|https>`, `hearth config agents <claude,codex|none>`, `hearth skill enable` / `disable <name>`, `hearth mcp enable` / `disable <name>`, `hearth subagent enable` / `disable <name>` themselves; repos are chosen by the user on the Mine tab of the interactive UI.
- Skills the team writes go in `skills/`; external skills are pinned to a commit with `hearth skill add` and not copied into the repo.
- `hearth sync` writes to places such as `.agents/`, `.claude/skills/`, `.claude/agents/`, `.mcp.json`, and `.codex/`, which the agent's sandbox may block; when it is blocked, ask the user to run it themselves.
- MCP server secrets are expressed only as environment variable names; do not write token or header values into any file.

## Several agents at once

When several agents change the same product, each uses its own git worktree; they do not share one checkout. See `workflows/development.md` for how.

## Security and privacy

- `.gitignore` only decides which files the Home Repo tracks. It is not a read/write permission and not a security boundary: agents can read and change the files in `products/`.
- Do not write secrets, credentials, customer personal data, or raw sensitive logs into any file in this repo, including `memory/`.

## Adding or changing team knowledge

- Additions and changes to memory, skills, and workflows all go through PR review.
- Conversation content does not automatically become team fact. For a conclusion that should be kept, open a PR in the format in `memory/README.md`.
- Knowledge that applies to only one product stays in that product's repo.

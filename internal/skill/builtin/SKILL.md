---
name: hearth
description: Use when a task needs to change this home repo's product list, its skills or MCP servers, or someone's agent, skill or MCP choices; explains which hearth command to run instead of editing hearth.toml or hearth.local.toml by hand.
---

# hearth

This home repo is managed with the `hearth` CLI. Two files hold its settings:

- `hearth.toml` is the team file: product repos (`[[repos]]`), external skills pinned to a commit (`[[skills]]`) and MCP servers (`[[mcp]]`). It is version-controlled; every change goes through a PR.
- `hearth.local.toml` is each person's own file: `protocol`, `agents`, the chosen `skills` and the chosen `mcp` servers. It is not version-controlled.

## Rules

- Do not edit `hearth.toml` or `hearth.local.toml` by hand. Change them with the commands below; `hearth` rewrites the whole file and checks every rule.
- Do not change someone's personal choices (`agents`, `skills`, `protocol`) unless they asked for it.
- If a command fails because your sandbox blocks the write (for example a read-only `.agents/` or the network), stop and ask the user to run that exact command themselves. Do not work around the sandbox.
- `hearth sync` writes `vendor/skills/`, `.hearth/`, `.claude/skills/`, `.agents/skills/`, `.mcp.json` and `.codex/config.toml`. Ask the user to run it; do not create or edit those paths yourself.
- MCP secrets are named by environment variable only. Never write a token or header value into any file.

## Commands

| Task | Command |
| --- | --- |
| Add a product repo | `hearth repo add <url> [--name <name>] [--path <path>] [--branch <branch>]` |
| Remove a product repo | `hearth repo remove <name>` |
| Add an external skill, pinned | `hearth skill add <url> [--name <name>] [--commit <sha>] [--subdir <dir>]` |
| Remove an external skill | `hearth skill remove <name>` |
| Move a skill to a new commit | `hearth skill update <name> [--commit <sha>]` |
| Choose skills for yourself | `hearth skill enable <name>...` |
| Stop using skills | `hearth skill disable <name>...` |
| Choose agents | `hearth config agents claude,codex` (or `none`) |
| Add a stdio MCP server | `hearth mcp add <name> [--env-var VAR]... -- <command> [args...]` |
| Add an http MCP server | `hearth mcp add <name> --url <url> [--bearer-token-env VAR] [--header-env 'Header=VAR']...` |
| Remove an MCP server | `hearth mcp remove <name>` |
| Choose MCP servers for yourself | `hearth mcp enable <name>...` |
| Stop using MCP servers | `hearth mcp disable <name>...` |
| Install skills and write MCP configs | `hearth sync` (`--force` overwrites MCP files edited by hand) |
| Install and link the chosen skills only | `hearth skill sync` |
| See every skill and its state | `hearth skill list` |
| See every MCP server and missing variables | `hearth mcp list` |
| Check the whole setup | `hearth doctor` |

- After `repo add`, `repo remove`, `skill add`, `skill remove`, `skill update`, `mcp add` or `mcp remove`, commit `hearth.toml` and open a PR.
- `skill list`, `mcp list` and `doctor` only read; they never connect to a remote or write a file.
- Team skills are directories in `skills/<name>/` with a `SKILL.md`; they are chosen with `skill enable` like external ones.

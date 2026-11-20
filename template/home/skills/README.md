# Skills

Reusable operational knowledge that is loaded on demand: a task that recurs, written as steps that an engineer or an agent can follow to finish it.

## Three sources

| Source | Where it lives | How to add it |
| --- | --- | --- |
| Team | This directory: `skills/<name>/SKILL.md` | Open a PR as in "How to add a team skill" below |
| External | Another repo, pinned to a full commit SHA. `hearth skill sync` clones it into `vendor/skills/<name>` | `hearth skill add <url> [--subdir <dir>]... [--name <name>] [--commit <sha>]`, then open a PR |
| Built-in | The `hearth` skill that ships with `hearth`, teaching agents to change the lists with `hearth` commands | Nothing to add; it is installed once any agent is chosen |

- Names must not repeat across sources, ignoring case; `hearth` is reserved for the built-in skill.
- When an external repo has several skills, `hearth skill add <url>` without `--subdir` lists the skills in the repo and creates one group: a single entry in `hearth.toml`, one commit shared by the whole group, and a single checkout in `vendor/skills/<group>`. Each skill in the group is named after the last segment of its directory; use that name with `enable`, or use the group's name to turn on the whole group.
- Directories whose names start with `_` are templates and do not count as skills. Directories whose names break the name rule (letters, digits, `.`, `_`, `-`, starting with a letter or digit) report `skill-invalid` and cannot be picked.

## Each person picks their own

Each person picks which agents and skills to use; the picks go only into their own `hearth.local.toml` and do not affect anyone else:

```sh
hearth config agents claude,codex   # or pick just one; none clears it
hearth skill enable <name>...       # team or external skills; a group's name means the whole group
hearth skill sync                   # installs and creates links
hearth skill list                   # each skill's source, whether it is picked, and its status
hearth skill disable <name>...      # no longer needed; the next sync removes the link
```

- `sync` symlinks the picked skills into `.claude/skills/<name>` (`claude`) and `.agents/skills/<name>` (`codex`). These directories, `vendor/`, and `.hearth/` are all ignored by `.gitignore`.
- `sync` deletes only links it created. Files or links you put in those directories yourself are left alone and reported as `link-conflict`.
- After an external skill's pin changes, `sync` switches only when the old checkout has no local changes, no ignored files, and no local commits; otherwise it reports `pin-mismatch` and leaves the old checkout as it is.
- When a teammate removes a skill you have picked, commands still run and warn that it is `unknown`; clear it from your personal file with `hearth skill disable <name>`.
- Claude Code started from `products/<name>/` cannot read the root `.claude/skills/`; start it from the Home Repo root.

## What each skill must state

| Section | Contents |
| --- | --- |
| When to use | Which tasks should use it, and when it should not be used |
| Prerequisites | The tools and permissions needed, and which repo and state to start in |
| Steps | Numbered steps, one action per step, with complete commands |
| Verification | How to confirm it was done right: commands to run and expected results |

## How to add a team skill

1. Copy `skills/_template/` to `skills/<name>/`, using kebab-case for `<name>`.
2. Fill in the frontmatter: `name` matches the directory name, and `description` states the use case in one line.
3. Follow the written steps once for real and confirm the verification passes.
4. Open a PR. After it merges, whoever wants it runs `hearth skill enable <name>` and `hearth skill sync` themselves.

## What does not go here

- Procedures that apply to only one product: put them in that product's repo.
- Secrets, credentials, tokens. When a step needs a credential, say where to get it, not its value.

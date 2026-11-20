# Agents

Prompt templates for handing a task to another agent (another coding agent, a subagent, or a teammate's agent session).

## Nothing discovers this directory automatically

- No agent tool reads `agents/` on its own. It is an ordinary folder: when you need a template, open it, copy the contents, fill them in, and paste them to the agent.
- Some tools have their own auto-load locations, such as Claude Code's `.claude/agents/` and Codex's `.codex/agents/`. Subagent definitions live in [`../subagents/`](../subagents/); they are generated into those two locations only after each person picks them with `hearth subagent enable <name>` and runs `hearth sync`; see "Model settings" below. The templates in this directory are not moved there.
- Shared rules still live only in [`AGENTS.md`](../AGENTS.md). Templates hold only "what this hand-off is"; they do not copy the rules.

## Templates

| File | When to use |
| --- | --- |
| [`_task-brief.md`](_task-brief.md) | Hand a development or review task to another agent |
| [`models.json`](models.json) | Look up which model and which effort the main conversation and subagents use by default |

## Model settings (Claude Code, Anthropic API)

This covers only Claude Code connecting directly to the Anthropic API.

| File | Who reads it | Contents |
| --- | --- | --- |
| [`models.json`](models.json) | No program; it is a record for people | Each role's provider, alias, model ID, effort |
| [`../.claude/settings.json`](../.claude/settings.json) | Claude Code | Only `env`: `CLAUDE_CODE_SUBAGENT_MODEL=sonnet` |
| [`../subagents/subagent.md`](../subagents/subagent.md) | `hearth sync` turns it into Claude Code's `.claude/agents/subagent.md` | A general-purpose subagent, with `model: sonnet` and `effort: medium` under `claude:`; off by default; to use it, run `hearth subagent enable subagent` |

Keep these three files consistent: the settings' `env` holds no credentials or other providers' switches, and the subagent's `model` and `effort` match `models.json`. When you change one, change the other two with it, and compare them during PR review.

### Defaults

| Role | alias | Resolved on 2026-11-16 to | effort |
| --- | --- | --- | --- |
| main | `opus` | `claude-opus-5-5` | `medium` |
| subagent | `sonnet` | `claude-sonnet-5-5` | `medium` |

- These are defaults you can replace, not guaranteed to stay available.
- On the Anthropic API an alias resolves to the latest version; the `model` field only records the ID it resolved to on the day it was checked, so the two differ once a new version ships.
- To pin a version: change the values in `.claude/settings.json` and `subagents/subagent.md` to a full model ID (for example `claude-sonnet-5-5`), then update `models.json` to match.

### The main conversation's model is not hard-coded

`.claude/settings.json` does not set `model`; whoever starts the session decides which model the main conversation uses:

```sh
claude --model opus --effort medium
```

Or switch with `/model` and `/effort` inside the session. Note that `/model` writes the choice into your personal user settings, and it becomes the default for later new sessions.

### Which model a subagent uses

Claude Code takes the first one that is set, in this order:

1. The `model` parameter the main conversation passes when it invokes the subagent (alias only)
2. The `model` field in the subagent definition
3. `CLAUDE_CODE_SUBAGENT_MODEL`
4. The main conversation's current model

So:

- A subagent without a `model` follows the main conversation's model and changes as soon as `/model` switches it. That is why `subagent.md` states `model: sonnet` explicitly.
- `CLAUDE_CODE_SUBAGENT_MODEL` affects only subagents without their own `model`, such as the built-in general-purpose; set on its own, it does not change the built-in Explore and Plan, which follow the main conversation's model.
- The main conversation can still override `subagent.md` by passing a `model` parameter when it invokes a subagent. To force every subagent onto the same model, add `CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1` to your own settings; this template does not add it.

### Not covered

- Amazon Bedrock, Google Vertex AI, Microsoft Foundry, LLM gateways: an alias may resolve to a different version on these platforms; this config is not tested on them and does not handle them.
- Codex, Grok, and other agent tools: each uses its own config file, which this config does not control.
- Credentials: do not write API keys, tokens, or base URLs into `.claude/settings.json`; it goes into version control. Put them in your own environment variables, or in `.claude/settings.local.json`, which `.gitignore` ignores.

Official docs: [Subagents](https://code.claude.com/docs/en/sub-agents), [Model configuration](https://code.claude.com/docs/en/model-config), [Environment variables](https://code.claude.com/docs/en/env-vars).

Files whose names start with `_` are templates, not content.

## How to add one

1. Copy `_task-brief.md` to a new `_<name>.md`, or edit it directly.
2. Write only the fields a hand-off must include; link to `AGENTS.md` for the shared rules.
3. Use it once on a real task and confirm the receiving agent can start without asking anything.
4. Open a PR.

## What does not go here

- Secrets, credentials, tokens, customer personal data.
- Prompts that apply to only one product: put them in that product's repo.

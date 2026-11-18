---
# Comments, quoting styles and flow lists are not kept.
name: reviewer
description: "Reviews a diff: correctness first, then style."
claude:
  tools: [Read, Grep, Glob]
  model: sonnet
  effort: medium
  hooks:
    PreToolUse:
      - matcher: Bash
        hooks:
          - type: command
            command: ./bin/check --strict
  version: "2"
  color: blue
codex:
  model: gpt-5.4
  model_reasoning_effort: high
  max_turns: 12
  temperature: 0.5
  budget: 3.0
  strict: true
  features:
    shell_tool: false
    web:
      search: true
  nicknames: ["Rev", 'R2', 3, false]
  "odd key": "x"
  empty: {}
---

You review diffs. Report each finding with "file:line".

Do not edit files.

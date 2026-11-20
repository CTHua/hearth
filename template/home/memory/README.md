# Team memory

Cross-product decisions and habits that the team has confirmed go here, in only two kinds:

| Directory | What goes in it |
| --- | --- |
| `decisions/` | Confirmed cross-product decisions, with their reasons and trade-offs |
| `conventions/` | The team's habits and conventions: how everyone agreed to do things |

## How the system works: the code is the authority

- Memory does not record the system's specification: APIs, data formats, config values, module behavior, the current architecture. For these, the product repo's code and tests are the only standard; to know them, read the code.
- A decision may say "why we chose this", but does not restate details the code already spells out; otherwise it goes stale as soon as the code changes.
- When memory and code disagree, the code wins; open a PR to update that entry, or change its `status`.

## How to read it

Do not read it all at once. Look at the summaries first, then read the few entries related to the task:

```sh
grep -H '^summary:' memory/decisions/*.md memory/conventions/*.md
```

Treat entries whose `status` is not `active` as background only, not as current rules. Files whose names start with `_` are templates.

## Entry format

- One file per item, named `YYYY-MM-DD-<slug>.md`, where the date is the date it was confirmed.
- Start by copying `_template.md` in the same directory.
- All five frontmatter fields are required:

| Field | Contents |
| --- | --- |
| `summary` | A one-line summary, so readers can decide whether to read the full text |
| `scope` | Where it applies: which products, which situations |
| `source` | Source: a link to the PR, issue, incident record, or meeting notes, and who confirmed it |
| `confirmed` | Date confirmed, in `YYYY-MM-DD` format |
| `status` | `active` (current), `superseded` (replaced; the body says which entry replaced it), `retired` (no longer applies) |

## How entries get in

- Only through PR review. The reviewer confirms the content is true, the source can be checked, and the scope is clearly stated.
- Chat logs and agents' conversation summaries do not automatically count as team fact. An agent may draft an entry, but someone has to confirm it in the PR.
- When the content no longer holds, open a PR to update `status`.

## What does not go here

- Knowledge that applies to only one product: keep it in that product's repo.
- System specifications and descriptions of behavior: the code and tests are the authority; see above.
- The incident narrative itself: record only the decisions or conventions drawn from it.
- Secrets, credentials, tokens, customer personal data, raw sensitive logs. When citing an incident, write a de-identified description and add a link to the internal system.
- Content that has not been confirmed yet.

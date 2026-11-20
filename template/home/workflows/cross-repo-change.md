# Cross-repo change workflow

Use this when one requirement needs changes in two or more repos. Development inside each repo follows `development.md`.

## 1. Write it down before starting

Write the four items below in the requirement's issue or PR description, and start changing things only after they are confirmed:

| Item | Contents |
| --- | --- |
| Scope of impact | Which repos change and what changes in each; which repos are affected but need no change |
| Dependency order | Which repo's change must merge or ship first; what breaks if the order is reversed; whether old and new versions can coexist during the transition |
| Per-repo verification | What verification each repo runs; how the cross-repo integration is verified |
| Linked PRs | One PR per repo, linked to each other, with the merge order marked |

Change only the repos listed in this table. If you find along the way that another repo needs changes, update this table and report it first; do not change it along the way.

## 2. Implementation

- Create a branch, commit, and open a PR separately in each repo. Do not mix changes from several repos together.
- Follow the dependency order, doing the side that others depend on first.
- When several agents work at once, each uses its own worktree for the same product; see `development.md` for how.

## 3. Verification

- Each repo runs its own verification.
- When doing cross-repo integration verification, record each repo's commit SHA at the time. A Home Repo commit does not represent a combination of product versions; only each repo's SHA can reproduce it. Run this at the Home Repo root:

  ```sh
  for d in products/*/; do
    [ -e "${d}.git" ] && printf '%s  %s\n' "$(git -C "$d" rev-parse HEAD)" "$d"
  done
  ```

  This command lists only the directories directly under `products/` that are Git checkouts themselves. When verifying in a worktree, also record that worktree's `git rev-parse HEAD`.

## 4. Delivery

- Each PR's description lists the related PRs and the merge order.
- Merge in dependency order. Merge and deploy only when someone asks.
- For decisions that affect several products, open a PR afterward following `memory/README.md` to record them in `memory/decisions/`.

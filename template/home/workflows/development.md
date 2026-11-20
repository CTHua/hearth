# Development workflow (single product)

1. Confirm the target repo. Find the product's `path` in `hearth.toml`; when the checkout does not exist, run `hearth clone <name>`.
2. Confirm the checkout's state. `hearth clone` does not update an existing checkout, so first look at the current branch and uncommitted changes:

   ```sh
   git -C products/<name> status --short --branch
   ```

   Whether to update it and where to branch from follow the product repo's rules. When the working directory has uncommitted changes you did not make, ask first.
3. Read the product repo's `AGENTS.md` or `CLAUDE.md`.
4. When needed, look up team memory and skills; see "Load by task" in the Home Repo's `AGENTS.md` for how.
5. Implement. Keep changes within the task's scope and this one repo.
6. Run the verification the product repo requires (tests, lint, build). Before running it, confirm the current directory is that product repo.
7. Report what you changed, which verification you ran and the results, and what you did not run.
8. Commit, push, and open a PR only when asked, and do it in the product repo.

## Several agents changing the same product

Only one agent uses a checkout at a time. Each other agent opens its own git worktree. Run this at the Home Repo root:

```sh
git -C products/<name> worktree add --no-track -b <branch> "$(pwd)/products/_worktrees/<name>-<task>" origin/<base-branch>
```

- Write the worktree path as an absolute path. `git -C` resolves a relative path under the product repo.
- `--no-track` keeps the new branch from setting `origin/<base-branch>` as its upstream, so a later push does not go to the base branch.
- `products/_worktrees/` is the suggested location: it is inside the Home Repo's ignored range, and it cannot collide with a `path` in `hearth.toml`, because every segment of a `path` must start with a letter or digit.
- A new worktree has no untracked files, such as installed dependencies. Reinstall them following the product repo's rules.
- Remove it when you are done:

  ```sh
  git -C products/<name> worktree remove "$(pwd)/products/_worktrees/<name>-<task>"
  ```

  This command refuses when the worktree has uncommitted changes. Deal with the changes first; do not add `--force`.

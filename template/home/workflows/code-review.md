# Code review workflow

1. Confirm what you are reviewing: which repo, which PR or diff, and what the base is. Look at the diff in the right repo.
2. Understand the purpose of the change: read the PR description and the related issue or requirement. When the purpose is unclear, ask first; do not guess.
3. Read the product repo's `AGENTS.md` or `CLAUDE.md` and follow its rules. For cross-product parts, compare against the Home Repo's `memory/decisions/`.
4. Read the diff, and the callers and related tests when needed. Check:
   - Whether the behavior matches the purpose, and whether it breaks existing behavior
   - Error handling, permissions, input validation, data consistency
   - Whether the tests really verify the behavior they claim to
   - Whether there are changes beyond the purpose
5. If you can run verification, run it following the product repo's rules and state what you ran. Say plainly what you did not run.
6. Report issues. For each, give the file and line number, the situation in which it fails, a suggested fix, and its severity. Write what you are unsure of as questions, not conclusions.
7. A review only gives comments. Unless asked, do not change code, commit, approve, or merge.

When a change spans several repos, also check it against the scope of impact and dependency order listed in `cross-repo-change.md`.

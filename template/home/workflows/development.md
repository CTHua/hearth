# 開發流程（單一產品）

1. 確認目標 repo。從 `hearth.toml` 找到產品的 `path`；checkout 不存在時，執行 `hearth clone <name>`。
2. 確認 checkout 的狀態。`hearth clone` 不更新既有 checkout，所以先看目前的分支和未提交的修改：

   ```sh
   git -C products/<name> status --short --branch
   ```

   要不要更新、從哪裡開分支，照產品 repo 的規範。工作目錄有不是你留下的未提交修改時，先問。
3. 讀產品 repo 的 `AGENTS.md` 或 `CLAUDE.md`。
4. 需要時查團隊記憶和 skills，方法見 Home Repo `AGENTS.md` 的「按任務載入」。
5. 實作。改動限在任務範圍和這一個 repo。
6. 跑產品 repo 規定的驗證（測試、lint、build）。執行前確認所在目錄是那個產品 repo。
7. 回報改了什麼、跑了哪些驗證和結果、哪些沒跑。
8. 被要求時才 commit、push、開 PR，而且在產品 repo 裡做。

## 多個 agent 同時改同一個產品

一份 checkout 同時只給一個 agent 用。其他 agent 各自開獨立的 git worktree。在 Home Repo 根目錄執行：

```sh
git -C products/<name> worktree add --no-track -b <branch> "$(pwd)/products/_worktrees/<name>-<task>" origin/<base-branch>
```

- worktree 的路徑要寫絕對路徑。相對路徑會被 `git -C` 解析到產品 repo 底下。
- `--no-track` 讓新分支不把 `origin/<base-branch>` 設成 upstream，避免之後 push 時推到 base 分支。
- `products/_worktrees/` 是建議位置：在 Home Repo 的忽略範圍內，也不會和 `hearth.toml` 的 `path` 相撞，因為 `path` 的每一段都必須以英數字開頭。
- 新的 worktree 沒有未追蹤的檔案，例如安裝好的相依套件。照產品 repo 的規範重新安裝。
- 用完後移除：

  ```sh
  git -C products/<name> worktree remove "$(pwd)/products/_worktrees/<name>-<task>"
  ```

  worktree 裡有未提交的修改時這個指令會拒絕。先處理修改，不要加 `--force`。

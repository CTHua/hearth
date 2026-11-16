# 跨 repo 變更流程

一個需求要改兩個以上的 repo 時用。每個 repo 內的開發照 `development.md`。

## 1. 動手前先寫下來

把下面四項寫在需求的 issue 或 PR 說明裡，確認後才開始改：

| 項目 | 內容 |
| --- | --- |
| 影響範圍 | 要改哪些 repo、各改什麼；哪些 repo 受影響但不用改 |
| 相依順序 | 哪個 repo 的改動要先合併或先上線；順序反了會壞什麼；過渡期間新舊版本能不能並存 |
| 各 repo 的驗證 | 每個 repo 各自跑什麼驗證；跨 repo 的整合怎麼驗 |
| PR 關聯 | 每個 repo 一個 PR，互相連結，標出合併順序 |

只改這張表列出的 repo。途中發現還要改其他 repo 時，先更新這張表並回報，不順便改。

## 2. 實作

- 每個 repo 分別開分支、分別 commit、分別開 PR。不把多個 repo 的改動混在一起。
- 照相依順序做，先做被依賴的那一邊。
- 多個 agent 同時進行時，同一個產品各用獨立的 worktree，做法見 `development.md`。

## 3. 驗證

- 每個 repo 跑自己的驗證。
- 做跨 repo 的整合驗證時，記錄當時每個 repo 的 commit SHA。Home Repo 的 commit 不代表產品的版本組合，只有各 repo 的 SHA 能重現。在 Home Repo 根目錄執行：

  ```sh
  for d in products/*/; do
    [ -e "${d}.git" ] && printf '%s  %s\n' "$(git -C "$d" rev-parse HEAD)" "$d"
  done
  ```

  這個指令只列 `products/` 第一層、本身是 Git checkout 的目錄。用 worktree 驗證時，另外記錄那個 worktree 的 `git rev-parse HEAD`。

## 4. 交付

- 每個 PR 的說明列出相關的其他 PR 和合併順序。
- 照相依順序合併。合併和部署都要有人要求才做。
- 影響跨產品的決定，事後照 `memory/README.md` 開 PR 記進 `memory/decisions/`。

# Skills

可重用、按需載入的操作知識：一件會重複做的事，寫成工程師或 agent 照著做都能完成的步驟。

## 三種來源

| 來源 | 放哪裡 | 怎麼加入 |
| --- | --- | --- |
| 團隊 | 這個目錄：`skills/<name>/SKILL.md` | 照下面「怎麼新增團隊 skill」開 PR |
| 外部 | 別的 repo，釘在一個完整 commit SHA。`hearth skill sync` 把它 clone 到 `vendor/skills/<name>` | `hearth skill add <url> [--subdir <dir>] [--name <name>] [--commit <sha>]`，再開 PR |
| 內建 | `hearth` 自己帶的 `hearth` skill，教 agent 用 `hearth` 指令改清單 | 不用加；選了任一個 agent 就會裝 |

- 名稱跨來源不分大小寫不能重複；`hearth` 保留給內建 skill。
- 目錄名以 `_` 開頭的是模板，不算 skill。目錄名不合 name 規則（英數字、`.`、`_`、`-`，以英數字開頭）的會報 `skill-invalid`，不能選。

## 每個人自己選

每個人選要用哪些 agent 和 skill，只寫進自己的 `hearth.local.toml`，不影響別人：

```sh
hearth config agents claude,codex   # 或只選一個；none 清空
hearth skill enable <name>...       # 團隊或外部 skill 都可以選
hearth skill sync                   # 安裝並建 link
hearth skill list                   # 每個 skill 的來源、有沒有選、狀態
hearth skill disable <name>...      # 不用了；下次 sync 會拿掉 link
```

- `sync` 把選到的 skill symlink 到 `.claude/skills/<name>`（`claude`）、`.agents/skills/<name>`（`codex`）。這些目錄和 `vendor/`、`.hearth/` 都被 `.gitignore` 忽略。
- `sync` 只刪自己建的 link。你自己放在那些目錄的檔案或 link 不會被動到，會報 `link-conflict`。
- 外部 skill 的 pin 改了之後，`sync` 只在舊 checkout 沒有本機改動、沒有 ignored 檔、也沒有本機 commit 時才換；否則報 `pin-mismatch`，舊 checkout 不動。
- 同事把你選著的 skill 移除後，指令照常能跑，會警告它是 `unknown`；用 `hearth skill disable <name>` 把它從個人檔清掉。
- Claude Code 從 `products/<name>/` 啟動時讀不到根目錄的 `.claude/skills/`，要從 Home Repo 根目錄啟動。

## 每個 skill 要寫明

| 段落 | 內容 |
| --- | --- |
| 適用情境 | 什麼任務該用，什麼時候不該用 |
| 前置條件 | 需要的工具、權限，以及要在哪個 repo、什麼狀態下開始 |
| 步驟 | 編號步驟，一步一個動作，指令寫完整 |
| 驗證方式 | 怎麼確認做對了：要跑的指令和預期結果 |

## 怎麼新增團隊 skill

1. 複製 `skills/_template/` 成 `skills/<name>/`，`<name>` 用 kebab-case。
2. 填 frontmatter：`name` 和目錄同名，`description` 用一行寫適用情境。
3. 照寫好的步驟實際做一次，確認驗證方式會過。
4. 開 PR。合併後，要用的人自己 `hearth skill enable <name>`、`hearth skill sync`。

## 不放這裡的

- 只適用單一產品的操作：放那個產品的 repo。
- 秘密、憑證、token。步驟需要憑證時，寫去哪裡取得，不寫值。

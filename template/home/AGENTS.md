# Home Repo 共同規則

這份檔案是工程師和 AI coding agents 在這個 repo 工作時的共同規則，也是唯一來源。
`CLAUDE.md` 只負責載入這份檔案；其他工具的入口檔也只指向這裡，不複製規則。

<!-- hearth:begin -->
## hearth（這一段由 hearth 管理，之後的版本會整段重寫）
- 安裝：`go install github.com/CTHua/hearth/cmd/hearth@latest`，其他方式見 https://github.com/CTHua/hearth 。
- clone 全部產品：`hearth clone`；看每個產品的狀態：`hearth repo list`；檢查環境和設定：`hearth doctor`。
- 產品清單在 `hearth.toml`。要新增或移除產品時，請使用者自己跑 `hearth repo add <url>` / `hearth repo remove <name>`，不要手改 `hearth.toml`；agent 的 sandbox 可能擋掉寫入。
- 每個人的 ssh／https 選擇在 `hearth.local.toml`（不進版控），用 `hearth config protocol` 查看或設定。
- skill 和 MCP server：團隊清單用 `hearth skill add <url>` / `remove <name>`、`hearth mcp add <name>` / `remove <name>`，個人選擇用 `hearth skill enable` / `disable <name>`、`hearth mcp enable` / `disable <name>`、`hearth config agents`，再由使用者跑 `hearth sync`；`hearth skill list`、`hearth mcp list` 看狀態。
<!-- hearth:end -->

## 這個 repo 是什麼

Home Repo 是工程團隊的共同工作入口，集中放團隊記憶、skills、workflows 和產品 repo 清單。

產品程式碼不在這裡。每個產品是獨立的 Git repo，由 `hearth clone` clone 到 `products/`，各自版控、各自提交。

## 目錄導覽

| 路徑 | 內容 | 什麼時候讀 |
| --- | --- | --- |
| `README.md` | 環境需求、快速開始、`hearth` 指令和錯誤處理 | 第一次設定，或 `hearth` 出錯 |
| `hearth.toml` | 產品 repo 清單（`[[repos]]`）和釘在 commit 的外部 skill（`[[skills]]`） | 要找產品 repo 或外部 skill |
| `hearth.local.toml` | 個人設定：`protocol`（`ssh` 或 `https`）、`agents`、`skills`、`mcp`。不在版控內 | 要知道使用者用哪個協定 clone、選了哪些 skill 和 MCP server |
| `products/` | 產品 repo 的 checkout，位置是 `hearth.toml` 裡該產品的 `path`。不在 Home Repo 的版控內 | 任務指定的那個產品 |
| `memory/decisions/` | 經確認的跨產品決策 | 任務涉及跨產品的設計或取捨 |
| `memory/conventions/` | 團隊的習慣和慣例 | 動手前確認團隊約定的做法 |
| `skills/<name>/SKILL.md` | 團隊自己寫的可重用操作步驟 | 任務符合某個 skill 的適用情境 |
| `vendor/skills/`、`.hearth/`、`.claude/skills/`、`.agents/skills/` | `hearth sync` 產生的外部 skill checkout、內建 skill 和各 agent 的 link。不在版控內，不要手改 | 不用讀；agent 會從自己的 skills 目錄載入 |
| `.mcp.json`、`.codex/config.toml` | `hearth sync` 照 `hearth.toml` 的 `[[mcp]]` 和個人選擇產生的 MCP 設定，只寫環境變數名稱。不在版控內，不要手改 | 不用讀；要改 MCP server 就改 `hearth.toml` |
| `agents/` | 派工給另一個 agent 用的 prompt 模板；`agents/models.json` 記錄各角色的預設模型 | 要把任務交給另一個 agent 時 |
| `.claude/settings.json`、`.claude/agents/` | Claude Code 的共用設定和 subagent 定義 | 要改 Claude Code 的模型設定時 |
| `workflows/` | 開發、code review、跨 repo 變更的最小流程 | 開始對應類型的任務前 |

## 按任務載入

- 只讀這次任務需要的文件。不要預先讀完所有記憶、skills 或產品程式碼。
- 記憶和 skill 先看摘要，再決定要不要讀全文：
  - `grep -H '^summary:' memory/decisions/*.md memory/conventions/*.md`
  - `grep -H '^description:' skills/*/SKILL.md`
- memory 只記決策和習慣。系統怎麼運作（API、資料格式、設定值、行為）以產品 repo 的 code 和測試為唯一標準；memory 和 code 對不上時信 code，並回報要更新哪一筆。
- 檔名或目錄名以 `_` 開頭的是模板，不是內容。
- 不要假設工具會自動發現這些目錄。需要時照上面的路徑自己去讀。

## 操作產品 repo

1. 產品清單以 `hearth.toml` 為準，checkout 的位置看該項目的 `path`。checkout 不存在時，執行 `hearth clone <name>`。它不會更新已經存在的 checkout。
2. 動手前先讀該產品 repo 適用的 `AGENTS.md` 或 `CLAUDE.md`：repo 根目錄的，以及你要改的子目錄裡的。
3. 執行 Git、測試、build 之前，確認操作的是正確的 repo：
   - `git -C <path> rev-parse --show-toplevel`
   - `git -C <path> remote get-url origin`
   - 在產品 checkout 以外的目錄執行 `git`，操作到的是 Home Repo。
4. 只改任務指定的 repo。任務涉及多個產品，不代表可以順便修改其他 repo；發現其他 repo 也需要改，先回報。
5. Home Repo 和產品 repo 分別提交。產品的改動提交在產品 repo，不複製進 Home Repo。

## 搜尋產品程式碼

`products/` 被 `.gitignore` 忽略。照 `.gitignore` 過濾的搜尋工具（ripgrep、Claude Code 的 Grep）在 Home Repo 根目錄不指定路徑時，搜不到產品程式碼。

- 把搜尋路徑指到產品的 checkout，例如 `rg <pattern> products/<name>`；或先進到那個 checkout 再搜。
- `git grep`、`git ls-files` 在 Home Repo 根目錄只看得到 Home Repo 自己的檔案；要查產品就用 `git -C <path>`。
- 回報「找不到」之前，先確認搜尋範圍有包含那個產品的 checkout。

## 規則的優先順序

- 這份檔案是團隊共通的基線。
- 產品的實作細節（語言、框架、測試、命名、發版流程）依產品 repo 自己的規範。
- 兩邊衝突，而且無法判定哪一條適用時，先指出是哪兩條規則衝突，停下來問。不要自行猜測。

## 未經要求不做的事

- 不建立 commit，不 push。
- 不執行部署。
- 不執行破壞性操作，例如 `git reset --hard`、`git clean`、force-push、刪除分支、覆寫或丟棄未提交的修改。

## 模型設定

細節見 `agents/README.md` 的「模型設定」。

- 只涵蓋 Claude Code 直接連 Anthropic API。Codex、Grok 和其他工具各自設定，這份設定控制不到它們。
- 主對話的模型不寫進共用設定，由啟動的人用 `claude --model` 或 `/model` 決定。
- subagent 定義一律明確寫 `model`，不靠繼承主對話的模型。
- `agents/models.json` 只是紀錄，沒有程式讀它。改模型時，它、`.claude/settings.json` 和 `.claude/agents/` 的定義要一起改。
- API key、token、base URL 不寫進 `.claude/settings.json`。

## 個人設定

- `hearth.local.toml` 是每個人自己的設定。不要替使用者建立或修改它；需要改 protocol、agents、skill 或 MCP 選擇時，請使用者自己跑 `hearth config protocol <ssh|https>`、`hearth config agents <claude,codex|none>`、`hearth skill enable` / `disable <name>`、`hearth mcp enable` / `disable <name>`。
- 團隊自己寫的 skill 放 `skills/`；外部 skill 用 `hearth skill add` 釘在 commit，不複製進 repo。
- `hearth sync` 會寫 `.agents/`、`.claude/skills/`、`.mcp.json`、`.codex/` 等位置，agent 的 sandbox 可能擋下；被擋時請使用者自己跑。
- MCP server 的秘密只用環境變數名稱表達，不要把 token 或 header 的值寫進任何檔案。

## 多個 agent 同時工作

多個 agent 修改同一個產品時，各自使用獨立的 git worktree，不共用同一份 checkout。做法見 `workflows/development.md`。

## 安全和隱私

- `.gitignore` 只決定 Home Repo 追蹤哪些檔案。它不是讀寫權限，也不是安全隔離：`products/` 裡的檔案，agent 讀得到也改得到。
- 不要把秘密、憑證、客戶個資、原始敏感 log 寫進這個 repo 的任何檔案，包含 `memory/`。

## 新增或修改團隊知識

- 記憶、skills、workflows 的新增和修改都走 PR review。
- 對話內容不自動成為團隊事實。要留下來的結論，照 `memory/README.md` 的格式開 PR。
- 只適用單一產品的知識，留在那個產品的 repo。

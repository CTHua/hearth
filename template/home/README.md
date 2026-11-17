# Home Repo

工程團隊和 AI coding agents 共用的工作入口。這裡集中維護團隊記憶、skills、workflows 和產品 repo 清單。

產品程式碼不在這個 repo。每個產品是獨立的 Git repo，由 `hearth clone` clone 到 `products/`，各自版控、各自提交。

共同規則在 [`AGENTS.md`](AGENTS.md)，工程師和 agent 都適用。

## 環境需求

- Git。
- `hearth`：`go install github.com/CTHua/hearth/cmd/hearth@latest`，其他安裝方式見 https://github.com/CTHua/hearth 。支援 macOS 和 Linux。
- 對 `hearth.toml` 列的 repo 有讀取權限。用 SSH 時要先設定好 SSH key。

## 快速開始

```sh
git clone <你的 Home Repo URL> home-repo
cd home-repo
hearth config protocol ssh   # 或 https；不設的話，第一次 clone 時會實際連線偵測
hearth clone                 # clone 全部產品
hearth repo list             # 每個產品的狀態
hearth doctor                # 檢查 git、設定檔和每個 checkout
```

`hearth` 從目前目錄往上找 `hearth.toml` 決定 Home Repo 根目錄，所以在 `products/<name>/` 裡也能跑；也可以用 `-C <dir>` 指定。

## 兩個設定檔

| 檔案 | 誰改 | 版控 |
| --- | --- | --- |
| `hearth.toml` | 團隊，透過 `hearth repo add` / `remove`、`hearth skill add` / `remove` / `update` 再開 PR | 進版控 |
| `hearth.local.toml` | 每個人自己，透過 `hearth config protocol`、`hearth config agents`、`hearth skill enable` / `disable` | 被 `.gitignore` 忽略 |

`hearth.toml` 的格式：

```toml
# Managed by hearth. Edit with `hearth repo add` / `hearth repo remove`; comments are not kept.
schema_version = 1
min_hearth_version = "0.1.0"

[[repos]]
name = "web-app"
url = "git@git.example.invalid:acme/web-app.git"
path = "products/web-app"
branch = "main"
```

| 欄位 | 規則 |
| --- | --- |
| `name` | 產品的識別名稱，也是 `hearth clone <name>` 用的名稱。只能用英數字、`.`、`_`、`-`，以英數字開頭。不分大小寫不得重複 |
| `url` | clone URL，只收 `git@<host>:<path>`、`ssh://git@<host>/<path>`、`https://<host>/<path>` 三種寫法 |
| `path` | 相對 Home Repo 的路徑，必須在 `products/` 底下，每一段的規則同 `name`。不分大小寫不得重複，也不得是另一個 `path` 的上層或下層 |
| `branch` | clone 時 checkout 的分支。必須是分支，不能是 tag |

- 四個欄位都必填，不能有其他欄位。
- `hearth` 改這個檔案時會整個重寫，只留第一行的註解；自己加的註解會消失，說明請寫在 PR 或這份 README。
- `min_hearth_version` 比你的 `hearth` 新，或 `schema_version` 比你的 `hearth` 支援的新時，指令會報 `version`、exit 2，什麼都不做。升級 `hearth` 即可。

## 加入和移除產品

```sh
hearth repo add git@git.example.invalid:acme/web-app.git            # name=web-app，path=products/web-app
hearth repo add https://git.example.invalid/acme/api.git --name api-service --branch develop
hearth repo remove web-app                                          # 只改 hearth.toml，checkout 留著
```

- 沒給 `--name` 時取 URL 最後一段、去掉 `.git`；沒給 `--path` 時是 `products/<name>`；沒給 `--branch` 時問遠端的預設分支。
- 不合規則或跟既有項目衝突時 exit 2，`hearth.toml` 不變，訊息會提示用哪個旗標指定。
- 改完記得 commit `hearth.toml` 並開 PR。

## Skills

```sh
hearth skill add https://git.example.invalid/acme/skills.git --subdir pdf   # 團隊檔，釘在遠端 HEAD 當下的 commit
hearth config agents claude,codex                                          # 個人檔；none 清空
hearth skill enable pdf                                                    # 個人檔
hearth skill sync                                                          # 安裝並建 link
hearth skill list                                                          # 只讀，不連網
```

`hearth.toml` 的外部 skill：

```toml
[[skills]]
name = "pdf"
url = "https://git.example.invalid/acme/skills.git"
commit = "0123456789abcdef0123456789abcdef01234567"
subdir = "pdf"
```

| 欄位 | 規則 |
| --- | --- |
| `name` | 規則同 `[[repos]]` 的 `name`，不能是 `hearth`（不分大小寫），也不能跟 `skills/` 底下的團隊 skill 同名 |
| `url` | 同 `[[repos]]` 的三種寫法，clone 時照 `protocol` 轉換 |
| `commit` | 完整的 40 或 64 個小寫 hex，不能是分支或 tag |
| `subdir` | `.` 或 repo 裡的相對路徑，不能有 `.`、`..`、`.git` 段；底下要有 `SKILL.md` |

`hearth.local.toml` 的個人選擇：`agents = ["claude", "codex"]`、`skills = ["pdf"]`，不存在代表空的。

- 沒給 `--commit` 時 `skill add`、`skill update` 用 `git ls-remote` 問遠端 `HEAD`；給了完整 SHA 時不連網。
- `skill sync` 把外部 skill clone 到 `vendor/skills/<name>`、detached 在 pin，內建的 `hearth` skill 寫到 `.hearth/skills/hearth/`，再 symlink 到 `.claude/skills/`（`claude`）、`.agents/skills/`（`codex`）。已經在 pin 的不 clone、不 fetch。
- `skill remove` 只改 `hearth.toml`：下次 `sync` 清掉 link，`vendor/skills/<name>` 要自己刪。
- `doctor` 也檢查選到的 skill。`not-installed`、`link-missing`、`unknown`、`agent-missing` 只是警告。

| 類別 | 意思 | 怎麼處理 |
| --- | --- | --- |
| `agent-missing` | 選了的 agent CLI 不在 PATH。只是警告，link 照樣建 | 安裝那個 agent，或 `hearth config agents` 拿掉它 |
| `not-installed` | 選了但還沒裝 | `hearth skill sync` |
| `pin-mismatch` | `vendor/skills/<name>` 不在 pin，而且有本機改動、ignored 檔、本機 commit 或不是 detached，所以沒換 | 自己確認內容，刪掉那個目錄再 `sync` |
| `skill-invalid` | `subdir` 底下沒有 `SKILL.md`、解析後跑出 checkout，或名稱跟別的來源衝突 | 修 `hearth.toml` 或改團隊 skill 的目錄名 |
| `link-missing`、`link-stale` | `hearth` 的 link 不見了或指錯 | `hearth skill sync` 會修 |
| `link-conflict` | 那個位置有你自己的檔案或 link，`hearth` 不動它 | 自己移走再 `sync` |
| `symlink-unavailable` | 這個檔案系統建不了 symlink；`hearth` 不改用複製 | 換一個支援 symlink 的位置 |
| `unknown` | 你選著的 skill 已經被移除；當成沒選 | `hearth skill disable <name>` |

## ssh 或 https

`hearth.toml` 每個產品只寫一個 URL，ssh 或 https 寫法都可以。clone 時照每個人 `hearth.local.toml` 的 `protocol` 轉換，`hearth.toml` 不會被改：

| `hearth.toml` 的寫法 | `protocol = "ssh"` | `protocol = "https"` |
| --- | --- | --- |
| `git@<host>:<path>` | 不變 | `https://<host>/<path>` |
| `ssh://git@<host>/<path>` | `git@<host>:<path>` | `https://<host>/<path>` |
| `https://<host>/<path>` | `git@<host>:<path>` | 不變 |

- `<host>`、`<path>` 原樣保留，包含結尾有沒有 `.git`。
- 帶 port、其他使用者、`http://`、本機路徑一律不收，因為轉不了。
- 沒設 `protocol` 時，`hearth clone` 會實際連線試 ssh 再試 https，把結果寫進 `hearth.local.toml` 並印出來。改法：`hearth config protocol <ssh|https>`。

## clone 做什麼、不做什麼

| 目標路徑的狀態 | 結果 |
| --- | --- |
| 不存在 | 用 `branch` clone，輸出 `cloned` |
| 已經是 Git checkout，而且 origin 和預期的 URL 相同 | 輸出 `skipped`，不做任何更動 |
| 其他情況 | 回報錯誤，不做任何更動 |

- 先驗證整份 `hearth.toml` 和指定的 `name`，有錯就完全不 clone。
- 一個產品失敗不影響其他產品。最後列出 cloned、skipped、failed 各有哪些。
- clone 先做在同一層的暫存目錄，成功後才改名到目標路徑。失敗時目標路徑不會留下半成品，下次執行會重新 clone。
- 對已經存在的 checkout，不 pull、不 fetch、不切分支、不 reset、不 stash、不 clean，也不修改 remote。

所以 **`hearth clone` 不保證既有 checkout 是最新的**，也不保證它停在 `branch` 指定的分支。要更新請自己到產品 repo 裡操作。

### origin 採精確比對

既有 checkout 的 `remote.origin.url` 要和預期的 URL 是完全相同的字串。SSH 和 HTTPS 不視為相同，結尾有沒有 `.git` 也不視為相同。

`protocol` 改了之後，`hearth clone`、`hearth doctor` 會報 `remote-mismatch`。`hearth repo fix-remote` 只在差異純粹是協定時改 origin；其他差異只報告，自己確認哪一邊才對。

### Exit code

| exit code | 意思 |
| --- | --- |
| 0 | 成功；`hearth.toml` 沒有產品也是 0 |
| 1 | 至少一個產品失敗，其他照常處理 |
| 2 | 設定、用法、版本或環境錯誤。這時沒有 clone 任何東西 |
| 130 | 執行中按 Ctrl-C 中斷。clone 到一半的暫存目錄會被清掉，重跑即可 |

### 錯誤類別

錯誤訊息的格式是 `error [<類別>] <說明>`，跟單一產品有關的是 `error [<類別>] <name>: <說明>`。

| 類別 | 意思 | 怎麼處理 |
| --- | --- | --- |
| `config` | `hearth.toml` 或 `hearth.local.toml` 的格式或內容不合規則 | 照訊息修那個檔案，訊息會寫出位置 |
| `usage` | 指令或參數不對，例如指定的 `name` 不在 `hearth.toml` | 訊息會列出可用的選擇 |
| `env` | 找不到 `git`，或連不上任何產品 | 安裝 git、檢查網路和權限 |
| `version` | 設定檔需要比你更新的 `hearth` | 升級 `hearth` |
| `path-conflict` | 目標路徑被佔用或無法使用：不是目錄、不是獨立的 Git checkout、路徑上有 symlink、無法建立目錄 | 自己確認那個路徑的內容，再決定移走或刪除。`hearth` 不會動它 |
| `remote-mismatch` | 既有 checkout 的 origin 和預期的 URL 不是同一個字串 | 只差協定就跑 `hearth repo fix-remote`；否則確認哪一邊才對 |
| `clone-failed` | `git clone` 失敗，或 `branch` 不是分支 | 看 git 印出的錯誤：權限、網路、URL、分支名稱 |

### 已知限制

- `products/` 本身，或 `path` 中間任何一層是 symlink 時，`hearth` 會拒絕，即使 symlink 指向的位置是安全的。
- 路徑檢查和 clone 之間有時間差。`hearth` 是本機開發工具，不防同一台機器上同時有人惡意改動目錄。
- 程序被 `kill` 終止時（Ctrl-C 不算），`products/` 底下可能留下 `.hearth-` 開頭的暫存目錄。它不會被當成 checkout，也不會被自動刪除，請手動刪除。
- Claude Code 從 `products/<name>/` 啟動時讀不到 Home Repo 根目錄的 `.claude/skills/`；要用 skill 就從根目錄啟動。
- Codex 的 workspace-write sandbox 下 `.agents/` 是唯讀，agent 自己跑 `hearth skill sync` 會失敗，請使用者自己跑。
- `products/` 被 `.gitignore` 忽略，所以 Claude Code 建 worktree 時不會帶上 `products/`，`@` 選單也不會列出裡面的檔案。

## 目錄

| 路徑 | 內容 |
| --- | --- |
| `AGENTS.md` | 共同規則，唯一來源 |
| `CLAUDE.md` | 只負責讓 Claude Code 載入 `AGENTS.md` |
| `hearth.toml` | 產品 repo 清單 |
| `products/` | 產品 repo 的 checkout。由 `hearth clone` 建立，不在版控內 |
| `memory/` | 團隊記憶：跨產品的決策和團隊習慣，不記系統規格，見 [`memory/README.md`](memory/README.md) |
| `skills/` | 團隊自己寫的 skill 和模板，見 [`skills/README.md`](skills/README.md) |
| `vendor/`、`.hearth/`、`.claude/skills/`、`.agents/skills/` | `hearth skill sync` 產生的外部 skill、內建 skill 和 link，不在版控內 |
| `agents/` | 派工給 agent 的 prompt 模板，以及記錄預設模型的 `models.json`，見 [`agents/README.md`](agents/README.md) |
| `.claude/` | Claude Code 的共用設定（`settings.json`）和 subagent 定義（`agents/subagent.md`） |
| `workflows/` | 開發、code review、跨 repo 變更的流程，見 [`workflows/README.md`](workflows/README.md) |

## 工作方式

從 Home Repo 根目錄啟動你的 agent 或編輯器，讓它讀到 `AGENTS.md`。

### 單一產品的任務

1. 確認產品已經 clone：`hearth clone <name>`。
2. 告訴 agent 這次要改的是哪個產品。
3. 照 [`workflows/development.md`](workflows/development.md)。

### 跨產品的任務

1. 確認相關產品都已經 clone：`hearth clone <name> <name>`。
2. 明確列出這次可以改哪些 repo。agent 不會因為任務涉及多個產品就修改沒列到的 repo。
3. 照 [`workflows/cross-repo-change.md`](workflows/cross-repo-change.md)。

### 交辦給另一個 agent

複製 [`agents/_task-brief.md`](agents/_task-brief.md) 填好再貼給對方。沒有工具會自動讀 `agents/`。

### 模型設定

只涵蓋 Claude Code 直接連 Anthropic API。主對話的模型不寫死，啟動時自己指定：

```sh
claude --model opus --effort medium
```

subagent 由 `.claude/agents/subagent.md` 明確指定 `model: sonnet`，不跟著主對話的 `/model` 切換。`agents/models.json` 是給人看的紀錄，沒有程式讀它。Codex、Grok 和其他工具要各自設定，這份設定控制不到它們。優先順序和怎麼固定版本，見 [`agents/README.md`](agents/README.md#模型設定claude-codeanthropic-api)。

### Claude Code 會不會讀到共同規則

`CLAUDE.md` 用 `@AGENTS.md` 載入共同規則。Claude Code 會往上層目錄找 `CLAUDE.md`，所以從 Home Repo 根目錄或 `products/<name>/` 底下啟動都讀得到；從 Home Repo 以外的目錄啟動讀不到。

其他工具不確定會不會自動讀 `AGENTS.md` 時，開始工作前先請它讀 Home Repo 根目錄的 `AGENTS.md`。

### 搜尋產品程式碼

`products/` 被 `.gitignore` 忽略，所以照 `.gitignore` 過濾的搜尋工具，在 Home Repo 根目錄不指定路徑時搜不到產品程式碼。把路徑指到產品的 checkout 就搜得到：

```sh
rg <pattern> products/<name>
```

## Home Repo 和產品 repo 分別管理

- Home Repo 和每個產品 repo 是各自獨立的 Git repo，分別 commit、分別開 PR。
- 產品的改動提交在產品 repo。在 `products/<name>/` 以外的目錄執行 `git`，操作到的是 Home Repo。
- Home Repo 的 commit 只記錄 `hearth.toml` 寫的 URL 和分支名稱，**不代表固定的產品版本組合**。同一個 Home Repo commit，不同人、不同時間 clone 到的產品內容可能不同。
- 需要重現跨產品的驗證時，另外記錄當時每個 repo 的 commit SHA。在 Home Repo 根目錄執行：

  ```sh
  for d in products/*/; do
    [ -e "${d}.git" ] && printf '%s  %s\n' "$(git -C "$d" rev-parse HEAD)" "$d"
  done
  ```

## 建好之後要改的

1. 這份 README 的標題和第一段：寫上團隊名稱和這個 Home Repo 的 clone URL。
2. `AGENTS.md`：在 hearth 管理的區段以外，補上團隊共通、但不屬於任何單一產品的規則。
3. 用 `hearth repo add` 加入產品。
4. 授權：沒有附 LICENSE。要公開或對外分享前，自己決定授權並加上檔案。

## 安全和限制

### .gitignore 不是安全隔離

`.gitignore` 忽略 `/products/`、`/hearth.local.toml`、`hearth skill sync` 產生的目錄、常見的憑證檔和 Claude Code 的本機設定（`.claude/settings.local.json`、`CLAUDE.local.md`），只代表 Home Repo 不追蹤那些檔案。它不是 agent 的讀寫權限設定，也不是安全隔離機制：

- agent 讀得到、也改得到 `products/` 裡的所有檔案，包含產品 repo 裡的憑證和本機設定。
- 要限制 agent 能讀寫的範圍，請用你的 agent 工具自己的權限設定或 sandbox。
- 秘密、憑證、客戶個資、原始敏感 log 不寫進這個 repo 的任何檔案，包含 `memory/`。

`.gitignore` 和 `AGENTS.md` 裡 `hearth:begin` 到 `hearth:end` 之間的內容由 `hearth` 管理，之後的版本會整段重寫；標記以外的內容歸團隊，`hearth` 不會改。

### 這一版不包含

- 安裝、登入或設定 agent 工具。
- 把 `agents/` 接到任何工具的自動載入位置。`.claude/agents/subagent.md` 是另外寫的一個定義，不是 `agents/` 的連結。skill 只有每個人選了、跑過 `hearth skill sync` 才會接到 `.claude/skills/`、`.agents/skills/`。
- MCP 設定。
- quality-gate 的選擇（要跑哪些 lint、型別檢查、測試門檻）。產品各自照自己 repo 的規範。
- LICENSE。
- Windows 支援。

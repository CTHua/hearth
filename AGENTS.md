# hearth 開發規則

給在這個 repo 工作的工程師和 AI coding agents。`CLAUDE.md` 只載入這份檔案。

## 結構

| 路徑 | 內容 |
| --- | --- |
| `cmd/hearth/main.go` | 入口：判斷 stdin、stdout 是不是 TTY，交給 `internal/cli` |
| `internal/cli` | 子指令和參數解析（標準庫 `flag`）：解析旗標 → 呼叫 `internal/core` → 印結果；沒參數時開 TUI 或印 help |
| `internal/core` | 每個使用者動作一個方法（repo、protocol、agent、skill、MCP、subagent、sync），吃已載入的 home、回傳結構化結果；`TeamChanges` 比較 `hearth.toml` 跟 HEAD；`core.Actions` 是 TUI 用的介面，測試換成假實作 |
| `internal/tui` | Bubble Tea v2 的 TUI：Mine（個人檔，含 skill、MCP、subagent 的開關）和 team 組的 Repos、Skills、MCP（團隊檔）四個分頁、置中表單、`?` 快捷鍵表、滑鼠（點擊照 render 時記下的區域判斷）和 `y` 剪貼簿，只透過 `core.Actions` 動作；背景動作一次一個，各自可用 `ctrl+c` 取消 |
| `internal/upgrade` | `hearth upgrade`：重寫標記區段、照遷移鏈升級設定檔的 `schema_version`；鏈和目標版本是參數 |
| `internal/config` | `hearth.toml`（`[[repos]]`、`[[skills]]`、`[[mcp]]`）、`hearth.local.toml`（`protocol`、`agents`、`skills`、`mcp`、`subagents`）的讀寫、驗證、版本 gate、根目錄搜尋 |
| `internal/mcp` | 產生 `.mcp.json`、`.codex/config.toml`（格式只在這裡，黃金檔在 `testdata/`）、環境變數和 PATH 檢查 |
| `internal/subagent` | 讀 `subagents/<name>.md`（YAML frontmatter 用 `yaml.Node` 保留順序）、驗證、轉成 `.claude/agents/<name>.md` 和 `.codex/agents/<name>.toml`（格式只在這裡，黃金檔在 `testdata/`；Codex 端還沒實測生效）、狀態、sync；擁有 state 裡 `.claude/agents/`、`.codex/agents/` 開頭的記錄 |
| `internal/genfile` | `.hearth/state.json` 的讀寫、hash、手改判斷用的檔案和 symlink 檢查，`internal/mcp`、`internal/subagent` 共用；各自只改自己的記錄 |
| `internal/tomlstr` | 手寫 TOML 值和 key（字串、陣列、inline table、dotted key），給 encoder 無法保留順序的地方用 |
| `internal/giturl` | 三種 URL 寫法的解析和 ssh／https 轉換 |
| `internal/gitx` | 拿掉繼承的 git local env vars 後執行 git |
| `internal/checkout` | 只讀檢查一個 checkout 的狀態；路徑上的 symlink 檢查和 checkout root 檢查也給 `internal/skill` 用 |
| `internal/clone` | clone 一個產品，或確認既有的 checkout |
| `internal/skill` | skill 的三種來源、狀態、detached-commit 的安裝和換 pin、各 agent 的 link；內建 skill 的 `SKILL.md` 在 `internal/skill/builtin/`（go:embed） |
| `internal/dirswap` | 換掉非空目錄，中途失敗時把原本的搬回來 |
| `internal/detect` | 用 `git ls-remote` 偵測 protocol |
| `internal/scaffold` | `hearth init`、標記區段檢查，和標記區段跟模板的逐 byte 比對（`upgrade`、`doctor` 共用） |
| `internal/apperr` | 錯誤類別、錯誤格式、exit code |
| `internal/brand` | 所有含套件名稱的字串 |
| `template/home/` | `hearth init` 寫出的 Home Repo 模板（go:embed） |
| `install.sh` | `curl \| sh` 的安裝腳本（POSIX sh）：選平台、從 Release 下載壓縮檔和 `SHA256SUMS`、驗 checksum、在安裝目錄試跑 `version` 後才換檔 |
| `scripts/` | `build-release.sh` 建 4 個平台的壓縮檔和 `SHA256SUMS`；`test-install.sh` 用 `file://` 離線測 `install.sh`（CI 和 release workflow 都跑） |
| `.github/workflows/release.yml` | 跑檢查和建置；tag 觸發時做 build provenance attestation、建 draft Release；PR 和手動觸發只建置、存成 workflow artifact |

## 規則

- 驗證：`gofmt -l .` 沒有輸出、`go vet ./...`、`go test ./...` 都過才算完成。
- 含套件名稱的字串常數只放 `internal/brand`。下面這行要沒有輸出：

  ```sh
  grep -rn '"[^"]*hearth' --include='*.go' . | grep -v '^./internal/brand/' | grep -v '"github.com/CTHua/hearth/'
  ```

- 測試只碰公開介面，不連網路。要用 git 的測試在 `TestMain` 呼叫 `gittest.Main`，遠端用 `gittest.BareRemote` 和 `gittest.Redirect`。
- 相依只有 `github.com/BurntSushi/toml`、讀 subagent frontmatter 的 `go.yaml.in/yaml/v3`、TUI 用的 Charm 套件（`charm.land/bubbletea/v2`、`charm.land/bubbles/v2`、`charm.land/lipgloss/v2`）和判斷 TTY 的 `github.com/charmbracelet/x/term`。新增或升級相依先討論。
- 設定檔格式、URL 規則和標記字串是對外的契約，改了就要把對應檔案的 `schema_version` 加一。- 模板（`template/home/`）不能提到 Python 腳本，也不能有機器路徑、真實的 GitHub URL 或真實 email；`internal/scaffold` 的測試會檢查。
- agent 的 MCP 設定（`.mcp.json`、`.codex/config.toml`）和 `.hearth/state.json` 只由 `hearth sync` / `hearth mcp sync` 寫到使用者的 Home Repo；`hearth init` 和模板不產生它們。被手改過的檔不覆寫也不刪，除非 `--force`；路徑上有 symlink 時一律不寫。秘密只寫環境變數名稱。
- subagent 的產出檔（`.claude/agents/<name>.md`、`.codex/agents/<name>.toml`）只由 `hearth sync` / `hearth subagent sync` 從 `subagents/` 寫到使用者的 Home Repo；`hearth init` 和模板不產生它們。規則同 MCP：被手改過的檔不覆寫也不刪，除非 `--force`；路徑上有 symlink 時一律不寫；沒記錄、也不在這次目標裡的檔不碰。
- skill 目錄（`vendor/skills/`、`.hearth/`、`.claude/skills/`、`.agents/skills/`）只由 `hearth sync` / `hearth skill sync` 寫到使用者的 Home Repo；`hearth init` 和模板不產生它們，`sync` 只刪自己建的 link。
- 發布：產物檔名 `hearth_<os>_<arch>.tar.gz`、`SHA256SUMS` 的格式（`<64 個 hex><兩個空白><檔名>`，照檔名排序）、`install.sh` 的三個環境變數（`HEARTH_VERSION`、`HEARTH_INSTALL_DIR`、`HEARTH_BASE_URL`）是對外的契約，第一個 Release 之後改了就會弄壞別人的安裝。發版步驟：在 Actions 手動觸發 `release` 做 dry-run → 打 `v<MAJOR.MINOR.PATCH>` tag 並 push → 檢查 draft Release 的產物 → 按 Publish → 確認發布出去的 Release 有 5 個產物，再跑一次 `install.sh`。`main` 上的 `install.sh` 一改就影響所有新安裝的人。
- 未經要求不 commit、不 push、不打 tag、不發 Release。

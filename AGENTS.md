# hearth 開發規則

給在這個 repo 工作的工程師和 AI coding agents。`CLAUDE.md` 只載入這份檔案。

## 結構

| 路徑 | 內容 |
| --- | --- |
| `cmd/hearth/main.go` | 入口，只做參數分派 |
| `internal/cli` | 子指令和參數解析（標準庫 `flag`） |
| `internal/config` | `hearth.toml`（`[[repos]]`、`[[skills]]`）、`hearth.local.toml`（`protocol`、`agents`、`skills`）的讀寫、驗證、版本 gate、根目錄搜尋 |
| `internal/giturl` | 三種 URL 寫法的解析和 ssh／https 轉換 |
| `internal/gitx` | 拿掉繼承的 git local env vars 後執行 git |
| `internal/checkout` | 只讀檢查一個 checkout 的狀態；路徑上的 symlink 檢查和 checkout root 檢查也給 `internal/skill` 用 |
| `internal/clone` | clone 一個產品，或確認既有的 checkout |
| `internal/skill` | skill 的三種來源、狀態、detached-commit 的安裝和換 pin、各 agent 的 link；內建 skill 的 `SKILL.md` 在 `internal/skill/builtin/`（go:embed） |
| `internal/dirswap` | 換掉非空目錄，中途失敗時把原本的搬回來 |
| `internal/detect` | 用 `git ls-remote` 偵測 protocol |
| `internal/scaffold` | `hearth init` 和標記區段檢查 |
| `internal/apperr` | 錯誤類別、錯誤格式、exit code |
| `internal/brand` | 所有含套件名稱的字串 |
| `template/home/` | `hearth init` 寫出的 Home Repo 模板（go:embed） |

## 規則

- 驗證：`gofmt -l .` 沒有輸出、`go vet ./...`、`go test ./...` 都過才算完成。
- 含套件名稱的字串常數只放 `internal/brand`。下面這行要沒有輸出：

  ```sh
  grep -rn '"[^"]*hearth' --include='*.go' . | grep -v '^./internal/brand/' | grep -v '"github.com/CTHua/hearth/'
  ```

- 測試只碰公開介面，不連網路。要用 git 的測試在 `TestMain` 呼叫 `gittest.Main`，遠端用 `gittest.BareRemote` 和 `gittest.Redirect`。
- 相依只有 `github.com/BurntSushi/toml`。新增或升級相依先討論。
- 設定檔格式、URL 規則和標記字串是對外的契約，改了就要把對應檔案的 `schema_version` 加一。
  - 例外：第一次發布前只有開發者自己用（2026-11-17 定），P2 加的 `[[skills]]`、`agents`、`skills` 直接加進 `schema_version = 1`，不寫遷移。發布之後這條例外失效。
- 模板（`template/home/`）不能提到 Python 腳本，也不能有機器路徑、真實的 GitHub URL 或真實 email；`internal/scaffold` 的測試會檢查。
- 不產生 agent 的自動載入設定：`.mcp.json`、`.codex/config.toml`。
- skill 目錄（`vendor/skills/`、`.hearth/`、`.claude/skills/`、`.agents/skills/`）只由 `hearth skill sync` 寫到使用者的 Home Repo；`hearth init` 和模板不產生它們，`sync` 只刪自己建的 link。
- 未經要求不 commit、不 push、不打 tag、不發 Release。

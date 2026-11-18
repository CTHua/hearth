# hearth

`hearth` 管理工程團隊的 Home Repo：團隊共用的規則、記憶、workflows，加上一份產品 repo 清單（`hearth.toml`）。每個人照自己的 ssh／https 偏好，把產品 clone 到 `products/`。

## 安裝

```sh
go install github.com/CTHua/hearth/cmd/hearth@latest
```

或從原始碼建置（Go 版本見 `.tool-versions`）：

```sh
go build -o bin/hearth ./cmd/hearth
```

支援 macOS、Linux 的 amd64、arm64。沒有用 ldflags 注入版本的 build，`hearth version` 印 `dev`；發布用的 build 這樣注入：

```sh
go build -ldflags "-X github.com/CTHua/hearth/internal/version.Version=0.1.0" -o bin/hearth ./cmd/hearth
```

## 用法

```sh
hearth init team-home                         # 在不存在或空的目錄建立 Home Repo，跑 git init，不 commit
cd team-home
hearth repo add git@git.example.invalid:acme/web-app.git   # name、path、branch 都有預設
hearth config protocol https                  # 個人設定，寫進 hearth.local.toml
hearth clone                                  # clone 還沒 clone 的產品
hearth repo list                              # 每個產品的狀態：ok、missing、remote-mismatch、path-conflict
hearth repo fix-remote                        # origin 只差協定時改成預期的 URL
hearth                                        # 在終端機裡開 TUI；不是終端機或不在 Home Repo 時印 help、exit 0
hearth ui                                     # 一定開 TUI；stdin、stdout 不是終端機時 exit 2
hearth upgrade [--check]                      # 換新版 hearth 後更新 AGENTS.md、.gitignore 的標記區段和設定檔的 schema_version；--check 只列出、有要改時 exit 1
hearth doctor                                 # 檢查 git、設定檔、標記區段、每個 checkout、選到的 skill、MCP server 和 subagent
```

skill：

```sh
hearth skill add https://git.example.invalid/acme/skills.git --subdir pdf   # 團隊檔；沒給 --commit 時釘在遠端 HEAD
hearth skill update pdf [--commit <sha>]      # 換 pin
hearth skill remove pdf                       # 只改團隊檔
hearth config agents claude,codex             # 個人檔；none 清空
hearth skill enable pdf                       # 個人檔；團隊在 skills/<name>/ 寫的 skill 也能選
hearth skill disable pdf                      # 也能清掉已被移除的名稱
hearth skill sync                             # clone 到 vendor/skills/、寫內建 skill、建 .claude/skills/、.agents/skills/ 的 link
hearth skill list                             # 只讀、不連網
```

MCP server：

```sh
hearth mcp add docs --env-var DOCS_TOKEN -- npx -y @acme/docs-mcp           # stdio；團隊檔
hearth mcp add tracker --url https://mcp.example.invalid/mcp \
  --bearer-token-env TRACKER_TOKEN --header-env 'X-Workspace=TRACKER_WORKSPACE'  # http；團隊檔
hearth mcp remove tracker                     # 只改團隊檔
hearth mcp enable docs                        # 個人檔
hearth mcp disable docs                       # 也能清掉已被移除的名稱
hearth mcp sync [--force]                     # 照選的 agent 寫 .mcp.json（claude）、.codex/config.toml（codex）
hearth mcp list                               # 只讀；列出缺的環境變數名稱
hearth sync [--force]                         # skill sync、mcp sync，再 subagent sync；--force 給 MCP 和 subagent
```

subagent：

```sh
hearth subagent enable reviewer               # 個人檔；定義是團隊在 subagents/<name>.md 寫的檔
hearth subagent disable reviewer              # 也能清掉已被移除的名稱
hearth subagent sync [--force]                # 照選的 agent 寫 .claude/agents/<name>.md（claude）、.codex/agents/<name>.toml（codex）
hearth subagent list                          # 只讀；列出狀態和不合格的原因
```

- 一份 `subagents/<name>.md`（Markdown 加 YAML frontmatter，`claude:`、`codex:` 放各工具的欄位）同時轉給兩個工具。格式見產生的 Home Repo 的 README。
- Codex 端還沒實測生效：`.codex/agents/<name>.toml` 照 Codex 官方文件產生，2026-11-18 用 Codex 0.162.1 試時，指名的角色沒有套用定義。
- 手改、symlink、`--force` 的規則跟 MCP 的產生檔相同，記錄也在同一個 `.hearth/state.json`；你自己放在 `.claude/agents/` 的其他檔不會被碰。
- 從舊版升級的 Home Repo：`hearth upgrade` 會把 `/.claude/agents/` 加進 `.gitignore` 的標記區段，但不會搬已經進版控的 `.claude/agents/subagent.md`。手動搬：
  1. 把它改寫成 `subagents/subagent.md`：`name`、`description` 留在頂層，`tools`、`model`、`effort` 搬到 `claude:` 底下，內文不變。
  2. `git rm .claude/agents/subagent.md`，連工作目錄的檔一起刪。
  3. `hearth subagent enable subagent`，再 `hearth sync`。

- 秘密只能寫環境變數名稱（`env_vars`、`bearer_token_env`、`header_envs`），沒有地方放值。
- 產生的檔案的 SHA-256 記在 `.hearth/state.json`；被手改過的檔 `sync` 不覆寫、不刪，報 `generated-modified`、exit 1，其他檔照常更新；`--force` 才覆寫。
- 環境變數沒設、`command` 不在 PATH 時，`sync` 和 `doctor` 只警告（`env-missing`），只印名稱。

TUI（`hearth` 或 `hearth ui`）：

- Mine 分頁改 `hearth.local.toml`：protocol、agent、每個 skill、MCP server 和 subagent 的開關（不合格的 subagent 不能打開），`s` 跑 `hearth sync`。
- team 分頁（Repos、Skills、MCP）改 `hearth.toml`：`a` 新增、`d` 按兩次移除；Skills 只列 external skill，`u` 換 pin。新增 MCP server 的表單只收 command、url、env vars；http 的 token、header 用 CLI。
- team 分頁的頁首列出 `hearth.toml` 跟 HEAD 比的改動數和被移除的名字，有改動的列標 `+`（新增）或 `~`（內容不同）。
- `?` 開快捷鍵表；終端機小於 80×24 時只顯示放大視窗的提示。
- 滑鼠：點分頁、列、Mine 的勾選框和底部說明列的鍵，效果跟按鍵相同，移除一樣要確認兩次；滾輪移動游標；表單或快捷鍵表開著時點外面等於 `esc`。tmux 裡要 `set -g mouse on`。
- `y` 用 OSC 52 把選到那列的 URL（沒有就送狀態訊息或目前的提示）送到剪貼簿；終端機不支援 OSC 52 時不會生效。
- `m` 關掉滑鼠、把拖曳選字還給終端機，右上角顯示 `mouse off`，再按一次恢復。不關也可以按住修飾鍵選字：iTerm2 是 Option，Ghostty、WezTerm 是 Shift。

- `hearth upgrade` 只改標記之間的內容和設定檔的 `schema_version`，標記外逐 byte 不變；標記區段被手改過也整段重寫。標記缺少、重複或順序錯時 exit 2、什麼都不寫。`hearth.local.toml` 不存在就跳過。`hearth doctor` 在標記區段跟目前版本不同時只警告。
- 所有指令從目前目錄往上找 `hearth.toml` 決定根目錄，也可以用 `-C <dir>`。
- protocol 沒設時，`hearth clone` 和沒給 `--branch` 的 `hearth repo add` 會實際用 `git ls-remote` 試 ssh 再試 https，把結果寫進 `hearth.local.toml`。
- 給了 `--commit` 的 `skill add`、`skill update` 不連網，protocol 沒設也不偵測；偵測延到 `skill sync`。
- `agent-missing`（agent CLI 不在 PATH）只是警告，不影響 exit code。
- exit code：0 成功；1 有產品、skill 或產生的檔案失敗；2 設定、用法、環境或版本錯誤；130 中斷。
- 錯誤格式：`error [<類別>] <說明>`，跟單一產品有關的是 `error [<類別>] <name>: <說明>`。

產生的 Home Repo 的 README 有完整的規則和錯誤類別說明，來源在 [`template/home/README.md`](template/home/README.md)。

## 開發

```sh
gofmt -l .
go vet ./...
go test ./...
```

- 測試不連網路：遠端是暫存的 bare repo，URL 用 git 的 `insteadOf` 導到本機；全域設定換成暫存檔，只允許 file 協定。
- `internal/brand` 以外的 Go 原始碼不能有含套件名稱的字串常值，見 [`AGENTS.md`](AGENTS.md)。
- Python 參考版測試的對照表在 [`internal/cli/testdata/bootstrap-parity.md`](internal/cli/testdata/bootstrap-parity.md)（`bootstrap`）和 [`internal/cli/testdata/setup-parity.md`](internal/cli/testdata/setup-parity.md)（`setup`）。

## 授權

MIT，見 [`LICENSE`](LICENSE)。

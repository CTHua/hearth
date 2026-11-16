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
hearth doctor                                 # 檢查 git、設定檔、標記區段、每個 checkout
```

- 所有指令從目前目錄往上找 `hearth.toml` 決定根目錄，也可以用 `-C <dir>`。
- protocol 沒設時，`hearth clone` 和沒給 `--branch` 的 `hearth repo add` 會實際用 `git ls-remote` 試 ssh 再試 https，把結果寫進 `hearth.local.toml`。
- exit code：0 成功；1 有產品失敗；2 設定、用法、環境或版本錯誤；130 中斷。
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
- Python 參考版測試的對照表在 [`internal/cli/testdata/bootstrap-parity.md`](internal/cli/testdata/bootstrap-parity.md)。

## 授權

MIT，見 [`LICENSE`](LICENSE)。

# Agents

把任務交給另一個 agent（另一個 coding agent、subagent 或同事的 agent session）時用的 prompt 模板。

## 這個目錄不會被自動發現

- 沒有任何 agent 工具會自動讀 `agents/`。它只是一般資料夾，要用時自己打開模板、複製內容、填好再貼給 agent。
- 有些工具有自己的自動載入位置，例如 Claude Code 的 `.claude/agents/`、Codex 的 `.codex/agents/`。subagent 定義寫在 [`../subagents/`](../subagents/)，每個人用 `hearth subagent enable <name>` 選、跑 `hearth sync` 後才產生到那兩個位置，見下面的「模型設定」；這個目錄的模板不會被搬過去。
- 共同規則仍然只寫在 [`AGENTS.md`](../AGENTS.md)。模板只放「這次要交辦什麼」，不複製規則。

## 有哪些模板

| 檔案 | 什麼時候用 |
| --- | --- |
| [`_task-brief.md`](_task-brief.md) | 把一個開發或 review 任務交給另一個 agent |
| [`models.json`](models.json) | 查主對話和 subagent 預設用哪個模型、哪個 effort |

## 模型設定（Claude Code、Anthropic API）

只涵蓋 Claude Code 直接連 Anthropic API 的情況。

| 檔案 | 誰會讀 | 內容 |
| --- | --- | --- |
| [`models.json`](models.json) | 沒有程式讀，給人看的紀錄 | 每個角色的 provider、alias、model ID、effort |
| [`../.claude/settings.json`](../.claude/settings.json) | Claude Code | 只有 `env`：`CLAUDE_CODE_SUBAGENT_MODEL=sonnet` |
| [`../subagents/subagent.md`](../subagents/subagent.md) | `hearth sync` 轉成 Claude Code 的 `.claude/agents/subagent.md` | 通用 subagent，`claude:` 底下 `model: sonnet`、`effort: medium`；預設不開，要用的人 `hearth subagent enable subagent` |

這三個檔案要保持一致：settings 的 `env` 不放憑證或其他 provider 的開關，subagent 的 `model` 和 `effort` 跟 `models.json` 相同。改了其中一個，另外兩個要一起改，在 PR review 時對照。

### 預設值

| 角色 | alias | 2026-11-16 解析成 | effort |
| --- | --- | --- | --- |
| main | `opus` | `claude-opus-5-5` | `medium` |
| subagent | `sonnet` | `claude-sonnet-5-5` | `medium` |

- 這些是可以換掉的預設，不保證永遠可用。
- alias 在 Anthropic API 上會解析成最新版；`model` 欄位只記錄查的那天解析出來的 ID，新版推出後兩者會不同。
- 要固定版本：把 `.claude/settings.json` 和 `subagents/subagent.md` 的值改成完整 model ID（例如 `claude-sonnet-5-5`），再同步改 `models.json`。

### 主對話的模型不寫死

`.claude/settings.json` 不設 `model`，主對話用哪個模型由啟動的人決定：

```sh
claude --model opus --effort medium
```

或在 session 裡用 `/model`、`/effort` 切換。注意 `/model` 會把選擇寫進你個人的 user settings，變成之後新 session 的預設。

### subagent 用哪個模型

Claude Code 依下面的順序，取第一個有設定的：

1. 主對話叫用 subagent 時傳的 `model` 參數（只能是 alias）
2. subagent 定義的 `model` 欄位
3. `CLAUDE_CODE_SUBAGENT_MODEL`
4. 主對話目前的模型

所以：

- 沒指定 `model` 的 subagent 會跟著主對話的模型走，`/model` 一切換就跟著變。`subagent.md` 因此明確寫 `model: sonnet`。
- `CLAUDE_CODE_SUBAGENT_MODEL` 只影響沒有自己 `model` 的 subagent，例如 built-in 的 general-purpose；它單獨設定時改不了 built-in 的 Explore 和 Plan，這兩個照主對話的模型。
- 主對話仍然可以在叫用時傳 `model` 參數蓋過 `subagent.md`。要強制所有 subagent 都用同一個模型，在自己的設定加 `CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1`；這個模板沒有加。

### 不涵蓋的

- Amazon Bedrock、Google Vertex AI、Microsoft Foundry、LLM gateway：alias 在這些平台可能解析成別的版本，這份設定沒有測，也不處理。
- Codex、Grok 和其他 agent 工具：各自用自己的設定檔，這份設定控制不到它們。
- 憑證：API key、token、base URL 不寫進 `.claude/settings.json`，它會進版控。放在自己的環境變數，或被 `.gitignore` 忽略的 `.claude/settings.local.json`。

官方文件：[Subagents](https://code.claude.com/docs/en/sub-agents)、[Model configuration](https://code.claude.com/docs/en/model-config)、[Environment variables](https://code.claude.com/docs/en/env-vars)。

檔名以 `_` 開頭的是模板，不是內容。

## 怎麼新增

1. 複製 `_task-brief.md` 成新的 `_<name>.md`，或直接修改它。
2. 只寫交辦時一定要給的欄位；共同規則用連結指到 `AGENTS.md`。
3. 拿一個真實任務實際用一次，確認收到的 agent 不用再問就能開始。
4. 開 PR。

## 不放這裡的

- 秘密、憑證、token、客戶個資。
- 只適用單一產品的 prompt：放那個產品的 repo。

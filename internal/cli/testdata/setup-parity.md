# Python `tests/test_setup.py` 對照表

來源：home-repo-template commit `0434bc1` 的 `tests/test_setup.py`，共 26 個測試。每個測試列出 Go 裡對應的測試，或標「刻意不同」並對到 P2 spec「替你決定的事」的哪一項：

- D1：團隊檔加 `[[skills]]`，四個 key，checkout 位置固定是 `vendor/skills/<name>`，不另存 `path`
- D2：個人檔加 `agents`、`skills`，不存在代表空陣列
- D3：agent 只收 `claude`、`codex`
- D4：個人檔的 `skills` 只查格式；不存在的名稱是失效選擇
- D5：換 pin：三個條件都成立才自動換，否則 `pin-mismatch`
- D6：`agent-missing` 只是警告
- D7：內建 skill 只要選了 agent 就裝

Python 的 `status` 對應 `hearth skill list` 和 `hearth doctor`，`init` 對應 `hearth skill sync`。Go 測試的位置用 `套件/測試名` 表示，套件在 `internal/` 底下；沒寫套件的是 `cli`。

| Python 測試 | Go 對應 | 備註 |
| --- | --- | --- |
| `test_01_missing_config_is_config_error_with_hint` | 刻意不同（D2） | 沒有個人檔等於什麼都沒選，`sync` 印 `Nothing enabled` 並 exit 0：`TestSkillUsageErrors` |
| `test_01_usage_errors` | `TestSkillUsageErrors` | 沒有子指令、未知子指令、多餘參數都是 `usage`、exit 2 |
| `test_01_help` | `TestVersionHelpAndUnknownCommand` | `-h` 由共用的 `parseArgs` 處理，每個子指令都印自己的 usage |
| `test_02_nothing_enabled_is_ready` | `TestSkillUsageErrors`、`TestBuiltinSkillFollowsAgentChoice` | 沒選東西時不建 `vendor/`、`.agents/`、`.hearth/` |
| `test_03_status_reports_found_and_missing_clis` | `TestMissingAgentCLIsAreOnlyWarnings` | 刻意不同（D6、D3）：缺 CLI 只警告，exit 0；`opencode`、`pi` 不收 |
| `test_03_init_reports_missing_cli_without_installing_it` | `TestMissingAgentCLIsAreOnlyWarnings` | 刻意不同（D6）：exit 0，link 照樣建；不安裝 agent |
| `test_03_all_clis_present_is_ready` | `TestMissingAgentCLIsAreOnlyWarnings` | PATH 裡有 `claude` 時不報它 |
| `test_04_init_installs_pinned_commit_not_branch_tip` | `TestSyncInstallsPinnedCommitDetachedAndRerunDoesNotFetch` | 遠端 HEAD 是較新的 commit，裝到的是 pin；檢查 detached、相對 link、兩個 agent 都建、不留暫存目錄 |
| `test_05_rerun_does_not_clone_or_fetch` | `TestSyncInstallsPinnedCommitDetachedAndRerunDoesNotFetch` | 遠端前進後重跑，`origin/main` 不變 |
| `test_06_status_does_not_install_or_link` | `TestListAndDoctorNeitherConnectNorWrite`、`TestBuiltinSkillFollowsAgentChoice` | `list` 照 P1 `repo list` 的慣例 exit 0，只報 `not-installed`；比對執行前後整個檔案樹，`GIT_TRACE` 裡沒有連網的 git 指令 |
| `test_07_pin_change_is_reported_until_checkout_is_removed` | `TestSyncMovesToNewPinWhenNothingIsLost`、`TestSyncKeepsCheckoutThatHoldsLocalState` | 刻意不同（D5）：沒有本機狀態時自動換到新 pin；有未 commit 改動、ignored 檔、本機 commit 或在分支上時才報 `pin-mismatch`，舊 checkout 逐 byte 不變。換到一半失敗時還原：`dirswap/TestFailedMoveRestoresOriginal` |
| `test_08_unknown_commit_is_clone_failure_without_leftovers` | `TestUnknownCommitIsCloneFailedWithoutLeftovers` | |
| `test_09_skill_dir_must_hold_skill_md_inside_checkout` | `TestSubdirMustHoldSkillFileInsideCheckout` | 另加 subdir 不存在的情況 |
| `test_10_protocol_applies_to_skill_source` | `TestProtocolAppliesToSkillURL`、`TestSyncDetectsProtocolOnlyWhenItClones` | 任何 host 都轉換（P1 決策）；protocol 沒設時 `sync` 偵測，並保留個人檔的其他 key |
| `test_11_user_files_are_never_overwritten` | `TestUserFilesAndForeignLinksAreNeverTouched` | 另驗清 link 時也不碰它們 |
| `test_12_missing_and_retargeted_links_are_restored` | `TestListReportsBrokenLinksAndSyncRepairsThem` | 兩個 agent 的狀態取最嚴重的，detail 寫路徑 |
| `test_13_disabled_tools_and_skills_lose_only_managed_links` | `TestDisablingRemovesOnlyManagedLinks` | 另驗 `list` 報 `disabled`、`doctor` 不提 |
| `test_14_symlinked_entry_directory_is_rejected` | `TestPathConflictsOnSkillDirectories` | 也涵蓋 `vendor/skills`、`.hearth` 是 symlink，`vendor`、`.agents` 是檔案 |
| `test_15_unavailable_symlinks_fail_without_copying` | `skill/TestOnlyUnsupportedSymlinkErrorsAreSymlinkUnavailable`、`TestUnwritableLinkDirIsPathConflictWithoutCopying` | 不 mock `os.Symlink`。Python 把所有 symlink 錯誤都歸 `symlink-unavailable`；這裡只有 ENOTSUP、EOPNOTSUPP、EPERM 算，用判斷錯誤類別的函式在單元層級驗。其他錯誤（例如唯讀目錄的 EACCES）報 `path-conflict` 附 OS 錯誤，兩種都不改用複製 |
| `test_16_invalid_skills_json_is_config_error` | `config/TestSkillValueRulesNameFileAndLocation`、`TestSkillAddRejectsBadValuesAndConflicts` | 刻意不同（D1）：沒有 `path` key，寫了就是 `unknown key`；`source` 改名為 `url`，只收三種 URL 寫法 |
| `test_16_enabled_skill_must_exist_in_skills_json` | `TestRemovedSkillIsAStaleChoice`、`config/TestPersonalListRules` | 刻意不同（D4）：不存在的名稱不是 config 錯誤，`list`、`sync`、`doctor` 警告 `unknown`，`skill disable` 可移除 |
| `test_16_missing_skills_json_is_config_error` | 刻意不同（D1） | 外部 skill 在 `hearth.toml` 裡，沒有獨立的檔案；`hearth.toml` 不存在的情況：`TestCloneWithMissingTeamFileIsConfigError`、`TestOutsideHomeRepoIsConfigError` |
| `test_16_invalid_personal_config_is_config_error` | `config/TestPersonalListRules` | `agents = ["cursor"]` 報 `hearth.local.toml: agents[0]: ...` |
| `test_17_generated_and_personal_files_stay_untracked` | `TestSyncLeavesHomeRepoClean`、`scaffold/TestGitignoreBlockIgnoresSkillOutputs` | |
| `test_18_old_python_is_env_error_without_traceback` | 刻意不同 | P2「替你決定的事」沒有對應項：沒有 Python 直譯器，沿用 P1 對照表 9d 的理由；版本保護是 `schema_version`、`min_hearth_version` |
| `test_18_real_old_interpreter_is_env_error` | 刻意不同 | 同上 |

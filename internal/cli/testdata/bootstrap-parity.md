# Python `tests/test_bootstrap.py` 對照表

來源：home-repo-template commit `0434bc1` 的 `tests/test_bootstrap.py`，共 52 個測試。每個測試列出 Go 裡對應的測試，或標「刻意不同」並對到 spec 驗收條件 9 的哪一項：

- 9a：URL 只收三種寫法，但任何 host 都轉換
- 9b：protocol 沒設時改成偵測
- 9c：根目錄改成往上找 `hearth.toml`
- 9d：設定檔改成 TOML

Go 測試的位置用 `套件/測試名` 表示，套件在 `internal/` 底下。

| Python 測試 | Go 對應 | 備註 |
| --- | --- | --- |
| `test_01_first_clone_uses_configured_branch_and_origin` | `cli/TestCloneUsesConfiguredBranchNotTagAndSetsOrigin` | 同名的 tag 和 branch 指到不同 commit，clone 到的是 branch；URL 改用 https 寫法導到本機（9a） |
| `test_02_rerun_skips_and_leaves_checkout_untouched` | `cli/TestRerunSkipsAndLeavesCheckoutUntouched` | |
| `test_03_single_name_clones_only_that_repo` | `cli/TestCloneSelectsNamesInTeamFileOrder` | 最後一段 `clone b` |
| `test_03_multiple_names_clone_only_those_repos` | `cli/TestCloneSelectsNamesInTeamFileOrder` | |
| `test_03_unknown_name_is_usage_error_without_cloning` | `cli/TestCloneUnknownNameIsUsageErrorWithoutCloning` | |
| `test_22_duplicate_and_reordered_names_follow_config_order` | `cli/TestCloneSelectsNamesInTeamFileOrder` | |
| `test_04_non_git_directory_is_path_conflict` | `cli/TestNonGitDirectoryOutsideAnyRepoIsPathConflict`、`checkout/TestPathConflicts` | |
| `test_04_regular_file_target_is_path_conflict` | `cli/TestExistingTargetsThatAreNotCheckouts` | |
| `test_05_plain_directory_inside_git_home_is_path_conflict` | `cli/TestExistingTargetsThatAreNotCheckouts` | `hearth init` 產生的 Home Repo 本身就是 git repo |
| `test_06_origin_mismatch_is_reported_and_not_changed` | `cli/TestOriginMismatchIsReportedAndNotChanged` | |
| `test_06_missing_origin_is_remote_mismatch` | `cli/TestOriginMismatchIsReportedAndNotChanged`、`checkout/TestStates` | |
| `test_07_missing_config_file_is_config_error` | `cli/TestCloneWithMissingTeamFileIsConfigError`、`cli/TestOutsideHomeRepoIsConfigError` | |
| `test_07_invalid_utf8_config_is_config_error` | `cli/TestCloneConfigErrorsCloneNothing` | 9d：TOML 解析器拒絕不合法的 UTF-8 |
| `test_07_invalid_config_shapes_are_config_errors` | `config/TestUnknownKeysAreConfigErrorsWithLocation`、`config/TestVersionGate`、`cli/TestCloneConfigErrorsCloneNothing` | 9d：JSON 形狀換成 TOML 的對應情況（語法錯、未知 key、缺 key、非字串、`repos` 不是陣列） |
| `test_08_duplicate_names_and_paths_are_config_errors` | `config/TestDuplicateAndNestedEntriesAreCaseInsensitive`、`config/TestWithRepoRejectsConflictsAndWithoutRemoves` | |
| `test_09_path_escape_is_config_error` | `config/TestRepoValueRules` | |
| `test_09_relative_url_is_config_error` | `config/TestRepoValueRules`、`giturl/TestParseRejectsUnconvertibleURLs` | 9a：相對路徑和所有本機路徑都拒絕 |
| `test_16_option_and_shell_injection_values_are_config_errors` | `config/TestRepoValueRules`、`cli/TestCloneConfigErrorsCloneNothing` | |
| `test_23_all_config_errors_are_listed` | `cli/TestCloneConfigErrorsCloneNothing`、`config/TestDuplicateAndNestedEntriesAreCaseInsensitive` | 位置寫成 `repos[0].path:`，跟 Python 版一樣從 0 起算 |
| `test_18_empty_list_prints_message_only` | `cli/TestCloneEmptyListPrintsMessageOnly` | |
| `test_10_products_symlink_is_rejected` | `cli/TestProductsSymlinkIsRejected`、`checkout/TestSymlinkedProductsDirIsRejected` | |
| `test_10_intermediate_symlink_is_rejected` | `cli/TestSymlinksOnThePathAreRejected`、`checkout/TestPathConflicts` | |
| `test_10_target_symlink_to_outside_checkout_is_rejected` | `cli/TestSymlinksOnThePathAreRejected` | |
| `test_10_broken_products_symlink_is_rejected` | `cli/TestProductsSymlinkIsRejected`、`checkout/TestSymlinkedProductsDirIsRejected` | |
| `test_11_clone_failures_leave_no_target_or_temp_dir` | `cli/TestCloneFailuresLeaveNoTargetOrTempDir` | |
| `test_12_rerun_after_fixing_remote_clones` | `cli/TestRerunAfterFixingRemoteClones` | |
| `test_19_file_as_parent_fails_only_that_repo` | `cli/TestFileAsParentFailsOnlyThatRepo` | |
| `test_13_mixed_results_continue_after_failure` | `cli/TestMixedResultsAndOutputFormat` | |
| `test_24_output_format` | `cli/TestMixedResultsAndOutputFormat` | 逐行比對 stdout |
| `test_14_runs_from_root_directory` | `cli/TestCloneFromOtherDirectories` | 9c：cwd 是 `/` 時用 `-C <home>` |
| `test_14_runs_from_other_directory` | `cli/TestCloneFromOtherDirectories`、`cli/TestRootIsFoundFromInsideProducts` | 9c：從 `products/<name>/` 底下往上找 |
| `test_14_runs_via_relative_path` | `cli/TestRelativeDirectoryOption` | 9c：根目錄看 cwd 或 `-C`，不看執行檔的位置；對應的是用相對路徑的 `-C` 從上層目錄執行 |
| `test_14_runs_via_symlink` | 刻意不同（9c） | 同上，執行檔的路徑跟根目錄無關 |
| `test_15_outer_git_ignores_products` | `cli/TestHomeRepoGitIgnoresProducts`、`scaffold/TestGitignoreBlockIgnoresProductsAndPersonalFile` | |
| `test_17_inherited_git_dir_does_not_leak` | `cli/TestCloneInheritedGitDirDoesNotLeak`、`cli/TestInheritedGitDirDoesNotLeak`、`gitx/TestInheritedGitDirDoesNotLeak` | |
| `test_16_validated_url_is_passed_literally` | 刻意不同（9a） | 含 `;`、`$(` 的 URL 在驗證就被拒絕（`giturl/TestParseRejectsUnconvertibleURLs`），不會送到 git |
| `test_20_ext_transport_is_blocked` | `cli/TestCloneExtTransportIsBlocked`、`gitx/TestExtTransportIsBlocked` | `ext::` 透過 `insteadOf` 導過去，因為 URL 本身不收 `ext::`（9a） |
| `test_21_missing_git_is_env_error` | `cli/TestCloneWithoutGitIsEnvError`、`gitx/TestMissingGitIsEnvError`、`cli/TestDoctorWithoutGitExitsTwo` | |
| `test_25_script_is_directly_executable` | `cli/TestVersionHelpAndUnknownCommand` | 執行檔由 `go build` 產生；保留的部分是 `-h`、`--help`、`help` 印 usage 並 exit 0 |
| `test_30_old_python_is_env_error_without_traceback` | 刻意不同（9d） | 沒有 Python 直譯器，也就沒有直譯器版本。對應的版本保護是 `min_hearth_version`、`schema_version`：`config/TestVersionGate`、`cli/TestEveryGatedCommandHonoursVersionGate` |
| `test_30_real_old_interpreter_is_env_error` | 刻意不同（9d） | 同上 |
| `test_26_https_protocol_converts_github_ssh_url` | `cli/TestCloneConvertsURLToPersonalProtocol`、`giturl/TestExpectedConvertsAndKeepsHostAndPath` | 9a：任何 host 都轉換，不限 github.com |
| `test_26_ssh_protocol_converts_github_https_url` | `cli/TestCloneConvertsURLToPersonalProtocol` | |
| `test_26_ssh_scheme_url_converts_to_https` | `cli/TestCloneConvertsURLToPersonalProtocol` | |
| `test_26_non_github_url_is_used_as_written` | 刻意不同（9a） | 其他 host 也轉換（`giturl/TestExpectedConvertsAndKeepsHostAndPath`）；轉不了的 URL 一開始就拒絕 |
| `test_27_existing_checkout_with_other_protocol_is_left_alone` | `cli/TestProtocolChangeReportsMismatchAndLeavesCheckout` | 訊息寫的是 `protocol = https in hearth.local.toml`，不是 `github.protocol`（9d） |
| `test_27_matching_protocol_skips_existing_checkout` | `cli/TestProtocolChangeReportsMismatchAndLeavesCheckout` | 最後一段 |
| `test_28_missing_config_clones_urls_as_written_with_hint` | 刻意不同（9b） | 改成偵測並寫進個人檔：`cli/TestCloneDetectsProtocolWhenUnset`、`cli/TestCloneDetectionFailureClonesAndWritesNothing` |
| `test_28_empty_repos_without_config_prints_nothing_else` | `cli/TestCloneEmptyListPrintsMessageOnly` | 不偵測、不寫個人檔；dev build 在 stderr 多一行 `min_hearth_version` 的警告 |
| `test_28_existing_config_prints_no_hint` | `cli/TestCloneWithProtocolSetDoesNotDetect` | 9b |
| `test_29_invalid_personal_config_is_config_error` | `cli/TestInvalidPersonalFileFailsEvenWithEmptyList`、`config/TestPersonalFileProtocolRules`、`config/TestUnknownKeysAreConfigErrorsWithLocation` | 9d：P1 時 `[agents]`、`[skills]` 段落算未知 key；P2 起 `agents`、`skills` 是字串陣列，寫成段落報「must be an array of strings」（`config/TestPersonalListRules`），一樣是 config 錯誤 |
| `test_29_invalid_personal_config_fails_even_with_empty_repos` | `cli/TestInvalidPersonalFileFailsEvenWithEmptyList` | |

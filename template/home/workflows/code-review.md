# Code review 流程

1. 確認 review 的對象：哪個 repo、哪個 PR 或哪段 diff、base 是什麼。在正確的 repo 裡看 diff。
2. 了解改動的目的：讀 PR 說明和相關的 issue 或需求。目的不清楚時先問，不猜。
3. 讀該產品 repo 的 `AGENTS.md` 或 `CLAUDE.md`，以它的規範為準。跨產品的部分對照 Home Repo 的 `memory/decisions/`。
4. 看 diff，需要時讀呼叫端和相關測試。檢查：
   - 行為是否符合目的，有沒有弄壞既有行為
   - 錯誤處理、權限、輸入驗證、資料一致性
   - 測試是否真的驗到它宣稱的行為
   - 有沒有超出目的的改動
5. 能跑驗證就照產品 repo 的規範跑，寫明跑了什麼。沒跑的明講。
6. 回報問題。每一點附檔案和行號、會出錯的情境、建議的修法，並標出嚴重程度。沒把握的寫成疑問，不寫成結論。
7. Review 只提意見。除非被要求，不修改程式碼、不 commit、不 approve、不 merge。

改動跨多個 repo 時，另外對照 `cross-repo-change.md` 列的影響範圍和相依順序。

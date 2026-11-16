# 任務交辦模板

複製下面的區塊，填好角括號的欄位，再貼給接手的 agent。用不到的欄位寫「無」，不要刪掉，讓對方知道你有想過。

```text
先讀 Home Repo 根目錄的 AGENTS.md，照裡面的規則工作。

<task>
目標：<這次要做完的事，一兩句>
repo：<只能改的 repo，例如 products/<name>；其他 repo 只讀>
要改的範圍：<檔案或目錄>
不做的事：<明確排除的部分>
</task>

<context>
相關的決策或慣例：<memory/decisions/... 或 memory/conventions/... 的路徑，沒有就寫「無」>
相關的 skill：<skills/<name>/SKILL.md，沒有就寫「無」>
其他參考：<issue、PR、文件連結>
</context>

<acceptance>
1. <可以判斷真假的驗收條件>
2. <...>
</acceptance>

<verify>
<要在產品 repo 裡跑的驗證指令>
</verify>

<report>
回報改了哪些檔案、跑了哪些指令和結果、哪些沒跑和原因。
沒被要求就不 commit、不 push、不部署。
</report>
```

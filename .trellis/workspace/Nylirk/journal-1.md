# Journal - Nylirk (Part 1)

> AI development session journal
> Started: 2026-06-15

---



## Session 1: RAG Evaluation V2

**Date**: 2026-06-15
**Task**: RAG Evaluation V2

### Summary

Implemented persistent RAG evaluation datasets, samples, runs, results, extensible metrics, V1 projection, comparison APIs, migrations, tests, Chinese management UI, and API documentation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6d29cb78` | (see git log) |
| `f9c01780` | (see git log) |
| `40fcfe17` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: RAG Evaluation V2 CI and review fixes

**Date**: 2026-06-15
**Task**: RAG Evaluation V2 CI and review fixes
**Branch**: `codex/rag-evaluation-v2`

### Summary

Added focused RAG Evaluation CI, fixed Admin RBAC, V1 Go client projection parsing and text normalization, verified backend/frontend paths, synchronized branches, and committed local Docker/Trellis configuration.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `1ae40cd7` | (see git log) |
| `b16641cd` | (see git log) |
| `383e2bcd` | (see git log) |
| `630b4bb7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: Fix evaluation startup dependency ordering

**Date**: 2026-06-15
**Task**: Fix evaluation startup dependency ordering
**Branch**: `codex/rag-evaluation-v2`

### Summary

Registered TaskEnqueuer and EventManager before evaluation reconciliation, added sync/Redis dependency graph regression coverage, and enabled the test in evaluation CI.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6b0f2b15` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Fix frontend vendor chunk cycle

**Date**: 2026-06-16
**Task**: Fix frontend vendor chunk cycle
**Branch**: `codex/rag-evaluation-v2`

### Summary

Merged TDesign packages into the Vue vendor chunk, normalized manual chunk path matching, verified frontend tests and production build, and noted unrelated existing type-check failures.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2ea770f1` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: Align evaluation UI with knowledge base cards

**Date**: 2026-06-16
**Task**: Align evaluation UI with knowledge base cards

### Summary

Reworked evaluation UI into knowledge-base-style dataset cards, query-driven detail/history views, sample editing/import flows, run creation, and refined card/table context preview styling for PR #6.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `d1300a0e` | (see git log) |
| `e0e716a1` | (see git log) |
| `abb8a68a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: 实现题库 DOC/DOCX/PDF 文件导入与预览

**Date**: 2026-06-18
**Task**: 实现题库 DOC/DOCX/PDF 文件导入与预览
**Branch**: `feat/document-question-import`

### Summary

新增 DOC/DOCX/PDF 文件导入预览功能。

后端：新增 import-file/preview endpoint，规则版 QuestionExtractionService 文本抽题，直接调用 docreader 不经过 chunk pipeline，ImportQuestions 支持 caller-controlled status，文件大小限制 20MB。

前端：QuestionFileImportDialog 文件上传/解析预览/确认导入，遮罩层不可关闭，AbortController + requestId 防 stale response，Word/PDF 类型隔离，导入菜单受控关闭，nullable field 安全计算。

修复：弹窗状态隔离、X/取消可用、parseBlock 双返回值编译问题、预览超时 120s、空文本不报硬错。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `d324df99` | (see git log) |
| `51bab9e1` | (see git log) |
| `4bb3216e` | (see git log) |
| `f045a189` | (see git log) |
| `f934ddab` | (see git log) |
| `a0b23a7d` | (see git log) |
| `9453d9e3` | (see git log) |
| `4d05cc2e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: 修复选择题选项切分与展示：A-Z范围、顺序标签、答案展示

**Date**: 2026-06-18
**Task**: 修复选择题选项切分与展示：A-Z范围、顺序标签、答案展示
**Branch**: `fix/question-import-choice-parsing`

### Summary

在 fix/question-import-choice-parsing stacked PR 上修复 QuestionExtractionService 的选择题解析。

选项标签 A-D → A-Z，支持全角/半角标记（.．、）、：）。
inlineOptionPattern 加 boundary anchor 防误切 e.g./Node.js。
splitInlineOptions 和 splitStemInlineOptions 强制顺序标签（A→B→C→...），跳过非连续候选。
extractChoiceAnswerFromStem 从题干括号抽取答案并交叉校验 option labels。
appendOptionsToStem 生成展示用题干（含选项），expandChoiceAnswerText 将单字母答案展开为完整选项内容。
修复 Go regexp panic（全角字符反斜杠转义非法）、首行 inline options 未拆分、bracket fallback 无校验删除。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `28f0519a` | (see git log) |
| `6283d681` | (see git log) |
| `9e94c440` | (see git log) |
| `eb3128db` | (see git log) |
| `0814f91b` | (see git log) |
| `d49e9666` | (see git log) |
| `9e7fc668` | (see git log) |
| `e2b1d1ae` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete

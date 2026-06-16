# Fix Task Enqueuer Registration Order

## Goal

Prevent WeKnora app startup from panicking with `missing type: interfaces.TaskEnqueuer` when Evaluation run reconciliation resolves `EvaluationService` through `KnowledgeBaseService`.

## Requirements

- Register task-enqueue infrastructure before any `dig.Invoke` that can resolve `KnowledgeBaseService` or `EvaluationService`.
- Preserve Redis mode: when `REDIS_ADDR` is non-empty, provide `router.NewAsyncqClient` as `interfaces.TaskEnqueuer` and register `AsynqServer` plus inspectors.
- Preserve Lite/synchronous mode: when `REDIS_ADDR` is empty, register one `router.SyncTaskExecutor` as both `interfaces.TaskEnqueuer` and its concrete type, plus `NoopTaskInspector`.
- Keep `reconcileEvaluationRuns` behavior unchanged.
- Do not change task execution business logic.
- Add a minimal regression test that validates the complete `BuildContainer` dependency graph before runtime constructors execute.

## Acceptance Criteria

- [ ] `BuildContainer` registers `interfaces.TaskEnqueuer` before `reconcileEvaluationRuns` is invoked.
- [ ] A dry-run container build resolves the Evaluation reconciliation dependency chain in synchronous mode.
- [ ] A dry-run container build resolves the dependency graph in Redis mode.
- [ ] Related Go tests pass, or exact environment blockers and successful fallback verification commands are reported.
- [ ] The fix is committed as `fix: register task enqueuer before evaluation reconciliation`.

## Technical Approach

Move the existing task-enqueuer registration block earlier in `BuildContainer`, before service registration and all invokes that may transitively resolve evaluation or knowledge-base services. Keep the existing Redis branch and synchronous branch intact. Add a container test using `dig.New(dig.DryRun(true))` so the full dependency graph is validated without opening databases, Redis connections, or background servers.

## Out of Scope

- Changing queue behavior, task handlers, retry policy, inspectors, or Evaluation reconciliation logic.
- Refactoring unrelated container registration code.
- Adding external services to tests.

## Technical Notes

- Root cause location: `internal/container/container.go`, where `container.Invoke(reconcileEvaluationRuns)` currently precedes TaskEnqueuer registration.
- Dependency chain: `reconcileEvaluationRuns -> EvaluationService -> KnowledgeBaseService -> interfaces.TaskEnqueuer`.

# Fix Frontend Vendor Chunk Cycle

## Goal

Fix the production blank page caused by a suspected Rollup manual chunk initialization cycle between Vue and TDesign vendor chunks.

## Requirements

* Update `frontend/vite.config.ts` only unless verification exposes a directly related issue.
* Do not split `tdesign-vue-next` into a separate `vendor-tdesign` chunk.
* Put `tdesign-vue-next` and `tdesign-icons-vue-next` into the existing `vendor-vue` chunk.
* Normalize module ids with `id.replaceAll('\\', '/')` before chunk matching so Windows paths behave like POSIX paths.
* Preserve existing large dependency chunks for Mermaid, Markdown/KaTeX, and highlight.js.
* Preserve embed preload filtering intent; remove or adapt stale `vendor-tdesign` filtering if that chunk no longer exists.

## Acceptance Criteria

* [ ] Production build no longer emits a separate `vendor-tdesign` chunk from manual chunking.
* [ ] Vue, Vue Router, Pinia, Vue I18n, TDesign, and TDesign icons resolve to `vendor-vue`.
* [ ] Mermaid, Markdown/KaTeX, and highlight.js manual chunks remain unchanged.
* [ ] `cd frontend && npm test` passes if tests exist.
* [ ] `cd frontend && npm run build` passes.

## Definition of Done

* Code is formatted consistently with the existing Vite config.
* Relevant tests/build commands have been run and blockers are reported directly.
* Commit message: `fix: avoid vue and tdesign vendor chunk cycle`.

## Out of Scope

* Reworking all frontend chunking strategy.
* Changing dependencies or lockfiles unless build validation requires it.
* Debugging unrelated frontend runtime errors.

## Technical Notes

* `frontend/vite.config.ts` currently returns `vendor-tdesign` for `tdesign-vue-next`.
* The user-reported production error is `Uncaught ReferenceError: Cannot access 'j_' before initialization` in `vendor-vue-*.js` / `vendor-tdesign-*.js`.
* `frontend/package.json` defines `test: node --test` and `build: vite build`.

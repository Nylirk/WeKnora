# Knowledge Point Matching Golden Test Data

This directory holds the golden-set evaluation data for knowledge point
semantic matching.

## Files

- `question_kp_goldens.json` - Golden test cases for knowledge point matching.

## Golden Case Format

Each case in `question_kp_goldens.json` has the following structure:

```json
{
  "id": "q001",
  "question_type": "single_choice",
  "difficulty": "medium",
  "stem_text": "题干文本",
  "question_body": { "options": [{"label":"A","content":"..."}] },
  "expected_knowledge_points": ["主要正确知识点"],
  "acceptable_knowledge_points": ["可接受别名"],
  "negative_knowledge_points": ["明显错误知识点"],
  "mock_results": [
    {
      "id": "chunk-1",
      "knowledge_id": "kp-1",
      "knowledge_title": "知识点标题",
      "content": "知识块内容...",
      "score": 0.91
    }
  ]
}
```

### Field Semantics

- `expected_knowledge_points`: The primary correct knowledge points. A
  candidate that matches any of these counts as a hit.
- `acceptable_knowledge_points`: Aliases or closely related knowledge points.
  A candidate that matches any of these also counts as a hit, but is
  secondary to `expected_knowledge_points`.
- `negative_knowledge_points`: Clearly wrong knowledge points. If the
  matching status is `matched` and the top-1 candidate falls here (or misses
  all expected/acceptable), it counts as a false positive.
- `mock_results`: The mock `SearchResult` list that `mockKBService` returns
  for this case. This lets the evaluation run without a real knowledge base
  or vector store.

## Case Coverage

The golden set (16 cases) now includes both **synthetic ideal cases** (q001-q008)
and **failure-oriented cases** (q009-q016) designed to expose algorithm
weaknesses.

### Synthetic ideal cases (q001-q008)

1. **Clear single-knowledge-point question** (q001, q002): one dominant
   correct knowledge point, weak distractor.
2. **Multiple close candidates** (q003, q005, q007): two relevant knowledge
   points with close scores that may trigger uncertain or margin-dependent
   classification.
3. **Low confidence should be unmatched** (q006): top score below
   `KnowledgePointMinScore`, expect `unmatched`.
4. **Uncertain case** (q005): top1 and top2 both above min score but margin
   below `KnowledgePointMinMargin`, expect `uncertain`.
5. **Chinese question stem** (q001, q003, q004, q005, q008).
6. **English / mixed question stem** (q002, q006, q007).

### Failure-oriented cases (q009-q016)

7. **Wrong top1 ranking** (q009, q015, q016): the wrong knowledge point
   outranks the correct one. Tests whether recall@3 can still find the
   correct answer and whether false positives are generated.
8. **Close terminology confusion** (q010, q012): sin vs cos, 光合作用 vs
   细胞呼吸. Semantic similarity is high but the knowledge points are
   distinct. Tests whether margin gates trigger uncertain.
9. **Short ambiguous question** (q011): minimal stem text, low score below
   `KnowledgePointMinScore`, expect `unmatched` rather than forced match.
10. **Multiple valid candidates** (q013): both Recursion and Iteration are
    legitimate knowledge points. Tests uncertain behavior when expected
    includes more than one valid answer.
11. **Empty knowledge_title (inferred_from_content)** (q014): all mock
    results have empty `knowledge_title`, forcing `inferred_from_content`
    labels. Tests whether truncated content can match expected labels (it
    typically cannot, exposing a limitation of the inferred-label path).
12. **Multi-distractor recall** (q015, q016): at least 5 mock results with
    the correct knowledge point not in position 1. Tests recall@3 under
    heavy distractor competition.

### Language coverage

- Chinese stems: q001, q003, q004, q005, q008, q009, q012, q015, q016.
- English / mixed stems: q002, q006, q007, q010, q011, q013, q014.

## Metric Definitions

The evaluation computes the following metrics (see
`question_semantic_matching_eval_test.go`):

| Metric | Definition |
|--------|------------|
| `top1_accuracy` | Fraction of cases where the top-1 candidate hits `expected` or `acceptable`. **top1_accuracy measures candidate ranking quality, independent of final matched/unmatched status.** An `unmatched` case with a low-scoring candidate can still contribute to top1_accuracy if that candidate's label is correct. |
| `recall_at_3` | Fraction of cases where any of the top-3 candidates hits `expected` or `acceptable`. |
| `false_positive_rate` | Fraction of cases where status is `matched` AND top-1 misses all `expected`/`acceptable` or falls into `negative`. **This measures the risk of the system confidently matching the wrong knowledge point.** Only `matched` cases can be false positives; `uncertain` and `unmatched` never count. |
| `uncertain_rate` | Fraction of cases with status `uncertain`. **`uncertain` is not a false positive** — it signals the system could not confidently distinguish the top candidates, which is the correct conservative behavior for ambiguous cases. |
| `unmatched_rate` | Fraction of cases with status `unmatched`. |
| `average_candidate_count` | Mean number of candidates across all cases. |

## Current Baseline Characteristics

The golden set (16 cases) combines synthetic ideal data (q001-q008) with
failure-oriented cases (q009-q016) that expose known algorithm weaknesses.

Current baseline metrics:
- `top1_accuracy` ≈ 0.63 — dropped from 1.0 because failure cases include
  wrong top1 rankings and inferred-label mismatches.
- `recall_at_3` ≈ 0.94 — still high because the correct knowledge point is
  usually within top 3 even when not top 1.
- `false_positive_rate` ≈ 0.06 — low but non-zero, driven by inferred-label
  cases where truncated content cannot match expected labels.
- `uncertain_rate` ≈ 0.56 — high because many failure cases deliberately
  have close top1/top2 scores.

This is a **constraining baseline** for future algorithm improvements: any
change that degrades recall@3 below 0.60 or raises FPR above 0.50 will fail
the test gate. As more real failure cases are added, thresholds may need
adjustment — always loosen conservatively, never tighten to mask regressions.

### Adding real business failure cases

When incorporating real failure cases from production data, **anonymize**
before committing:

- Do not retain student names, IDs, or any personally identifiable info.
- Do not retain school, class, or exam identifiers.
- Keep only: question stem, question body structure, knowledge point
  candidates, and necessary evidence text.
- If the original stem contains identifying info, paraphrase or replace with
  an equivalent non-identifying stem that preserves the matching difficulty.

## Adding New Cases

When adding new golden cases:

- Keep the `id` unique and sequential.
- Provide at least one `expected_knowledge_points` or
  `acceptable_knowledge_points` entry.
- Set `mock_results` so the case can run without external services.
- Ensure the case covers a distinct scenario not already covered.

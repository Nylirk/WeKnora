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

The golden set covers:

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

## Adding New Cases

When adding new golden cases:

- Keep the `id` unique and sequential.
- Provide at least one `expected_knowledge_points` or
  `acceptable_knowledge_points` entry.
- Set `mock_results` so the case can run without external services.
- Ensure the case covers a distinct scenario not already covered.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// ---- shared candidate / fusion types ----

// questionSearchCandidate represents a single question hit from either
// keyword or semantic search, used as input to RRF fusion.
type questionSearchCandidate struct {
	Result        QuestionBankSearchResult
	QuestionID    string
	KeywordRank   int     // 1-based rank in keyword results; 0 = not in keyword
	SemanticRank  int     // 1-based rank in semantic results; 0 = not in semantic
	SemanticScore float64 // raw vector similarity score from semantic retrieval
	RRFScore      float64 // computed RRF fusion score
}

// rrfK is the constant k used in Reciprocal Rank Fusion.
const rrfK = 60

// fuseCandidatesRRF merges keyword and semantic candidates by question ID
// and computes an RRF score for each. When a question appears in only one
// source, the missing rank contributes 0 to the score.
// Results are sorted descending by rrf_score, then by semantic_score as
// a tiebreaker.
func fuseCandidatesRRF(keywordCands, semanticCands []questionSearchCandidate) []questionSearchCandidate {
	byID := make(map[string]*questionSearchCandidate)

	for i := range keywordCands {
		c := &keywordCands[i]
		qid := c.QuestionID
		if qid == "" {
			qid = c.Result.ID
		}
		m, ok := byID[qid]
		if !ok {
			clone := *c
			m = &clone
			m.QuestionID = qid
			byID[qid] = m
		}
		m.KeywordRank = c.KeywordRank
	}

	for i := range semanticCands {
		c := &semanticCands[i]
		qid := c.QuestionID
		if qid == "" {
			qid = c.Result.ID
		}
		m, ok := byID[qid]
		if !ok {
			clone := *c
			m = &clone
			m.QuestionID = qid
			byID[qid] = m
		}
		m.SemanticRank = c.SemanticRank
		m.SemanticScore = c.SemanticScore
	}

	out := make([]questionSearchCandidate, 0, len(byID))
	for _, c := range byID {
		score := 0.0
		if c.KeywordRank > 0 {
			score += 1.0 / float64(rrfK+c.KeywordRank)
		}
		if c.SemanticRank > 0 {
			score += 1.0 / float64(rrfK+c.SemanticRank)
		}
		c.RRFScore = score
		out = append(out, *c)
	}

	// Sort descending by RRF score; tie-break on semantic score.
	sortCandidatesByRRF(out)
	return out
}

func sortCandidatesByRRF(cands []questionSearchCandidate) {
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			if cands[i].RRFScore < cands[j].RRFScore ||
				(cands[i].RRFScore == cands[j].RRFScore && cands[i].SemanticScore < cands[j].SemanticScore) {
				cands[i], cands[j] = cands[j], cands[i]
			}
		}
	}
}

// ---- shared DB-access helpers (take *gorm.DB explicitly so both tools can reuse them) ----

// listQuestionsByIDs fetches questions scoped to a tenant. Uses explicit
// column selection to avoid time.Time scan issues with SQLite in tests.
func listQuestionsByIDs(ctx context.Context, db *gorm.DB, tenantID uint64, ids []string) ([]*types.Question, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var questions []*types.Question
	if err := db.WithContext(ctx).
		Select("id, tenant_id, question_set_id, knowledge_base_id, question_type, "+
			"stem_text, question_body, answer_text, answer_body, analysis_text, "+
			"difficulty, knowledge_points, tags, status, deleted_at").
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, ids).
		Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

// batchGetQuestionSetNames resolves question set IDs to names.
func batchGetQuestionSetNames(ctx context.Context, db *gorm.DB, setIDs []string) map[string]string {
	if len(setIDs) == 0 {
		return nil
	}
	type row struct {
		ID   string `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []row
	if err := db.WithContext(ctx).
		Table("question_sets").
		Select("id, name").
		Where("id IN ?", setIDs).
		Find(&rows).Error; err != nil {
		return nil
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.ID] = r.Name
	}
	return m
}

// ---- shared filter helpers (pure functions) ----

// questionMatchesFilters checks a question against the structured filters
// (used in semantic/hybrid mode where filtering happens post-retrieval in Go).
func questionMatchesFilters(q *types.Question, input QuestionBankSearchInput) bool {
	if q == nil {
		return false
	}
	if input.Status != "" && string(q.Status) != input.Status {
		return false
	}
	if input.QuestionType != "" && q.QuestionType != input.QuestionType {
		return false
	}
	if input.Difficulty != "" && string(q.Difficulty) != input.Difficulty {
		return false
	}
	if len(input.KnowledgePoints) > 0 {
		if !anyJSONContains(string(q.KnowledgePoints), input.KnowledgePoints) {
			return false
		}
	}
	if len(input.Tags) > 0 {
		if !anyJSONContains(string(q.Tags), input.Tags) {
			return false
		}
	}
	return true
}

// anyJSONContains checks whether a JSON array string (e.g. `["a","b"]`)
// contains at least one of the target strings.
func anyJSONContains(jsonStr string, targets []string) bool {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return false
	}
	for _, t := range targets {
		if strings.Contains(strings.ToLower(jsonStr), strings.ToLower(t)) {
			return true
		}
	}
	return false
}

// buildQuestionStructFilter builds SQL filter clauses for structured fields
// (question_set_id, question_type, difficulty, knowledge_points, tags,
// exclude_question_ids) applicable to keyword mode.
func buildQuestionStructFilter(input QuestionBankSearchInput, dialect string) (string, []interface{}) {
	var clauses []string
	var args []interface{}

	if input.QuestionSetID != "" {
		clauses = append(clauses, "questions.question_set_id = ?")
		args = append(args, input.QuestionSetID)
	}
	if input.QuestionType != "" {
		clauses = append(clauses, "questions.question_type = ?")
		args = append(args, input.QuestionType)
	}
	if input.Difficulty != "" {
		clauses = append(clauses, "questions.difficulty = ?")
		args = append(args, input.Difficulty)
	}
	for _, kp := range input.KnowledgePoints {
		escaped := escapeLike(kp)
		pattern := "%" + escaped + "%"
		clauses = append(clauses, kpLikeClause("questions.knowledge_points", dialect))
		args = append(args, pattern)
	}
	for _, tag := range input.Tags {
		escaped := escapeLike(tag)
		pattern := "%" + escaped + "%"
		clauses = append(clauses, kpLikeClause("questions.tags", dialect))
		args = append(args, pattern)
	}
	if len(input.ExcludeQuestionIDs) > 0 {
		clauses = append(clauses, "questions.id NOT IN ?")
		args = append(args, input.ExcludeQuestionIDs)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// kpLikeClause returns a dialect-appropriate LIKE clause for a JSON column.
func kpLikeClause(column, dialect string) string {
	switch {
	case dialect == "postgres" || dialect == "postgresql":
		return column + `::text ILIKE ? ESCAPE '\'`
	default:
		return `LOWER(CAST(` + column + ` AS TEXT)) LIKE LOWER(?) ESCAPE '\'`
	}
}

// escapeLike escapes %, _, and \ for a LIKE pattern.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// compositeSupportsVector checks whether a CompositeRetrieveEngine supports
// vector retrieval across any of its internal engines.
func compositeSupportsVector(engine *retriever.CompositeRetrieveEngine) bool {
	if engine == nil {
		return false
	}
	return engine.SupportRetriever(types.VectorRetrieverType)
}

// escapeXML escapes user-controlled text for safe XML embedding.
func escapeXML(s string) string {
	s = html.EscapeString(s)
	return s
}

// formatQuestionBankSearchResults produces a human-readable and LLM-friendly
// formatted output from the search results.
func formatQuestionBankSearchResults(results []QuestionBankSearchResult, query string, limit int, mode string) string {
	var b strings.Builder

	if len(results) == 0 {
		if query == "" {
			b.WriteString("No questions found in the question bank knowledge bases in scope.\n")
		} else {
			fmt.Fprintf(&b, "No questions matched the query %q in the question bank knowledge bases in scope.\n", query)
		}
		return b.String()
	}

	modeLabel := ""
	if mode == "semantic" {
		modeLabel = " (semantic)"
	} else if mode == "hybrid" {
		modeLabel = " (hybrid)"
	}
	if query == "" {
		fmt.Fprintf(&b, "Recent questions in scope%s (%d results, limit %d):\n\n", modeLabel, len(results), limit)
	} else {
		fmt.Fprintf(&b, "Question bank search results for %q%s (%d results, limit %d):\n\n", query, modeLabel, len(results), limit)
	}

	for i, r := range results {
		fmt.Fprintf(&b, "--- Result %d ---\n", i+1)
		fmt.Fprintf(&b, "question_id: %s\n", r.ID)
		fmt.Fprintf(&b, "question_set_id: %s\n", r.QuestionSetID)
		fmt.Fprintf(&b, "question_set_name: %s\n", escapeXML(r.QuestionSetName))
		fmt.Fprintf(&b, "knowledge_base_id: %s\n", r.KnowledgeBaseID)
		fmt.Fprintf(&b, "question_type: %s\n", r.QuestionType)
		fmt.Fprintf(&b, "difficulty: %s\n", r.Difficulty)
		fmt.Fprintf(&b, "status: %s\n", r.Status)
		if r.MatchType != "" {
			fmt.Fprintf(&b, "match_type: %s\n", r.MatchType)
		}
		if r.KeywordRank > 0 {
			fmt.Fprintf(&b, "keyword_rank: %d\n", r.KeywordRank)
		}
		if r.SemanticRank > 0 {
			fmt.Fprintf(&b, "semantic_rank: %d\n", r.SemanticRank)
		}
		if r.RRFScore != 0 {
			fmt.Fprintf(&b, "rrf_score: %.4f\n", r.RRFScore)
		}
		if r.Score != 0 {
			fmt.Fprintf(&b, "score: %.4f\n", r.Score)
		}
		fmt.Fprintf(&b, "stem_text: %s\n", escapeXML(r.StemText))
		if string(r.QuestionBody) != "" && string(r.QuestionBody) != "null" {
			fmt.Fprintf(&b, "question_body: %s\n", escapeXML(string(r.QuestionBody)))
		}
		fmt.Fprintf(&b, "answer_text: %s\n", escapeXML(r.AnswerText))
		if string(r.AnswerBody) != "" && string(r.AnswerBody) != "null" {
			fmt.Fprintf(&b, "answer_body: %s\n", escapeXML(string(r.AnswerBody)))
		}
		if r.AnalysisText != "" {
			fmt.Fprintf(&b, "analysis_text: %s\n", escapeXML(r.AnalysisText))
		}
		if string(r.KnowledgePoints) != "" && string(r.KnowledgePoints) != "null" && string(r.KnowledgePoints) != "[]" {
			fmt.Fprintf(&b, "knowledge_points: %s\n", escapeXML(string(r.KnowledgePoints)))
		}
		if string(r.Tags) != "" && string(r.Tags) != "null" && string(r.Tags) != "[]" {
			fmt.Fprintf(&b, "tags: %s\n", escapeXML(string(r.Tags)))
		}
		b.WriteString("\n")
	}

	if len(results) == limit {
		b.WriteString("(Results may have been truncated; consider narrowing the query or increasing the limit.)\n")
	}

	return b.String()
}

const questionIndexMaxChars = 20000

// buildQuestionIndexContent builds the embedding input from a question.
// Mirrors service.BuildQuestionIndexContent but lives in the tools package
// to avoid an import cycle (tools cannot import service).
func buildQuestionIndexContent(q *types.Question) string {
	if q == nil {
		return ""
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "question_type", value: strings.TrimSpace(q.QuestionType)},
		{name: "difficulty", value: strings.TrimSpace(string(q.Difficulty))},
		{name: "stem_text", value: strings.TrimSpace(q.StemText)},
		{name: "question_body", value: readableJSON(q.QuestionBody)},
		{name: "knowledge_points", value: readableJSON(q.KnowledgePoints)},
		{name: "tags", value: readableJSON(q.Tags)},
	}

	var builder strings.Builder
	for _, field := range fields {
		if field.value == "" || field.value == "{}" || field.value == "[]" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(field.name)
		builder.WriteString(": ")
		builder.WriteString(field.value)
	}

	content := builder.String()
	if utf8.RuneCountInString(content) <= questionIndexMaxChars {
		return content
	}
	return string([]rune(content)[:questionIndexMaxChars])
}

func readableJSON(raw types.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return formatJSONValue(value)
}

func formatJSONValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool, float64:
		return fmt.Sprint(typed)
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if formatted := formatJSONValue(item); formatted != "" {
				parts = append(parts, formatted)
			}
		}
		if len(parts) == 0 {
			return "[]"
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if !isForbiddenKey(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if formatted := formatJSONValue(typed[key]); formatted != "" {
				parts = append(parts, key+": "+formatted)
			}
		}
		if len(parts) == 0 {
			return "{}"
		}
		return "{" + strings.Join(parts, "; ") + "}"
	default:
		return fmt.Sprint(typed)
	}
}

func isForbiddenKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	return strings.Contains(normalized, "answer") ||
		strings.Contains(normalized, "analysis") ||
		strings.Contains(normalized, "explanation") ||
		strings.Contains(normalized, "solution") ||
		strings.Contains(normalized, "rubric")
}

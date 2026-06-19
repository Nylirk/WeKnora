package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// questionBankSearchTool is the base definition (name, description, schema).
var questionBankSearchTool = BaseTool{
	name: ToolQuestionBankSearch,
	description: `Search questions in a question bank knowledge base.

Accepts an optional keyword query and searches across stem_text, answer_text,
analysis_text, question_body, answer_body, knowledge_points, and tags fields.
When the query is empty or whitespace, lists recent questions in scope.

## When to use
- Use when the user asks about questions, exam problems, or quiz content in a
  question bank knowledge base.
- Do NOT use this for general document or chunk search — use knowledge_search
  or grep_chunks instead.

## Returns per result
- question_id, question_set_id, question_set_name, knowledge_base_id
- question_type, stem_text, question_body, answer_text, answer_body
- analysis_text, difficulty, knowledge_points, tags, status`,
	schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Optional keyword to search for across question fields. When empty or omitted, lists recent questions in scope."
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of results to return (1-50, default 20)",
      "default": 20,
      "minimum": 1,
      "maximum": 50
    },
    "status": {
      "type": "string",
      "description": "Optional status filter: draft, reviewed, or rejected. When omitted, returns all non-deleted questions."
    }
  }
}`),
}

// QuestionBankSearchInput defines the input parameters for question bank search.
type QuestionBankSearchInput struct {
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Status string `json:"status,omitempty"`
}

// QuestionBankSearchResult represents a single question result returned by the tool.
type QuestionBankSearchResult struct {
	ID              string     `json:"question_id"       gorm:"column:id"`
	QuestionSetID   string     `json:"question_set_id"   gorm:"column:question_set_id"`
	QuestionSetName string     `json:"question_set_name" gorm:"column:question_set_name"`
	KnowledgeBaseID string     `json:"knowledge_base_id" gorm:"column:knowledge_base_id"`
	QuestionType    string     `json:"question_type"     gorm:"column:question_type"`
	StemText        string     `json:"stem_text"         gorm:"column:stem_text"`
	QuestionBody    types.JSON `json:"question_body"     gorm:"column:question_body"`
	AnswerText      string     `json:"answer_text"       gorm:"column:answer_text"`
	AnswerBody      types.JSON `json:"answer_body"       gorm:"column:answer_body"`
	AnalysisText    string     `json:"analysis_text"     gorm:"column:analysis_text"`
	Difficulty      string     `json:"difficulty"        gorm:"column:difficulty"`
	KnowledgePoints types.JSON `json:"knowledge_points"  gorm:"column:knowledge_points"`
	Tags            types.JSON `json:"tags"              gorm:"column:tags"`
	Status          string     `json:"status"            gorm:"column:status"`
}

// QuestionBankSearchTool searches questions in question bank KBs.
type QuestionBankSearchTool struct {
	BaseTool
	db            *gorm.DB
	searchTargets types.SearchTargets
}

// NewQuestionBankSearchTool creates a new question bank search tool.
func NewQuestionBankSearchTool(db *gorm.DB, searchTargets types.SearchTargets) *QuestionBankSearchTool {
	return &QuestionBankSearchTool{
		BaseTool:      questionBankSearchTool,
		db:            db,
		searchTargets: searchTargets,
	}
}

// Execute runs the question bank search.
func (t *QuestionBankSearchTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input QuestionBankSearchInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse arguments: %v", err),
		}, err
	}

	query := strings.TrimSpace(input.Query)
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Build empty-result Data shape used on all success paths.
	emptyData := func() map[string]interface{} {
		return map[string]interface{}{
			"results":      []QuestionBankSearchResult{},
			"result_count": 0,
			"display_type": "question_bank_results",
			"query":        query,
			"limit":        limit,
		}
	}

	// Build tenant-isolated OR predicates per SearchTarget.
	kbIDs := t.searchTargets.GetAllKnowledgeBaseIDs()
	kbTenantMap := t.searchTargets.GetKBTenantMap()
	var orClauses []string
	var orArgs []interface{}
	for _, kbID := range kbIDs {
		tenantID := kbTenantMap[kbID]
		if kbID == "" || tenantID == 0 {
			continue
		}
		orClauses = append(orClauses, "(questions.knowledge_base_id = ? AND questions.tenant_id = ?)")
		orArgs = append(orArgs, kbID, tenantID)
	}
	if len(orClauses) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  "No valid question bank knowledge bases in scope.",
			Data:    emptyData(),
		}, nil
	}
	kbFilter := "(" + strings.Join(orClauses, " OR ") + ")"

	// Build search clause with dialect-appropriate syntax.
	dialect := t.db.Dialector.Name()
	var searchClause string
	var searchArgs []interface{}

	if query != "" {
		// Escape LIKE wildcards so user query is treated literally.
		escapedQuery := strings.ReplaceAll(query, "\\", "\\\\")
		escapedQuery = strings.ReplaceAll(escapedQuery, "%", "\\%")
		escapedQuery = strings.ReplaceAll(escapedQuery, "_", "\\_")
		pattern := "%" + escapedQuery + "%"

		switch {
		case dialect == "postgres" || dialect == "postgresql":
			searchClause = ` AND (` +
				`questions.stem_text ILIKE ? ESCAPE '\'` +
				` OR questions.answer_text ILIKE ? ESCAPE '\'` +
				` OR questions.analysis_text ILIKE ? ESCAPE '\'` +
				` OR questions.question_body::text ILIKE ? ESCAPE '\'` +
				` OR questions.answer_body::text ILIKE ? ESCAPE '\'` +
				` OR questions.knowledge_points::text ILIKE ? ESCAPE '\'` +
				` OR questions.tags::text ILIKE ? ESCAPE '\'` +
				`)`
		case dialect == "sqlite" || dialect == "sqlite3":
			searchClause = ` AND (` +
				`LOWER(questions.stem_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.answer_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.analysis_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.question_body AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.answer_body AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.knowledge_points AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.tags AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				`)`
		default:
			// MySQL / other — use CHAR cast
			searchClause = ` AND (` +
				`LOWER(questions.stem_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.answer_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.analysis_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.question_body AS CHAR)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.answer_body AS CHAR)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.knowledge_points AS CHAR)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.tags AS CHAR)) LIKE LOWER(?) ESCAPE '\'` +
				`)`
		}
		searchArgs = []interface{}{pattern, pattern, pattern, pattern, pattern, pattern, pattern}
	}

	// Status filter — only apply when explicitly specified.
	var statusFilter string
	var statusArgs []interface{}
	if input.Status != "" {
		statusFilter = ` AND questions.status = ?`
		statusArgs = []interface{}{input.Status}
	}

	// Build and execute the query.
	baseSQL := `SELECT
		questions.id,
		questions.question_set_id,
		question_sets.name AS question_set_name,
		questions.knowledge_base_id,
		questions.question_type,
		questions.stem_text,
		questions.question_body,
		questions.answer_text,
		questions.answer_body,
		questions.analysis_text,
		questions.difficulty,
		questions.knowledge_points,
		questions.tags,
		questions.status,
		questions.created_at
	FROM questions
	JOIN knowledge_bases ON knowledge_bases.id = questions.knowledge_base_id
		AND knowledge_bases.type = 'question_bank'
		AND knowledge_bases.deleted_at IS NULL
	JOIN question_sets ON question_sets.id = questions.question_set_id
		AND question_sets.deleted_at IS NULL
	WHERE questions.deleted_at IS NULL
		AND ` + kbFilter + searchClause + statusFilter + `
	ORDER BY questions.created_at DESC
	LIMIT ?`

	allArgs := append(orArgs, searchArgs...)
	allArgs = append(allArgs, statusArgs...)
	allArgs = append(allArgs, limit)

	var results []QuestionBankSearchResult
	if err := t.db.WithContext(ctx).Raw(baseSQL, allArgs...).Scan(&results).Error; err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Question bank search failed: %v", err),
		}, err
	}

	if results == nil {
		results = []QuestionBankSearchResult{}
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatQuestionBankSearchResults(results, query, limit),
		Data: map[string]interface{}{
			"results":      results,
			"result_count": len(results),
			"display_type": "question_bank_results",
			"query":        query,
			"limit":        limit,
		},
	}, nil
}

// escapeXML escapes user-controlled text for safe XML embedding.
func escapeXML(s string) string {
	s = html.EscapeString(s)
	return s
}

// formatQuestionBankSearchResults produces a human-readable and LLM-friendly
// formatted output from the search results.
func formatQuestionBankSearchResults(results []QuestionBankSearchResult, query string, limit int) string {
	var b strings.Builder

	if len(results) == 0 {
		if query == "" {
			b.WriteString("No questions found in the question bank knowledge bases in scope.\n")
		} else {
			fmt.Fprintf(&b, "No questions matched the query %q in the question bank knowledge bases in scope.\n", query)
		}
		return b.String()
	}

	if query == "" {
		fmt.Fprintf(&b, "Recent questions in scope (%d results, limit %d):\n\n", len(results), limit)
	} else {
		fmt.Fprintf(&b, "Question bank search results for %q (%d results, limit %d):\n\n", query, len(results), limit)
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

// Ensure QuestionBankSearchTool implements Tool interface.
var _ types.Tool = (*QuestionBankSearchTool)(nil)

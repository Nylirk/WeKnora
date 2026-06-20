package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// similarQuestionSearchTool is the base definition (name, description, schema).
var similarQuestionSearchTool = BaseTool{
	name: ToolSimilarQuestionSearch,
	description: `Find questions semantically similar to a given question in a question bank knowledge base.

Uses the question's content (stem, body, knowledge points, tags) to generate
an embedding and retrieves the most semantically similar questions from the
vector index. The source question itself is always excluded from results.

## When to use
- Find duplicate or near-duplicate questions
- Discover variant questions for exercise generation
- Pre-deduplication before composing exam papers
- Only works with question_bank knowledge bases.

## Returns per result
- question_id, question_set_id, question_set_name, knowledge_base_id
- question_type, stem_text, question_body, answer_text, answer_body
- analysis_text, difficulty, knowledge_points, tags, status
- match_type = "similar", score, rank`,
	schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "question_id": {
      "type": "string",
      "description": "Required. The ID of the source question to find similar questions for."
    },
    "question_set_id": {
      "type": "string",
      "description": "Optional question set ID to restrict search scope."
    },
    "knowledge_base_id": {
      "type": "string",
      "description": "Optional knowledge base ID to restrict search scope. Must be a question_bank KB in the current search targets."
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
      "description": "Optional status filter. Defaults to reviewed to avoid recommending unreviewed questions.",
      "default": "reviewed"
    },
    "question_type": {
      "type": "string",
      "description": "Optional question type filter."
    },
    "difficulty": {
      "type": "string",
      "description": "Optional difficulty filter: easy, medium, or hard."
    },
    "exclude_question_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional question IDs to exclude from results."
    },
    "include_same_question_set": {
      "type": "boolean",
      "description": "Whether to include similar questions from the same question set. Default true.",
      "default": true
    }
  }
}`),
}

// SimilarQuestionSearchInput defines the input parameters for similar question search.
type SimilarQuestionSearchInput struct {
	QuestionID             string   `json:"question_id"`
	QuestionSetID          string   `json:"question_set_id,omitempty"`
	KnowledgeBaseID        string   `json:"knowledge_base_id,omitempty"`
	Limit                  int      `json:"limit,omitempty"`
	Status                 string   `json:"status,omitempty"`
	QuestionType           string   `json:"question_type,omitempty"`
	Difficulty             string   `json:"difficulty,omitempty"`
	ExcludeQuestionIDs     []string `json:"exclude_question_ids,omitempty"`
	IncludeSameQuestionSet *bool    `json:"include_same_question_set,omitempty"`
}

// SimilarQuestionSearchTool finds semantically similar questions.
type SimilarQuestionSearchTool struct {
	BaseTool
	db                   *gorm.DB
	searchTargets        types.SearchTargets
	knowledgeBaseService interfaces.KnowledgeBaseService
	modelService         interfaces.ModelService
	engineRegistry       interfaces.RetrieveEngineRegistry
	ownership            retriever.TenantStoreOwnership
}

// NewSimilarQuestionSearchTool creates a new similar question search tool.
func NewSimilarQuestionSearchTool(
	db *gorm.DB,
	searchTargets types.SearchTargets,
	knowledgeBaseService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
	engineRegistry interfaces.RetrieveEngineRegistry,
	ownership retriever.TenantStoreOwnership,
) *SimilarQuestionSearchTool {
	return &SimilarQuestionSearchTool{
		BaseTool:             similarQuestionSearchTool,
		db:                   db,
		searchTargets:        searchTargets,
		knowledgeBaseService: knowledgeBaseService,
		modelService:         modelService,
		engineRegistry:       engineRegistry,
		ownership:            ownership,
	}
}

// Execute runs the similar question search.
func (t *SimilarQuestionSearchTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input SimilarQuestionSearchInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse arguments: %v", err),
		}, err
	}

	questionID := strings.TrimSpace(input.QuestionID)
	if questionID == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "question_id is required for similar question search.",
		}, fmt.Errorf("question_id is required")
	}

	// Defaults.
	if input.IncludeSameQuestionSet == nil {
		defTrue := true
		input.IncludeSameQuestionSet = &defTrue
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 50 {
		input.Limit = 50
	}
	validStatuses := map[string]bool{"": true, "draft": true, "reviewed": true, "rejected": true}
	if !validStatuses[input.Status] {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid status %q", input.Status),
		}, fmt.Errorf("invalid status %q", input.Status)
	}

	// Nil guard.
	if t.knowledgeBaseService == nil || t.modelService == nil ||
		t.engineRegistry == nil || t.ownership == nil {
		return &types.ToolResult{
			Success: false,
			Error:   "Similar question search is not available: required services are not configured.",
		}, fmt.Errorf("similar question search requires all dependencies")
	}

	// 1. Lookup source question scoped to current search targets.
	kbTenantMap := t.searchTargets.GetKBTenantMap()
	sourceQuestion, err := t.lookupSourceQuestion(ctx, questionID, kbTenantMap)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	// 2. Resolve the source question's KB for engine + embedding.
	kb, err := t.knowledgeBaseService.GetKnowledgeBaseByIDOnly(ctx, sourceQuestion.KnowledgeBaseID)
	if err != nil || kb == nil || kb.Type != types.KnowledgeBaseTypeQuestionBank {
		return &types.ToolResult{
			Success: false,
			Error:   "Source question's knowledge base is not available or is not a question bank.",
		}, fmt.Errorf("source question KB not available")
	}
	if kb.EmbeddingModelID == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "Source question's knowledge base has no embedding model configured.",
		}, fmt.Errorf("no embedding model for source KB")
	}

	engine, err := retriever.CreateRetrieveEngineForKB(
		ctx, t.engineRegistry, t.ownership, sourceQuestion.TenantID, kb.VectorStoreID,
	)
	if err != nil || engine == nil || !compositeSupportsVector(engine) {
		return &types.ToolResult{
			Success: false,
			Error:   "Source question's knowledge base has no vector retriever available.",
		}, fmt.Errorf("no vector retriever for source KB")
	}

	// 3. Build query embedding from source question content.
	content := buildQuestionIndexContent(sourceQuestion)
	if content == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "Source question has no indexable content.",
		}, fmt.Errorf("source question content empty")
	}

	embedder, err := t.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Cannot get embedding model: %v", err),
		}, err
	}
	embedding, err := embedder.Embed(ctx, content)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Embedding failed: %v", err),
		}, err
	}

	// 4. Vector retrieve.
	topK := input.Limit * semanticOverfetchFactor
	if topK < semanticMinTopK {
		topK = semanticMinTopK
	}
	if topK > semanticMaxTopK {
		topK = semanticMaxTopK
	}

	// Determine which KB IDs to search.
	searchKBIDs := []string{sourceQuestion.KnowledgeBaseID}
	if input.KnowledgeBaseID != "" {
		if !t.searchTargets.ContainsKB(input.KnowledgeBaseID) {
			return &types.ToolResult{
				Success: false,
				Error:   "Requested knowledge_base_id is not in the current search scope.",
			}, fmt.Errorf("knowledge_base_id not in scope")
		}
		searchKBIDs = []string{input.KnowledgeBaseID}
	}

	params := types.RetrieveParams{
		Query:            content,
		Embedding:        embedding,
		KnowledgeBaseIDs: searchKBIDs,
		TopK:             topK,
		RetrieverType:    types.VectorRetrieverType,
		KnowledgeType:    types.KnowledgeTypeQuestion,
	}
	if input.QuestionSetID != "" {
		params.KnowledgeIDs = []string{input.QuestionSetID}
	}
	results, err := engine.Retrieve(ctx, []types.RetrieveParams{params})
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Vector retrieval failed: %v", err),
		}, err
	}

	// 5. Collect source IDs, exclude self. Track KB mapping for backfill.
	var orderedIDs []string
	idToScore := make(map[string]float64)
	idToKB := make(map[string]string)
	for _, retrieveResult := range results {
		if retrieveResult == nil {
			continue
		}
		for _, idx := range retrieveResult.Results {
			if idx == nil || idx.SourceID == "" || idx.SourceID == questionID {
				continue
			}
			if _, seen := idToScore[idx.SourceID]; seen {
				continue
			}
			idToScore[idx.SourceID] = idx.Score
			if idx.KnowledgeBaseID != "" {
				idToKB[idx.SourceID] = idx.KnowledgeBaseID
			}
			orderedIDs = append(orderedIDs, idx.SourceID)
		}
	}

	if len(orderedIDs) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("No similar questions found for question %q.", questionID),
			Data: map[string]interface{}{
				"source_question_id": questionID,
				"results":            []QuestionBankSearchResult{},
				"result_count":       0,
				"display_type":       "similar_question_results",
				"limit":              input.Limit,
			},
		}, nil
	}

	// 6. SQL backfill — group IDs by tenant.
	tenantToIDs := make(map[uint64][]string)
	for _, id := range orderedIDs {
		kbID := idToKB[id]
		if kbID == "" {
			kbID = sourceQuestion.KnowledgeBaseID
		}
		tid := kbTenantMap[kbID]
		if tid == 0 {
			continue
		}
		tenantToIDs[tid] = append(tenantToIDs[tid], id)
	}

	var allQuestions []*types.Question
	for tenantID, ids := range tenantToIDs {
		questions, err := listQuestionsByIDs(ctx, t.db, tenantID, ids)
		if err != nil {
			logger.Warnf(ctx, "Similar question search: backfill failed for tenant %d: %v", tenantID, err)
			continue
		}
		allQuestions = append(allQuestions, questions...)
	}

	questionMap := make(map[string]*types.Question)
	for _, q := range allQuestions {
		if q != nil && q.DeletedAt.Time.IsZero() && q.ID != questionID {
			questionMap[q.ID] = q
		}
	}

	// 7. Build results in vector rank order.
	excludeSet := make(map[string]bool)
	for _, eid := range input.ExcludeQuestionIDs {
		excludeSet[eid] = true
	}
	sourceSetID := sourceQuestion.QuestionSetID

	var out []QuestionBankSearchResult
	for _, id := range orderedIDs {
		if len(out) >= input.Limit {
			break
		}
		if excludeSet[id] {
			continue
		}
		q, ok := questionMap[id]
		if !ok {
			continue
		}
		// Tenant scope.
		tid := kbTenantMap[q.KnowledgeBaseID]
		if q.TenantID != tid {
			continue
		}
		// KB scope.
		if !t.searchTargets.ContainsKB(q.KnowledgeBaseID) {
			continue
		}
		// Same question set exclusion.
		if input.IncludeSameQuestionSet != nil && !*input.IncludeSameQuestionSet && q.QuestionSetID == sourceSetID {
			continue
		}
		// Structured filters (wrapped as QuestionBankSearchInput for reuse).
		qbInput := QuestionBankSearchInput{
			Status:       input.Status,
			QuestionType: input.QuestionType,
			Difficulty:   input.Difficulty,
		}
		if !questionMatchesFilters(q, qbInput) {
			continue
		}

		out = append(out, QuestionBankSearchResult{
			ID:               q.ID,
			QuestionSetID:    q.QuestionSetID,
			KnowledgeBaseID:  q.KnowledgeBaseID,
			QuestionType:     q.QuestionType,
			StemText:         q.StemText,
			QuestionBody:     q.QuestionBody,
			AnswerText:       q.AnswerText,
			AnswerBody:       q.AnswerBody,
			AnalysisText:     q.AnalysisText,
			Difficulty:       string(q.Difficulty),
			KnowledgePoints:  q.KnowledgePoints,
			Tags:             q.Tags,
			Status:           string(q.Status),
			MatchType:        "similar",
			Score:            idToScore[id],
			SourceQuestionID: questionID,
		})
	}

	// Populate question set names.
	setIDs := make([]string, 0, len(out))
	seenSets := make(map[string]bool)
	for _, r := range out {
		if !seenSets[r.QuestionSetID] {
			seenSets[r.QuestionSetID] = true
			setIDs = append(setIDs, r.QuestionSetID)
		}
	}
	setNameMap := batchGetQuestionSetNames(ctx, t.db, setIDs)
	for i := range out {
		out[i].Rank = i + 1
		if name, ok := setNameMap[out[i].QuestionSetID]; ok {
			out[i].QuestionSetName = name
		}
	}

	if out == nil {
		out = []QuestionBankSearchResult{}
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatSimilarResults(out, questionID, input.Limit),
		Data: map[string]interface{}{
			"source_question_id": questionID,
			"results":            out,
			"result_count":       len(out),
			"display_type":       "similar_question_results",
			"limit":              input.Limit,
		},
	}, nil
}

// lookupSourceQuestion fetches a question and validates it belongs to the
// current search targets scope. Uses explicit column selection to avoid
// time.Time scan issues with SQLite in tests.
func (t *SimilarQuestionSearchTool) lookupSourceQuestion(
	ctx context.Context,
	questionID string,
	kbTenantMap map[string]uint64,
) (*types.Question, error) {
	var q types.Question
	if err := t.db.WithContext(ctx).
		Select("id, tenant_id, question_set_id, knowledge_base_id, question_type, "+
			"stem_text, question_body, answer_text, answer_body, analysis_text, "+
			"difficulty, knowledge_points, tags, status, deleted_at").
		Where("id = ? AND deleted_at IS NULL", questionID).
		First(&q).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("source question %q not found or has been deleted", questionID)
		}
		return nil, fmt.Errorf("failed to lookup source question: %w", err)
	}

	// Validate tenant + KB scope.
	expectedTenant := kbTenantMap[q.KnowledgeBaseID]
	if q.TenantID != expectedTenant {
		return nil, fmt.Errorf("source question %q does not belong to current tenant scope", questionID)
	}
	if !t.searchTargets.ContainsKB(q.KnowledgeBaseID) {
		return nil, fmt.Errorf("source question %q is not in the current search scope", questionID)
	}
	return &q, nil
}

// formatSimilarResults produces a human-readable output for similar question results.
func formatSimilarResults(results []QuestionBankSearchResult, sourceID string, limit int) string {
	var b strings.Builder

	if len(results) == 0 {
		fmt.Fprintf(&b, "No similar questions found for question %q.\n", sourceID)
		return b.String()
	}

	fmt.Fprintf(&b, "Similar questions for %q (%d results, limit %d):\n\n", sourceID, len(results), limit)

	for i, r := range results {
		fmt.Fprintf(&b, "--- Result %d ---\n", i+1)
		fmt.Fprintf(&b, "question_id: %s\n", r.ID)
		fmt.Fprintf(&b, "question_set_id: %s\n", r.QuestionSetID)
		fmt.Fprintf(&b, "question_set_name: %s\n", escapeXML(r.QuestionSetName))
		fmt.Fprintf(&b, "knowledge_base_id: %s\n", r.KnowledgeBaseID)
		fmt.Fprintf(&b, "question_type: %s\n", r.QuestionType)
		fmt.Fprintf(&b, "difficulty: %s\n", r.Difficulty)
		fmt.Fprintf(&b, "status: %s\n", r.Status)
		fmt.Fprintf(&b, "match_type: %s\n", r.MatchType)
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

// Ensure SimilarQuestionSearchTool implements Tool interface.
var _ types.Tool = (*SimilarQuestionSearchTool)(nil)

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

const (
	questionBankSearchModeKeyword  = "keyword"
	questionBankSearchModeSemantic = "semantic"

	// semanticOverfetchFactor multiplies the user-requested limit to get the
	// vector topK so that SQL-side filtering still leaves enough candidates.
	semanticOverfetchFactor = 5

	// semanticMinTopK is the floor for the vector retrieval topK.
	semanticMinTopK = 100

	// semanticMaxTopK is the ceiling for the vector retrieval topK.
	semanticMaxTopK = 300
)

// questionBankSearchTool is the base definition (name, description, schema).
var questionBankSearchTool = BaseTool{
	name: ToolQuestionBankSearch,
	description: `Search questions in a question bank knowledge base.

Accepts an optional keyword query and searches across stem_text, answer_text,
analysis_text, question_body, answer_body, knowledge_points, and tags fields.
When the query is empty or whitespace, lists recent questions in scope.

Supports two modes:
- keyword (default): SQL LIKE search across question fields
- semantic: vector/embedding-based semantic search using question vector indexes

## When to use
- Use when the user asks about questions, exam problems, or quiz content in a
  question bank knowledge base.
- Do NOT use this for general document or chunk search — use knowledge_search
  or grep_chunks instead.

## Returns per result
- question_id, question_set_id, question_set_name, knowledge_base_id
- question_type, stem_text, question_body, answer_text, answer_body
- analysis_text, difficulty, knowledge_points, tags, status
- mode, match_type, score (semantic mode only)`,
	schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Optional keyword or semantic query to search for across question fields. When empty or omitted, lists recent questions in scope."
    },
    "mode": {
      "type": "string",
      "enum": ["keyword", "semantic"],
      "description": "Search mode: keyword (SQL LIKE, default) or semantic (vector embedding). Defaults to keyword for backward compatibility.",
      "default": "keyword"
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
    },
    "question_set_id": {
      "type": "string",
      "description": "Optional question set ID to restrict search scope."
    },
    "question_type": {
      "type": "string",
      "description": "Optional question type filter (e.g. single_choice, multiple_choice, short_answer)."
    },
    "difficulty": {
      "type": "string",
      "description": "Optional difficulty filter: easy, medium, or hard."
    },
    "knowledge_points": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional knowledge points to filter by. Questions must have at least one matching knowledge point."
    },
    "tags": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional tags to filter by. Questions must have at least one matching tag."
    },
    "exclude_question_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional question IDs to exclude from results."
    }
  }
}`),
}

// QuestionBankSearchInput defines the input parameters for question bank search.
type QuestionBankSearchInput struct {
	Query              string   `json:"query,omitempty"`
	Mode               string   `json:"mode,omitempty"`
	Limit              int      `json:"limit,omitempty"`
	Status             string   `json:"status,omitempty"`
	QuestionSetID      string   `json:"question_set_id,omitempty"`
	QuestionType       string   `json:"question_type,omitempty"`
	Difficulty         string   `json:"difficulty,omitempty"`
	KnowledgePoints    []string `json:"knowledge_points,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	ExcludeQuestionIDs []string `json:"exclude_question_ids,omitempty"`
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
	MatchType       string     `json:"match_type,omitempty" gorm:"-"`
	Score           float64    `json:"score,omitempty"       gorm:"-"`
	Rank            int        `json:"rank,omitempty"        gorm:"-"`
}

// QuestionBankSearchTool searches questions in question bank KBs.
type QuestionBankSearchTool struct {
	BaseTool
	db                   *gorm.DB
	searchTargets        types.SearchTargets
	knowledgeBaseService interfaces.KnowledgeBaseService
	modelService         interfaces.ModelService
	engineRegistry       interfaces.RetrieveEngineRegistry
	ownership            retriever.TenantStoreOwnership
}

// NewQuestionBankSearchTool creates a new question bank search tool.
func NewQuestionBankSearchTool(
	db *gorm.DB,
	searchTargets types.SearchTargets,
	knowledgeBaseService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
	engineRegistry interfaces.RetrieveEngineRegistry,
	ownership retriever.TenantStoreOwnership,
) *QuestionBankSearchTool {
	return &QuestionBankSearchTool{
		BaseTool:             questionBankSearchTool,
		db:                   db,
		searchTargets:        searchTargets,
		knowledgeBaseService: knowledgeBaseService,
		modelService:         modelService,
		engineRegistry:       engineRegistry,
		ownership:            ownership,
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
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = questionBankSearchModeKeyword
	}
	if mode != questionBankSearchModeKeyword && mode != questionBankSearchModeSemantic {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid mode %q: must be keyword or semantic", mode),
		}, fmt.Errorf("invalid mode %q", mode)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Validate status if provided: only allow known statuses.
	validStatuses := map[string]bool{"": true, "draft": true, "reviewed": true, "rejected": true}
	if !validStatuses[input.Status] {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid status %q: must be one of draft, reviewed, rejected, or empty", input.Status),
		}, fmt.Errorf("invalid status %q", input.Status)
	}

	// Build empty-result Data shape used on all success paths.
	emptyData := func() map[string]interface{} {
		return map[string]interface{}{
			"results":      []QuestionBankSearchResult{},
			"result_count": 0,
			"display_type": "question_bank_results",
			"query":        query,
			"limit":        limit,
			"mode":         mode,
		}
	}

	if mode == questionBankSearchModeSemantic {
		return t.executeSemanticSearch(ctx, input, query, limit, emptyData)
	}
	return t.executeKeywordSearch(ctx, input, query, limit, emptyData)
}

// executeKeywordSearch performs the existing SQL LIKE keyword search.
func (t *QuestionBankSearchTool) executeKeywordSearch(
	ctx context.Context,
	input QuestionBankSearchInput,
	query string,
	limit int,
	emptyData func() map[string]interface{},
) (*types.ToolResult, error) {
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
		default:
			// SQLite (including test environments); project does not use MySQL.
			searchClause = ` AND (` +
				`LOWER(questions.stem_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.answer_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(questions.analysis_text) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.question_body AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.answer_body AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.knowledge_points AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
				` OR LOWER(CAST(questions.tags AS TEXT)) LIKE LOWER(?) ESCAPE '\'` +
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

	// Structured filter clauses for keyword mode.
	structFilter, structArgs := buildQuestionStructFilter(input, dialect)

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
		AND question_sets.tenant_id = questions.tenant_id
		AND question_sets.knowledge_base_id = questions.knowledge_base_id
		AND question_sets.deleted_at IS NULL
	WHERE questions.deleted_at IS NULL
		AND ` + kbFilter + searchClause + statusFilter + structFilter + `
	ORDER BY questions.created_at DESC
	LIMIT ?`

	allArgs := append(orArgs, searchArgs...)
	allArgs = append(allArgs, statusArgs...)
	allArgs = append(allArgs, structArgs...)
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
		Output:  formatQuestionBankSearchResults(results, query, limit, questionBankSearchModeKeyword),
		Data: map[string]interface{}{
			"results":      results,
			"result_count": len(results),
			"display_type": "question_bank_results",
			"query":        query,
			"limit":        limit,
			"mode":         questionBankSearchModeKeyword,
		},
	}, nil
}

// executeSemanticSearch performs vector-based semantic search then SQL backfill.
func (t *QuestionBankSearchTool) executeSemanticSearch(
	ctx context.Context,
	input QuestionBankSearchInput,
	query string,
	limit int,
	emptyData func() map[string]interface{},
) (*types.ToolResult, error) {
	if query == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "Semantic search requires a non-empty query. Use mode=keyword for recent questions.",
		}, fmt.Errorf("semantic search requires a non-empty query")
	}

	// Guard against nil dependencies so semantic mode returns a clear
	// error instead of panicking. Keyword mode is unaffected.
	if t.knowledgeBaseService == nil || t.modelService == nil ||
		t.engineRegistry == nil || t.ownership == nil {
		return &types.ToolResult{
			Success: false,
			Error:   "Semantic question search is not available: required services are not configured.",
		}, fmt.Errorf("semantic question search requires knowledgeBaseService, modelService, engineRegistry, and ownership")
	}

	// 1. Collect question_bank KBs in scope.
	type kbTarget struct {
		kb       *types.KnowledgeBase
		tenantID uint64
	}
	var kbTargets []kbTarget
	kbIDs := t.searchTargets.GetAllKnowledgeBaseIDs()
	kbTenantMap := t.searchTargets.GetKBTenantMap()
	for _, kbID := range kbIDs {
		tenantID := kbTenantMap[kbID]
		if kbID == "" || tenantID == 0 {
			continue
		}
		kb, err := t.knowledgeBaseService.GetKnowledgeBaseByIDOnly(ctx, kbID)
		if err != nil {
			logger.Warnf(ctx, "Semantic question search: skip KB %s: %v", kbID, err)
			continue
		}
		if kb == nil || kb.Type != types.KnowledgeBaseTypeQuestionBank || kb.DeletedAt.Valid {
			continue
		}
		kbTargets = append(kbTargets, kbTarget{kb: kb, tenantID: tenantID})
	}
	if len(kbTargets) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  "No valid question bank knowledge bases in scope for semantic search.",
			Data:    emptyData(),
		}, nil
	}

	// 2. For each KB, resolve embedding + engine via retriever factory,
	//    generate embedding, retrieve.
	type kbRetrieval struct {
		kbID             string
		tenantID         uint64
		embeddingModelID string
		engine           *retriever.CompositeRetrieveEngine
	}
	var retrievals []kbRetrieval
	for _, kbt := range kbTargets {
		kb := kbt.kb
		if kb.EmbeddingModelID == "" {
			logger.Warnf(ctx, "Semantic question search: KB %s has no embedding model configured", kb.ID)
			continue
		}
		engine, err := retriever.CreateRetrieveEngineForKB(
			ctx, t.engineRegistry, t.ownership, kbt.tenantID, kb.VectorStoreID,
		)
		if err != nil {
			logger.Warnf(ctx, "Semantic question search: cannot resolve engine for KB %s: %v", kb.ID, err)
			continue
		}
		if engine == nil {
			logger.Warnf(ctx, "Semantic question search: KB %s has no vector retriever available", kb.ID)
			continue
		}
		// Check that the engine supports vector retrieval.
		if !compositeSupportsVector(engine) {
			logger.Warnf(ctx, "Semantic question search: KB %s engine does not support vector retrieval", kb.ID)
			continue
		}
		retrievals = append(retrievals, kbRetrieval{
			kbID:             kb.ID,
			tenantID:         kbt.tenantID,
			embeddingModelID: kb.EmbeddingModelID,
			engine:           engine,
		})
	}
	if len(retrievals) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "Semantic question search requires a question bank knowledge base with a vector retriever and embedding model configured.",
		}, fmt.Errorf("semantic question search requires vector retriever and embedding model")
	}

	// 3. Compute topK with overfetch.
	topK := limit * semanticOverfetchFactor
	if topK < semanticMinTopK {
		topK = semanticMinTopK
	}
	if topK > semanticMaxTopK {
		topK = semanticMaxTopK
	}

	// 4. For each KB retrieval, generate embedding (cached by model ID)
	//    and query its own engine. Each KB uses its own resolved engine so
	//    that KBs with different vector stores are queried independently.
	embeddingCache := make(map[string][]float32)

	var orderedIDs []string
	idToScore := make(map[string]float64)
	idToKB := make(map[string]string)
	seen := make(map[string]bool)

	for _, r := range retrievals {
		// Resolve or cache the query embedding.
		embedding, ok := embeddingCache[r.embeddingModelID]
		if !ok {
			embedder, err := t.modelService.GetEmbeddingModel(ctx, r.embeddingModelID)
			if err != nil {
				logger.Warnf(ctx, "Semantic question search: cannot get embedding model %s: %v", r.embeddingModelID, err)
				continue
			}
			embedding, err = embedder.Embed(ctx, query)
			if err != nil {
				logger.Warnf(ctx, "Semantic question search: embedding failed for model %s: %v", r.embeddingModelID, err)
				continue
			}
			embeddingCache[r.embeddingModelID] = embedding
		}

		params := types.RetrieveParams{
			Query:            query,
			Embedding:        embedding,
			KnowledgeBaseIDs: []string{r.kbID},
			TopK:             topK,
			RetrieverType:    types.VectorRetrieverType,
			KnowledgeType:    types.KnowledgeTypeQuestion,
		}
		if input.QuestionSetID != "" {
			params.KnowledgeIDs = []string{input.QuestionSetID}
		}
		results, err := r.engine.Retrieve(ctx, []types.RetrieveParams{params})
		if err != nil {
			logger.Warnf(ctx, "Semantic question search: retrieve failed for KB %s: %v", r.kbID, err)
			continue
		}
		for _, retrieveResult := range results {
			if retrieveResult == nil {
				continue
			}
			for _, idx := range retrieveResult.Results {
				if idx == nil || idx.SourceID == "" {
					continue
				}
				if seen[idx.SourceID] {
					continue
				}
				seen[idx.SourceID] = true
				idToScore[idx.SourceID] = idx.Score
				idToKB[idx.SourceID] = idx.KnowledgeBaseID
				orderedIDs = append(orderedIDs, idx.SourceID)
			}
		}
	}

	if len(orderedIDs) == 0 {
		return &types.ToolResult{
			Success: true,
			Output:  formatQuestionBankSearchResults(nil, query, limit, questionBankSearchModeSemantic),
			Data:    emptyData(),
		}, nil
	}

	// 5. SQL backfill: fetch questions for the retrieved IDs, scoped to the correct tenants.
	// Group IDs by tenant for proper isolation.
	tenantIDToIDs := make(map[uint64][]string)
	for _, id := range orderedIDs {
		kbID := idToKB[id]
		if kbID == "" {
			continue
		}
		tid := kbTenantMap[kbID]
		if tid == 0 {
			continue
		}
		tenantIDToIDs[tid] = append(tenantIDToIDs[tid], id)
	}

	var allQuestions []*types.Question
	for tenantID, ids := range tenantIDToIDs {
		if len(ids) == 0 {
			continue
		}
		questions, err := t.listQuestionsByIDs(ctx, tenantID, ids)
		if err != nil {
			logger.Warnf(ctx, "Semantic question search: backfill query failed for tenant %d: %v", tenantID, err)
			continue
		}
		allQuestions = append(allQuestions, questions...)
	}

	// Build lookup map: question_id → *Question.
	questionMap := make(map[string]*types.Question)
	for _, q := range allQuestions {
		if q != nil && q.DeletedAt.Time.IsZero() {
			questionMap[q.ID] = q
		}
	}

	// 6. Apply structured filters and build results in vector rank order.
	//    Results preserve the original vector retrieval order — no cross-engine
	//    score-based reordering.
	excludeSet := make(map[string]bool)
	for _, eid := range input.ExcludeQuestionIDs {
		excludeSet[eid] = true
	}

	var results []QuestionBankSearchResult
	for _, id := range orderedIDs {
		if len(results) >= limit {
			break
		}
		if excludeSet[id] {
			continue
		}
		q, ok := questionMap[id]
		if !ok {
			continue
		}
		// Tenant scope: must match the KB's tenant.
		kbID := idToKB[id]
		if kbID == "" {
			continue
		}
		expectedTenant := kbTenantMap[kbID]
		if q.TenantID != expectedTenant {
			continue
		}
		// KB scope: only question_bank KBs in searchTargets.
		if !t.searchTargets.ContainsKB(q.KnowledgeBaseID) {
			continue
		}
		// Per-KB question_set_id restriction.
		if input.QuestionSetID != "" && q.QuestionSetID != input.QuestionSetID {
			continue
		}
		// Structured filters.
		if !questionMatchesFilters(q, input) {
			continue
		}

		results = append(results, QuestionBankSearchResult{
			ID:              q.ID,
			QuestionSetID:   q.QuestionSetID,
			KnowledgeBaseID: q.KnowledgeBaseID,
			QuestionType:    q.QuestionType,
			StemText:        q.StemText,
			QuestionBody:    q.QuestionBody,
			AnswerText:      q.AnswerText,
			AnswerBody:      q.AnswerBody,
			AnalysisText:    q.AnalysisText,
			Difficulty:      string(q.Difficulty),
			KnowledgePoints: q.KnowledgePoints,
			Tags:            q.Tags,
			Status:          string(q.Status),
			MatchType:       "semantic",
			Score:           idToScore[id],
		})
	}

	// Batch-fetch question set names.
	setIDs := make([]string, 0, len(results))
	seenSets := make(map[string]bool)
	for _, r := range results {
		if !seenSets[r.QuestionSetID] {
			seenSets[r.QuestionSetID] = true
			setIDs = append(setIDs, r.QuestionSetID)
		}
	}
	setNameMap := t.batchGetQuestionSetNames(ctx, setIDs)

	for i := range results {
		results[i].Rank = i + 1
		if name, ok := setNameMap[results[i].QuestionSetID]; ok {
			results[i].QuestionSetName = name
		}
	}
	if results == nil {
		results = []QuestionBankSearchResult{}
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatQuestionBankSearchResults(results, query, limit, questionBankSearchModeSemantic),
		Data: map[string]interface{}{
			"results":      results,
			"result_count": len(results),
			"display_type": "question_bank_results",
			"query":        query,
			"limit":        limit,
			"mode":         questionBankSearchModeSemantic,
		},
	}, nil
}

// compositeSupportsVector checks whether a CompositeRetrieveEngine supports
// vector retrieval across any of its internal engines.
func compositeSupportsVector(engine *retriever.CompositeRetrieveEngine) bool {
	if engine == nil {
		return false
	}
	return engine.SupportRetriever(types.VectorRetrieverType)
}

// listQuestionsByIDs fetches questions scoped to a tenant. Uses explicit
// column selection to avoid time.Time scan issues with SQLite in tests.
func (t *QuestionBankSearchTool) listQuestionsByIDs(ctx context.Context, tenantID uint64, ids []string) ([]*types.Question, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var questions []*types.Question
	if err := t.db.WithContext(ctx).
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
func (t *QuestionBankSearchTool) batchGetQuestionSetNames(ctx context.Context, setIDs []string) map[string]string {
	if len(setIDs) == 0 {
		return nil
	}
	type row struct {
		ID   string `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []row
	if err := t.db.WithContext(ctx).
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
	// Knowledge points: for each target KP, check if it appears in the JSON array.
	// Use LIKE with escaped wildcards; dialect-aware operator selection.
	for _, kp := range input.KnowledgePoints {
		escaped := escapeLike(kp)
		pattern := "%" + escaped + "%"
		clauses = append(clauses, kpLikeClause("questions.knowledge_points", dialect))
		args = append(args, pattern)
	}
	// Tags: same approach as knowledge points.
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

// questionMatchesFilters checks a question against the structured filters
// (used in semantic mode where filtering happens post-retrieval in Go).
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
	// Knowledge points: question must have at least one matching KP.
	if len(input.KnowledgePoints) > 0 {
		if !anyJSONContains(string(q.KnowledgePoints), input.KnowledgePoints) {
			return false
		}
	}
	// Tags: question must have at least one matching tag.
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
	if mode == questionBankSearchModeSemantic {
		modeLabel = " (semantic)"
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

// Ensure QuestionBankSearchTool implements Tool interface.
var _ types.Tool = (*QuestionBankSearchTool)(nil)

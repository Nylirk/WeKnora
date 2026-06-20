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

const (
	questionBankSearchModeKeyword  = "keyword"
	questionBankSearchModeSemantic = "semantic"
	questionBankSearchModeHybrid   = "hybrid"

	semanticOverfetchFactor = 5
	semanticMinTopK         = 100
	semanticMaxTopK         = 300

	keywordOverfetchFactor = 5
	keywordCandidateMin    = 100
	keywordCandidateMax    = 300
)

// questionBankSearchTool is the base definition (name, description, schema).
var questionBankSearchTool = BaseTool{
	name: ToolQuestionBankSearch,
	description: `Search questions in a question bank knowledge base.

Accepts an optional keyword query and searches across stem_text, answer_text,
analysis_text, question_body, answer_body, knowledge_points, and tags fields.
When the query is empty or whitespace, lists recent questions in scope.

Supports three modes:
- keyword (default): SQL LIKE search across question fields
- semantic: vector/embedding-based semantic search using question vector indexes
- hybrid: RRF fusion of keyword and semantic results

## When to use
- Use when the user asks about questions, exam problems, or quiz content in a
  question bank knowledge base.
- Do NOT use this for general document or chunk search — use knowledge_search
  or grep_chunks instead.

## Returns per result
- question_id, question_set_id, question_set_name, knowledge_base_id
- question_type, stem_text, question_body, answer_text, answer_body
- analysis_text, difficulty, knowledge_points, tags, status
- mode, match_type, score, rank`,
	schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Optional keyword or semantic query to search for across question fields. When empty or omitted, lists recent questions in scope."
    },
    "mode": {
      "type": "string",
      "enum": ["keyword", "semantic", "hybrid"],
      "description": "Search mode: keyword (SQL LIKE, default), semantic (vector embedding), or hybrid (RRF fusion of keyword + semantic). Defaults to keyword for backward compatibility.",
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
	ID               string     `json:"question_id"       gorm:"column:id"`
	QuestionSetID    string     `json:"question_set_id"   gorm:"column:question_set_id"`
	QuestionSetName  string     `json:"question_set_name" gorm:"column:question_set_name"`
	KnowledgeBaseID  string     `json:"knowledge_base_id" gorm:"column:knowledge_base_id"`
	QuestionType     string     `json:"question_type"     gorm:"column:question_type"`
	StemText         string     `json:"stem_text"         gorm:"column:stem_text"`
	QuestionBody     types.JSON `json:"question_body"     gorm:"column:question_body"`
	AnswerText       string     `json:"answer_text"       gorm:"column:answer_text"`
	AnswerBody       types.JSON `json:"answer_body"       gorm:"column:answer_body"`
	AnalysisText     string     `json:"analysis_text"     gorm:"column:analysis_text"`
	Difficulty       string     `json:"difficulty"        gorm:"column:difficulty"`
	KnowledgePoints  types.JSON `json:"knowledge_points"  gorm:"column:knowledge_points"`
	Tags             types.JSON `json:"tags"              gorm:"column:tags"`
	Status           string     `json:"status"            gorm:"column:status"`
	MatchType        string     `json:"match_type,omitempty"       gorm:"-"`
	Score            float64    `json:"score,omitempty"             gorm:"-"`
	Rank             int        `json:"rank,omitempty"              gorm:"-"`
	KeywordRank      int        `json:"keyword_rank,omitempty"      gorm:"-"`
	SemanticRank     int        `json:"semantic_rank,omitempty"     gorm:"-"`
	RRFScore         float64    `json:"rrf_score,omitempty"         gorm:"-"`
	SourceQuestionID string     `json:"source_question_id,omitempty" gorm:"-"`
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
	if mode != questionBankSearchModeKeyword && mode != questionBankSearchModeSemantic && mode != questionBankSearchModeHybrid {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid mode %q: must be keyword, semantic, or hybrid", mode),
		}, fmt.Errorf("invalid mode %q", mode)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	validStatuses := map[string]bool{"": true, "draft": true, "reviewed": true, "rejected": true}
	if !validStatuses[input.Status] {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Invalid status %q: must be one of draft, reviewed, rejected, or empty", input.Status),
		}, fmt.Errorf("invalid status %q", input.Status)
	}

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

	switch mode {
	case questionBankSearchModeSemantic:
		return t.executeSemanticSearch(ctx, input, query, limit, emptyData)
	case questionBankSearchModeHybrid:
		return t.executeHybridSearch(ctx, input, query, limit, emptyData)
	default:
		return t.executeKeywordSearch(ctx, input, query, limit, emptyData)
	}
}

// ---- keyword search ----

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

	dialect := t.db.Dialector.Name()
	var searchClause string
	var searchArgs []interface{}

	if query != "" {
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

	var statusFilter string
	var statusArgs []interface{}
	if input.Status != "" {
		statusFilter = ` AND questions.status = ?`
		statusArgs = []interface{}{input.Status}
	}

	structFilter, structArgs := buildQuestionStructFilter(input, dialect)

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

// ---- semantic search ----

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

	if t.knowledgeBaseService == nil || t.modelService == nil ||
		t.engineRegistry == nil || t.ownership == nil {
		return &types.ToolResult{
			Success: false,
			Error:   "Semantic question search is not available: required services are not configured.",
		}, fmt.Errorf("semantic question search requires knowledgeBaseService, modelService, engineRegistry, and ownership")
	}

	candidates, err := t.runSemanticCandidate(ctx, input, query, limit)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Semantic search failed: %v", err),
		}, err
	}

	results := candidatesToResults(candidates)
	if results == nil {
		results = []QuestionBankSearchResult{}
	}
	for i := range results {
		results[i].Rank = i + 1
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

// runSemanticCandidate returns raw candidates (unpaged, unfiltered by limit —
// the caller applies limit and final formatting). It is shared by semantic
// and hybrid modes.
func (t *QuestionBankSearchTool) runSemanticCandidate(
	ctx context.Context,
	input QuestionBankSearchInput,
	query string,
	limit int,
) ([]questionSearchCandidate, error) {
	type kbTarget struct {
		kb       *types.KnowledgeBase
		tenantID uint64
	}
	kbIDs := t.searchTargets.GetAllKnowledgeBaseIDs()
	kbTenantMap := t.searchTargets.GetKBTenantMap()
	var kbTargets []kbTarget
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
		return nil, nil
	}

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
		if engine == nil || !compositeSupportsVector(engine) {
			logger.Warnf(ctx, "Semantic question search: KB %s has no vector retriever available", kb.ID)
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
		return nil, fmt.Errorf("semantic question search requires vector retriever and embedding model")
	}

	topK := limit * semanticOverfetchFactor
	if topK < semanticMinTopK {
		topK = semanticMinTopK
	}
	if topK > semanticMaxTopK {
		topK = semanticMaxTopK
	}

	embeddingCache := make(map[string][]float32)
	var orderedIDs []string
	idToScore := make(map[string]float64)
	idToKB := make(map[string]string)
	seen := make(map[string]bool)

	for _, r := range retrievals {
		emb, ok := embeddingCache[r.embeddingModelID]
		if !ok {
			embedder, err := t.modelService.GetEmbeddingModel(ctx, r.embeddingModelID)
			if err != nil {
				logger.Warnf(ctx, "Semantic question search: cannot get embedding model %s: %v", r.embeddingModelID, err)
				continue
			}
			emb, err = embedder.Embed(ctx, query)
			if err != nil {
				logger.Warnf(ctx, "Semantic question search: embedding failed for model %s: %v", r.embeddingModelID, err)
				continue
			}
			embeddingCache[r.embeddingModelID] = emb
		}
		params := types.RetrieveParams{
			Query:            query,
			Embedding:        emb,
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
		return nil, nil
	}

	// SQL backfill.
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
		questions, err := listQuestionsByIDs(ctx, t.db, tenantID, ids)
		if err != nil {
			logger.Warnf(ctx, "Semantic question search: backfill query failed for tenant %d: %v", tenantID, err)
			continue
		}
		allQuestions = append(allQuestions, questions...)
	}

	questionMap := make(map[string]*types.Question)
	for _, q := range allQuestions {
		if q != nil && q.DeletedAt.Time.IsZero() {
			questionMap[q.ID] = q
		}
	}

	excludeSet := make(map[string]bool)
	for _, eid := range input.ExcludeQuestionIDs {
		excludeSet[eid] = true
	}

	var candidates []questionSearchCandidate
	for _, id := range orderedIDs {
		if excludeSet[id] {
			continue
		}
		q, ok := questionMap[id]
		if !ok {
			continue
		}
		kbID := idToKB[id]
		if kbID == "" {
			continue
		}
		expectedTenant := kbTenantMap[kbID]
		if q.TenantID != expectedTenant {
			continue
		}
		if !t.searchTargets.ContainsKB(q.KnowledgeBaseID) {
			continue
		}
		if input.QuestionSetID != "" && q.QuestionSetID != input.QuestionSetID {
			continue
		}
		if !questionMatchesFilters(q, input) {
			continue
		}
		candidates = append(candidates, questionSearchCandidate{
			QuestionID:    q.ID,
			SemanticScore: idToScore[id],
			Result: QuestionBankSearchResult{
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
			},
		})
	}

	// Populate question set names.
	setIDs := make([]string, 0, len(candidates))
	seenSets := make(map[string]bool)
	for _, c := range candidates {
		if !seenSets[c.Result.QuestionSetID] {
			seenSets[c.Result.QuestionSetID] = true
			setIDs = append(setIDs, c.Result.QuestionSetID)
		}
	}
	setNameMap := batchGetQuestionSetNames(ctx, t.db, setIDs)
	for i := range candidates {
		if name, ok := setNameMap[candidates[i].Result.QuestionSetID]; ok {
			candidates[i].Result.QuestionSetName = name
		}
	}

	// Assign semantic ranks (1-based, in vector order).
	for i := range candidates {
		candidates[i].SemanticRank = i + 1
	}

	return candidates, nil
}

// ---- hybrid search ----

func (t *QuestionBankSearchTool) executeHybridSearch(
	ctx context.Context,
	input QuestionBankSearchInput,
	query string,
	limit int,
	emptyData func() map[string]interface{},
) (*types.ToolResult, error) {
	if query == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "Hybrid search requires a non-empty query.",
		}, fmt.Errorf("hybrid search requires a non-empty query")
	}

	var keywordCandidates, semanticCandidates []questionSearchCandidate
	var semanticWarning string
	keywordCandidateCount := 0
	semanticCandidateCount := 0

	// Run keyword candidate.
	kwLimit := limit * keywordOverfetchFactor
	if kwLimit < keywordCandidateMin {
		kwLimit = keywordCandidateMin
	}
	if kwLimit > keywordCandidateMax {
		kwLimit = keywordCandidateMax
	}
	var kwErr error
	keywordCandidates, kwErr = t.runKeywordCandidate(ctx, input, query, kwLimit)
	if kwErr != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Hybrid search keyword phase failed: %v", kwErr),
		}, kwErr
	}
	keywordCandidateCount = len(keywordCandidates)

	// Run semantic candidate (best-effort).
	if t.knowledgeBaseService != nil && t.modelService != nil &&
		t.engineRegistry != nil && t.ownership != nil {
		semCands, semErr := t.runSemanticCandidate(ctx, input, query, limit)
		if semErr != nil {
			semanticWarning = fmt.Sprintf("Semantic phase unavailable: %v", semErr)
			logger.Warnf(ctx, "Hybrid search: semantic phase failed: %v", semErr)
		} else {
			semanticCandidates = semCands
		}
	} else {
		semanticWarning = "Semantic search is not available: required services are not configured."
		logger.Warnf(ctx, "Hybrid search: semantic dependencies not configured, using keyword-only")
	}
	semanticCandidateCount = len(semanticCandidates)

	// RRF fusion.
	fused := fuseCandidatesRRF(keywordCandidates, semanticCandidates)
	results := candidatesToResults(fused)

	// Apply limit.
	if len(results) > limit {
		results = results[:limit]
	}
	if results == nil {
		results = []QuestionBankSearchResult{}
	}
	for i := range results {
		results[i].Rank = i + 1
		results[i].MatchType = "hybrid"
	}

	data := map[string]interface{}{
		"results":                  results,
		"result_count":             len(results),
		"display_type":             "question_bank_results",
		"query":                    query,
		"limit":                    limit,
		"mode":                     questionBankSearchModeHybrid,
		"fusion":                   "rrf",
		"keyword_candidate_count":  keywordCandidateCount,
		"semantic_candidate_count": semanticCandidateCount,
	}
	if semanticWarning != "" {
		data["semantic_warning"] = semanticWarning
	}

	return &types.ToolResult{
		Success: true,
		Output:  formatQuestionBankSearchResults(results, query, limit, questionBankSearchModeHybrid),
		Data:    data,
	}, nil
}

// runKeywordCandidate returns raw keyword candidates (overfetched, unfiltered
// by final limit). It is shared by keyword and hybrid modes.
func (t *QuestionBankSearchTool) runKeywordCandidate(
	ctx context.Context,
	input QuestionBankSearchInput,
	query string,
	limit int,
) ([]questionSearchCandidate, error) {
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
		return nil, nil
	}
	kbFilter := "(" + strings.Join(orClauses, " OR ") + ")"

	dialect := t.db.Dialector.Name()
	var searchClause string
	var searchArgs []interface{}

	if query != "" {
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

	var statusFilter string
	var statusArgs []interface{}
	if input.Status != "" {
		statusFilter = ` AND questions.status = ?`
		statusArgs = []interface{}{input.Status}
	}

	structFilter, structArgs := buildQuestionStructFilter(input, dialect)

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

	var rows []QuestionBankSearchResult
	if err := t.db.WithContext(ctx).Raw(baseSQL, allArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	candidates := make([]questionSearchCandidate, 0, len(rows))
	for i, row := range rows {
		candidates = append(candidates, questionSearchCandidate{
			QuestionID:  row.ID,
			KeywordRank: i + 1,
			Result:      row,
		})
	}
	return candidates, nil
}

// candidatesToResults converts fused/sorted candidates to results with ranks.
func candidatesToResults(candidates []questionSearchCandidate) []QuestionBankSearchResult {
	results := make([]QuestionBankSearchResult, 0, len(candidates))
	for i := range candidates {
		c := &candidates[i]
		r := c.Result
		r.KeywordRank = c.KeywordRank
		r.SemanticRank = c.SemanticRank
		r.RRFScore = c.RRFScore
		if r.Score == 0 {
			r.Score = c.SemanticScore
		}
		results = append(results, r)
	}
	return results
}

// Ensure QuestionBankSearchTool implements Tool interface.
var _ types.Tool = (*QuestionBankSearchTool)(nil)

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mock ModelService for rerank tests ──

type mockModelService struct {
	interfaces.ModelService
	reranker  rerank.Reranker
	rerankErr error
	chatCalls int
}

func (m *mockModelService) GetRerankModel(_ context.Context, _ string) (rerank.Reranker, error) {
	if m.rerankErr != nil {
		return nil, m.rerankErr
	}
	return m.reranker, nil
}

func (m *mockModelService) GetChatModel(_ context.Context, _ string) (chat.Chat, error) {
	m.chatCalls++
	return nil, errors.New("GetChatModel should not be called")
}

// ── Mock Reranker ──

type mockReranker struct {
	rerank.Reranker
	results    []rerank.RankResult
	err        error
	called     bool
	capturedQuery string
	capturedDocs []string
}

func (m *mockReranker) Rerank(_ context.Context, query string, documents []string) ([]rerank.RankResult, error) {
	m.called = true
	m.capturedQuery = query
	m.capturedDocs = documents
	return m.results, m.err
}

func (m *mockReranker) GetModelName() string { return "mock-reranker" }
func (m *mockReranker) GetModelID() string   { return "rerank-mock-1" }

// ── Helpers ──

func makeRerankTestService(kbSvc *mockKBService, modelSvc *mockModelService) *QuestionService {
	return &QuestionService{
		repository:     &matchingTestRepo{},
		knowledgeBaseSvc: kbSvc,
		modelService:   modelSvc,
	}
}

func makeRerankConfig(kbID string, rerankModelID string, enabled bool) *types.QuestionBankConfig {
	return &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   kbID,
		KnowledgePointRerankModelID:     rerankModelID,
		KnowledgePointRerankEnabled:     enabled,
	}
}

func extractTaggingMeta(t *testing.T, q *types.Question) map[string]any {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(q.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("failed to parse extraction_metadata: %v", err)
	}
	autoProc, _ := meta["auto_processing"].(map[string]any)
	tagging, _ := autoProc["auto_tagging"].(map[string]any)
	return tagging
}

func extractCandidates(t *testing.T, q *types.Question) []map[string]any {
	t.Helper()
	tagging := extractTaggingMeta(t, q)
	candidates, _ := tagging["candidates"].([]any)
	parsed := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		if cm, ok := c.(map[string]any); ok {
			parsed = append(parsed, cm)
		}
	}
	return parsed
}

// ── Tests ──

// Test 1: Configured rerank model is called and results are used.
func TestKnowledgePointModelRerank_UsesConfiguredRerankModel(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "Correct KP", Content: "correct content", Score: 0.70},
		{ID: "c2", KnowledgeID: "k2", KnowledgeTitle: "Wrong KP", Content: "wrong content", Score: 0.80},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{
			{Index: 0, RelevanceScore: 0.95},
			{Index: 1, RelevanceScore: 0.50},
		},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r1")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reranker.called {
		t.Fatal("expected reranker.Rerank to be called")
	}
	if reranker.capturedQuery == "" {
		t.Error("expected non-empty query passed to reranker")
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "model" {
		t.Errorf("expected rerank_mode=model, got %v", tagging["rerank_mode"])
	}
	if tagging["rerank_model_id"] != "rerank-1" {
		t.Errorf("expected rerank_model_id=rerank-1, got %v", tagging["rerank_model_id"])
	}
}

// Test 2: Rerank reorders candidates — raw top1 is wrong, rerank puts correct on top.
func TestKnowledgePointModelRerank_ReordersCandidates(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "Wrong KP", Content: "wrong", Score: 0.85},
		{ID: "c2", KnowledgeID: "k2", KnowledgeTitle: "Correct KP", Content: "correct", Score: 0.70},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{
			{Index: 1, RelevanceScore: 0.95},
			{Index: 0, RelevanceScore: 0.40},
		},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r2")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	top1, _ := candidates[0]["knowledge_point"].(string)
	if top1 != "Correct KP" {
		t.Errorf("expected top1=Correct KP after rerank, got %q", top1)
	}
}

// Test 3: No rerank model configured → fallback, no error.
func TestKnowledgePointModelRerank_FallsBackWhenNoModelConfigured(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	modelSvc := &mockModelService{}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}
	q := makeTestQuestion("q-r3")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "disabled" {
		t.Errorf("expected rerank_mode=disabled, got %v", tagging["rerank_mode"])
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates despite no rerank model")
	}
}

// Test 4: Model error → fallback with rerank_error in metadata.
func TestKnowledgePointModelRerank_FallsBackOnModelError(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
		{ID: "c2", KnowledgeID: "k2", KnowledgeTitle: "KP2", Content: "content2", Score: 0.72},
	}
	kbSvc := makeMockKBService(results, nil)
	modelSvc := &mockModelService{rerankErr: errors.New("model unavailable")}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r4")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "rule_fallback" {
		t.Errorf("expected rerank_mode=rule_fallback, got %v", tagging["rerank_mode"])
	}
	if tagging["rerank_error"] == nil || tagging["rerank_error"] == "" {
		t.Error("expected rerank_error in metadata")
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates despite model error")
	}
}

// Test 4b: Rerank call returns error → fallback with rerank_error.
func TestKnowledgePointModelRerank_FallsBackOnRerankCallError(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{err: errors.New("rerank API timeout")}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r4b")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "rule_fallback" {
		t.Errorf("expected rerank_mode=rule_fallback, got %v", tagging["rerank_mode"])
	}
	if tagging["rerank_error"] == nil || tagging["rerank_error"] == "" {
		t.Error("expected rerank_error in metadata")
	}
}

// Test 5: Candidate metadata includes raw_score, rerank_score, match_signals.
func TestKnowledgePointModelRerank_CandidateMetadata(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.80},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{
			{Index: 0, RelevanceScore: 0.90},
		},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r5")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	c0 := candidates[0]
	if c0["raw_score"] == nil {
		t.Error("candidate missing raw_score")
	}
	if c0["rerank_score"] == nil {
		t.Error("candidate missing rerank_score")
	}
	if c0["match_signals"] == nil {
		t.Error("candidate missing match_signals")
	}
	signals, _ := c0["match_signals"].([]any)
	foundModelRerank := false
	for _, s := range signals {
		if s == "model_rerank" {
			foundModelRerank = true
		}
	}
	if !foundModelRerank {
		t.Error("expected model_rerank in match_signals")
	}
	if c0["evidence_chunk_ids"] == nil {
		t.Error("candidate missing evidence_chunk_ids")
	}
}

// Test 6: Formal fields not mutated.
func TestKnowledgePointModelRerank_DoesNotMutateFormalFields(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.90},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.95}},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r6")
	q.KnowledgePoints = types.JSON(`["人工知识点"]`)

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Status != types.QuestionStatusDraft {
		t.Errorf("expected status=draft, got %s", q.Status)
	}
	var kps []string
	json.Unmarshal(q.KnowledgePoints, &kps)
	if len(kps) != 1 || kps[0] != "人工知识点" {
		t.Errorf("knowledge_points mutated to %v, want [人工知识点]", kps)
	}
}

// Test 7: ChatModel is never called.
func TestKnowledgePointModelRerank_DoesNotUseChatModel(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.90},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.95}},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r7")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modelSvc.chatCalls > 0 {
		t.Errorf("GetChatModel was called %d times, expected 0", modelSvc.chatCalls)
	}
}

// Test 8: Rerank returns empty results → fallback to rule.
func TestKnowledgePointModelRerank_FallsBackOnEmptyRerankResults(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{results: []rerank.RankResult{}}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r8")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "rule_fallback" {
		t.Errorf("expected rerank_mode=rule_fallback for empty rerank, got %v", tagging["rerank_mode"])
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates after fallback from empty rerank")
	}
}

// Test 9: Projection aggregation merges same-label results.
func TestKnowledgePointModelRerank_AggregatesSameLabel(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "evidence A", Score: 0.80},
		{ID: "c2", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "evidence B", Score: 0.75},
		{ID: "c3", KnowledgeID: "k2", KnowledgeTitle: "KP2", Content: "other", Score: 0.70},
	}
	kbSvc := makeMockKBService(results, nil)
	modelSvc := &mockModelService{}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}
	q := makeTestQuestion("q-r9")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	projCount, _ := tagging["projection_count"].(float64)
	if int(projCount) != 2 {
		t.Errorf("expected 2 projections (KP1 aggregated, KP2), got %v", tagging["projection_count"])
	}
	candidates := extractCandidates(t, q)
	c0 := candidates[0]
	chunkIDs, _ := c0["evidence_chunk_ids"].([]any)
	if len(chunkIDs) < 2 {
		t.Errorf("expected aggregated candidate to have >=2 evidence_chunk_ids, got %d", len(chunkIDs))
	}
}

// Test 10: Empty knowledge_title forces inferred_from_content.
func TestKnowledgePointModelRerank_InferredFromContentLabel(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "", Content: "This is a long paragraph about calculus derivatives that should be truncated.", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	modelSvc := &mockModelService{}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}
	q := makeTestQuestion("q-r10")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	candidates := extractCandidates(t, q)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0]["reason"] != "inferred_from_content" {
		t.Errorf("expected reason=inferred_from_content, got %v", candidates[0]["reason"])
	}
}

// Test 11: Rerank index out of bounds is safely ignored.
func TestKnowledgePointModelRerank_IgnoresOutOfBoundsRerankIndex(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{
			{Index: 5, RelevanceScore: 0.99},
			{Index: 0, RelevanceScore: 0.80},
		},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r11")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "model" {
		t.Errorf("expected rerank_mode=model, got %v", tagging["rerank_mode"])
	}
	candidates := extractCandidates(t, q)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

// Test 12: modelService nil with rerank enabled → rule_fallback.
func TestKnowledgePointModelRerank_NilModelServiceFallsBack(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	svc := &QuestionService{
		repository:       &matchingTestRepo{},
		knowledgeBaseSvc: kbSvc,
		modelService:     nil,
	}
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r12")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["rerank_mode"] != "rule_fallback" {
		t.Errorf("expected rerank_mode=rule_fallback for nil modelService, got %v", tagging["rerank_mode"])
	}
}

// Test 13: HybridSearch still used, no direct embedding calls.
func TestKnowledgePointModelRerank_StillUsesHybridSearch(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.90}},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r13")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kbSvc.capturedParams) == 0 {
		t.Fatal("expected HybridSearch to be called")
	}
	for i, p := range kbSvc.capturedParams {
		if p.DisableKeywordsMatch {
			t.Errorf("HybridSearch[%d] should not disable keywords for KP matching", i)
		}
	}
}

// Test 14: Metadata includes scoring and algorithm_version fields.
func TestKnowledgePointModelRerank_MetadataFields(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.90}},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r14")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagging := extractTaggingMeta(t, q)
	if tagging["scoring"] != "model_rerank_v1" {
		t.Errorf("expected scoring=model_rerank_v1, got %v", tagging["scoring"])
	}
	if tagging["algorithm_version"] != KnowledgePointAlgorithmVersion {
		t.Errorf("expected algorithm_version=%s, got %v", KnowledgePointAlgorithmVersion, tagging["algorithm_version"])
	}
	if tagging["projection_count"] == nil {
		t.Error("expected projection_count in metadata")
	}
	if tagging["candidate_count_before_limit"] == nil {
		t.Error("expected candidate_count_before_limit in metadata")
	}
}

// Test 15: Rule rerank match signals include title_source for KnowledgeTitle labels.
func TestKnowledgePointModelRerank_RuleRerankMatchSignals(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: 0.85},
	}
	kbSvc := makeMockKBService(results, nil)
	modelSvc := &mockModelService{}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}
	q := makeTestQuestion("q-r15")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	signals, _ := candidates[0]["match_signals"].([]any)
	foundTitleSource := false
	for _, s := range signals {
		if s == "title_source" {
			foundTitleSource = true
		}
	}
	if !foundTitleSource {
		t.Error("expected title_source in match_signals for KnowledgeTitle label")
	}
}

// Test 16: Verify scoring blend formula.
func TestKnowledgePointModelRerank_ScoringBlendFormula(t *testing.T) {
	rawScore := 0.70
	rerankScore := 0.90
	results := []*types.SearchResult{
		{ID: "c1", KnowledgeID: "k1", KnowledgeTitle: "KP1", Content: "content", Score: rawScore},
	}
	kbSvc := makeMockKBService(results, nil)
	reranker := &mockReranker{
		results: []rerank.RankResult{{Index: 0, RelevanceScore: rerankScore}},
	}
	modelSvc := &mockModelService{reranker: reranker}
	svc := makeRerankTestService(kbSvc, modelSvc)
	cfg := makeRerankConfig("kp-kb", "rerank-1", true)
	q := makeTestQuestion("q-r16")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	candidates := extractCandidates(t, q)
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	score, _ := candidates[0]["score"].(float64)
	expected := kpRerankScoreWeight*rerankScore + kpRawScoreWeight*rawScore
	if score < expected-0.01 || score > expected+0.01 {
		t.Errorf("expected score≈%.4f (blend), got %.4f", expected, score)
	}
}

// Test 17: String helper for overlap detection.
func TestLabelOverlap(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		candidate string
		want      bool
	}{
		{name: "exact word match", query: "recursion programming", candidate: "Recursion", want: true},
		{name: "no match", query: "TCP handshake", candidate: "光合作用", want: false},
		{name: "empty query", query: "", candidate: "something", want: false},
		{name: "empty candidate", query: "something", candidate: "", want: false},
		{name: "short token ignored", query: "x y", candidate: "x", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelOverlap(tt.query, tt.candidate)
			if got != tt.want {
				t.Errorf("labelOverlap(%q, %q) = %v, want %v", tt.query, tt.candidate, got, tt.want)
			}
		})
	}
}

// Test 18: truncateText edge cases.
func TestTruncateText_EdgeCases(t *testing.T) {
	if truncateText("", 10) != "" {
		t.Error("expected empty string for empty input")
	}
	if truncateText("hello", 0) != "" {
		t.Error("expected empty string for maxLen=0")
	}
	if truncateText("hello", -1) != "" {
		t.Error("expected empty string for negative maxLen")
	}
	long := strings.Repeat("a", 100)
	if len(truncateText(long, 10)) != 10 {
		t.Error("expected 10 chars truncation")
	}
}

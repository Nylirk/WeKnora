package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mock KnowledgeBaseService for semantic matching tests ──

type mockKBService struct {
	interfaces.KnowledgeBaseService
	searchResults []*types.SearchResult
	searchErr     error
}

func (m *mockKBService) HybridSearch(_ context.Context, _ string, _ types.SearchParams) ([]*types.SearchResult, error) {
	return m.searchResults, m.searchErr
}

// ── Mock QuestionRepository for semantic matching tests ──

type matchingTestRepo struct {
	interfaces.QuestionRepository
	updatedQuestions []*types.Question
}

func (r *matchingTestRepo) UpdateQuestion(_ context.Context, q *types.Question) error {
	r.updatedQuestions = append(r.updatedQuestions, q)
	return nil
}

// ── Helper ──

func makeTestQuestion(id string) *types.Question {
	return &types.Question{
		ID:           id,
		QuestionType: "single_choice",
		Difficulty:   types.QuestionDifficultyMedium,
		StemText:     "What is the capital of France?",
		QuestionBody: types.JSON(json.RawMessage(`{"options":[{"label":"A","content":"Paris"},{"label":"B","content":"London"}]}`)),
		Status:       types.QuestionStatusDraft,
		ExtractionMetadata: types.JSON(json.RawMessage(`{}`)),
	}
}

func makeMockKBService(results []*types.SearchResult, err error) *mockKBService {
	return &mockKBService{searchResults: results, searchErr: err}
}

// ── Tests ──

// Test 1: No knowledge point KB → auto_tagging = paused
func TestAutoTagging_Paused_WhenNoKB(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(nil, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: ""}
	q := makeTestQuestion("q1")

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.AutoTaggingStatus != "paused" {
		t.Errorf("expected auto_tagging_status=paused, got %s", q.AutoTaggingStatus)
	}
}

// Test 2: No syllabus KB → syllabus_checking = paused
func TestSyllabusFiltering_Paused_WhenNoKB(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(nil, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: ""}
	q := makeTestQuestion("q2")

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SyllabusCheckingStatus != "paused" {
		t.Errorf("expected syllabus_checking_status=paused, got %s", q.SyllabusCheckingStatus)
	}
}

// Test 3: High-score knowledge point chunk → candidates written, status = matched
func TestAutoTagging_Matched_WithHighScore(t *testing.T) {
	results := []*types.SearchResult{
		{
			ID:             "chunk1",
			Content:        "This is about French geography and capitals.",
			KnowledgeID:    "know1",
			KnowledgeTitle: "Geography",
			Score:          0.85,
		},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp_kb_1"}
	q := makeTestQuestion("q3")

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.AutoTaggingStatus != "matched" {
		t.Errorf("expected auto_tagging_status=matched, got %s", q.AutoTaggingStatus)
	}

	// Verify metadata contains candidates.
	var meta map[string]any
	if err := json.Unmarshal(q.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("failed to parse extraction_metadata: %v", err)
	}
	autoProc, ok := meta["auto_processing"].(map[string]any)
	if !ok {
		t.Fatal("auto_processing missing")
	}
	tagging, ok := autoProc["auto_tagging"].(map[string]any)
	if !ok {
		t.Fatal("auto_tagging missing")
	}
	if tagging["status"] != "matched" {
		t.Errorf("expected status=matched, got %v", tagging["status"])
	}
	candidates, ok := tagging["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatal("expected candidates to be present")
	}
	// Verify candidate structure.
	c0, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatal("candidate[0] is not a map")
	}
	if c0["knowledge_point"] == nil || c0["knowledge_point"] == "" {
		t.Error("candidate missing knowledge_point")
	}
	if c0["confidence"] == nil {
		t.Error("candidate missing confidence")
	}
	if c0["evidence_chunk_id"] == nil || c0["evidence_chunk_id"] == "" {
		t.Error("candidate missing evidence_chunk_id")
	}
}

// Test 4: High-score syllabus chunk → in_scope
func TestSyllabusFiltering_InScope_WhenHighScore(t *testing.T) {
	results := []*types.SearchResult{
		{
			ID:          "chunk_syl1",
			Content:     "Unit 1: European Geography — capitals and countries",
			KnowledgeID: "syl_know1",
			Score:       0.85,
		},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl_kb_1"}
	q := makeTestQuestion("q4")

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SyllabusCheckingStatus != "completed" {
		t.Errorf("expected syllabus_checking_status=completed, got %s", q.SyllabusCheckingStatus)
	}
	if q.SyllabusScopeResult != "in_scope" {
		t.Errorf("expected syllabus_scope_result=in_scope, got %s", q.SyllabusScopeResult)
	}
}

// Test 5: Medium-score syllabus chunk → uncertain
func TestSyllabusFiltering_Uncertain_WhenMediumScore(t *testing.T) {
	results := []*types.SearchResult{
		{
			ID:          "chunk_syl2",
			Content:     "Some partially related content",
			KnowledgeID: "syl_know2",
			Score:       0.60,
		},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl_kb_2"}
	q := makeTestQuestion("q5")

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SyllabusScopeResult != "uncertain" {
		t.Errorf("expected syllabus_scope_result=uncertain, got %s", q.SyllabusScopeResult)
	}
}

// Test 6: Low-score or empty syllabus results → out_of_scope
func TestSyllabusFiltering_OutOfScope_WhenLowScore(t *testing.T) {
	results := []*types.SearchResult{
		{
			ID:          "chunk_syl3",
			Content:     "Unrelated content",
			KnowledgeID: "syl_know3",
			Score:       0.30,
		},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl_kb_3"}
	q := makeTestQuestion("q6")

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SyllabusScopeResult != "out_of_scope" {
		t.Errorf("expected syllabus_scope_result=out_of_scope, got %s", q.SyllabusScopeResult)
	}
}

// Test 6b: Empty syllabus results → out_of_scope
func TestSyllabusFiltering_OutOfScope_WhenEmpty(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService([]*types.SearchResult{}, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl_kb_4"}
	q := makeTestQuestion("q6b")

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SyllabusScopeResult != "out_of_scope" {
		t.Errorf("expected syllabus_scope_result=out_of_scope, got %s", q.SyllabusScopeResult)
	}
}

// Test 7: question.status stays draft after matching
func TestQuestionStatus_StaysDraft(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", Content: "x", KnowledgeID: "k1", Score: 0.90},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp_kb",
		SyllabusKnowledgeBaseID:       "syl_kb",
	}
	q := makeTestQuestion("q7")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("auto_tagging error: %v", err)
	}
	if err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("syllabus_checking error: %v", err)
	}
	if q.Status != types.QuestionStatusDraft {
		t.Errorf("expected status=draft, got %s", q.Status)
	}
}

// Test 8: No reviewed / rejected auto-transition
func TestNoAutoReviewOrReject(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c1", Content: "x", KnowledgeID: "k1", Score: 0.90},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp_kb",
	}
	q := makeTestQuestion("q8")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("auto_tagging error: %v", err)
	}
	if q.Status == types.QuestionStatusReviewed || q.Status == types.QuestionStatusRejected {
		t.Errorf("question should not be auto-reviewed or auto-rejected, got %s", q.Status)
	}
}

// Test 9: Empty search results → auto_tagging_status = unmatched
func TestAutoTagging_Unmatched_WhenEmptyResults(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService([]*types.SearchResult{}, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp_kb"}
	q := makeTestQuestion("q9")

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.AutoTaggingStatus != "unmatched" {
		t.Errorf("expected auto_tagging_status=unmatched, got %s", q.AutoTaggingStatus)
	}

	// Verify metadata status is "unmatched".
	var meta map[string]any
	if err := json.Unmarshal(q.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("failed to parse extraction_metadata: %v", err)
	}
	autoProc := meta["auto_processing"].(map[string]any)
	tagging := autoProc["auto_tagging"].(map[string]any)
	if tagging["status"] != "unmatched" {
		t.Errorf("expected metadata status=unmatched, got %v", tagging["status"])
	}
	candidates := tagging["candidates"].([]any)
	if len(candidates) != 0 {
		t.Errorf("expected empty candidates, got %d", len(candidates))
	}
}

// Test 10: Matched status stays matched, not overwritten by later calls.
func TestAutoTagging_Matched_StaysMatched(t *testing.T) {
	results := []*types.SearchResult{
		{
			ID:             "chunk10",
			Content:        "Geography lesson",
			KnowledgeID:    "know10",
			KnowledgeTitle: "Geography",
			Score:          0.75,
		},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp_kb"}
	q := makeTestQuestion("q10")

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.AutoTaggingStatus != "matched" {
		t.Errorf("expected auto_tagging_status=matched, got %s", q.AutoTaggingStatus)
	}
	// Still draft after matched.
	if q.Status != types.QuestionStatusDraft {
		t.Errorf("expected status=draft, got %s", q.Status)
	}
}

// Test 11: Paused status from missing config is still preserved.
func TestAutoTagging_Paused_Preserved(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(nil, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: ""}
	q := makeTestQuestion("q11")
	// Pre-set a value to ensure it gets overwritten to paused.
	q.AutoTaggingStatus = "pending"

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.AutoTaggingStatus != "paused" {
		t.Errorf("expected auto_tagging_status=paused when KB missing, got %s", q.AutoTaggingStatus)
	}
}

// Test 12: Reprocess overwrites existing auto_tagging candidates.
func TestRunKnowledgePointMatching_OverwritesExistingAutoTagging(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "new-chunk", Content: "New knowledge", KnowledgeID: "know-new", KnowledgeTitle: "新知识点", Score: 0.92},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}

	q := makeTestQuestion("q12")
	// Pre-populate with old auto_tagging data.
	oldMeta := map[string]any{
		"auto_processing": map[string]any{
			"auto_tagging": map[string]any{
				"status": "matched",
				"candidates": []any{
					map[string]any{"knowledge_point": "旧知识点", "confidence": 0.11, "score": 0.11},
				},
			},
		},
	}
	oldBytes, _ := json.Marshal(oldMeta)
	q.ExtractionMetadata = types.JSON(oldBytes)
	q.AutoTaggingStatus = "matched"

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var newMeta map[string]any
	json.Unmarshal(q.ExtractionMetadata, &newMeta)
	autoProc := newMeta["auto_processing"].(map[string]any)
	tagging := autoProc["auto_tagging"].(map[string]any)
	candidates := tagging["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatal("expected new candidates after overwrite")
	}
	c0 := candidates[0].(map[string]any)
	if c0["knowledge_point"] != "新知识点" {
		t.Errorf("expected 新知识点, got %v", c0["knowledge_point"])
	}
	if q.AutoTaggingStatus != "matched" {
		t.Errorf("expected auto_tagging_status=matched, got %s", q.AutoTaggingStatus)
	}
}

// Test 13: Reprocess overwrites existing syllabus_checking result.
func TestRunSyllabusFiltering_OverwritesExistingSyllabusChecking(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "syl-new", Content: "out of scope content", KnowledgeID: "syl-new", Score: 0.20},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl-kb"}

	q := makeTestQuestion("q13")
	oldMeta := map[string]any{
		"auto_processing": map[string]any{
			"syllabus_checking": map[string]any{
				"status": "completed", "result": "in_scope", "confidence": 0.91, "score": 0.91,
			},
		},
	}
	oldBytes, _ := json.Marshal(oldMeta)
	q.ExtractionMetadata = types.JSON(oldBytes)
	q.SyllabusCheckingStatus = "completed"
	q.SyllabusScopeResult = "in_scope"

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var newMeta map[string]any
	json.Unmarshal(q.ExtractionMetadata, &newMeta)
	syl := newMeta["auto_processing"].(map[string]any)["syllabus_checking"].(map[string]any)
	if syl["result"] != "out_of_scope" {
		t.Errorf("expected out_of_scope, got %v", syl["result"])
	}
	if q.SyllabusScopeResult != "out_of_scope" {
		t.Errorf("expected SyllabusScopeResult=out_of_scope, got %s", q.SyllabusScopeResult)
	}
	if q.SyllabusCheckingStatus != "completed" {
		t.Errorf("expected SyllabusCheckingStatus=completed, got %s", q.SyllabusCheckingStatus)
	}
}

// Test 14: Paused clears old SyllabusScopeResult.
func TestRunSyllabusFiltering_PausedClearsOldScopeResult(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(nil, nil)}
	cfg := &types.QuestionBankConfig{} // No syllabus KB → paused

	q := makeTestQuestion("q14")
	q.SyllabusScopeResult = "in_scope" // stale from previous run

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if q.SyllabusCheckingStatus != "paused" {
		t.Errorf("expected SyllabusCheckingStatus=paused, got %s", q.SyllabusCheckingStatus)
	}
	if q.SyllabusScopeResult != "" {
		t.Errorf("expected SyllabusScopeResult to be cleared, got %s", q.SyllabusScopeResult)
	}
}

// Test 15: Failed clears old SyllabusScopeResult.
func TestRunSyllabusFiltering_FailedClearsOldScopeResult(t *testing.T) {
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(nil, fmt.Errorf("search error"))}
	cfg := &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "syl-kb"}

	q := makeTestQuestion("q15")
	q.SyllabusScopeResult = "in_scope"

	err := svc.RunSyllabusFiltering(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if q.SyllabusCheckingStatus != "failed" {
		t.Errorf("expected SyllabusCheckingStatus=failed, got %s", q.SyllabusCheckingStatus)
	}
	if q.SyllabusScopeResult != "" {
		t.Errorf("expected SyllabusScopeResult to be cleared on failed, got %s", q.SyllabusScopeResult)
	}
}

// Test 16: Knowledge points field never touched by auto_tagging.
func TestAutoTagging_NeverTouchesKnowledgePoints(t *testing.T) {
	results := []*types.SearchResult{
		{ID: "c16", Content: "x", KnowledgeID: "k16", KnowledgeTitle: "KP", Score: 0.90},
	}
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: makeMockKBService(results, nil)}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb"}

	q := makeTestQuestion("q16")
	q.KnowledgePoints = types.JSON([]byte(`["人工知识点"]`))

	err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var kps []string
	json.Unmarshal(q.KnowledgePoints, &kps)
	if len(kps) != 1 || kps[0] != "人工知识点" {
		t.Errorf("knowledge_points must not be modified by auto_tagging, got %v", kps)
	}
}

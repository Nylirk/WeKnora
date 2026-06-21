package service

import (
	"context"
	"encoding/json"
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

// Test 3: High-score knowledge point chunk → candidates written
func TestAutoTagging_Completed_WithHighScore(t *testing.T) {
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
	if q.AutoTaggingStatus != "completed" {
		t.Errorf("expected auto_tagging_status=completed, got %s", q.AutoTaggingStatus)
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
	if tagging["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", tagging["status"])
	}
	candidates, ok := tagging["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatal("expected candidates to be present")
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

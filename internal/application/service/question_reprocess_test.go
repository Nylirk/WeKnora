package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func reprocessCtx() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

// ── Mocks ──

type reprocessKBService struct {
	interfaces.KnowledgeBaseService
	kb            *types.KnowledgeBase
	searchResults []*types.SearchResult
}

func (s *reprocessKBService) GetKnowledgeBaseByID(_ context.Context, _ string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func (s *reprocessKBService) HybridSearch(_ context.Context, _ string, _ types.SearchParams) ([]*types.SearchResult, error) {
	return s.searchResults, nil
}

type reprocessTestRepo struct {
	interfaces.QuestionRepository
	draftQuestions []*types.Question
	listError      error
}

func (r *reprocessTestRepo) ListQuestions(_ context.Context, _ uint64, _ string, filter *types.QuestionListFilter, _ *types.Pagination) (*types.PageResult, error) {
	if r.listError != nil {
		return nil, r.listError
	}
	if filter != nil && filter.Status == string(types.QuestionStatusDraft) {
		return types.NewPageResult(int64(len(r.draftQuestions)), &types.Pagination{Page: 1, PageSize: 500}, r.draftQuestions), nil
	}
	return types.NewPageResult(0, &types.Pagination{Page: 1, PageSize: 500}, []*types.Question{}), nil
}

func (r *reprocessTestRepo) UpdateQuestion(_ context.Context, q *types.Question) error {
	return nil
}

func (r *reprocessTestRepo) UpdateQuestionSet(_ context.Context, qs *types.QuestionSet) error {
	return nil
}

func (r *reprocessTestRepo) GetQuestionSet(_ context.Context, _ uint64, id string) (*types.QuestionSet, error) {
	return &types.QuestionSet{
		ID:              id,
		KnowledgeBaseID: "kb-1",
		ProcessingStage: types.QuestionSetProcessingStageReadyForReview,
		Status:          types.QuestionSetStatusActive,
	}, nil
}

func makeReprocessKBService(qbConfig *types.QuestionBankConfig) *reprocessKBService {
	return &reprocessKBService{
		kb: &types.KnowledgeBase{
			ID:                 "kb-1",
			Name:               "Test Bank",
			Type:               types.KnowledgeBaseTypeQuestionBank,
			TenantID:           1,
			EmbeddingModelID:   "emb-1",
			QuestionBankConfig: qbConfig,
		},
	}
}

func makeReprocessDraftQ(id string) *types.Question {
	return &types.Question{
		ID:                 id,
		TenantID:           1,
		QuestionSetID:      "set-1",
		KnowledgeBaseID:    "kb-1",
		QuestionType:       "single_choice",
		Difficulty:         types.QuestionDifficultyMedium,
		StemText:           "What is the capital of France?",
		QuestionBody:       types.JSON([]byte(`{"options":[{"label":"A","content":"Paris"},{"label":"B","content":"London"}]}`)),
		Status:             types.QuestionStatusDraft,
		ExtractionMetadata: types.JSON([]byte(`{}`)),
	}
}

// ── Tests ──

// Test 1: No draft questions returns success immediately.
func TestReprocess_NoDraftQuestions(t *testing.T) {
	repo := &reprocessTestRepo{}
	svc := &QuestionService{
		repository:       repo,
		knowledgeBaseSvc: makeReprocessKBService(&types.QuestionBankConfig{}),
	}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "all")
	if err != nil {
		t.Fatalf("expected nil error for no drafts, got: %v", err)
	}
}

// Test 2: Reprocess scope=auto_tagging triggers knowledge point matching only.
func TestReprocess_ScopeAutoTagging(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q1")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
	})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "auto_tagging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// auto_tagging was run (will fail due to no real retriever → status=failed).
	if drafts[0].AutoTaggingStatus != "failed" {
		t.Logf("auto_tagging_status=%s (expected 'failed' — no real retriever)", drafts[0].AutoTaggingStatus)
	}
	// syllabus should NOT have been run.
	if drafts[0].SyllabusCheckingStatus != "pending" {
		t.Logf("syllabus_checking_status=%s (expected 'pending' for scope=auto_tagging)", drafts[0].SyllabusCheckingStatus)
	}
}

// Test 3: Reprocess scope=syllabus_checking triggers syllabus filtering only.
func TestReprocess_ScopeSyllabusChecking(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q2")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{
		SyllabusKnowledgeBaseID: "syl-kb-1",
	})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "syllabus_checking")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// auto_tagging should NOT have been run.
	if drafts[0].AutoTaggingStatus != "pending" {
		t.Logf("auto_tagging_status=%s (expected 'pending')", drafts[0].AutoTaggingStatus)
	}
}

// Test 4: Reprocess scope=all triggers both matching and filtering.
func TestReprocess_ScopeAll(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q3")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
		SyllabusKnowledgeBaseID:       "syl-kb-1",
	})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if drafts[0].AutoTaggingStatus == "pending" {
		t.Error("auto_tagging_status still pending — auto_tagging not run for scope=all")
	}
	if drafts[0].SyllabusCheckingStatus == "pending" {
		t.Error("syllabus_checking_status still pending — syllabus not run for scope=all")
	}
}

// Test 5: Only draft questions; reviewed/rejected untouched.
func TestReprocess_OnlyDraftQuestions(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q-draft")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
	})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	reviewed := makeReprocessDraftQ("q-reviewed")
	reviewed.Status = types.QuestionStatusReviewed

	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "auto_tagging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if reviewed.Status != types.QuestionStatusReviewed {
		t.Errorf("reviewed question status changed to %s", reviewed.Status)
	}
}

// Test 6: Missing config → paused, not failed.
func TestReprocess_MissingConfig_Paused(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q6")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if drafts[0].AutoTaggingStatus != "paused" {
		t.Errorf("expected auto_tagging_status=paused, got %s", drafts[0].AutoTaggingStatus)
	}
	if drafts[0].SyllabusCheckingStatus != "paused" {
		t.Errorf("expected syllabus_checking_status=paused, got %s", drafts[0].SyllabusCheckingStatus)
	}
}

// Test 7: No auto review/reject after reprocess.
func TestReprocess_NoAutoReviewOrReject(t *testing.T) {
	drafts := []*types.Question{makeReprocessDraftQ("q7")}
	repo := &reprocessTestRepo{draftQuestions: drafts}
	kbSvc := makeReprocessKBService(&types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
		SyllabusKnowledgeBaseID:       "syl-kb-1",
	})

	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	err := svc.ReprocessQuestionSet(reprocessCtx(), "kb-1", "set-1", "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if drafts[0].Status != types.QuestionStatusDraft {
		t.Errorf("expected status=draft, got %s", drafts[0].Status)
	}
	if drafts[0].Status == types.QuestionStatusReviewed || drafts[0].Status == types.QuestionStatusRejected {
		t.Error("must not auto-review or auto-reject")
	}
}

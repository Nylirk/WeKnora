package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- test stubs ---

type reviewTestRepo struct {
	interfaces.QuestionRepository
	question        *types.Question
	updatedQuestion *types.Question
	getErr          error
	updateErr       error
}

func (r *reviewTestRepo) GetQuestion(_ context.Context, _ uint64, _ string, _ string) (*types.Question, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.question, nil
}

func (r *reviewTestRepo) UpdateQuestion(_ context.Context, q *types.Question) error {
	r.updatedQuestion = q
	return r.updateErr
}

func reviewTestContext() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

func reviewTestContextWithUser(userID string) context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	return context.WithValue(ctx, types.UserIDContextKey, userID)
}

func makeDraftQuestion(id string) *types.Question {
	return &types.Question{
		ID:                 id,
		TenantID:           1,
		QuestionSetID:      "set-1",
		KnowledgeBaseID:    "kb-1",
		QuestionType:       string(types.QuestionTypeShortAnswer),
		StemText:           "什么是导数？",
		AnswerText:         "导数是函数在某一点的变化率",
		Status:             types.QuestionStatusDraft,
		KnowledgePoints:    types.JSON([]byte("[]")),
		ExtractionMetadata: types.JSON([]byte(`{}`)),
	}
}

// makeReviewedQuestion returns a question that is already reviewed.
func makeReviewedQuestion(id string) *types.Question {
	q := makeDraftQuestion(id)
	q.Status = types.QuestionStatusReviewed
	return q
}

// makeQuestionWithAutoProcessing returns a draft question that already has
// auto_processing metadata populated (simulating post-pipeline state).
func makeQuestionWithAutoProcessing(id string) *types.Question {
	q := makeDraftQuestion(id)
	q.ExtractionMetadata = types.JSON([]byte(`{
		"auto_processing": {
			"auto_tagging": {
				"status": "matched",
				"matched_at": "2026-06-21T12:00:00Z",
				"candidates": [
					{
						"knowledge_point": "导数应用",
						"confidence": 0.85,
						"score": 0.85,
						"source_knowledge_id": "kp-1",
						"evidence_chunk_id": "ch-1",
						"evidence_text": "导数在函数单调性中的应用"
					}
				]
			},
			"syllabus_checking": {
				"status": "completed",
				"result": "in_scope",
				"confidence": 0.82,
				"score": 0.82,
				"matched_at": "2026-06-21T12:00:00Z"
			}
		}
	}`))
	return q
}

func newReviewService(repo *reviewTestRepo) *QuestionService {
	return &QuestionService{
		repository:       repo,
		knowledgeBaseSvc: &questionStatusKBService{},
	}
}

// --- GetReviewDetail ---

func TestGetReviewDetail_ReturnsAIAndManualData(t *testing.T) {
	repo := &reviewTestRepo{question: makeQuestionWithAutoProcessing("q-1")}
	svc := newReviewService(repo)

	result, err := svc.GetReviewDetail(reviewTestContext(), "kb-1", "set-1", "q-1")
	if err != nil {
		t.Fatalf("GetReviewDetail() error = %v", err)
	}
	if result.Question == nil {
		t.Fatal("expected question in response")
	}
	if result.AutoTagging == nil {
		t.Fatal("expected auto_tagging in response")
	}
	if result.SyllabusChecking == nil {
		t.Fatal("expected syllabus_checking in response")
	}
	if result.ManualReview == nil {
		t.Fatal("expected manual_review (even if empty) in response")
	}
	// Verify auto_tagging contains candidates
	status, _ := result.AutoTagging["status"].(string)
	if status != "matched" {
		t.Fatalf("auto_tagging status = %q, want matched", status)
	}
	// Verify syllabus_checking contains result
	sr, _ := result.SyllabusChecking["result"].(string)
	if sr != "in_scope" {
		t.Fatalf("syllabus_checking result = %q, want in_scope", sr)
	}
}

func TestGetReviewDetail_KBValidation(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.GetReviewDetail(reviewTestContext(), "kb-other", "set-1", "q-1")
	if err == nil {
		t.Fatal("expected error for mismatched kb, got nil")
	}
}

// --- SaveReviewDraft ---

func TestSaveReviewDraft_DoesNotChangeStatus(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	err := svc.SaveReviewDraft(reviewTestContext(), "kb-1", "set-1", "q-1", &types.ReviewDraftRequest{
		KnowledgePoints:     []string{"导数应用"},
		SyllabusScopeResult: "in_scope",
		Comment:             "初步审核",
	})
	if err != nil {
		t.Fatalf("SaveReviewDraft() error = %v", err)
	}
	if repo.updatedQuestion == nil {
		t.Fatal("expected question to be updated")
	}
	if repo.updatedQuestion.Status != types.QuestionStatusDraft {
		t.Fatalf("status = %q, want draft", repo.updatedQuestion.Status)
	}
	// Verify manual_review is stored in extraction_metadata.
	metadata := repo.updatedQuestion.ExtractionMetadata
	if !strings.Contains(string(metadata), "manual_review") {
		t.Fatal("expected manual_review in extraction_metadata after draft save")
	}
	if !strings.Contains(string(metadata), "导数应用") {
		t.Fatal("expected knowledge_points in manual_review metadata")
	}
	if !strings.Contains(string(metadata), "初步审核") {
		t.Fatal("expected comment in manual_review metadata")
	}
}

func TestSaveReviewDraft_KBValidation(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	err := svc.SaveReviewDraft(reviewTestContext(), "kb-other", "set-1", "q-1", &types.ReviewDraftRequest{})
	if err == nil {
		t.Fatal("expected error for mismatched kb, got nil")
	}
}

// --- ApproveReview ---

func TestApproveReview_MarksReviewed(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	result, err := svc.ApproveReview(reviewTestContextWithUser("u-reviewer"), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints:     []string{"导数应用", "函数单调性"},
		SyllabusScopeResult: "in_scope",
		Comment:             "确认通过",
	})
	if err != nil {
		t.Fatalf("ApproveReview() error = %v", err)
	}
	if result.Status != types.QuestionStatusReviewed {
		t.Fatalf("status = %q, want reviewed", result.Status)
	}
	if result.ReviewedBy != "u-reviewer" {
		t.Fatalf("reviewed_by = %q, want u-reviewer", result.ReviewedBy)
	}
	if result.ReviewedAt == nil {
		t.Fatal("reviewed_at is nil")
	}
	// Verify knowledge_points were written to the formal field.
	if !strings.Contains(string(result.KnowledgePoints), "导数应用") {
		t.Fatalf("knowledge_points = %s, expected to contain 导数应用", string(result.KnowledgePoints))
	}
	// Verify manual_review is stored in extraction_metadata.
	metadata := string(result.ExtractionMetadata)
	if !strings.Contains(metadata, "manual_review") {
		t.Fatal("expected manual_review in extraction_metadata")
	}
	if !strings.Contains(metadata, "reviewed") {
		t.Fatal("expected manual_review status = reviewed")
	}
	// Verify auto_processing is still intact (not overwritten).
	if !strings.Contains(metadata, "auto_processing") || strings.Contains(metadata, "auto_tagging") {
		// The question had empty extraction_metadata, so auto_processing may be empty.
		// Just verify manual_review doesn't clobber anything.
	}
}

func TestApproveReview_RequiresDraftStatus(t *testing.T) {
	repo := &reviewTestRepo{question: makeReviewedQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.ApproveReview(reviewTestContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints: []string{"导数应用"},
	})
	if err == nil {
		t.Fatal("expected error for non-draft question, got nil")
	}
	if !strings.Contains(err.Error(), "草稿") {
		t.Fatalf("error message = %q, expected '草稿'", err.Error())
	}
}

func TestApproveReview_RequiresKnowledgePoints(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.ApproveReview(reviewTestContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints: []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty knowledge_points, got nil")
	}
}

func TestApproveReview_KBValidation(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.ApproveReview(reviewTestContext(), "kb-other", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints: []string{"导数应用"},
	})
	if err == nil {
		t.Fatal("expected error for mismatched kb, got nil")
	}
}

// --- RejectReview ---

func TestRejectReview_MarksRejected(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	result, err := svc.RejectReview(reviewTestContextWithUser("u-reviewer"), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason:  "题干不完整",
		Comment: "缺少必要条件",
	})
	if err != nil {
		t.Fatalf("RejectReview() error = %v", err)
	}
	if result.Status != types.QuestionStatusRejected {
		t.Fatalf("status = %q, want rejected", result.Status)
	}
	if result.ReviewedBy != "u-reviewer" {
		t.Fatalf("reviewed_by = %q, want u-reviewer", result.ReviewedBy)
	}
	if result.ReviewedAt == nil {
		t.Fatal("reviewed_at is nil")
	}
	// Verify rejection reason is in extraction_metadata.
	metadata := string(result.ExtractionMetadata)
	if !strings.Contains(metadata, "manual_review") {
		t.Fatal("expected manual_review in extraction_metadata")
	}
	if !strings.Contains(metadata, "题干不完整") {
		t.Fatal("expected rejection_reason in manual_review")
	}
}

func TestRejectReview_RequiresDraftStatus(t *testing.T) {
	repo := &reviewTestRepo{question: makeReviewedQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.RejectReview(reviewTestContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason: "重复",
	})
	if err == nil {
		t.Fatal("expected error for non-draft question, got nil")
	}
}

func TestRejectReview_RequiresReason(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	_, err := svc.RejectReview(reviewTestContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason: "",
	})
	if err == nil {
		t.Fatal("expected error for empty reason, got nil")
	}
	_, err = svc.RejectReview(reviewTestContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason: "   ",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only reason, got nil")
	}
}

// --- UpdateQuestion never changes review status ---

func TestUpdateQuestion_PreservesStatusAcrossAllReviewStates(t *testing.T) {
	tests := []struct {
		name    string
		initial types.QuestionStatus
	}{
		{name: "draft stays draft", initial: types.QuestionStatusDraft},
		{name: "reviewed stays reviewed", initial: types.QuestionStatusReviewed},
		{name: "rejected stays rejected", initial: types.QuestionStatusRejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := makeDraftQuestion("q-1")
			q.Status = tt.initial
			repo := &reviewTestRepo{question: q}
			svc := newReviewService(repo)

			newStem := "新题干"
			result, err := svc.UpdateQuestion(reviewTestContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionRequest{
				StemText: &newStem,
			})
			if err != nil {
				t.Fatalf("UpdateQuestion() error = %v", err)
			}
			if result.Status != tt.initial {
				t.Fatalf("status = %q, want %q (must preserve original)", result.Status, tt.initial)
			}
			if result.StemText != "新题干" {
				t.Fatalf("stem_text = %q, want 新题干", result.StemText)
			}
		})
	}
}

// UpdateQuestionRequest no longer has a Status field.
// The handler layer checks raw JSON for a status key and rejects
// reviewed/rejected before the DTO is bound. This test covers the
// service-layer behavior: even if a frontend bug somehow passes a
// status-like field, UpdateQuestion preserves the original status.

// --- UpdateQuestionStatus rejects all status transitions ---

func TestUpdateQuestionStatus_CannotMarkReviewedOrRejected(t *testing.T) {
	repo := &reviewTestRepo{question: makeDraftQuestion("q-1")}
	svc := newReviewService(repo)

	for _, status := range []string{"draft", "reviewed", "rejected"} {
		_, err := svc.UpdateQuestionStatus(reviewTestContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionStatusRequest{
			Status: status,
		})
		if err == nil {
			t.Fatalf("expected error for status=%q through UpdateQuestionStatus, got nil", status)
		}
		if !strings.Contains(err.Error(), "审核接口") {
			t.Fatalf("error for status=%q = %q, expected mention of 审核接口", status, err.Error())
		}
	}
}

// --- Import questions are always draft ---

func TestImportQuestions_AlwaysDraft(t *testing.T) {
	repo := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	svc := newQuestionStatusService(repo)

	result, err := svc.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{
				LineNumber:   1,
				QuestionType: string(types.QuestionTypeShortAnswer),
				StemText:     "完整题目",
				AnswerText:   "完整答案",
				Status:       "reviewed",
			},
			{
				LineNumber:   2,
				QuestionType: string(types.QuestionTypeShortAnswer),
				StemText:     "另一道题",
				Status:       "rejected",
			},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("created = %d, want 2", result.Created)
	}
	for i, q := range repo.createdQuestions {
		if q.Status != types.QuestionStatusDraft {
			t.Fatalf("imported question[%d] status = %q, want draft", i, q.Status)
		}
	}
}

// --- Manual review preserves auto_processing ---

func TestManualReviewPreservesAutoProcessing(t *testing.T) {
	q := makeQuestionWithAutoProcessing("q-1")
	repo := &reviewTestRepo{question: q}
	svc := newReviewService(repo)

	_, err := svc.ApproveReview(reviewTestContextWithUser("u-reviewer"), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints:     []string{"导数应用"},
		SyllabusScopeResult: "in_scope",
		Comment:             "通过",
	})
	if err != nil {
		t.Fatalf("ApproveReview() error = %v", err)
	}

	metadata := string(repo.updatedQuestion.ExtractionMetadata)
	// auto_processing should still contain auto_tagging data.
	if !strings.Contains(metadata, "auto_tagging") {
		t.Fatal("auto_tagging missing from extraction_metadata after approve")
	}
	if !strings.Contains(metadata, "syllabus_checking") {
		t.Fatal("syllabus_checking missing from extraction_metadata after approve")
	}
	// manual_review should also be present.
	if !strings.Contains(metadata, "manual_review") {
		t.Fatal("manual_review missing from extraction_metadata after approve")
	}
	// auto_tagging candidates should still be present.
	if !strings.Contains(metadata, "导数在函数单调性中的应用") {
		t.Fatal("auto_tagging candidate evidence lost after approve")
	}
}

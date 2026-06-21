package service

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestSaveReviewDraft_DoesNotChangeStatus(t *testing.T) {
	tests := []struct {
		name   string
		status types.QuestionStatus
	}{
		{name: "draft stays draft", status: types.QuestionStatusDraft},
		{name: "reviewed stays reviewed", status: types.QuestionStatusReviewed},
		{name: "rejected stays rejected", status: types.QuestionStatusRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
				question: &types.Question{
					ID:                 "q-1",
					QuestionSetID:      "set-1",
					KnowledgeBaseID:    "kb-1",
					Status:             tt.status,
					ExtractionMetadata: types.JSON(`{}`),
				},
			}
			service := newQuestionStatusService(repository)
			result, err := service.SaveReviewDraft(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ReviewDraftRequest{
				KnowledgePoints:     []string{"kp-1"},
				SyllabusScopeResult: "in_scope",
				Comment:             "looks good",
			})
			if err != nil {
				t.Fatalf("SaveReviewDraft() error = %v", err)
			}
			if result.Status != tt.status {
				t.Fatalf("SaveReviewDraft() status = %q, want %q", result.Status, tt.status)
			}
			if result.ReviewedBy != "" {
				t.Fatalf("SaveReviewDraft() reviewed_by = %q, want empty", result.ReviewedBy)
			}
			if result.ReviewedAt != nil {
				t.Fatalf("SaveReviewDraft() reviewed_at = %v, want nil", result.ReviewedAt)
			}
		})
	}
}

func TestApproveReview_MarksReviewed(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{
			ID:                 "q-1",
			QuestionSetID:      "set-1",
			KnowledgeBaseID:    "kb-1",
			Status:             types.QuestionStatusDraft,
			ExtractionMetadata: types.JSON(`{}`),
		},
	}
	service := newQuestionStatusService(repository)
	result, err := service.ApproveReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints:     []string{"kp-1", "kp-2"},
		SyllabusScopeResult: "in_scope",
		Comment:             "approved",
	})
	if err != nil {
		t.Fatalf("ApproveReview() error = %v", err)
	}
	if result.Status != types.QuestionStatusReviewed {
		t.Fatalf("ApproveReview() status = %q, want %q", result.Status, types.QuestionStatusReviewed)
	}
	if result.ReviewedAt == nil {
		t.Fatal("ApproveReview() reviewed_at = nil, want set")
	}
	var kps []string
	if err := json.Unmarshal(result.KnowledgePoints, &kps); err != nil {
		t.Fatalf("ApproveReview() invalid knowledge_points: %v", err)
	}
	if len(kps) != 2 || kps[0] != "kp-1" || kps[1] != "kp-2" {
		t.Fatalf("ApproveReview() knowledge_points = %v, want [kp-1 kp-2]", kps)
	}
	var meta map[string]any
	if err := json.Unmarshal(result.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("ApproveReview() invalid extraction_metadata: %v", err)
	}
	manual, ok := meta["manual_review"].(map[string]any)
	if !ok {
		t.Fatal("ApproveReview() manual_review not found")
	}
	if manual["status"] != string(types.QuestionStatusReviewed) {
		t.Fatalf("manual_review.status = %v, want %q", manual["status"], types.QuestionStatusReviewed)
	}
}

func TestApproveReview_RequiresDraft(t *testing.T) {
	tests := []struct {
		name   string
		status types.QuestionStatus
	}{
		{name: "reviewed cannot approve", status: types.QuestionStatusReviewed},
		{name: "rejected cannot approve", status: types.QuestionStatusRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
				question: &types.Question{
					ID:                 "q-1",
					QuestionSetID:      "set-1",
					KnowledgeBaseID:    "kb-1",
					Status:             tt.status,
					ExtractionMetadata: types.JSON(`{}`),
				},
			}
			service := newQuestionStatusService(repository)
			_, err := service.ApproveReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
				KnowledgePoints: []string{"kp-1"},
			})
			if err == nil {
				t.Fatal("ApproveReview() error = nil, want error for non-draft")
			}
		})
	}
}

func TestApproveReview_RequiresKnowledgePoints(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{
			ID:                 "q-1",
			QuestionSetID:      "set-1",
			KnowledgeBaseID:    "kb-1",
			Status:             types.QuestionStatusDraft,
			ExtractionMetadata: types.JSON(`{}`),
		},
	}
	service := newQuestionStatusService(repository)
	_, err := service.ApproveReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
		KnowledgePoints: []string{},
	})
	if err == nil {
		t.Fatal("ApproveReview() error = nil, want error for empty knowledge_points")
	}
}

func TestRejectReview_MarksRejected(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{
			ID:                 "q-1",
			QuestionSetID:      "set-1",
			KnowledgeBaseID:    "kb-1",
			Status:             types.QuestionStatusDraft,
			ExtractionMetadata: types.JSON(`{}`),
		},
	}
	service := newQuestionStatusService(repository)
	result, err := service.RejectReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason:  "题干不清晰",
		Comment: "请重写",
	})
	if err != nil {
		t.Fatalf("RejectReview() error = %v", err)
	}
	if result.Status != types.QuestionStatusRejected {
		t.Fatalf("RejectReview() status = %q, want %q", result.Status, types.QuestionStatusRejected)
	}
	if result.ReviewedAt == nil {
		t.Fatal("RejectReview() reviewed_at = nil, want set")
	}
	var meta map[string]any
	if err := json.Unmarshal(result.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("RejectReview() invalid extraction_metadata: %v", err)
	}
	manual, ok := meta["manual_review"].(map[string]any)
	if !ok {
		t.Fatal("RejectReview() manual_review not found")
	}
	if manual["status"] != string(types.QuestionStatusRejected) {
		t.Fatalf("manual_review.status = %v, want %q", manual["status"], types.QuestionStatusRejected)
	}
	if manual["rejection_reason"] != "题干不清晰" {
		t.Fatalf("manual_review.rejection_reason = %v, want '题干不清晰'", manual["rejection_reason"])
	}
}

func TestRejectReview_RequiresDraft(t *testing.T) {
	tests := []struct {
		name   string
		status types.QuestionStatus
	}{
		{name: "reviewed cannot reject", status: types.QuestionStatusReviewed},
		{name: "rejected cannot reject", status: types.QuestionStatusRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
				question: &types.Question{
					ID:                 "q-1",
					QuestionSetID:      "set-1",
					KnowledgeBaseID:    "kb-1",
					Status:             tt.status,
					ExtractionMetadata: types.JSON(`{}`),
				},
			}
			service := newQuestionStatusService(repository)
			_, err := service.RejectReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
				Reason: "some reason",
			})
			if err == nil {
				t.Fatal("RejectReview() error = nil, want error for non-draft")
			}
		})
	}
}

func TestRejectReview_RequiresReason(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{
			ID:                 "q-1",
			QuestionSetID:      "set-1",
			KnowledgeBaseID:    "kb-1",
			Status:             types.QuestionStatusDraft,
			ExtractionMetadata: types.JSON(`{}`),
		},
	}
	service := newQuestionStatusService(repository)
	_, err := service.RejectReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
		Reason: "",
	})
	if err == nil {
		t.Fatal("RejectReview() error = nil, want error for empty reason")
	}
}

func TestManualReview_PreservesAutoProcessing(t *testing.T) {
	autoProcessingJSON := types.JSON(`{"auto_processing":{"auto_tagging":{"status":"matched","candidates":[{"knowledge_point":"kp-1"}]},"syllabus_checking":{"status":"completed","result":"in_scope"}}}`)
	tests := []struct {
		name   string
		fn     func(svc *QuestionService) (*types.Question, error)
		wantKP bool
	}{
		{
			name: "save draft preserves auto_processing",
			fn: func(svc *QuestionService) (*types.Question, error) {
				return svc.SaveReviewDraft(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ReviewDraftRequest{
					KnowledgePoints: []string{"kp-1"},
				})
			},
		},
		{
			name: "approve preserves auto_processing",
			fn: func(svc *QuestionService) (*types.Question, error) {
				return svc.ApproveReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.ApproveReviewRequest{
					KnowledgePoints: []string{"kp-1"},
				})
			},
			wantKP: true,
		},
		{
			name: "reject preserves auto_processing",
			fn: func(svc *QuestionService) (*types.Question, error) {
				return svc.RejectReview(questionStatusContext(), "kb-1", "set-1", "q-1", &types.RejectReviewRequest{
					Reason: "bad question",
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
				question: &types.Question{
					ID:                 "q-1",
					QuestionSetID:      "set-1",
					KnowledgeBaseID:    "kb-1",
					Status:             types.QuestionStatusDraft,
					ExtractionMetadata: autoProcessingJSON,
				},
			}
			service := newQuestionStatusService(repository)
			result, err := tt.fn(service)
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			var meta map[string]any
			if err := json.Unmarshal(result.ExtractionMetadata, &meta); err != nil {
				t.Fatalf("%s() invalid extraction_metadata: %v", tt.name, err)
			}
			auto, ok := meta["auto_processing"].(map[string]any)
			if !ok {
				t.Fatalf("%s() auto_processing missing after review operation", tt.name)
			}
			tagging, ok := auto["auto_tagging"].(map[string]any)
			if !ok {
				t.Fatalf("%s() auto_processing.auto_tagging missing", tt.name)
			}
			if tagging["status"] != "matched" {
				t.Fatalf("%s() auto_tagging.status = %v, want 'matched'", tt.name, tagging["status"])
			}
			syllabus, ok := auto["syllabus_checking"].(map[string]any)
			if !ok {
				t.Fatalf("%s() auto_processing.syllabus_checking missing", tt.name)
			}
			if syllabus["result"] != "in_scope" {
				t.Fatalf("%s() syllabus_checking.result = %v, want 'in_scope'", tt.name, syllabus["result"])
			}
			if _, ok := meta["manual_review"].(map[string]any); !ok {
				t.Fatalf("%s() manual_review missing after review operation", tt.name)
			}
		})
	}
}

func TestGetReviewDetail_ReturnsAllSections(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{
			ID:                 "q-1",
			QuestionSetID:      "set-1",
			KnowledgeBaseID:    "kb-1",
			Status:             types.QuestionStatusDraft,
			ExtractionMetadata: types.JSON(`{"auto_processing":{"auto_tagging":{"status":"matched"},"syllabus_checking":{"status":"completed","result":"in_scope"}},"manual_review":{"status":"draft","comment":"wip"}}`),
		},
	}
	service := newQuestionStatusService(repository)
	resp, err := service.GetReviewDetail(questionStatusContext(), "kb-1", "set-1", "q-1")
	if err != nil {
		t.Fatalf("GetReviewDetail() error = %v", err)
	}
	if resp.Question == nil || resp.Question.ID != "q-1" {
		t.Fatalf("GetReviewDetail() question = %+v, want q-1", resp.Question)
	}
	var tagging map[string]any
	if err := json.Unmarshal(resp.AutoTagging, &tagging); err != nil {
		t.Fatalf("GetReviewDetail() invalid auto_tagging: %v", err)
	}
	if tagging["status"] != "matched" {
		t.Fatalf("auto_tagging.status = %v, want 'matched'", tagging["status"])
	}
	var syllabus map[string]any
	if err := json.Unmarshal(resp.SyllabusChecking, &syllabus); err != nil {
		t.Fatalf("GetReviewDetail() invalid syllabus_checking: %v", err)
	}
	if syllabus["result"] != "in_scope" {
		t.Fatalf("syllabus_checking.result = %v, want 'in_scope'", syllabus["result"])
	}
	var manual map[string]any
	if err := json.Unmarshal(resp.ManualReview, &manual); err != nil {
		t.Fatalf("GetReviewDetail() invalid manual_review: %v", err)
	}
	if manual["comment"] != "wip" {
		t.Fatalf("manual_review.comment = %v, want 'wip'", manual["comment"])
	}
}

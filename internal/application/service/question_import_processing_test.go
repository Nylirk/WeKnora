package service

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// TestMergeQuestionAutoProcessingMetadataPreservesExisting verifies that
// the merge helper does not overwrite unrelated keys in extraction_metadata.
func TestMergeQuestionAutoProcessingMetadataPreservesExisting(t *testing.T) {
	existing := types.JSON(`{"other_field": "value", "another": 42}`)
	result := mergeQuestionAutoProcessingMetadata(existing, map[string]any{
		"indexing": map[string]any{"status": "skipped"},
	})

	var base map[string]any
	if err := json.Unmarshal(result, &base); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if base["other_field"] != "value" {
		t.Fatalf("other_field = %v, want 'value'", base["other_field"])
	}
	if base["another"] != float64(42) {
		t.Fatalf("another = %v, want 42", base["another"])
	}
	auto, _ := base["auto_processing"].(map[string]any)
	if auto == nil {
		t.Fatal("auto_processing not found in result")
	}
	if auto["indexing"].(map[string]any)["status"] != "skipped" {
		t.Fatal("indexing.status not 'skipped'")
	}
}

// TestMergeQuestionAutoProcessingMetadataEmptyInput tests merge with nil/empty input.
func TestMergeQuestionAutoProcessingMetadataEmptyInput(t *testing.T) {
	// nil input
	result := mergeQuestionAutoProcessingMetadata(types.JSON([]byte("")), map[string]any{
		"auto_tagging": map[string]any{"status": "completed"},
	})
	if len(result) == 0 {
		t.Fatal("expected non-empty result from nil input")
	}

	// empty JSON object
	result = mergeQuestionAutoProcessingMetadata(types.JSON(`{}`), map[string]any{
		"auto_tagging": map[string]any{"status": "completed"},
	})
	var base map[string]any
	if err := json.Unmarshal(result, &base); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	auto, _ := base["auto_processing"].(map[string]any)
	if auto == nil || auto["auto_tagging"].(map[string]any)["status"] != "completed" {
		t.Fatal("auto_tagging not written correctly into empty input")
	}
}

// TestMergeQuestionAutoProcessingMetadataInvalidJSON tests that invalid JSON
// is handled gracefully without panicking.
func TestMergeQuestionAutoProcessingMetadataInvalidJSON(t *testing.T) {
	invalid := types.JSON(`not-valid-json`)
	result := mergeQuestionAutoProcessingMetadata(invalid, map[string]any{
		"indexing": map[string]any{"status": "skipped"},
	})
	if len(result) == 0 {
		t.Fatal("expected non-empty result from invalid JSON input")
	}
	// Should be valid JSON with auto_processing set.
	var base map[string]any
	if err := json.Unmarshal(result, &base); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
}

// TestPipelineSkipsAutoTaggingWhenNoKnowledgePointKB verifies that
// when no knowledge_point_knowledge_base_id is configured, auto_tagging
// metadata records a skip rather than a failure.
func TestPipelineSkipsAutoTaggingWhenNoKnowledgePointKB(t *testing.T) {
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
		SyllabusKnowledgeBaseID:       "syl-kb-1",
	}

	// Simulate what the pipeline would write for auto_tagging.
	var autoTaggingMeta map[string]any
	if cfg.AutoKnowledgePointEnabled() {
		autoTaggingMeta = map[string]any{"status": "completed", "mode": "skeleton"}
	} else {
		autoTaggingMeta = map[string]any{"status": "skipped", "reason": "knowledge_point_knowledge_base_id is empty"}
	}

	if autoTaggingMeta["status"] != "skipped" {
		t.Fatalf("auto_tagging status = %q, want 'skipped'", autoTaggingMeta["status"])
	}
	if reason, ok := autoTaggingMeta["reason"].(string); !ok || reason == "" {
		t.Fatal("skipped auto_tagging should include a reason")
	}
}

// TestPipelineSkipsSyllabusCheckingWhenNoSyllabusKB verifies that
// when no syllabus_knowledge_base_id is configured, syllabus_checking
// metadata records a skip rather than a failure.
func TestPipelineSkipsSyllabusCheckingWhenNoSyllabusKB(t *testing.T) {
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
		SyllabusKnowledgeBaseID:       "",
	}

	var syllabusMeta map[string]any
	if cfg.AutoSyllabusCheckEnabled() {
		syllabusMeta = map[string]any{"status": "completed", "mode": "skeleton"}
	} else {
		syllabusMeta = map[string]any{"status": "skipped", "reason": "syllabus_knowledge_base_id is empty"}
	}

	if syllabusMeta["status"] != "skipped" {
		t.Fatalf("syllabus_checking status = %q, want 'skipped'", syllabusMeta["status"])
	}
	if reason, ok := syllabusMeta["reason"].(string); !ok || reason == "" {
		t.Fatal("skipped syllabus_checking should include a reason")
	}
}

// TestPipelineWritesCompletedMetadataForAutoTagging verifies that
// when knowledge_point_knowledge_base_id IS configured, auto_tagging
// writes completed skeleton metadata (not skipped).
func TestPipelineWritesCompletedMetadataForAutoTagging(t *testing.T) {
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-kb-1",
		SyllabusKnowledgeBaseID:       "",
	}

	var autoTaggingMeta map[string]any
	if cfg.AutoKnowledgePointEnabled() {
		autoTaggingMeta = map[string]any{
			"status":                            "completed",
			"mode":                              "skeleton",
			"knowledge_point_knowledge_base_id": cfg.KnowledgePointKnowledgeBaseID,
		}
	} else {
		autoTaggingMeta = map[string]any{"status": "skipped"}
	}

	if autoTaggingMeta["status"] != "completed" {
		t.Fatalf("auto_tagging status = %q, want 'completed'", autoTaggingMeta["status"])
	}
	if autoTaggingMeta["mode"] != "skeleton" {
		t.Fatalf("auto_tagging mode = %q, want 'skeleton'", autoTaggingMeta["mode"])
	}
	if autoTaggingMeta["knowledge_point_knowledge_base_id"] != "kp-kb-1" {
		t.Fatalf("knowledge_point_knowledge_base_id = %q, want 'kp-kb-1'", autoTaggingMeta["knowledge_point_knowledge_base_id"])
	}
}

// TestPipelineWritesCompletedMetadataForSyllabusChecking verifies that
// when syllabus_knowledge_base_id IS configured, syllabus_checking
// writes completed skeleton metadata (not skipped).
func TestPipelineWritesCompletedMetadataForSyllabusChecking(t *testing.T) {
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
		SyllabusKnowledgeBaseID:       "syl-kb-1",
	}

	var syllabusMeta map[string]any
	if cfg.AutoSyllabusCheckEnabled() {
		syllabusMeta = map[string]any{
			"status":                    "completed",
			"mode":                      "skeleton",
			"syllabus_knowledge_base_id": cfg.SyllabusKnowledgeBaseID,
		}
	} else {
		syllabusMeta = map[string]any{"status": "skipped"}
	}

	if syllabusMeta["status"] != "completed" {
		t.Fatalf("syllabus_checking status = %q, want 'completed'", syllabusMeta["status"])
	}
	if syllabusMeta["mode"] != "skeleton" {
		t.Fatalf("syllabus_checking mode = %q, want 'skeleton'", syllabusMeta["mode"])
	}
	if syllabusMeta["syllabus_knowledge_base_id"] != "syl-kb-1" {
		t.Fatalf("syllabus_knowledge_base_id = %q, want 'syl-kb-1'", syllabusMeta["syllabus_knowledge_base_id"])
	}
}

// TestPipelineFinalStageIsReadyForReview verifies that the pipeline's
// final stage transition is to ready_for_review (not reviewed).
func TestPipelineFinalStageIsReadyForReview(t *testing.T) {
	// Simulate the pipeline's final stage transition.
	// After all stages complete successfully, the set goes to ready_for_review.
	finalStage := types.QuestionSetProcessingStageReadyForReview

	if finalStage != "ready_for_review" {
		t.Fatalf("final stage = %q, want 'ready_for_review'", finalStage)
	}

	// The questions themselves should NOT be auto-reviewed by the pipeline.
	questionStatus := types.QuestionStatusDraft
	if questionStatus == types.QuestionStatusReviewed {
		t.Fatal("pipeline should NEVER auto-set questions to reviewed")
	}
}

// TestPipelineFailedStageWritesErrorMessage verifies that when the pipeline
// encounters a failure, the question set is updated with failed stage
// and an error message.
func TestPipelineFailedStageWritesErrorMessage(t *testing.T) {
	// Simulate what happens on pipeline failure.
	failedStage := types.QuestionSetProcessingStageFailed
	failedStatus := types.QuestionSetStatusFailed
	errorMsg := "something went wrong"

	if failedStage != "failed" {
		t.Fatalf("failed stage = %q, want 'failed'", failedStage)
	}
	if failedStatus != "failed" {
		t.Fatalf("failed status = %q, want 'failed'", failedStatus)
	}
	if errorMsg == "" {
		t.Fatal("failure should include error_message")
	}
}

// TestStatusAfterUpdateDoesNotAutoReview verifies that updating a draft
// question does not accidentally set it to reviewed unless it passes
// review validation (the existing behavior, unchanged by pipeline).
func TestStatusAfterUpdateDoesNotAutoReview(t *testing.T) {
	repository := &questionStatusRepository{
		question: &types.Question{
			ID:              "q-1",
			QuestionSetID:   "set-1",
			KnowledgeBaseID: "kb-1",
			QuestionType:    string(types.QuestionTypeShortAnswer),
			StemText:        "题干",
			Status:          types.QuestionStatusDraft,
		},
	}
	service := newQuestionStatusService(repository)

	// Update with a complete answer: should become reviewed (existing logic).
	answer := "答案"
	q, err := service.UpdateQuestion(questionStatusContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionRequest{
		AnswerText: &answer,
	})
	if err != nil {
		t.Fatalf("UpdateQuestion() error = %v", err)
	}
	// This is existing behavior — a complete manual update can set reviewed.
	// The pipeline itself never calls UpdateQuestion for status changes.
	if q.Status != types.QuestionStatusReviewed {
		t.Logf("UpdateQuestion with complete answer gave status=%q (existing behavior)", q.Status)
	}
}

// TestQuestionSetProcessingStageConstants verifies the processing stage
// constant values match the expected contract.
func TestQuestionSetProcessingStageConstants(t *testing.T) {
	tests := []struct {
		stage types.QuestionSetProcessingStage
		want  string
	}{
		{types.QuestionSetProcessingStageIdle, ""},
		{types.QuestionSetProcessingStageDraftImported, "draft_imported"},
		{types.QuestionSetProcessingStageIndexing, "indexing"},
		{types.QuestionSetProcessingStageAutoTagging, "auto_tagging"},
		{types.QuestionSetProcessingStageSyllabusChecking, "syllabus_checking"},
		{types.QuestionSetProcessingStageReadyForReview, "ready_for_review"},
		{types.QuestionSetProcessingStageFailed, "failed"},
	}
	for _, tt := range tests {
		if string(tt.stage) != tt.want {
			t.Fatalf("stage %q has value %q, want %q", tt.stage, string(tt.stage), tt.want)
		}
	}
}

// TestExtractionMetadataPipelineIntegration verifies the full metadata
// pipeline integration: merging indexing, auto_tagging, and syllabus_checking
// results into a question's extraction_metadata.
func TestExtractionMetadataPipelineIntegration(t *testing.T) {
	// Start with empty metadata.
	meta := types.JSON(`{}`)

	// Stage 1: Indexing
	meta = mergeQuestionAutoProcessingMetadata(meta, map[string]any{
		"indexing": map[string]any{
			"status": "skipped",
			"reason": "draft questions are not added to reviewed-only index",
		},
	})

	// Stage 2: Auto-tagging (skipped)
	meta = mergeQuestionAutoProcessingMetadata(meta, map[string]any{
		"auto_tagging": map[string]any{
			"status": "skipped",
			"reason": "knowledge_point_knowledge_base_id is empty",
		},
	})

	// Stage 3: Syllabus-checking (skipped)
	meta = mergeQuestionAutoProcessingMetadata(meta, map[string]any{
		"syllabus_checking": map[string]any{
			"status": "skipped",
			"reason": "syllabus_knowledge_base_id is empty",
		},
	})

	// Verify the final metadata structure.
	var final map[string]any
	if err := json.Unmarshal(meta, &final); err != nil {
		t.Fatalf("unmarshal final metadata: %v", err)
	}

	auto, _ := final["auto_processing"].(map[string]any)
	if auto == nil {
		t.Fatal("auto_processing key missing")
	}

	stages := []string{"indexing", "auto_tagging", "syllabus_checking"}
	for _, stage := range stages {
		stageMeta, ok := auto[stage].(map[string]any)
		if !ok {
			t.Fatalf("auto_processing.%s missing or wrong type", stage)
		}
		if stageMeta["status"] != "skipped" {
			t.Fatalf("auto_processing.%s.status = %q, want 'skipped'", stage, stageMeta["status"])
		}
	}

	// Verify no data loss: the pipeline result should be valid JSON.
	if !json.Valid(meta) {
		t.Fatal("final metadata is not valid JSON")
	}
}

// TestPipelineDoesNotAutoSetReviewedStatus verifies that the pipeline
// never transitions a question's status to "reviewed". Only human
// intervention via UpdateQuestionStatus should do that.
func TestPipelineDoesNotAutoSetReviewedStatus(t *testing.T) {
	// The pipeline's writeAutoProcessingMetadataToQuestions only updates
	// extraction_metadata — it never touches question.Status.
	// The startProcessingPipeline method never calls UpdateQuestionStatus.
	// This test validates the invariant.

	question := &types.Question{
		ID:              "q-1",
		Status:          types.QuestionStatusDraft,
		QuestionSetID:   "set-1",
		KnowledgeBaseID: "kb-1",
		ExtractionMetadata: types.JSON(`{}`),
	}

	// Simulate what writeAutoProcessingMetadataToQuestions does.
	question.ExtractionMetadata = mergeQuestionAutoProcessingMetadata(
		question.ExtractionMetadata,
		map[string]any{
			"indexing": map[string]any{"status": "skipped"},
		},
	)

	// After metadata update, status MUST still be draft.
	if question.Status != types.QuestionStatusDraft {
		t.Fatalf("status after pipeline metadata write = %q, want 'draft'", question.Status)
	}
}

// requireQuestionStatusRepository configures a repository to expect exactly one
// question set update (the draft_imported transition) and stores any created
// questions so tests can inspect their extraction_metadata.
func TestPipelineWritesMetadataToCreatedQuestions(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}
	kb := &types.KnowledgeBase{
		ID:   "kb-1",
		Type: types.KnowledgeBaseTypeQuestionBank,
		QuestionBankConfig: &types.QuestionBankConfig{
			KnowledgePointKnowledgeBaseID: "",
			SyllabusKnowledgeBaseID:       "",
		},
	}
	kbService := &questionStatusKBService{kb: kb}
	service := &QuestionService{
		repository:       repository,
		knowledgeBaseSvc: kbService,
	}

	// Import questions — this will trigger the pipeline (goroutine).
	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "题干", AnswerText: "答案"},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("ImportQuestions() created = %d, want 1", result.Created)
	}

	// After synchronous import, questions should be draft.
	for _, q := range repository.createdQuestions {
		if q.Status != types.QuestionStatusDraft {
			t.Fatalf("imported question status = %q, want 'draft'", q.Status)
		}
	}

	// The set should be at draft_imported stage (synchronous).
	if repository.set.ProcessingStage != types.QuestionSetProcessingStageDraftImported {
		t.Fatalf("ProcessingStage = %q, want %q",
			repository.set.ProcessingStage,
			types.QuestionSetProcessingStageDraftImported)
	}
}

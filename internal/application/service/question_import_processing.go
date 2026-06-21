package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// mergeQuestionAutoProcessingMetadata merges auto_processing stage metadata
// into the existing extraction_metadata JSON value, preserving all existing fields.
// A nil or empty existing is treated as an empty object.
// If existing is not valid JSON, it logs a warning and starts from an empty object.
func mergeQuestionAutoProcessingMetadata(existing types.JSON, patch map[string]any) types.JSON {
	var base map[string]any
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &base); err != nil {
			// Use background context for logging in this pure helper.
			logger.Warnf(context.Background(),
				"mergeQuestionAutoProcessingMetadata: failed to parse existing extraction_metadata, starting fresh: %v", err)
			base = make(map[string]any)
		}
	}
	if base == nil {
		base = make(map[string]any)
	}

	autoProcessing, _ := base["auto_processing"].(map[string]any)
	if autoProcessing == nil {
		autoProcessing = make(map[string]any)
	}

	for k, v := range patch {
		autoProcessing[k] = v
	}

	base["auto_processing"] = autoProcessing

	data, err := json.Marshal(base)
	if err != nil {
		return types.JSON([]byte("{}"))
	}
	return types.JSON(data)
}

// updateQuestionSetProcessingStage fetches the question set from the database,
// applies the given stage/status/error, and persists the update.
// An empty status string means "do not modify the current status".
func (s *QuestionService) updateQuestionSetProcessingStage(
	ctx context.Context,
	setID string,
	stage types.QuestionSetProcessingStage,
	status types.QuestionSetStatus,
	errorMessage string,
) error {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return fmt.Errorf("get question set %s: %w", setID, err)
	}
	qs.ProcessingStage = stage
	if status != "" {
		qs.Status = status
	}
	if errorMessage != "" {
		qs.ErrorMessage = errorMessage
	}
	return s.repository.UpdateQuestionSet(ctx, qs)
}

// writeAutoProcessingMetadataToQuestions updates each question's extraction_metadata
// with the given stage's auto_processing entry. Individual write failures are logged
// as warnings but do not halt the pipeline.
func (s *QuestionService) writeAutoProcessingMetadataToQuestions(
	ctx context.Context,
	questions []*types.Question,
	stage string,
	meta map[string]any,
) {
	for _, q := range questions {
		q.ExtractionMetadata = mergeQuestionAutoProcessingMetadata(
			q.ExtractionMetadata,
			map[string]any{stage: meta},
		)
		if err := s.repository.UpdateQuestion(ctx, q); err != nil {
			logger.Warnf(ctx,
				"failed to write auto_processing.%s metadata for question %s: %v",
				stage, q.ID, err)
		}
	}
}

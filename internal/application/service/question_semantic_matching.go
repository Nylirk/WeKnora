package service

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	defaultTopK                = 5
	syllabusDefaultTopK        = 3
	SyllabusInScopeThreshold   = 0.70
	SyllabusUncertainThreshold = 0.50
)

// RunKnowledgePointMatching performs semantic matching of draft questions against
// the configured knowledge point knowledge base. Results are written to
// extraction_metadata.auto_processing.auto_tagging and redundant status fields.
// Individual question failures are logged but do not halt the pipeline.
func (s *QuestionService) RunKnowledgePointMatching(
	ctx context.Context,
	cfg *types.QuestionBankConfig,
	questions []*types.Question,
) error {
	if !cfg.AutoKnowledgePointEnabled() {
		return s.writePausedMetadata(ctx, questions, "auto_tagging",
			"未关联知识点知识库")
	}

	for _, q := range questions {
		if q == nil {
			continue
		}
		query := types.BuildQuestionSemanticQuery(q)
		if query == "" {
			logger.Warnf(ctx, "[auto_tagging] empty query for question %s, skipping", q.ID)
			s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", map[string]any{
				"status": "failed",
				"reason": "empty query text",
			}, "failed", "")
			continue
		}

		results, err := s.semanticSearchInKB(ctx, cfg.KnowledgePointKnowledgeBaseID, query, defaultTopK)
		if err != nil {
			logger.Warnf(ctx, "[auto_tagging] search failed for question %s: %v", q.ID, err)
			s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", map[string]any{
				"status": "failed",
				"reason": truncateError(err),
			}, "failed", "")
			continue
		}

		candidates := buildKnowledgePointCandidates(results)
		statusValue := "matched"
		if len(candidates) == 0 {
			statusValue = "unmatched"
		}
		meta := map[string]any{
			"status":     statusValue,
			"matched_at": time.Now().UTC().Format(time.RFC3339),
			"candidates": candidates,
		}
		s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", meta, statusValue, "")
	}

	return nil
}

// RunSyllabusFiltering performs semantic matching of draft questions against the
// configured syllabus knowledge base. Results are written to
// extraction_metadata.auto_processing.syllabus_checking and redundant status fields.
func (s *QuestionService) RunSyllabusFiltering(
	ctx context.Context,
	cfg *types.QuestionBankConfig,
	questions []*types.Question,
) error {
	if !cfg.AutoSyllabusCheckEnabled() {
		return s.writePausedMetadata(ctx, questions, "syllabus_checking",
			"未配置考纲")
	}

	for _, q := range questions {
		if q == nil {
			continue
		}
		query := types.BuildQuestionSemanticQuery(q)
		if query == "" {
			logger.Warnf(ctx, "[syllabus_checking] empty query for question %s, skipping", q.ID)
			s.writeSingleQuestionMetadata(ctx, q, "syllabus_checking", map[string]any{
				"status": "failed",
				"reason": "empty query text",
			}, "failed", "")
			continue
		}

		results, err := s.semanticSearchInKB(ctx, cfg.SyllabusKnowledgeBaseID, query, syllabusDefaultTopK)
		if err != nil {
			logger.Warnf(ctx, "[syllabus_checking] search failed for question %s: %v", q.ID, err)
			s.writeSingleQuestionMetadata(ctx, q, "syllabus_checking", map[string]any{
				"status": "failed",
				"reason": truncateError(err),
			}, "failed", "")
			continue
		}

		result, topScore := classifySyllabusResult(results)
		meta := map[string]any{
			"status":     "completed",
			"result":     result,
			"confidence": topScore,
			"score":      topScore,
			"matched_at": time.Now().UTC().Format(time.RFC3339),
			"evidence":   buildSyllabusEvidence(results),
		}
		s.writeSingleQuestionMetadata(ctx, q, "syllabus_checking", meta, "completed", result)
	}

	return nil
}

// semanticSearchInKB performs vector-only semantic search in the target KB.
func (s *QuestionService) semanticSearchInKB(
	ctx context.Context,
	targetKBID string,
	query string,
	topK int,
) ([]*types.SearchResult, error) {
	params := types.SearchParams{
		QueryText:            query,
		MatchCount:           topK,
		DisableKeywordsMatch: true,
	}
	return s.knowledgeBaseSvc.HybridSearch(ctx, targetKBID, params)
}

// writePausedMetadata writes paused status metadata to all given questions.
func (s *QuestionService) writePausedMetadata(
	ctx context.Context,
	questions []*types.Question,
	stage string,
	reason string,
) error {
	meta := map[string]any{
		"status": "paused",
		"reason": reason,
	}
	for _, q := range questions {
		if q == nil {
			continue
		}
		merged := mergeQuestionAutoProcessingMetadata(q.ExtractionMetadata, map[string]any{stage: meta})
		q.ExtractionMetadata = merged
		s.syncQuestionStatusFromStage(q, stage, "paused", "")
		if err := s.repository.UpdateQuestion(ctx, q); err != nil {
			logger.Warnf(ctx,
				"failed to write auto_processing.%s paused metadata for question %s: %v",
				stage, q.ID, err)
		}
	}
	return nil
}

// writeSingleQuestionMetadata merges stage metadata into the question and syncs
// the redundant status fields. For auto_tagging, the meta status value is used for
// auto_tagging_status. For syllabus_checking, the meta status value is used for
// syllabus_checking_status and syllabusScopeResult (if set) is used for syllabus_scope_result.
func (s *QuestionService) writeSingleQuestionMetadata(
	ctx context.Context,
	q *types.Question,
	stage string,
	meta map[string]any,
	statusValue string,
	syllabusScopeResult string,
) {
	merged := mergeQuestionAutoProcessingMetadata(q.ExtractionMetadata, map[string]any{stage: meta})
	q.ExtractionMetadata = merged
	s.syncQuestionStatusFromStage(q, stage, statusValue, syllabusScopeResult)
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		logger.Warnf(ctx,
			"failed to write auto_processing.%s metadata for question %s: %v",
			stage, q.ID, err)
	}
}

// syncQuestionStatusFromStage updates the redundant filter columns based on
// which pipeline stage just completed.
func (s *QuestionService) syncQuestionStatusFromStage(
	q *types.Question,
	stage string,
	statusValue string,
	syllabusScopeResult string,
) {
	switch stage {
	case "auto_tagging":
		q.AutoTaggingStatus = statusValue
	case "syllabus_checking":
		q.SyllabusCheckingStatus = statusValue
		if syllabusScopeResult != "" {
			q.SyllabusScopeResult = syllabusScopeResult
		}
	}
}

// buildKnowledgePointCandidates converts search results to candidate structures.
func buildKnowledgePointCandidates(results []*types.SearchResult) []map[string]any {
	candidates := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if r == nil {
			continue
		}
		candidates = append(candidates, map[string]any{
			"knowledge_point":    firstNonEmpty(r.KnowledgeTitle, truncateText(r.Content, 100)),
			"confidence":         r.Score,
			"score":              r.Score,
			"source_knowledge_id": r.KnowledgeID,
			"evidence_chunk_id":  r.ID,
			"evidence_text":      truncateText(r.Content, 500),
		})
	}
	return candidates
}

// classifySyllabusResult determines the scope result from search scores.
func classifySyllabusResult(results []*types.SearchResult) (result string, topScore float64) {
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Score > topScore {
			topScore = r.Score
		}
	}
	switch {
	case topScore >= SyllabusInScopeThreshold:
		return "in_scope", topScore
	case topScore >= SyllabusUncertainThreshold:
		return "uncertain", topScore
	default:
		return "out_of_scope", topScore
	}
}

// buildSyllabusEvidence converts search results to evidence structures.
func buildSyllabusEvidence(results []*types.SearchResult) []map[string]any {
	evidence := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if r == nil {
			continue
		}
		evidence = append(evidence, map[string]any{
			"syllabus_chunk_id":   r.ID,
			"source_knowledge_id": r.KnowledgeID,
			"text":                truncateText(r.Content, 500),
		})
	}
	return evidence
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}


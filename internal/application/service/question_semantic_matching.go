package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	defaultTopK                = 5
	syllabusDefaultTopK        = 3
	SyllabusInScopeThreshold   = 0.70
	SyllabusUncertainThreshold = 0.50

	KnowledgePointDefaultTopK      = 20
	KnowledgePointCandidateLimit   = 5
	KnowledgePointMinScore         = 0.68
	KnowledgePointMinMargin        = 0.08
	KnowledgePointAlgorithmVersion = "kp_match_v3"

	kpRerankScoreWeight = 0.7
	kpRawScoreWeight    = 0.3
	kpMaxEvidence       = 3
)

// knowledgePointProjection is an in-memory candidate structure built from
// SearchResult. It is NOT persisted — it exists only to carry aggregated
// candidate data through the rerank and classification pipeline.
type knowledgePointProjection struct {
	Label            string
	NormalizedLabel  string
	SourceKnowledgeID string
	EvidenceChunkIDs []string
	EvidenceTexts    []string
	Reason           string
	RawScore         float64
	RerankScore      float64
	Score            float64
	MatchSignals     []string
}

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
		query := types.BuildKnowledgePointMatchingQuery(q)
		if query == "" {
			logger.Warnf(ctx, "[auto_tagging] empty query for question %s, skipping", q.ID)
			s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", map[string]any{
				"status": "failed",
				"reason": "empty query text",
			}, "failed", "")
			continue
		}

		topK := KnowledgePointDefaultTopK
		if cfg.KnowledgePointRerankTopK > 0 {
			topK = cfg.KnowledgePointRerankTopK
		}
		results, err := s.semanticSearchKnowledgePoints(ctx, cfg.KnowledgePointKnowledgeBaseID, query, topK)
		if err != nil {
			logger.Warnf(ctx, "[auto_tagging] search failed for question %s: %v", q.ID, err)
			s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", map[string]any{
				"status": "failed",
				"reason": truncateError(err),
			}, "failed", "")
			continue
		}

		projections := buildKnowledgePointProjections(results)
		rerankMode := "disabled"
		rerankModelID := ""
		var rerankErr string

		if cfg.KnowledgePointRerankEnabledModel() && s.modelService != nil {
			rerankModelID = cfg.KnowledgePointRerankModelID
			reranker, rErr := s.modelService.GetRerankModel(ctx, rerankModelID)
			if rErr != nil {
				rerankErr = truncateError(rErr)
				logger.Warnf(ctx, "[auto_tagging] GetRerankModel failed for question %s: %v", q.ID, rErr)
				applyRuleRerank(query, projections)
				rerankMode = "rule_fallback"
		} else {
			reranked, rrErr := rerankKnowledgePointProjectionsWithModel(ctx, query, projections, reranker)
			if rrErr != nil || len(reranked) == 0 {
				if rrErr != nil {
					rerankErr = truncateError(rrErr)
				}
				logger.Warnf(ctx, "[auto_tagging] model rerank returned empty for question %s, falling back", q.ID)
				applyRuleRerank(query, projections)
				rerankMode = "rule_fallback"
			} else {
				projections = reranked
				rerankMode = "model"
			}
		}
		} else {
			applyRuleRerank(query, projections)
			if cfg.KnowledgePointRerankEnabledModel() && s.modelService == nil {
				rerankMode = "rule_fallback"
			}
		}

		sort.Slice(projections, func(i, j int) bool {
			return projections[i].Score > projections[j].Score
		})

		statusValue, topScore, secondScore := classifyProjections(projections)
		candidates := projectionsToCandidates(projections, KnowledgePointCandidateLimit)
		meta := map[string]any{
			"status":                       statusValue,
			"matched_at":                   time.Now().UTC().Format(time.RFC3339),
			"algorithm_version":            KnowledgePointAlgorithmVersion,
			"query":                        query,
			"min_score":                    KnowledgePointMinScore,
			"min_margin":                   KnowledgePointMinMargin,
			"candidates":                   candidates,
			"scoring":                      "model_rerank_v1",
			"rerank_mode":                  rerankMode,
			"projection_count":             len(projections),
			"candidate_count_before_limit": len(projections),
		}
		if rerankModelID != "" {
			meta["rerank_model_id"] = rerankModelID
		}
		if rerankErr != "" {
			meta["rerank_error"] = rerankErr
		}
		if topScore > 0 {
			meta["top_score"] = topScore
		}
		if secondScore > 0 {
			meta["second_score"] = secondScore
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

// semanticSearchKnowledgePoints performs hybrid (vector + keyword) search in the
// knowledge point KB. Unlike semanticSearchInKB, it does not disable keyword
// matching, allowing lexical recall to complement vector similarity for
// knowledge-point matching against unstructured text KBs.
func (s *QuestionService) semanticSearchKnowledgePoints(
	ctx context.Context,
	targetKBID string,
	query string,
	topK int,
) ([]*types.SearchResult, error) {
	params := types.SearchParams{
		QueryText:  query,
		MatchCount: topK,
	}
	return s.knowledgeBaseSvc.HybridSearch(ctx, targetKBID, params)
}

// buildKnowledgePointProjections converts raw search results into aggregated
// knowledgePointProjection entries. Results sharing the same normalized label
// (case-folded, trimmed KnowledgeTitle or inferred content prefix) are merged:
// RawScore takes the max, evidence chunk IDs/texts are capped at kpMaxEvidence.
func buildKnowledgePointProjections(results []*types.SearchResult) []knowledgePointProjection {
	byLabel := make(map[string]*knowledgePointProjection)
	order := make([]string, 0, len(results))

	for _, r := range results {
		if r == nil {
			continue
		}
		label, reason := knowledgePointLabel(r)
		if label == "" {
			continue
		}
		norm := normalizeKnowledgePointLabel(label)
		proj, exists := byLabel[norm]
		if !exists {
			proj = &knowledgePointProjection{
				Label:            label,
				NormalizedLabel:  norm,
				SourceKnowledgeID: r.KnowledgeID,
				Reason:           reason,
				RawScore:         r.Score,
			}
			byLabel[norm] = proj
			order = append(order, norm)
		} else {
			if r.Score > proj.RawScore {
				proj.RawScore = r.Score
			}
		}
		if r.ID != "" && len(proj.EvidenceChunkIDs) < kpMaxEvidence {
			proj.EvidenceChunkIDs = append(proj.EvidenceChunkIDs, r.ID)
			proj.EvidenceTexts = append(proj.EvidenceTexts, truncateText(r.Content, 500))
		}
	}

	projections := make([]knowledgePointProjection, 0, len(order))
	for _, norm := range order {
		projections = append(projections, *byLabel[norm])
	}
	return projections
}

// normalizeKnowledgePointLabel produces a case-folded, trimmed key for
// aggregating projections that differ only in casing or whitespace.
func normalizeKnowledgePointLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

// rerankKnowledgePointProjectionsWithModel calls the rerank model to re-score
// projections. Each projection is serialized as a passage combining label and
// the first evidence text. RerankScore is set from RankResult.RelevanceScore.
// The final Score blends rerank and raw scores:
//   Score = kpRerankScoreWeight * rerank_score + kpRawScoreWeight * raw_score
// clamped to [0, 1]. Projections whose passages are empty are skipped (but
// still keep their raw score). Returns the input slice (mutated) on success.
func rerankKnowledgePointProjectionsWithModel(
	ctx context.Context,
	query string,
	projections []knowledgePointProjection,
	reranker rerank.Reranker,
) ([]knowledgePointProjection, error) {
	documents := make([]string, len(projections))
	validIdx := make([]int, 0, len(projections))
	for i, p := range projections {
		passages := p.Label
		if len(p.EvidenceTexts) > 0 && strings.TrimSpace(p.EvidenceTexts[0]) != "" {
			passages += "\n" + p.EvidenceTexts[0]
		}
		documents[i] = passages
		if strings.TrimSpace(passages) != "" {
			validIdx = append(validIdx, i)
		}
	}
	if len(validIdx) == 0 {
		return nil, nil
	}

	cleanDocs := make([]string, len(validIdx))
	for j, idx := range validIdx {
		cleanDocs[j] = documents[idx]
	}

	rankResults, err := reranker.Rerank(ctx, query, cleanDocs)
	if err != nil {
		return nil, err
	}
	if len(rankResults) == 0 {
		return nil, nil
	}

	for _, rr := range rankResults {
		if rr.Index < 0 || rr.Index >= len(validIdx) {
			continue
		}
		projIdx := validIdx[rr.Index]
		projections[projIdx].RerankScore = rr.RelevanceScore
		projections[projIdx].Score = clamp01(kpRerankScoreWeight*rr.RelevanceScore + kpRawScoreWeight*projections[projIdx].RawScore)
		projections[projIdx].MatchSignals = append(projections[projIdx].MatchSignals, "model_rerank")
	}

	for i := range projections {
		if projections[i].RerankScore == 0 && projections[i].Score == 0 {
			projections[i].Score = projections[i].RawScore
		}
	}

	return projections, nil
}

// applyRuleRerank computes a deterministic Score for each projection using
// simple text-overlap signals. This is the fallback when model rerank is
// unavailable, disabled, or fails. The base is RawScore; small bonuses are
// added for query/label and query/evidence overlap, and for labels sourced
// from KnowledgeTitle. Overly long inferred labels receive a small penalty.
func applyRuleRerank(query string, projections []knowledgePointProjection) {
	queryLower := strings.ToLower(query)
	for i := range projections {
		p := &projections[i]
		score := p.RawScore
		labelLower := strings.ToLower(p.Label)
		if p.Reason == "knowledge_title" {
			score += 0.02
			p.MatchSignals = append(p.MatchSignals, "title_source")
		}
		if labelOverlap(queryLower, labelLower) {
			score += 0.03
			p.MatchSignals = append(p.MatchSignals, "label_overlap")
		}
		for _, et := range p.EvidenceTexts {
			if labelOverlap(queryLower, strings.ToLower(et)) {
				score += 0.02
				p.MatchSignals = append(p.MatchSignals, "evidence_overlap")
				break
			}
		}
		runeLen := len([]rune(p.Label))
		if p.Reason == "inferred_from_content" && runeLen > 40 {
			score -= 0.02
			p.MatchSignals = append(p.MatchSignals, "long_label_penalty")
		}
		p.RerankScore = 0
		p.Score = clamp01(score)
	}
}

func labelOverlap(query, candidate string) bool {
	if query == "" || candidate == "" {
		return false
	}
	queryTokens := tokenizeForOverlap(query)
	candidateLower := strings.ToLower(candidate)
	for _, tok := range queryTokens {
		if len(tok) >= 2 && strings.Contains(candidateLower, tok) {
			return true
		}
	}
	return false
}

func tokenizeForOverlap(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '.' || r == ';' || r == ':' || r == '/' || r == '(' || r == ')'
	})
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// classifyProjections applies score and margin gates to determine the
// auto_tagging status from the final projection scores.
//   - unmatched: no projections or top1 < KnowledgePointMinScore
//   - uncertain: top1 >= KnowledgePointMinScore but margin < KnowledgePointMinMargin
//   - matched:   top1 >= KnowledgePointMinScore and margin >= KnowledgePointMinMargin
func classifyProjections(projections []knowledgePointProjection) (string, float64, float64) {
	if len(projections) == 0 {
		return "unmatched", 0, 0
	}
	topScore := projections[0].Score
	secondScore := 0.0
	if len(projections) > 1 {
		secondScore = projections[1].Score
	}
	if topScore < KnowledgePointMinScore {
		return "unmatched", topScore, secondScore
	}
	if topScore-KnowledgePointMinMargin < secondScore {
		return "uncertain", topScore, secondScore
	}
	return "matched", topScore, secondScore
}

// projectionsToCandidates converts the final sorted projections into the
// candidate map slice written to auto_tagging metadata. Capped at limit.
// Preserves all existing candidate fields and adds raw_score, rerank_score,
// rerank_mode (via match_signals), evidence_chunk_ids, and match_signals.
func projectionsToCandidates(projections []knowledgePointProjection, limit int) []map[string]any {
	if limit <= 0 {
		return []map[string]any{}
	}
	candidates := make([]map[string]any, 0, limit)
	for _, p := range projections {
		if len(candidates) >= limit {
			break
		}
		chunkID := ""
		if len(p.EvidenceChunkIDs) > 0 {
			chunkID = p.EvidenceChunkIDs[0]
		}
		evidenceText := ""
		if len(p.EvidenceTexts) > 0 {
			evidenceText = p.EvidenceTexts[0]
		}
		candidates = append(candidates, map[string]any{
			"knowledge_point":      p.Label,
			"confidence":           p.Score,
			"score":                p.Score,
			"source_knowledge_id":  p.SourceKnowledgeID,
			"evidence_chunk_id":    chunkID,
			"evidence_text":        evidenceText,
			"reason":               p.Reason,
			"raw_score":            p.RawScore,
			"rerank_score":         p.RerankScore,
			"evidence_chunk_ids":   p.EvidenceChunkIDs,
			"match_signals":        p.MatchSignals,
		})
	}
	return candidates
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
		q.SyllabusScopeResult = syllabusScopeResult
	}
}

// knowledgePointLabel derives a short knowledge-point label from a search result.
// Prefers KnowledgeTitle; falls back to a short truncation of chunk content.
func knowledgePointLabel(r *types.SearchResult) (string, string) {
	if title := strings.TrimSpace(r.KnowledgeTitle); title != "" {
		return title, "knowledge_title"
	}
	if content := strings.TrimSpace(r.Content); content != "" {
		return truncateText(content, 60), "inferred_from_content"
	}
	return "", ""
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
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}


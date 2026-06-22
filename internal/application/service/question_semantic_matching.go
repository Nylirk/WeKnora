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

	// Resolve rerank config once for the entire batch to avoid repeated
	// tenant config lookups. Explicit QuestionBankConfig takes priority,
	// then fall back to tenant RetrievalConfig, then disabled.
	rc := s.resolveRerankConfig(ctx, cfg)
	rerankModelID := rc.modelID
	rerankModelSource := rc.source
	rerankThreshold := rc.threshold
	rerankTopK := rc.topK

	// Resolve reranker once if a model is configured. This avoids calling
	// GetRerankModel for every question in the batch.
	var reranker rerank.Reranker
	var rerankResolveErr string
	if rerankModelID != "" && s.modelService != nil {
		var rErr error
		reranker, rErr = s.modelService.GetRerankModel(ctx, rerankModelID)
		if rErr != nil {
			rerankResolveErr = truncateError(rErr)
			logger.Warnf(ctx, "[auto_tagging] GetRerankModel failed: %v", rErr)
			reranker = nil
		}
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

		results, err := s.semanticSearchKnowledgePoints(ctx, cfg.KnowledgePointKnowledgeBaseID, query, KnowledgePointDefaultTopK)
		if err != nil {
			logger.Warnf(ctx, "[auto_tagging] search failed for question %s: %v", q.ID, err)
			s.writeSingleQuestionMetadata(ctx, q, "auto_tagging", map[string]any{
				"status": "failed",
				"reason": truncateError(err),
			}, "failed", "")
			continue
		}

		projections := buildKnowledgePointProjections(results)
		var rerankErr string
		currentRerankMode := "disabled"

		if reranker != nil {
			projectionsToRerank := projections
			if rerankTopK > 0 && len(projections) > rerankTopK {
				sort.Slice(projections, func(i, j int) bool {
					return projections[i].RawScore > projections[j].RawScore
				})
				projectionsToRerank = projections[:rerankTopK]
			}
			rerankApplied, rrErr := rerankKnowledgePointProjectionsWithModel(ctx, query, projectionsToRerank, reranker)
			if rrErr != nil || rerankApplied == nil {
				if rrErr != nil {
					rerankErr = truncateError(rrErr)
				}
				logger.Warnf(ctx, "[auto_tagging] model rerank failed for question %s, falling back", q.ID)
				applyRuleRerank(query, projections)
				currentRerankMode = "rule_fallback"
			} else {
				applyRerankMissingPenalty(projections, projectionsToRerank)
				if rerankThreshold > 0 {
					applyRerankThreshold(projections, rerankThreshold)
				}
				currentRerankMode = "model"
			}
		} else {
			applyRuleRerank(query, projections)
			if rerankModelID != "" && s.modelService == nil {
				currentRerankMode = "rule_fallback"
			} else if rerankResolveErr != "" {
				currentRerankMode = "rule_fallback"
				rerankErr = rerankResolveErr
			}
		}

		sort.Slice(projections, func(i, j int) bool {
			return projections[i].Score > projections[j].Score
		})

		statusValue, topScore, secondScore := classifyProjections(projections)
		candidates := projectionsToCandidates(projections, KnowledgePointCandidateLimit, currentRerankMode)
		meta := map[string]any{
			"status":                       statusValue,
			"matched_at":                   time.Now().UTC().Format(time.RFC3339),
			"algorithm_version":            KnowledgePointAlgorithmVersion,
			"query":                        query,
			"min_score":                    KnowledgePointMinScore,
			"min_margin":                   KnowledgePointMinMargin,
			"candidates":                   candidates,
			"scoring":                      "model_rerank_v1",
			"rerank_mode":                  currentRerankMode,
			"projection_count":             len(projections),
			"candidate_count_before_limit": len(projections),
		}
		if rerankModelID != "" {
			meta["rerank_model_id"] = rerankModelID
		}
		meta["rerank_model_source"] = rerankModelSource
		if rerankErr != "" {
			meta["rerank_error"] = rerankErr
		}
		if rerankThreshold > 0 {
			meta["rerank_threshold"] = rerankThreshold
		}
		if rerankTopK > 0 {
			meta["rerank_top_k"] = rerankTopK
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

// kpRerankConfig holds the resolved rerank configuration for knowledge point
// matching, with the source of each field for metadata attribution.
type kpRerankConfig struct {
	modelID   string
	source    string
	threshold float64
	topK      int
}

// resolveRerankConfig resolves the rerank configuration with the following
// priority:
//  1. Explicit QuestionBankConfig fields (when KnowledgePointRerankEnabled is
//     true and KnowledgePointRerankModelID is non-empty).
//  2. Tenant RetrievalConfig (when the KB has a linked knowledge point KB and
//     the tenant's RetrievalConfig.RerankModelID is non-empty).
//  3. Unavailable (no rerank).
//
// Threshold and topK from QuestionBankConfig take priority over tenant config.
func (s *QuestionService) resolveRerankConfig(ctx context.Context, cfg *types.QuestionBankConfig) kpRerankConfig {
	// Priority 1: explicit QuestionBankConfig rerank.
	if cfg != nil && cfg.KnowledgePointRerankEnabled && cfg.KnowledgePointRerankModelID != "" {
		return kpRerankConfig{
			modelID:   cfg.KnowledgePointRerankModelID,
			source:    "question_bank_config",
			threshold: cfg.KnowledgePointRerankThreshold,
			topK:      cfg.KnowledgePointRerankTopK,
		}
	}

	// Priority 2: tenant RetrievalConfig fallback.
	// Only active when the KB has a linked knowledge point KB.
	if cfg != nil && cfg.KnowledgePointKnowledgeBaseID != "" && s.tenantService != nil {
		tenantID, ok := types.TenantIDFromContext(ctx)
		if !ok {
			return kpRerankConfig{source: "unavailable"}
		}
		tenant, err := s.tenantService.GetTenantByID(ctx, tenantID)
		if err != nil || tenant == nil || tenant.RetrievalConfig == nil {
			return kpRerankConfig{source: "unavailable"}
		}
		rc := tenant.RetrievalConfig
		if rc.RerankModelID == "" {
			return kpRerankConfig{source: "unavailable"}
		}
		resolved := kpRerankConfig{
			modelID: rc.RerankModelID,
			source:  "tenant_retrieval_config",
			topK:    rc.GetEffectiveRerankTopK(),
		}
		// Use tenant threshold, but if QuestionBankConfig has an explicit
		// non-zero threshold, it takes priority.
		if cfg.KnowledgePointRerankThreshold > 0 {
			resolved.threshold = cfg.KnowledgePointRerankThreshold
		} else {
			resolved.threshold = rc.GetEffectiveRerankThreshold()
		}
		return resolved
	}

	return kpRerankConfig{source: "unavailable"}
}
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

// normalizeKnowledgePointLabel produces a normalized key for aggregating
// projections that differ only in casing, whitespace, hyphens, underscores,
// or common CJK/Latin punctuation. Chinese characters are preserved.
func normalizeKnowledgePointLabel(label string) string {
	s := strings.TrimSpace(label)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "\u3000", " ")
	for _, p := range kpLabelPunctuation {
		s = strings.ReplaceAll(s, p, "")
	}
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

var kpLabelPunctuation = []string{
	",", ".", ";", ":",
	"：", "，", "。", "、",
	"（", "）", "(", ")",
	"[", "]", "【", "】",
	"\"", "'", "“", "”",
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

	rerankedSet := make(map[int]bool, len(rankResults))
	for _, rr := range rankResults {
		if rr.Index < 0 || rr.Index >= len(validIdx) {
			continue
		}
		projIdx := validIdx[rr.Index]
		rerankedSet[projIdx] = true
		projections[projIdx].RerankScore = rr.RelevanceScore
		projections[projIdx].Score = clamp01(kpRerankScoreWeight*rr.RelevanceScore + kpRawScoreWeight*projections[projIdx].RawScore)
		projections[projIdx].MatchSignals = append(projections[projIdx].MatchSignals, "model_rerank")
	}

	for i := range projections {
		if !rerankedSet[i] {
			projections[i].MatchSignals = append(projections[i].MatchSignals, "rerank_missing")
			projections[i].Score = projections[i].RawScore * 0.5
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

// applyRerankMissingPenalty marks projections beyond the rerank subset as
// rerank_missing and halves their Score so they cannot dominate projections
// that received an explicit model score.
func applyRerankMissingPenalty(all, reranked []knowledgePointProjection) {
	if len(reranked) >= len(all) {
		return
	}
	rerankedSet := make(map[string]bool, len(reranked))
	for _, p := range reranked {
		rerankedSet[p.NormalizedLabel] = true
	}
	for i := range all {
		if !rerankedSet[all[i].NormalizedLabel] {
			all[i].MatchSignals = append(all[i].MatchSignals, "rerank_missing")
			all[i].Score = all[i].RawScore * 0.5
		}
	}
}

// applyRerankThreshold penalizes any projection whose RerankScore falls
// below the configured threshold. The Score is halved so that a weak
// model signal cannot be masked by a high raw score, and the projection
// will not pass the KnowledgePointMinScore gate for matched status.
func applyRerankThreshold(projections []knowledgePointProjection, threshold float64) {
	for i := range projections {
		if projections[i].RerankScore < threshold {
			projections[i].Score = projections[i].Score * 0.5
			projections[i].MatchSignals = append(projections[i].MatchSignals, "below_rerank_threshold")
		}
	}
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
// rerank_mode, evidence_chunk_ids, and match_signals.
func projectionsToCandidates(projections []knowledgePointProjection, limit int, rerankMode string) []map[string]any {
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
			"rerank_mode":          rerankMode,
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
// Prefers KnowledgeTitle; falls back to inferKnowledgePointLabelFromContent.
func knowledgePointLabel(r *types.SearchResult) (string, string) {
	if title := strings.TrimSpace(r.KnowledgeTitle); title != "" {
		return title, "knowledge_title"
	}
	if label := inferKnowledgePointLabelFromContent(r.Content); label != "" {
		return label, "inferred_from_content"
	}
	return "", ""
}

// inferKnowledgePointLabelFromContent extracts a short label from unstructured
// chunk content. It tries colon/dash separators first, then sentence
// boundaries, and finally rune-safe truncation to 60 characters.
func inferKnowledgePointLabelFromContent(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	for _, sep := range kpLabelSeparators {
		if idx := strings.Index(s, sep); idx > 0 {
			prefix := strings.TrimSpace(s[:idx])
			if len([]rune(prefix)) >= 2 {
				return truncateText(prefix, 60)
			}
		}
	}
	for _, sep := range kpLabelSentenceEnds {
		if idx := strings.Index(s, sep); idx > 0 {
			prefix := strings.TrimSpace(s[:idx])
			if len([]rune(prefix)) >= 2 {
				return truncateText(prefix, 60)
			}
		}
	}
	return truncateText(s, 60)
}

var kpLabelSeparators = []string{":", "：", "—", "-"}

var kpLabelSentenceEnds = []string{".", "。", "；", ";"}

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


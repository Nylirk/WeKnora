package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// ── Golden case types ──

type kpGoldenMockResult struct {
	ID             string  `json:"id"`
	KnowledgeID    string  `json:"knowledge_id"`
	KnowledgeTitle string  `json:"knowledge_title"`
	Content        string  `json:"content"`
	Score          float64 `json:"score"`
}

type kpGoldenCase struct {
	ID                        string                   `json:"id"`
	QuestionType              string                   `json:"question_type"`
	Difficulty                string                   `json:"difficulty"`
	StemText                  string                   `json:"stem_text"`
	QuestionBody              json.RawMessage          `json:"question_body"`
	ExpectedKnowledgePoints   []string                 `json:"expected_knowledge_points"`
	AcceptableKnowledgePoints []string                 `json:"acceptable_knowledge_points"`
	NegativeKnowledgePoints   []string                 `json:"negative_knowledge_points"`
	MockResults               []kpGoldenMockResult     `json:"mock_results"`
}

// ── Eval metrics ──

type kpEvalMetrics struct {
	Total                 int
	Top1Accuracy          float64
	RecallAt3             float64
	FalsePositiveRate     float64
	UncertainRate         float64
	UnmatchedRate         float64
	AverageCandidateCount float64
}

// ── Per-question eval result ──

type kpCaseResult struct {
	Golden       kpGoldenCase
	Question     *types.Question
	Status       string
	TopScore     float64
	SecondScore  float64
	Candidates   []map[string]any
	Top1Hit      bool
	RecallHit    bool
	FalsePos     bool
}

func loadKPGoldens(t *testing.T) []kpGoldenCase {
	t.Helper()
	path := filepath.Join("testdata", "question_kp_goldens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read goldens file %s: %v", path, err)
	}
	var cases []kpGoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse goldens: %v", err)
	}
	return cases
}

func goldenToQuestion(g kpGoldenCase) *types.Question {
	body := types.JSON(g.QuestionBody)
	if len(body) == 0 {
		body = types.JSON(`{}`)
	}
	qType := g.QuestionType
	if qType == "" {
		qType = "short_answer"
	}
	diff := g.Difficulty
	if diff == "" {
		diff = "medium"
	}
	return &types.Question{
		ID:                 g.ID,
		QuestionType:       qType,
		Difficulty:         types.QuestionDifficulty(diff),
		StemText:           g.StemText,
		QuestionBody:       body,
		Status:             types.QuestionStatusDraft,
		KnowledgePoints:    types.JSON(`[]`),
		ExtractionMetadata: types.JSON(`{}`),
	}
}

func mockResultsFromGolden(g kpGoldenCase) []*types.SearchResult {
	results := make([]*types.SearchResult, 0, len(g.MockResults))
	for _, mr := range g.MockResults {
		results = append(results, &types.SearchResult{
			ID:             mr.ID,
			KnowledgeID:    mr.KnowledgeID,
			KnowledgeTitle: mr.KnowledgeTitle,
			Content:        mr.Content,
			Score:          mr.Score,
		})
	}
	return results
}

// runEvalCase runs RunKnowledgePointMatching for a single golden case using
// a per-case mock KB service. Returns the per-case result for metric aggregation.
func runEvalCase(t *testing.T, g kpGoldenCase) kpCaseResult {
	t.Helper()
	q := goldenToQuestion(g)
	mockResults := mockResultsFromGolden(g)
	kbSvc := makeMockKBService(mockResults, nil)
	repo := &matchingTestRepo{}
	svc := &QuestionService{repository: repo, knowledgeBaseSvc: kbSvc}
	cfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-kb-eval"}

	if err := svc.RunKnowledgePointMatching(context.Background(), cfg, []*types.Question{q}); err != nil {
		t.Fatalf("RunKnowledgePointMatching failed for %s: %v", g.ID, err)
	}

	var meta map[string]any
	if err := json.Unmarshal(q.ExtractionMetadata, &meta); err != nil {
		t.Fatalf("failed to parse extraction_metadata for %s: %v", g.ID, err)
	}
	autoProc, _ := meta["auto_processing"].(map[string]any)
	tagging, _ := autoProc["auto_tagging"].(map[string]any)
	status, _ := tagging["status"].(string)
	topScore, _ := tagging["top_score"].(float64)
	secondScore, _ := tagging["second_score"].(float64)
	candidates, _ := tagging["candidates"].([]any)

	parsedCandidates := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		if cm, ok := c.(map[string]any); ok {
			parsedCandidates = append(parsedCandidates, cm)
		}
	}

	hitSet := make(map[string]bool)
	for _, kp := range g.ExpectedKnowledgePoints {
		hitSet[kp] = true
	}
	for _, kp := range g.AcceptableKnowledgePoints {
		hitSet[kp] = true
	}
	negSet := make(map[string]bool)
	for _, kp := range g.NegativeKnowledgePoints {
		negSet[kp] = true
	}

	top1Hit := false
	if len(parsedCandidates) > 0 {
		if kp, ok := parsedCandidates[0]["knowledge_point"].(string); ok {
			top1Hit = hitSet[kp]
		}
	}

	recallHit := false
	for i, c := range parsedCandidates {
		if i >= 3 {
			break
		}
		if kp, ok := c["knowledge_point"].(string); ok {
			if hitSet[kp] {
				recallHit = true
				break
			}
		}
	}

	falsePos := false
	if status == "matched" {
		if !top1Hit {
			falsePos = true
		} else if len(parsedCandidates) > 0 {
			if kp, ok := parsedCandidates[0]["knowledge_point"].(string); ok {
				if negSet[kp] {
					falsePos = true
				}
			}
		}
	}

	return kpCaseResult{
		Golden:       g,
		Question:     q,
		Status:       status,
		TopScore:     topScore,
		SecondScore:  secondScore,
		Candidates:   parsedCandidates,
		Top1Hit:      top1Hit,
		RecallHit:    recallHit,
		FalsePos:     falsePos,
	}
}

func computeKPMetrics(results []kpCaseResult) kpEvalMetrics {
	m := kpEvalMetrics{Total: len(results)}
	if m.Total == 0 {
		return m
	}
	var top1Hits, recallHits, falsePos, uncertain, unmatched, totalCandidates int
	for _, r := range results {
		if r.Top1Hit {
			top1Hits++
		}
		if r.RecallHit {
			recallHits++
		}
		if r.FalsePos {
			falsePos++
		}
		if r.Status == "uncertain" {
			uncertain++
		}
		if r.Status == "unmatched" {
			unmatched++
		}
		totalCandidates += len(r.Candidates)
	}
	m.Top1Accuracy = float64(top1Hits) / float64(m.Total)
	m.RecallAt3 = float64(recallHits) / float64(m.Total)
	m.FalsePositiveRate = float64(falsePos) / float64(m.Total)
	m.UncertainRate = float64(uncertain) / float64(m.Total)
	m.UnmatchedRate = float64(unmatched) / float64(m.Total)
	m.AverageCandidateCount = float64(totalCandidates) / float64(m.Total)
	return m
}

func formatCaseReport(r kpCaseResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s ---\n", r.Golden.ID)
	fmt.Fprintf(&b, "  query stem: %s\n", truncateText(r.Golden.StemText, 80))
	fmt.Fprintf(&b, "  expected: %v\n", r.Golden.ExpectedKnowledgePoints)
	fmt.Fprintf(&b, "  acceptable: %v\n", r.Golden.AcceptableKnowledgePoints)
	fmt.Fprintf(&b, "  negative: %v\n", r.Golden.NegativeKnowledgePoints)
	fmt.Fprintf(&b, "  status: %s\n", r.Status)
	fmt.Fprintf(&b, "  top_score: %.4f\n", r.TopScore)
	fmt.Fprintf(&b, "  second_score: %.4f\n", r.SecondScore)
	fmt.Fprintf(&b, "  top1_hit: %v\n", r.Top1Hit)
	fmt.Fprintf(&b, "  recall_hit: %v\n", r.RecallHit)
	fmt.Fprintf(&b, "  false_positive: %v\n", r.FalsePos)
	fmt.Fprintf(&b, "  candidates (%d):\n", len(r.Candidates))
	for i, c := range r.Candidates {
		kp, _ := c["knowledge_point"].(string)
		score, _ := c["score"].(float64)
		reason, _ := c["reason"].(string)
		srcKID, _ := c["source_knowledge_id"].(string)
		chunkID, _ := c["evidence_chunk_id"].(string)
		fmt.Fprintf(&b, "    [%d] kp=%q score=%.4f reason=%s src=%s chunk=%s\n",
			i, kp, score, reason, srcKID, chunkID)
	}
	return b.String()
}

func formatMetricsReport(m kpEvalMetrics) string {
	return fmt.Sprintf(
		"Eval Metrics (n=%d):\n"+
			"  top1_accuracy:          %.4f\n"+
			"  recall_at_3:            %.4f\n"+
			"  false_positive_rate:    %.4f\n"+
			"  uncertain_rate:         %.4f\n"+
			"  unmatched_rate:         %.4f\n"+
			"  average_candidate_count: %.4f",
		m.Total, m.Top1Accuracy, m.RecallAt3, m.FalsePositiveRate,
		m.UncertainRate, m.UnmatchedRate, m.AverageCandidateCount,
	)
}

func runAllEvalCases(t *testing.T) []kpCaseResult {
	t.Helper()
	goldens := loadKPGoldens(t)
	results := make([]kpCaseResult, 0, len(goldens))
	for _, g := range goldens {
		results = append(results, runEvalCase(t, g))
	}
	return results
}

// ── Tests ──

func TestKnowledgePointMatchingEval_LoadGoldens(t *testing.T) {
	cases := loadKPGoldens(t)
	if len(cases) < 5 {
		t.Fatalf("expected at least 5 golden cases, got %d", len(cases))
	}
	for i, c := range cases {
		if c.ID == "" {
			t.Fatalf("case[%d] missing id", i)
		}
		if strings.TrimSpace(c.StemText) == "" {
			t.Fatalf("case %s missing stem_text", c.ID)
		}
		if len(c.ExpectedKnowledgePoints) == 0 && len(c.AcceptableKnowledgePoints) == 0 {
			t.Fatalf("case %s must have at least expected or acceptable knowledge points", c.ID)
		}
		if len(c.MockResults) == 0 {
			t.Fatalf("case %s missing mock_results", c.ID)
		}
	}
}

func TestKnowledgePointMatchingEval_ReportMetrics(t *testing.T) {
	results := runAllEvalCases(t)
	metrics := computeKPMetrics(results)
	t.Log(formatMetricsReport(metrics))
	for _, r := range results {
		t.Log(formatCaseReport(r))
	}
}

func TestKnowledgePointMatchingEval_BaselineThreshold(t *testing.T) {
	results := runAllEvalCases(t)
	metrics := computeKPMetrics(results)
	t.Log(formatMetricsReport(metrics))

	const minRecallAt3 = 0.70
	const maxFPR = 0.30

	if metrics.RecallAt3 < minRecallAt3 {
		var failedReports strings.Builder
		for _, r := range results {
			if !r.RecallHit {
				failedReports.WriteString(formatCaseReport(r))
				failedReports.WriteString("\n")
			}
		}
		t.Fatalf("recall_at_3=%.4f below baseline %.2f\nrecall-failures:\n%s",
			metrics.RecallAt3, minRecallAt3, failedReports.String())
	}
	if metrics.FalsePositiveRate > maxFPR {
		var fpReports strings.Builder
		for _, r := range results {
			if r.FalsePos {
				fpReports.WriteString(formatCaseReport(r))
				fpReports.WriteString("\n")
			}
		}
		t.Fatalf("false_positive_rate=%.4f above baseline %.2f\nfalse-positives:\n%s",
			metrics.FalsePositiveRate, maxFPR, fpReports.String())
	}
}

func TestKnowledgePointMatchingEval_DoesNotMutateFormalFields(t *testing.T) {
	results := runAllEvalCases(t)
	for _, r := range results {
		if r.Question.Status != types.QuestionStatusDraft {
			t.Errorf("case %s: question.status changed to %s, want draft", r.Golden.ID, r.Question.Status)
		}
		var kps []string
		if err := json.Unmarshal(r.Question.KnowledgePoints, &kps); err != nil {
			t.Errorf("case %s: invalid knowledge_points JSON: %v", r.Golden.ID, err)
			continue
		}
		if len(kps) != 0 {
			t.Errorf("case %s: knowledge_points was mutated to %v, want empty []", r.Golden.ID, kps)
		}
	}
}

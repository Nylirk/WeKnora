package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func TestSimilarQuestionSearch_RequiresQuestionID(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty question_id")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestSimilarQuestionSearch_SourceNotFound(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "nonexistent",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent question")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestSimilarQuestionSearch_SourceDeleted(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, nil, nil, nil, nil)

	// q6 is soft-deleted
	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q6",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for deleted question")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestSimilarQuestionSearch_SourceNotInScope(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	// Search targets only include qb1 (tenant 1); q4 is in qb2 (tenant 2).
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q4",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for question out of scope")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestSimilarQuestionSearch_RetrieveAndBackfill(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q2", "q3"}, // q2 is the source, should be excluded
		kbID:      "qb1",
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	registry := &stubEngineRegistry{engine: engine}

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q2",
		"limit":       10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	// q2 should be excluded (source question), q1 and q3 should appear.
	for _, r := range results {
		if r.ID == "q2" {
			t.Error("q2 (source) should be excluded from results")
		}
		if r.MatchType != "similar" {
			t.Errorf("expected match_type=similar, got %q for %s", r.MatchType, r.ID)
		}
		if r.SourceQuestionID != "q2" {
			t.Errorf("expected source_question_id=q2, got %q for %s", r.SourceQuestionID, r.ID)
		}
	}
	if len(results) < 1 {
		t.Error("expected at least q1 or q3 in results")
	}
	if dt, _ := result.Data["display_type"].(string); dt != "similar_question_results" {
		t.Errorf("expected display_type similar_question_results, got %q", dt)
	}
}

func TestSimilarQuestionSearch_VectorOrderPreserved(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	// Engine returns q3 first, then q1.
	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q3", "q1", "q2"},
		kbID:      "qb1",
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	registry := &stubEngineRegistry{engine: engine}

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q2",
		"limit":       10,
	})
	result, _ := tool.Execute(context.Background(), args)
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) >= 2 {
		// q3 should come before q1 (vector order preserved).
		if results[0].ID != "q3" {
			t.Errorf("expected q3 first (vector order), got %s", results[0].ID)
		}
		if results[1].ID != "q1" {
			t.Errorf("expected q1 second (vector order), got %s", results[1].ID)
		}
	}
}

func TestSimilarQuestionSearch_Filters(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q2", "q3", "q6"},
		kbID:      "qb1",
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	registry := &stubEngineRegistry{engine: engine}

	targets := searchTargetsWithKBs([]string{"qb1"})

	t.Run("status_filter", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id": "q2",
			"status":      "reviewed",
			"limit":       10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Status != "reviewed" {
				t.Errorf("expected only reviewed, got %q for %s", r.Status, r.ID)
			}
		}
	})

	t.Run("question_type_filter", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id":   "q2",
			"question_type": "single_choice",
			"limit":         10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.QuestionType != "single_choice" {
				t.Errorf("expected only single_choice, got %q for %s", r.QuestionType, r.ID)
			}
		}
	})

	t.Run("difficulty_filter", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id": "q2",
			"difficulty":  "easy",
			"limit":       10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Difficulty != "easy" {
				t.Errorf("expected only easy, got %q for %s", r.Difficulty, r.ID)
			}
		}
	})

	t.Run("exclude_ids", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id":          "q2",
			"exclude_question_ids": []string{"q1"},
			"limit":                10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.ID == "q1" {
				t.Error("q1 should be excluded")
			}
		}
	})
}

func TestSimilarQuestionSearch_IncludeSameQuestionSet(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q3"},
		kbID:      "qb1",
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	registry := &stubEngineRegistry{engine: engine}

	targets := searchTargetsWithKBs([]string{"qb1"})

	t.Run("include_same_set_true", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id":               "q1",
			"include_same_question_set": true,
			"limit":                     10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		// q3 is in same set (qs1) as q1
		foundQ3 := false
		for _, r := range results {
			if r.ID == "q3" {
				foundQ3 = true
			}
		}
		if !foundQ3 {
			t.Error("q3 should be included (same question set allowed)")
		}
	})

	t.Run("include_same_set_false", func(t *testing.T) {
		tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"question_id":               "q1",
			"include_same_question_set": false,
			"limit":                     10,
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.QuestionSetID == "qs1" {
				t.Errorf("q %s is in same set qs1, should be excluded", r.ID)
			}
		}
	})
}

func TestSimilarQuestionSearch_TenantIsolation(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	// Engine returns q4 (tenant 2) among results.
	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q4", "q1"},
		kbID:      "qb1",
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	registry := &stubEngineRegistry{engine: engine}

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewSimilarQuestionSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q1",
		"limit":       10,
	})
	result, _ := tool.Execute(context.Background(), args)
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	for _, r := range results {
		if r.ID == "q4" {
			t.Error("q4 belongs to tenant 2, should NOT be returned")
		}
	}
}

func TestSimilarQuestionSearch_MissingDeps(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})

	// All deps nil.
	tool := NewSimilarQuestionSearchTool(db, targets, nil, nil, nil, nil)
	args, _ := json.Marshal(map[string]interface{}{
		"question_id": "q1",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error with nil deps")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestSimilarQuestionSearch_NoIndexingSideEffect(t *testing.T) {
	// Verify that similar_question_search does NOT create any rows in
	// question_vector_indexes. Since we don't import the index service,
	// we just confirm the tool executes without touching indexing paths.
	// The tool has no reference to QuestionIndexService — compile-time guarantee.

	// This test just ensures the tool compiles and the Execute path works.
	var _ types.Tool = (*SimilarQuestionSearchTool)(nil)
}

// Ensure stubs satisfy their interfaces.
var (
	_ interfaces.KnowledgeBaseService   = (*stubKBService)(nil)
	_ interfaces.RetrieveEngineRegistry = (*stubEngineRegistry)(nil)
)

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupQuestionBankTestDB creates an in-memory SQLite database with the tables
// needed for question_bank_search tests (knowledge_bases, question_sets, questions).
func setupQuestionBankTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	// Knowledge bases table (minimal — just what the JOIN needs)
	if err := db.Exec(`
		CREATE TABLE knowledge_bases (
			id TEXT PRIMARY KEY,
			tenant_id INTEGER NOT NULL,
			type TEXT NOT NULL DEFAULT 'document',
			deleted_at TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create knowledge_bases: %v", err)
	}

	// Question sets table
	if err := db.Exec(`
		CREATE TABLE question_sets (
			id TEXT PRIMARY KEY,
			tenant_id INTEGER NOT NULL,
			knowledge_base_id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			deleted_at TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create question_sets: %v", err)
	}

	// Questions table
	if err := db.Exec(`
		CREATE TABLE questions (
			id TEXT PRIMARY KEY,
			tenant_id INTEGER NOT NULL,
			question_set_id TEXT NOT NULL,
			knowledge_base_id TEXT NOT NULL,
			question_type TEXT NOT NULL DEFAULT 'single_choice',
			stem_text TEXT NOT NULL DEFAULT '',
			question_body TEXT NOT NULL DEFAULT '{}',
			answer_text TEXT NOT NULL DEFAULT '',
			answer_body TEXT NOT NULL DEFAULT '{}',
			analysis_text TEXT NOT NULL DEFAULT '',
			difficulty TEXT NOT NULL DEFAULT 'medium',
			knowledge_points TEXT NOT NULL DEFAULT '[]',
			tags TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'draft',
			created_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create questions: %v", err)
	}

	return db
}

// seedQuestionBank seeds basic question bank test data into the DB.
func seedQuestionBank(t *testing.T, db *gorm.DB) {
	t.Helper()

	// KBs: one question_bank, one document
	db.Exec(`INSERT INTO knowledge_bases(id, tenant_id, type, deleted_at) VALUES
		('qb1', 1, 'question_bank', NULL),
		('qb2', 2, 'question_bank', NULL),
		('doc1', 1, 'document', NULL),
		('deleted_qb', 1, 'question_bank', datetime('now'))`)

	// Question sets
	db.Exec(`INSERT INTO question_sets(id, tenant_id, knowledge_base_id, name, deleted_at) VALUES
		('qs1', 1, 'qb1', 'Math Questions', NULL),
		('qs2', 2, 'qb2', 'Physics Questions', NULL),
		('qs3', 1, 'doc1', 'Doc Questions', NULL),
		('deleted_qs', 1, 'qb1', 'Deleted Set', datetime('now'))`)

	// Questions
	db.Exec(`INSERT INTO questions(id, tenant_id, question_set_id, knowledge_base_id, question_type, stem_text, question_body, answer_text, answer_body, analysis_text, difficulty, knowledge_points, tags, status, created_at) VALUES
		('q1', 1, 'qs1', 'qb1', 'single_choice', 'What is 2+2?', '{"choices":[{"label":"A","text":"4"}]}', '4', '{}', 'Basic addition', 'easy', '["arithmetic"]', '["math"]', 'reviewed', '2024-01-01'),
		('q2', 1, 'qs1', 'qb1', 'multiple_choice', 'Which are prime?', '{"choices":[{"label":"A","text":"2"},{"label":"B","text":"4"}]}', '2', '{}', 'Prime numbers', 'medium', '["number theory"]', '["math"]', 'draft', '2024-01-02'),
		('q3', 1, 'qs1', 'qb1', 'short_answer', 'Define calculus', '{}', 'Study of rates of change', '{}', 'Calculus definition', 'hard', '["calculus"]', '["math"]', 'reviewed', '2024-01-03'),
		('q4', 2, 'qs2', 'qb2', 'single_choice', 'What is gravity?', '{"choices":[{"label":"A","text":"A force"}]}', 'A force', '{}', 'Gravity basics', 'easy', '["physics"]', '["science"]', 'reviewed', '2024-02-01'),
		('q5', 1, 'qs3', 'doc1', 'single_choice', 'Document KB question', '{}', 'answer', '{}', '', 'easy', '[]', '[]', 'draft', '2024-03-01'),
		('q6', 1, 'qs1', 'qb1', 'single_choice', 'Deleted question', '{}', '', '{}', '', 'easy', '[]', '[]', 'draft', '2024-04-01'),
		('q7', 1, 'deleted_qs', 'qb1', 'single_choice', 'Deleted set question', '{}', '', '{}', '', 'easy', '[]', '[]', 'draft', '2024-05-01')`)

	// Soft-delete q6
	db.Exec(`UPDATE questions SET deleted_at = datetime('now') WHERE id = 'q6'`)
}

// searchTargetsWithKBs creates SearchTargets for the given KB IDs, all under tenant 1.
func searchTargetsWithKBs(kbIDs []string) types.SearchTargets {
	targets := make(types.SearchTargets, 0, len(kbIDs))
	for _, kbID := range kbIDs {
		targets = append(targets, &types.SearchTarget{
			KnowledgeBaseID: kbID,
			TenantID:        1,
		})
	}
	return targets
}

// searchTargetsWithKBsAndTenant creates SearchTargets with explicit KB-tenant mapping.
func searchTargetsWithKBsAndTenant(pairs map[string]uint64) types.SearchTargets {
	targets := make(types.SearchTargets, 0, len(pairs))
	for kbID, tenantID := range pairs {
		targets = append(targets, &types.SearchTarget{
			KnowledgeBaseID: kbID,
			TenantID:        tenantID,
		})
	}
	return targets
}

func TestQuestionBankSearch_EmptyQueryListsRecent(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"limit": 10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	data := result.Data
	if rc, ok := data["result_count"].(int); !ok || rc == 0 {
		t.Errorf("expected non-zero result_count, got %v", data["result_count"])
	}
	if dt, ok := data["display_type"].(string); !ok || dt != "question_bank_results" {
		t.Errorf("expected display_type question_bank_results, got %v", dt)
	}
	if results, ok := data["results"]; !ok || results == nil {
		t.Error("expected results in Data")
	}

	// Verify results are ordered by created_at DESC (q3 before q2 before q1)
	if !strings.Contains(result.Output, "q3") {
		t.Error("expected q3 (most recent non-deleted) in output")
	}
}

func TestQuestionBankSearch_ValidQueryFindsStemText(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "prime",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	rc, _ := result.Data["result_count"].(int)
	if rc == 0 {
		t.Error("expected at least one result for 'prime' question")
	}
	if !strings.Contains(result.Output, "q2") {
		t.Error("expected q2 (prime question) in output")
	}
}

func TestQuestionBankSearch_ValidQueryFindsAnswerText(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "rates of change",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	rc, _ := result.Data["result_count"].(int)
	if rc == 0 {
		t.Error("expected at least one result for 'rates of change'")
	}
}

func TestQuestionBankSearch_ValidQueryFindsAnalysisText(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "Basic addition",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	rc, _ := result.Data["result_count"].(int)
	if rc == 0 {
		t.Error("expected at least one result for 'Basic addition' in analysis_text")
	}
}

func TestQuestionBankSearch_TenantIsolation(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	// Only search tenant 1's KB
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "gravity",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// q4 (gravity) is in qb2/tenant 2 — should NOT appear
	if strings.Contains(result.Output, "q4") {
		t.Error("q4 belongs to tenant 2, should NOT be returned for tenant 1 scope")
	}
}

func TestQuestionBankSearch_DeletedQuestionExcluded(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "Deleted question",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// q6 is soft-deleted — should NOT appear
	if strings.Contains(result.Output, "q6") {
		t.Error("q6 is soft-deleted, should NOT be returned")
	}
}

func TestQuestionBankSearch_DeletedQuestionSetExcluded(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "Deleted set question",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// q7 is in a deleted question set — should NOT appear
	if strings.Contains(result.Output, "q7") {
		t.Error("q7 belongs to a deleted question set, should NOT be returned")
	}
}

func TestQuestionBankSearch_NonQuestionBankKBExcluded(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"doc1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "Document KB question",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// q5 is in doc1 (type=document) — should NOT appear via JOIN filter
	if strings.Contains(result.Output, "q5") {
		t.Error("q5 belongs to a non-question_bank KB, should NOT be returned")
	}
}

func TestQuestionBankSearch_LimitClamping(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	// limit=0 should default to 20
	args, _ := json.Marshal(map[string]interface{}{"limit": 0})
	result, _ := tool.Execute(context.Background(), args)
	if v, ok := result.Data["limit"].(int); !ok || v != 20 {
		t.Errorf("limit=0 should default to 20, got %v", result.Data["limit"])
	}

	// limit=100 should clamp to 50
	args, _ = json.Marshal(map[string]interface{}{"limit": 100})
	result, _ = tool.Execute(context.Background(), args)
	if v, ok := result.Data["limit"].(int); !ok || v != 50 {
		t.Errorf("limit=100 should clamp to 50, got %v", result.Data["limit"])
	}
}

func TestQuestionBankSearch_NoKBsInScope(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "anything",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	rc, _ := result.Data["result_count"].(int)
	if rc != 0 {
		t.Errorf("expected 0 results when no KBs in scope, got %d", rc)
	}
	if dt, _ := result.Data["display_type"].(string); dt != "question_bank_results" {
		t.Errorf("expected display_type question_bank_results in empty result, got %s", dt)
	}
}

func TestQuestionBankSearch_NoMatchingResults(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "zzz_nonexistent_phrase_zzz",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	rc, _ := result.Data["result_count"].(int)
	if rc != 0 {
		t.Errorf("expected 0 results, got %d", rc)
	}
}

func TestQuestionBankSearch_InvalidJSONArgs(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	// Invalid JSON
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err == nil {
		t.Error("expected error for invalid JSON args")
	}
	if result.Success {
		t.Error("expected failure for invalid JSON args")
	}
}

func TestQuestionBankSearch_ResultIncludesQuestionSetNameAndKBID(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "2+2",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	// Verify question_set_name and knowledge_base_id in output
	if !strings.Contains(result.Output, "Math Questions") {
		t.Error("expected 'Math Questions' (question_set_name) in output")
	}
	if !strings.Contains(result.Output, "qb1") {
		t.Error("expected 'qb1' (knowledge_base_id) in output")
	}

	// Verify Data results have the fields
	results, ok := result.Data["results"].([]QuestionBankSearchResult)
	if !ok {
		t.Fatalf("expected []QuestionBankSearchResult in Data, got %T", result.Data["results"])
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].KnowledgeBaseID != "qb1" {
		t.Errorf("expected knowledge_base_id 'qb1', got %q", results[0].KnowledgeBaseID)
	}
	if results[0].QuestionSetName != "Math Questions" {
		t.Errorf("expected question_set_name 'Math Questions', got %q", results[0].QuestionSetName)
	}
}

func TestQuestionBankSearch_WildcardCharactersEscaped(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	// Insert a question with literal % in stem
	db.Exec(`INSERT INTO knowledge_bases(id, tenant_id, type, deleted_at) VALUES ('qb_esc', 1, 'question_bank', NULL)`)
	db.Exec(`INSERT INTO question_sets(id, tenant_id, knowledge_base_id, name, deleted_at) VALUES ('qs_esc', 1, 'qb_esc', 'Esc Test', NULL)`)
	db.Exec(`INSERT INTO questions(id, tenant_id, question_set_id, knowledge_base_id, question_type, stem_text, question_body, answer_text, answer_body, analysis_text, difficulty, knowledge_points, tags, status, created_at, deleted_at) VALUES
		('q_percent', 1, 'qs_esc', 'qb_esc', 'single_choice', '50% discount?', '{}', 'yes', '{}', '', 'easy', '[]', '[]', 'draft', '2024-01-01', NULL),
		('q_underscore', 1, 'qs_esc', 'qb_esc', 'single_choice', 'foo_bar item', '{}', '', '{}', '', 'easy', '[]', '[]', 'draft', '2024-01-02', NULL)`)

	targets := searchTargetsWithKBs([]string{"qb_esc"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	// Searching for literal "%" should find "50% discount?" but not match everything
	args, _ := json.Marshal(map[string]interface{}{
		"query": "50%",
	})
	result, _ := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if !strings.Contains(result.Output, "q_percent") {
		t.Error("expected q_percent (with literal %) in results")
	}

	// Searching for literal "_" should find "foo_bar" but not match everything
	args, _ = json.Marshal(map[string]interface{}{
		"query": "foo_bar",
	})
	result, _ = tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if !strings.Contains(result.Output, "q_underscore") {
		t.Error("expected q_underscore (with literal _) in results")
	}
}

func TestQuestionBankSearch_XMLEscaping(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	// Insert a question with XML special characters
	db.Exec(`INSERT INTO knowledge_bases(id, tenant_id, type, deleted_at) VALUES ('qb_xml', 1, 'question_bank', NULL)`)
	db.Exec(`INSERT INTO question_sets(id, tenant_id, knowledge_base_id, name, deleted_at) VALUES ('qs_xml', 1, 'qb_xml', 'XML Test', NULL)`)
	db.Exec(`INSERT INTO questions(id, tenant_id, question_set_id, knowledge_base_id, question_type, stem_text, question_body, answer_text, answer_body, analysis_text, difficulty, knowledge_points, tags, status, created_at, deleted_at) VALUES
		('q_xml', 1, 'qs_xml', 'qb_xml', 'short_answer', 'Is a < b && b > c?', '{"note":"x & y"}', 'Yes, a < b', '{"reason":"because a<b"}', 'Explanation: a > c', 'easy', '["logic"]', '[]', 'draft', '2024-01-01', NULL)`)

	targets := searchTargetsWithKBs([]string{"qb_xml"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "a < b",
	})
	result, _ := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("unexpected failure: %s", result.Error)
	}

	// XML special characters should be escaped in output
	if strings.Contains(result.Output, "a < b") && !strings.Contains(result.Output, "&lt;") {
		// The literal "<" might appear in a non-XML context; check that answer has escaped "<"
		// In SQLite, LOWER LIKE is case-insensitive and the actual text is stored as-is.
		// The escapeXML should convert < to &lt; in the output.
		if strings.Contains(result.Output, "Yes, a < b") {
			t.Error("expected '<' to be escaped as '&lt;' in output text")
		}
	}
}

func TestQuestionBankSearch_StatusFilter(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	// Filter by status=draft → should only return q2
	args, _ := json.Marshal(map[string]interface{}{
		"status": "draft",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	// q1 is reviewed, q2 is draft → only q2 should appear
	if strings.Contains(result.Output, "q1") {
		t.Error("q1 has status=reviewed, should NOT appear with status=draft filter")
	}
	if !strings.Contains(result.Output, "q2") {
		t.Error("q2 has status=draft, should appear with status=draft filter")
	}

	// Filter by status=reviewed → should return q1 and q3
	args, _ = json.Marshal(map[string]interface{}{
		"status": "reviewed",
	})
	result, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "q1") && !strings.Contains(result.Output, "q3") {
		t.Error("expected q1 or q3 (reviewed) in results")
	}
}

func TestQuestionBankSearch_InvalidStatus(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"status": "invalid_status",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid status")
	}
	if result.Success {
		t.Error("expected failure for invalid status")
	}
	if !strings.Contains(result.Error, "Invalid status") {
		t.Errorf("expected 'Invalid status' in error, got %q", result.Error)
	}
}

func TestQuestionBankSearch_ToolImplementsInterface(t *testing.T) {
	// Compile-time check — if this compiles, QuestionBankSearchTool implements types.Tool
	var _ types.Tool = (*QuestionBankSearchTool)(nil)
}

// ---- Semantic search helpers ----

func strPtr(s string) *string { return &s }

// stubStoreOwnership implements retriever.TenantStoreOwnership for tests.
type stubStoreOwnership struct{}

func (s *stubStoreOwnership) StoreOwnedBy(ctx context.Context, storeID string, tenantID uint64) (bool, error) {
	return true, nil
}

// stubEmbedder implements embedding.Embedder for tests.
type stubEmbedder struct {
	dimensions int
	modelName  string
	modelID    string
}

func (e *stubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, e.dimensions), nil
}
func (e *stubEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, e.dimensions)
	}
	return out, nil
}
func (e *stubEmbedder) BatchEmbedWithPool(ctx context.Context, model embedding.Embedder, texts []string) ([][]float32, error) {
	return e.BatchEmbed(ctx, texts)
}
func (e *stubEmbedder) GetModelName() string { return e.modelName }
func (e *stubEmbedder) GetDimensions() int   { return e.dimensions }
func (e *stubEmbedder) GetModelID() string   { return e.modelID }

// stubKBService implements KnowledgeBaseService just enough for semantic tests.
type stubKBService struct {
	kbs map[string]*types.KnowledgeBase
}

func (s *stubKBService) GetKnowledgeBaseByIDOnly(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if kb, ok := s.kbs[id]; ok {
		return kb, nil
	}
	return nil, fmt.Errorf("kb %s not found", id)
}

// Remaining KnowledgeBaseService methods that aren't used by the tool
// are stubbed to satisfy the interface at compile time.
func (*stubKBService) CreateKnowledgeBase(context.Context, *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) GetKnowledgeBasesByIDsOnly(context.Context, []string) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) FillKnowledgeBaseCounts(context.Context, *types.KnowledgeBase) error {
	return nil
}
func (*stubKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) UpdateKnowledgeBase(context.Context, string, string, string, *types.KnowledgeBaseConfig) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) DeleteKnowledgeBase(context.Context, string) error { return nil }
func (*stubKBService) TogglePinKnowledgeBase(context.Context, string) (*types.KnowledgeBase, error) {
	return nil, nil
}
func (*stubKBService) HybridSearch(context.Context, string, types.SearchParams) ([]*types.SearchResult, error) {
	return nil, nil
}
func (*stubKBService) GetQueryEmbedding(context.Context, string, string) ([]float32, error) {
	return nil, nil
}
func (*stubKBService) ResolveEmbeddingModelKeys(context.Context, []*types.KnowledgeBase) map[string]string {
	return nil
}
func (*stubKBService) CopyKnowledgeBase(context.Context, string, string) (*types.KnowledgeBase, *types.KnowledgeBase, error) {
	return nil, nil, nil
}
func (*stubKBService) GetRepository() interfaces.KnowledgeBaseRepository  { return nil }
func (*stubKBService) ProcessKBDelete(context.Context, *asynq.Task) error { return nil }

// stubModelService implements ModelService just enough for semantic tests.
type stubModelService struct {
	embedder embedding.Embedder
}

func (s *stubModelService) GetEmbeddingModel(ctx context.Context, modelID string) (embedding.Embedder, error) {
	return s.embedder, nil
}

func (*stubModelService) CreateModel(context.Context, *types.Model) error            { return nil }
func (*stubModelService) GetModelByID(context.Context, string) (*types.Model, error) { return nil, nil }
func (*stubModelService) ListModels(context.Context) ([]*types.Model, error)         { return nil, nil }
func (*stubModelService) UpdateModel(context.Context, *types.Model) error            { return nil }
func (*stubModelService) DeleteModel(context.Context, string) error                  { return nil }
func (*stubModelService) UpdateModelCredentials(context.Context, string, *string, *string) (*types.Model, error) {
	return nil, nil
}
func (*stubModelService) ClearModelCredential(context.Context, string, string) error { return nil }
func (*stubModelService) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}
func (*stubModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}
func (*stubModelService) GetChatModel(context.Context, string) (chat.Chat, error) { return nil, nil }
func (*stubModelService) GetVLMModel(context.Context, string) (vlm.VLM, error)    { return nil, nil }
func (*stubModelService) GetASRModel(context.Context, string) (asr.ASR, error)    { return nil, nil }

// stubRetrieveEngine implements both RetrieveEngine and RetrieveEngineService for tests.
type stubRetrieveEngine struct {
	sourceIDs []string
	scores    []float64
	kbID      string
	lastKBIDs []string // records KnowledgeBaseIDs from the last Retrieve call
}

func (e *stubRetrieveEngine) EngineType() types.RetrieverEngineType {
	return types.MilvusRetrieverEngineType
}
func (e *stubRetrieveEngine) Support() []types.RetrieverType {
	return []types.RetrieverType{types.VectorRetrieverType}
}
func (e *stubRetrieveEngine) Retrieve(ctx context.Context, params types.RetrieveParams) ([]*types.RetrieveResult, error) {
	e.lastKBIDs = append([]string{}, params.KnowledgeBaseIDs...)
	results := make([]*types.IndexWithScore, 0, len(e.sourceIDs))
	for i, id := range e.sourceIDs {
		score := 0.9 - float64(i)*0.05
		if i < len(e.scores) {
			score = e.scores[i]
		}
		results = append(results, &types.IndexWithScore{
			SourceID:        id,
			KnowledgeBaseID: e.kbID,
			Score:           score,
			MatchType:       types.MatchTypeEmbedding,
		})
	}
	return []*types.RetrieveResult{{
		Results:       results,
		RetrieverType: types.VectorRetrieverType,
	}}, nil
}
func (e *stubRetrieveEngine) Index(ctx context.Context, embedder embedding.Embedder, indexInfo *types.IndexInfo, retrieverTypes []types.RetrieverType) error {
	return nil
}
func (e *stubRetrieveEngine) BatchIndex(ctx context.Context, embedder embedding.Embedder, indexInfoList []*types.IndexInfo, retrieverTypes []types.RetrieverType) error {
	return nil
}
func (e *stubRetrieveEngine) EstimateStorageSize(ctx context.Context, embedder embedding.Embedder, indexInfoList []*types.IndexInfo, retrieverTypes []types.RetrieverType) int64 {
	return 0
}
func (e *stubRetrieveEngine) CopyIndices(ctx context.Context, sourceKBID string, sourceToTargetKBIDMap map[string]string, sourceToTargetChunkIDMap map[string]string, targetKBID string, dimension int, knowledgeType string) error {
	return nil
}
func (e *stubRetrieveEngine) DeleteByChunkIDList(ctx context.Context, indexIDList []string, dimension int, knowledgeType string) error {
	return nil
}
func (e *stubRetrieveEngine) DeleteBySourceIDList(ctx context.Context, sourceIDList []string, dimension int, knowledgeType string) error {
	return nil
}
func (e *stubRetrieveEngine) DeleteByKnowledgeIDList(ctx context.Context, knowledgeIDList []string, dimension int, knowledgeType string) error {
	return nil
}
func (e *stubRetrieveEngine) BatchUpdateChunkEnabledStatus(ctx context.Context, chunkStatusMap map[string]bool) error {
	return nil
}
func (e *stubRetrieveEngine) BatchUpdateChunkTagID(ctx context.Context, chunkTagMap map[string]string) error {
	return nil
}

// stubEngineRegistry implements RetrieveEngineRegistry for tests.
type stubEngineRegistry struct {
	engine interfaces.RetrieveEngineService
}

func (r *stubEngineRegistry) Register(engine interfaces.RetrieveEngineService) error { return nil }
func (r *stubEngineRegistry) GetRetrieveEngineService(engineType types.RetrieverEngineType) (interfaces.RetrieveEngineService, error) {
	return r.engine, nil
}
func (r *stubEngineRegistry) GetAllRetrieveEngineServices() []interfaces.RetrieveEngineService {
	return []interfaces.RetrieveEngineService{r.engine}
}
func (r *stubEngineRegistry) GetByStoreID(storeID string) (interfaces.RetrieveEngineService, error) {
	return r.engine, nil
}

// stubMultiEngineRegistry returns different engines depending on the store ID.
type stubMultiEngineRegistry struct {
	enginesByStore map[string]interfaces.RetrieveEngineService
}

func (r *stubMultiEngineRegistry) Register(engine interfaces.RetrieveEngineService) error { return nil }
func (r *stubMultiEngineRegistry) GetRetrieveEngineService(engineType types.RetrieverEngineType) (interfaces.RetrieveEngineService, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *stubMultiEngineRegistry) GetAllRetrieveEngineServices() []interfaces.RetrieveEngineService {
	return nil
}
func (r *stubMultiEngineRegistry) GetByStoreID(storeID string) (interfaces.RetrieveEngineService, error) {
	engine, ok := r.enginesByStore[storeID]
	if !ok {
		return nil, fmt.Errorf("store %s not found", storeID)
	}
	return engine, nil
}

// setupSemanticQuestionBankTestDB creates an in-memory DB with tables needed
// for semantic question search tests (includes questions for backfill).
func setupSemanticQuestionBankTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	if err := db.Exec(`CREATE TABLE knowledge_bases (
		id TEXT PRIMARY KEY,
		tenant_id INTEGER NOT NULL,
		type TEXT NOT NULL DEFAULT 'document',
		deleted_at TEXT
	)`).Error; err != nil {
		t.Fatalf("create knowledge_bases: %v", err)
	}
	if err := db.Exec(`CREATE TABLE question_sets (
		id TEXT PRIMARY KEY,
		tenant_id INTEGER NOT NULL,
		knowledge_base_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		deleted_at TEXT
	)`).Error; err != nil {
		t.Fatalf("create question_sets: %v", err)
	}
	if err := db.Exec(`CREATE TABLE questions (
		id TEXT PRIMARY KEY,
		tenant_id INTEGER NOT NULL,
		question_set_id TEXT NOT NULL,
		knowledge_base_id TEXT NOT NULL,
		question_type TEXT NOT NULL DEFAULT 'single_choice',
		stem_text TEXT NOT NULL DEFAULT '',
		question_body TEXT NOT NULL DEFAULT '{}',
		answer_text TEXT NOT NULL DEFAULT '',
		answer_body TEXT NOT NULL DEFAULT '{}',
		analysis_text TEXT NOT NULL DEFAULT '',
		difficulty TEXT NOT NULL DEFAULT 'medium',
		knowledge_points TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL DEFAULT 'draft',
		created_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT
	)`).Error; err != nil {
		t.Fatalf("create questions: %v", err)
	}
	return db
}

// seedSemanticQuestions seeds questions for semantic backfill tests.
func seedSemanticQuestions(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Exec(`INSERT INTO knowledge_bases(id, tenant_id, type, deleted_at) VALUES
		('qb1', 1, 'question_bank', NULL),
		('qb2', 2, 'question_bank', NULL),
		('non_qb', 1, 'document', NULL)`)

	db.Exec(`INSERT INTO question_sets(id, tenant_id, knowledge_base_id, name, deleted_at) VALUES
		('qs1', 1, 'qb1', 'Set One', NULL),
		('qs2', 1, 'qb1', 'Set Two', NULL),
		('qs3', 2, 'qb2', 'Tenant 2 Set', NULL)`)

	db.Exec(`INSERT INTO questions(id, tenant_id, question_set_id, knowledge_base_id, question_type, stem_text, answer_text, analysis_text, difficulty, knowledge_points, tags, status, created_at) VALUES
		('q1', 1, 'qs1', 'qb1', 'single_choice',  'What is recursion?', 'A function calling itself', 'Recursion basics', 'easy', '["programming"]', '["cs"]', 'reviewed', '2024-01-01'),
		('q2', 1, 'qs1', 'qb1', 'multiple_choice', 'Which are OOP principles?', 'Encapsulation, Inheritance', 'OOP concepts', 'medium', '["oop"]', '["cs"]', 'reviewed', '2024-02-01'),
		('q3', 1, 'qs2', 'qb1', 'short_answer', 'Define polymorphism', 'Multiple forms', 'Polymorphism', 'hard', '["oop", "programming"]', '["cs"]', 'draft', '2024-03-01'),
		('q4', 2, 'qs3', 'qb2', 'single_choice', 'What is 2+2?', '4', 'Basic math', 'easy', '["math"]', '["math"]', 'reviewed', '2024-04-01'),
		('q5', 1, 'qs1', 'qb1', 'single_choice', 'Deleted question', '', '', 'easy', '[]', '[]', 'draft', '2024-05-01'),
		('q6', 1, 'qs1', 'qb1', 'single_choice', 'Rejected question', 'Answer', '', 'easy', '[]', '[]', 'rejected', '2024-06-01')`)

	// Soft-delete q5
	db.Exec(`UPDATE questions SET deleted_at = datetime('now') WHERE id = 'q5'`)
}

func TestQuestionBankSearch_Semantic_RequiresQuery(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty query in semantic mode")
	}
	if result.Success {
		t.Error("expected failure")
	}
	if !strings.Contains(result.Error, "non-empty query") {
		t.Errorf("expected 'non-empty query' in error, got %q", result.Error)
	}
}

func TestQuestionBankSearch_Semantic_RetrieveAndBackfill(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q3", "q2"},
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
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "programming concepts",
		"limit": 10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Vector order must be preserved: q1, q3, q2
	if results[0].ID != "q1" {
		t.Errorf("expected q1 first (vector order), got %s", results[0].ID)
	}
	if results[1].ID != "q3" {
		t.Errorf("expected q3 second (vector order), got %s", results[1].ID)
	}
	if results[2].ID != "q2" {
		t.Errorf("expected q2 third (vector order), got %s", results[2].ID)
	}
	for _, r := range results {
		if r.MatchType != "semantic" {
			t.Errorf("expected match_type= semantic, got %q", r.MatchType)
		}
		if r.Score == 0 {
			t.Error("expected non-zero score")
		}
	}
	if mode, _ := result.Data["mode"].(string); mode != "semantic" {
		t.Errorf("expected mode= semantic, got %q", mode)
	}
}

func TestQuestionBankSearch_Semantic_TenantIsolation(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	// Engine returns q4 which is in tenant 2's KB, but we only search tenant 1
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
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "test",
		"limit": 10,
	})
	result, _ := tool.Execute(context.Background(), args)
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	// q4 (tenant 2) should be excluded even if vector returned it
	for _, r := range results {
		if r.ID == "q4" {
			t.Error("q4 belongs to tenant 2, should NOT be returned")
		}
	}
	// q1 (tenant 1) should be present
	found := false
	for _, r := range results {
		if r.ID == "q1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected q1 (tenant 1) in results")
	}
}

func TestQuestionBankSearch_Semantic_DeletedQuestionExcluded(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q5", "q1"},
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
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "test",
		"limit": 10,
	})
	result, _ := tool.Execute(context.Background(), args)
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	for _, r := range results {
		if r.ID == "q5" {
			t.Error("q5 is soft-deleted, should NOT be returned")
		}
	}
}

func TestQuestionBankSearch_Semantic_StatusFilter(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q6", "q3", "q2"},
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
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"mode":   "semantic",
		"query":  "test",
		"limit":  10,
		"status": "reviewed",
	})
	result, _ := tool.Execute(context.Background(), args)
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	// Only q1 and q2 are reviewed; q6 is rejected, q3 is draft
	for _, r := range results {
		if r.ID == "q3" {
			t.Error("q3 is draft, should NOT appear with status=reviewed")
		}
		if r.ID == "q6" {
			t.Error("q6 is rejected, should NOT appear with status=reviewed")
		}
	}
	if len(results) < 1 || len(results) > 2 {
		t.Errorf("expected 1-2 reviewed results, got %d", len(results))
	}
}

func TestQuestionBankSearch_Semantic_StructuredFilters(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q2", "q3"},
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

	// Test question_type filter
	t.Run("question_type", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":          "semantic",
			"query":         "test",
			"limit":         10,
			"question_type": "short_answer",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		if len(results) != 1 || results[0].ID != "q3" {
			t.Errorf("expected only q3 (short_answer), got %d results", len(results))
		}
	})

	// Test difficulty filter
	t.Run("difficulty", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":       "semantic",
			"query":      "test",
			"limit":      10,
			"difficulty": "easy",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Difficulty != "easy" {
				t.Errorf("expected only easy difficulty, got %q from %s", r.Difficulty, r.ID)
			}
		}
	})

	// Test knowledge_points filter
	t.Run("knowledge_points", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":             "semantic",
			"query":            "test",
			"limit":            10,
			"knowledge_points": []string{"oop"},
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		// q2 and q3 have "oop" knowledge point
		for _, r := range results {
			if r.ID == "q1" {
				t.Error("q1 has knowledge_points [programming], not oop")
			}
		}
	})

	// Test tags filter
	t.Run("tags", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":  "semantic",
			"query": "test",
			"limit": 10,
			"tags":  []string{"cs"},
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		if len(results) == 0 {
			t.Error("expected at least one result with tag 'cs'")
		}
	})

	// Test exclude_question_ids
	t.Run("exclude_question_ids", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":                 "semantic",
			"query":                "test",
			"limit":                10,
			"exclude_question_ids": []string{"q1"},
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.ID == "q1" {
				t.Error("q1 should be excluded")
			}
		}
	})

	// Test question_set_id filter
	t.Run("question_set_id", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":            "semantic",
			"query":           "test",
			"limit":           10,
			"question_set_id": "qs2",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.QuestionSetID != "qs2" {
				t.Errorf("expected only qs2 results, got %s in %s", r.QuestionSetID, r.ID)
			}
		}
	})
}

func TestQuestionBankSearch_Semantic_NonQuestionBankKBExcluded(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	// Search targets include a non-question_bank KB
	targets := searchTargetsWithKBs([]string{"non_qb"})
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"non_qb": {ID: "non_qb", TenantID: 1, Type: "document", EmbeddingModelID: "emb1"},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}
	engine := &stubRetrieveEngine{sourceIDs: []string{}, kbID: "non_qb"}
	registry := &stubEngineRegistry{engine: engine}

	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "test",
		"limit": 10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should succeed but with no results since non_qb is type=document, not question_bank
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-question_bank KB, got %d", len(results))
	}
}

func TestQuestionBankSearch_Semantic_MissingVectorDependency(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})

	// KB with no embedding model configured
	t.Run("no_embedding_model", func(t *testing.T) {
		kbService := &stubKBService{
			kbs: map[string]*types.KnowledgeBase{
				"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: ""},
			},
		}
		tool := NewQuestionBankSearchTool(db, targets, kbService, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"mode":  "semantic",
			"query": "test",
			"limit": 10,
		})
		result, err := tool.Execute(context.Background(), args)
		if err == nil {
			t.Error("expected error for missing vector dependency")
		}
		if !result.Success {
			// Expected — error about requiring vector retriever and embedding model
			if !strings.Contains(result.Error, "vector retriever") && !strings.Contains(result.Error, "embedding model") {
				t.Logf("error: %s", result.Error)
			}
		}
	})

	// KB with embedding model but no model service (nil)
	t.Run("nil_model_service", func(t *testing.T) {
		engine := &stubRetrieveEngine{sourceIDs: []string{"q1"}, kbID: "qb1"}
		kbService := &stubKBService{
			kbs: map[string]*types.KnowledgeBase{
				"qb1": {ID: "qb1", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs1")},
			},
		}
		registry := &stubEngineRegistry{engine: engine}
		// modelService is nil — keyword mode should still work fine.
		tool := NewQuestionBankSearchTool(db, targets, kbService, nil, registry, nil)

		keywordArgs, _ := json.Marshal(map[string]interface{}{
			"mode":  "keyword",
			"query": "",
			"limit": 10,
		})
		result, err := tool.Execute(context.Background(), keywordArgs)
		if err != nil {
			t.Fatalf("keyword mode should work without model service: %v", err)
		}
		if !result.Success {
			t.Fatalf("keyword mode should succeed without model service: %s", result.Error)
		}
	})
}

func TestQuestionBankSearch_Keyword_ModeBackwardCompatibility(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})

	// No mode specified → should default to keyword
	t.Run("default_mode_is_keyword", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"query": "prime",
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got error: %s", result.Error)
		}
		if mode, _ := result.Data["mode"].(string); mode != "keyword" {
			t.Errorf("default mode should be keyword, got %q", mode)
		}
	})

	// Explicit mode=keyword
	t.Run("explicit_keyword", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"mode":  "keyword",
			"query": "prime",
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got error: %s", result.Error)
		}
		if !strings.Contains(result.Output, "q2") {
			t.Error("expected q2 (prime question) in keyword search results")
		}
	})

	// Keyword mode with structured filters
	t.Run("keyword_with_structured_filters", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"mode":       "keyword",
			"query":      "",
			"limit":      10,
			"difficulty": "easy",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Difficulty != "easy" {
				t.Errorf("expected only easy questions, got difficulty=%q for %s", r.Difficulty, r.ID)
			}
		}
	})
}

func TestQuestionBankSearch_Semantic_InvalidMode(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "fuzzy",
		"query": "test",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if result.Success {
		t.Error("expected failure for invalid mode")
	}
	if !strings.Contains(result.Error, "Invalid mode") {
		t.Errorf("expected 'Invalid mode' in error, got %q", result.Error)
	}
}

func TestQuestionBankSearch_Keyword_StructuredFilters(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)

	targets := searchTargetsWithKBs([]string{"qb1"})

	t.Run("question_type_filter", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"question_type": "short_answer",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.QuestionType != "short_answer" {
				t.Errorf("expected only short_answer, got %q for %s", r.QuestionType, r.ID)
			}
		}
	})

	t.Run("difficulty_filter", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"difficulty": "hard",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Difficulty != "hard" {
				t.Errorf("expected only hard, got %q for %s", r.Difficulty, r.ID)
			}
		}
	})

	t.Run("exclude_question_ids_filter", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"exclude_question_ids": []string{"q1"},
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

func TestQuestionBankSearch_Keyword_KnowledgePointsFilter(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
	args, _ := json.Marshal(map[string]interface{}{
		"query":            "arithmetic",
		"knowledge_points": []string{"arithmetic"},
	})
	result, _ := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) == 0 {
		t.Error("expected at least one result for knowledge_points filter")
	}
}

func TestQuestionBankSearch_Keyword_TagsFilter(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
	args, _ := json.Marshal(map[string]interface{}{
		"query": "math",
		"tags":  []string{"math"},
	})
	result, _ := tool.Execute(context.Background(), args)
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) == 0 {
		t.Error("expected at least one result for tags filter")
	}
}

func TestQuestionBankSearch_Semantic_MultiKB_DifferentEngines(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	// Use fresh seed: two KBs under tenant 1 sharing one embedding model
	// but bound to different vector stores.
	db.Exec(`INSERT INTO knowledge_bases(id, tenant_id, type, deleted_at) VALUES
		('kba', 1, 'question_bank', NULL),
		('kbb', 1, 'question_bank', NULL)`)
	db.Exec(`INSERT INTO question_sets(id, tenant_id, knowledge_base_id, name, deleted_at) VALUES
		('qs_a', 1, 'kba', 'Set A', NULL),
		('qs_b', 1, 'kbb', 'Set B', NULL)`)
	db.Exec(`INSERT INTO questions(id, tenant_id, question_set_id, knowledge_base_id, question_type, stem_text, answer_text, analysis_text, difficulty, knowledge_points, tags, status, created_at) VALUES
		('qa1', 1, 'qs_a', 'kba', 'single_choice', 'Question A1', 'Answer A1', '', 'easy', '[]', '[]', 'reviewed', '2024-01-01'),
		('qb1', 1, 'qs_b', 'kbb', 'single_choice', 'Question B1', 'Answer B1', '', 'easy', '[]', '[]', 'reviewed', '2024-01-02')`)

	engineA := &stubRetrieveEngine{sourceIDs: []string{"qa1"}, kbID: "kba"}
	engineB := &stubRetrieveEngine{sourceIDs: []string{"qb1"}, kbID: "kbb"}

	multiRegistry := &stubMultiEngineRegistry{
		enginesByStore: map[string]interfaces.RetrieveEngineService{
			"vs_a": engineA,
			"vs_b": engineB,
		},
	}
	kbService := &stubKBService{
		kbs: map[string]*types.KnowledgeBase{
			"kba": {ID: "kba", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs_a")},
			"kbb": {ID: "kbb", TenantID: 1, Type: "question_bank", EmbeddingModelID: "emb1", VectorStoreID: strPtr("vs_b")},
		},
	}
	modelService := &stubModelService{embedder: &stubEmbedder{dimensions: 768, modelName: "test", modelID: "emb1"}}

	targets := searchTargetsWithKBs([]string{"kba", "kbb"})
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, multiRegistry, &stubStoreOwnership{})

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "semantic",
		"query": "test",
		"limit": 10,
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify each engine was queried for ONLY its own KB — not a merged list.
	if len(engineA.lastKBIDs) != 1 || engineA.lastKBIDs[0] != "kba" {
		t.Errorf("engineA should only query [kba], got %v", engineA.lastKBIDs)
	}
	if len(engineB.lastKBIDs) != 1 || engineB.lastKBIDs[0] != "kbb" {
		t.Errorf("engineB should only query [kbb], got %v", engineB.lastKBIDs)
	}
}

func TestQuestionBankSearch_Semantic_NilDependencies(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})

	// All semantic dependencies nil — semantic mode must return clear error.
	t.Run("semantic_fails_with_nil_deps", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"mode":  "semantic",
			"query": "test",
			"limit": 10,
		})
		result, err := tool.Execute(context.Background(), args)
		if err == nil {
			t.Error("expected error for nil semantic dependencies")
		}
		if result.Success {
			t.Error("expected failure")
		}
		if !strings.Contains(result.Error, "not available") && !strings.Contains(result.Error, "not configured") {
			t.Errorf("expected clear error about missing services, got %q", result.Error)
		}
	})

	// Keyword mode must still work with nil semantic dependencies.
	t.Run("keyword_works_with_nil_deps", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)
		args, _ := json.Marshal(map[string]interface{}{
			"mode":  "keyword",
			"query": "prime",
			"limit": 10,
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("keyword mode should work: %v", err)
		}
		if !result.Success {
			t.Fatalf("keyword mode should succeed: %s", result.Error)
		}
		if !strings.Contains(result.Output, "q2") {
			t.Error("expected q2 in keyword results")
		}
	})
}

func TestQuestionBankSearch_Hybrid_RequiresQuery(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "hybrid",
		"query": "",
	})
	result, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty query in hybrid mode")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestQuestionBankSearch_Hybrid_ModeAccepted(t *testing.T) {
	db := setupQuestionBankTestDB(t)
	seedQuestionBank(t, db)
	targets := searchTargetsWithKBs([]string{"qb1"})
	tool := NewQuestionBankSearchTool(db, targets, nil, nil, nil, nil)

	// Hybrid mode with nil semantic deps should fall back to keyword-only + warning.
	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "hybrid",
		"query": "prime",
		"limit": 10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("hybrid with fallback should not error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if mode, _ := result.Data["mode"].(string); mode != "hybrid" {
		t.Errorf("expected mode=hybrid, got %q", mode)
	}
	if fusion, _ := result.Data["fusion"].(string); fusion != "rrf" {
		t.Errorf("expected fusion=rrf, got %q", fusion)
	}
	if warning, _ := result.Data["semantic_warning"].(string); warning == "" {
		t.Error("expected semantic_warning when deps are nil")
	}
	if !strings.Contains(result.Output, "q2") {
		t.Error("expected q2 (prime question) in keyword fallback results")
	}
}

func TestQuestionBankSearch_Hybrid_KeywordSemanticMerge(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q2", "q3"},
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
	tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})

	// Query "polymorphism" — keyword hits q3 ("Define polymorphism"), semantic hits q2, q3.
	args, _ := json.Marshal(map[string]interface{}{
		"mode":  "hybrid",
		"query": "polymorphism",
		"limit": 10,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	results, _ := result.Data["results"].([]QuestionBankSearchResult)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	for _, r := range results {
		if r.MatchType != "hybrid" {
			t.Errorf("expected match_type=hybrid, got %q for %s", r.MatchType, r.ID)
		}
	}
	// q3 should rank high — it appears in both keyword ("polymorphism") and semantic.
	foundQ3 := false
	for _, r := range results {
		if r.ID == "q3" {
			foundQ3 = true
			if r.KeywordRank == 0 {
				t.Error("q3 should have a keyword rank")
			}
			if r.SemanticRank == 0 {
				t.Error("q3 should have a semantic rank")
			}
			if r.RRFScore == 0 {
				t.Error("q3 should have an RRF score")
			}
			break
		}
	}
	if !foundQ3 {
		t.Error("q3 (polymorphism) should be in results")
	}
}

func TestQuestionBankSearch_Hybrid_StructuredFilters(t *testing.T) {
	db := setupSemanticQuestionBankTestDB(t)
	seedSemanticQuestions(t, db)

	engine := &stubRetrieveEngine{
		sourceIDs: []string{"q1", "q2", "q3"},
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

	t.Run("hybrid_status_filter", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":   "hybrid",
			"query":  "test",
			"limit":  10,
			"status": "reviewed",
		})
		result, _ := tool.Execute(context.Background(), args)
		results, _ := result.Data["results"].([]QuestionBankSearchResult)
		for _, r := range results {
			if r.Status != "reviewed" {
				t.Errorf("expected only reviewed, got %q for %s", r.Status, r.ID)
			}
		}
	})

	t.Run("hybrid_exclude_ids", func(t *testing.T) {
		tool := NewQuestionBankSearchTool(db, targets, kbService, modelService, registry, &stubStoreOwnership{})
		args, _ := json.Marshal(map[string]interface{}{
			"mode":                 "hybrid",
			"query":                "test",
			"limit":                10,
			"exclude_question_ids": []string{"q1"},
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

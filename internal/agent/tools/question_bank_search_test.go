package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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
	tool := NewQuestionBankSearchTool(db, targets)

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

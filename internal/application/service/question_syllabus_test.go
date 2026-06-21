package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mock KB service + repo for syllabus tests ──

type syllabusKBRepo struct {
	interfaces.KnowledgeBaseRepository
	createdKB  *types.KnowledgeBase
	purposeKBs map[string]*types.KnowledgeBase
}

func (r *syllabusKBRepo) GetKnowledgeBaseByPurpose(_ context.Context, _ uint64, purpose string, _ string) (*types.KnowledgeBase, error) {
	if r.purposeKBs != nil {
		return r.purposeKBs[purpose], nil
	}
	return nil, nil
}

func (r *syllabusKBRepo) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) error {
	r.createdKB = kb
	return nil
}

type syllabusKBService struct {
	interfaces.KnowledgeBaseService
	repo *syllabusKBRepo
}

func (s *syllabusKBService) GetRepository() interfaces.KnowledgeBaseRepository {
	return s.repo
}

func (s *syllabusKBService) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	// Return a valid question bank parent.
	return &types.KnowledgeBase{
		ID:                 id,
		Name:               "Test Bank",
		Type:               types.KnowledgeBaseTypeQuestionBank,
		TenantID:           1,
		EmbeddingModelID:   "emb-1",
		QuestionBankConfig: &types.QuestionBankConfig{},
	}, nil
}

// ── Tests ──

// Test 1: Hidden syllabus KB question_bank_config is not nil.
func TestSyllabusKB_HasNonNullQuestionBankConfig(t *testing.T) {
	repo := &syllabusKBRepo{}
	kbSvc := &syllabusKBService{repo: repo}

	svc := &QuestionService{
		knowledgeBaseSvc: kbSvc,
	}
	svc.repository = &syllabusQuestionRepo{} // satisfy nil check for UploadSyllabus path

	// Call findOrCreateSyllabusKB directly.
	parent := &types.KnowledgeBase{
		ID:               "parent-1",
		Name:             "Test Bank",
		Type:             types.KnowledgeBaseTypeQuestionBank,
		TenantID:         1,
		EmbeddingModelID: "emb-1",
	}
	parent.EnsureDefaults()

	kb, err := svc.findOrCreateSyllabusKB(context.Background(), parent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = kb

	created := repo.createdKB
	if created == nil {
		t.Fatal("expected CreateKnowledgeBase to be called")
	}
	if created.QuestionBankConfig == nil {
		t.Fatal("hidden syllabus KB question_bank_config must not be nil")
	}
	val, err := created.QuestionBankConfig.Value()
	if err != nil {
		t.Fatalf("QuestionBankConfig.Value() failed: %v", err)
	}
	raw, ok := val.([]byte)
	if !ok {
		t.Fatalf("expected []byte from Value(), got %T", val)
	}
	if string(raw) == "null" {
		t.Fatal("question_bank_config serialized as null, expected {}")
	}
	if !json.Valid(raw) {
		t.Fatalf("question_bank_config not valid JSON: %s", string(raw))
	}
}

// Test 2: Reuses existing syllabus KB when found.
func TestSyllabusKB_ReusesExisting(t *testing.T) {
	existing := &types.KnowledgeBase{
		ID:                 "existing-syl-1",
		Name:               "Test Bank-考纲",
		Type:               types.KnowledgeBaseTypeDocument,
		TenantID:           1,
		QuestionBankConfig: &types.QuestionBankConfig{},
	}

	repo := &syllabusKBRepo{
		purposeKBs: map[string]*types.KnowledgeBase{
			types.KBPurposeQuestionBankSyllabus: existing,
		},
	}
	kbSvc := &syllabusKBService{repo: repo}
	svc := &QuestionService{knowledgeBaseSvc: kbSvc}

	parent := &types.KnowledgeBase{
		ID:   "parent-1",
		Name: "Test Bank",
		Type: types.KnowledgeBaseTypeQuestionBank,
	}

	kb, err := svc.findOrCreateSyllabusKB(context.Background(), parent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kb.ID != "existing-syl-1" {
		t.Errorf("expected reuse of existing syllabus KB, got ID=%s", kb.ID)
	}
	if repo.createdKB != nil {
		t.Error("must not create new KB when existing one found")
	}
}

// Test 3: NormalizeNotNullJSONB ensures nil config becomes empty.
func TestNormalizeNotNullJSONB_FixesNil(t *testing.T) {
	kb := &types.KnowledgeBase{
		Name:               "Test",
		Type:               types.KnowledgeBaseTypeDocument,
		QuestionBankConfig: nil,
	}
	kb.EnsureDefaults()
	if kb.QuestionBankConfig != nil {
		t.Fatal("EnsureDefaults should set QuestionBankConfig=nil for Document type")
	}
	kb.NormalizeNotNullJSONB()
	if kb.QuestionBankConfig == nil {
		t.Fatal("NormalizeNotNullJSONB must set non-nil QuestionBankConfig")
	}
}

// Test 4: NormalizeNotNullJSONB leaves existing config untouched.
func TestNormalizeNotNullJSONB_PreservesExisting(t *testing.T) {
	cfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-1",
	}
	kb := &types.KnowledgeBase{
		Name:               "Test",
		Type:               types.KnowledgeBaseTypeQuestionBank,
		QuestionBankConfig: cfg,
	}
	kb.EnsureDefaults()
	kb.NormalizeNotNullJSONB()
	if kb.QuestionBankConfig != cfg {
		t.Fatal("NormalizeNotNullJSONB must not replace existing config")
	}
	if kb.QuestionBankConfig.KnowledgePointKnowledgeBaseID != "kp-1" {
		t.Error("NormalizeNotNullJSONB must preserve config fields")
	}
}

// ── Minimal QuestionRepository that satisfies the interface for compilation ──

type syllabusQuestionRepo struct {
	interfaces.QuestionRepository
}

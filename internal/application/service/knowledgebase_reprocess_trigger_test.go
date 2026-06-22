package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mocks for KB update tests ──

type kbUpdateTestRepo struct {
	interfaces.KnowledgeBaseRepository
	kb        *types.KnowledgeBase
	refKBs    map[string]*types.KnowledgeBase
	updateErr error
	updatedKB *types.KnowledgeBase
}

func (r *kbUpdateTestRepo) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if r.kb != nil && r.kb.ID == id {
		cp := *r.kb
		return &cp, nil
	}
	return r.kb, nil
}

func (r *kbUpdateTestRepo) GetKnowledgeBaseByIDAndTenant(_ context.Context, id string, tenantID uint64) (*types.KnowledgeBase, error) {
	if r.refKBs != nil {
		if kb, ok := r.refKBs[id]; ok {
			return kb, nil
		}
	}
	return nil, nil
}

func (r *kbUpdateTestRepo) GetKnowledgeBaseByName(_ context.Context, _ uint64, _ string) (*types.KnowledgeBase, error) {
	return nil, nil
}

func (r *kbUpdateTestRepo) UpdateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedKB = kb
	if r.kb != nil {
		*r.kb = *kb
	}
	return nil
}

// ── Helpers ──

func newKBUpdateService(repo *kbUpdateTestRepo) *knowledgeBaseService {
	return &knowledgeBaseService{
		repo: repo,
	}
}

func kbUpdateCtx() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

func makeQuestionBankKB(id string, cfg *types.QuestionBankConfig) *types.KnowledgeBase {
	return &types.KnowledgeBase{
		ID:                 id,
		TenantID:           1,
		Name:               "Test Bank",
		Type:               types.KnowledgeBaseTypeQuestionBank,
		QuestionBankConfig: cfg,
	}
}

// ── Tests for QuestionBankAutoTaggingConfigChanged ──

func TestQuestionBankAutoTaggingConfigChanged_NoChange(t *testing.T) {
	old := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	new := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	if QuestionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected false for identical configs")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_NilEqualsEmpty(t *testing.T) {
	if QuestionBankAutoTaggingConfigChanged(nil, &types.QuestionBankConfig{}) {
		t.Error("expected false for nil vs empty config")
	}
	if QuestionBankAutoTaggingConfigChanged(nil, nil) {
		t.Error("expected false for nil vs nil")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_KBIDChanged(t *testing.T) {
	old := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	new := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-2"}
	if !QuestionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected true when KnowledgePointKnowledgeBaseID changed")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_RerankConfigChanged(t *testing.T) {
	old := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-1",
		KnowledgePointRerankModelID:   "rm-1",
		KnowledgePointRerankEnabled:   false,
		KnowledgePointRerankTopK:      0,
		KnowledgePointRerankThreshold: 0,
	}
	new := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-1",
		KnowledgePointRerankModelID:   "rm-2",
		KnowledgePointRerankEnabled:   true,
		KnowledgePointRerankTopK:      5,
		KnowledgePointRerankThreshold: 0.8,
	}
	if !QuestionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected true when rerank config changed")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_NameOnlyReturnsFalse(t *testing.T) {
	// Name changes are not part of QuestionBankConfig, so this is always false.
	old := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	new := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	if QuestionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected false for config-unchanged (name-only change scenario)")
	}
}

// ── Tests for UpdateKnowledgeBase preserving config ──

func TestUpdateKnowledgeBase_DoesNotTriggerReprocessWhenConfigUnchanged(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	repo := &kbUpdateTestRepo{
		kb:     kb,
		refKBs: map[string]*types.KnowledgeBase{"kp-B": {ID: "kp-B", Type: types.KnowledgeBaseTypeDocument}},
	}
	svc := newKBUpdateService(repo)

	// Deep-copy old config before update.
	oldCfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-B"}
	updatedKB, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "New Name", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	// Service no longer triggers reprocess; verify config unchanged so
	// handler-side QuestionBankAutoTaggingConfigChanged returns false.
	if QuestionBankAutoTaggingConfigChanged(oldCfg, updatedKB.QuestionBankConfig) {
		t.Error("expected no config change when same KP KB ID submitted")
	}
}

func TestUpdateKnowledgeBase_DoesNotTriggerForNameOnlyChange(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	svc := newKBUpdateService(repo)

	// Deep-copy old config before update.
	oldCfg := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-A"}
	// Only change name, no questionBankConfig
	updatedKB, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Changed Name", "desc", nil, nil)
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	if QuestionBankAutoTaggingConfigChanged(oldCfg, updatedKB.QuestionBankConfig) {
		t.Error("expected no config change for name-only edit")
	}
}

func TestUpdateKnowledgeBase_TriggersWhenKnowledgePointKBChanged(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	repo := &kbUpdateTestRepo{
		kb:     kb,
		refKBs: map[string]*types.KnowledgeBase{"kp-B": {ID: "kp-B", Type: types.KnowledgeBaseTypeDocument}},
	}
	svc := newKBUpdateService(repo)

	// Deep-copy old config before update (repo.UpdateKnowledgeBase mutates kb in place).
	oldCfg := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: kb.QuestionBankConfig.KnowledgePointKnowledgeBaseID,
	}
	updatedKB, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Test Bank", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	// Handler would check: did config change?
	if !QuestionBankAutoTaggingConfigChanged(oldCfg, updatedKB.QuestionBankConfig) {
		t.Error("expected config change detected when KP KB ID changed")
	}
}

func TestUpdateKnowledgeBase_TriggersWhenRerankConfigChanged(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	svc := newKBUpdateService(repo)

	// Deep-copy old config before update.
	oldCfg := &types.QuestionBankConfig{}
	updatedKB, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Test Bank", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   "",
		KnowledgePointRerankModelID:     "rm-1",
		KnowledgePointRerankEnabled:     true,
		KnowledgePointRerankTopK:        5,
		KnowledgePointRerankThreshold:   0.8,
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	if !QuestionBankAutoTaggingConfigChanged(oldCfg, updatedKB.QuestionBankConfig) {
		t.Error("expected config change detected when rerank config changed")
	}
}

func TestUpdateKnowledgeBase_PreservesSyllabusIDWhenSavingQuestionBankConfig(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
		SyllabusKnowledgeBaseID:       "syl-1",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	svc := newKBUpdateService(repo)

	_, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Test Bank", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   "",
		KnowledgePointRerankModelID:     "rm-1",
		KnowledgePointRerankEnabled:     true,
		KnowledgePointRerankTopK:        5,
		KnowledgePointRerankThreshold:   0.8,
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	if repo.updatedKB.QuestionBankConfig.SyllabusKnowledgeBaseID != "syl-1" {
		t.Errorf("expected syllabus ID preserved as syl-1, got %s",
			repo.updatedKB.QuestionBankConfig.SyllabusKnowledgeBaseID)
	}
	if repo.updatedKB.QuestionBankConfig.KnowledgePointRerankModelID != "rm-1" {
		t.Errorf("expected rerank model ID updated to rm-1, got %s",
			repo.updatedKB.QuestionBankConfig.KnowledgePointRerankModelID)
	}
}

func TestUpdateKnowledgeBase_PreservesExistingRerankConfigWhenRequestOnlyContainsKnowledgePointKB(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   "kp-A",
		KnowledgePointRerankModelID:     "rm-1",
		KnowledgePointRerankEnabled:     true,
		KnowledgePointRerankTopK:        5,
		KnowledgePointRerankThreshold:   0.8,
	})
	repo := &kbUpdateTestRepo{
		kb:     kb,
		refKBs: map[string]*types.KnowledgeBase{"kp-B": {ID: "kp-B", Type: types.KnowledgeBaseTypeDocument}},
	}
	svc := newKBUpdateService(repo)

	// Request only sends knowledge_point_knowledge_base_id — no rerank fields.
	// This simulates the current frontend behavior.
	_, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Test Bank", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	saved := repo.updatedKB.QuestionBankConfig
	if saved.KnowledgePointKnowledgeBaseID != "kp-B" {
		t.Errorf("expected kp ID updated to kp-B, got %s", saved.KnowledgePointKnowledgeBaseID)
	}
	if saved.KnowledgePointRerankModelID != "rm-1" {
		t.Errorf("expected rerank model ID preserved as rm-1, got %s", saved.KnowledgePointRerankModelID)
	}
	if !saved.KnowledgePointRerankEnabled {
		t.Error("expected rerank enabled preserved as true")
	}
	if saved.KnowledgePointRerankTopK != 5 {
		t.Errorf("expected rerank topK preserved as 5, got %d", saved.KnowledgePointRerankTopK)
	}
	if saved.KnowledgePointRerankThreshold != 0.8 {
		t.Errorf("expected rerank threshold preserved as 0.8, got %f", saved.KnowledgePointRerankThreshold)
	}
}

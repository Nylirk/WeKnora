package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mocks for KB update reprocess trigger tests ──

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

type kbUpdateQuestionRepo struct {
	interfaces.QuestionRepository
	sets []*types.QuestionSet
}

func (r *kbUpdateQuestionRepo) ListQuestionSets(_ context.Context, _ uint64, kbID string, page *types.Pagination) (*types.PageResult, error) {
	filtered := make([]*types.QuestionSet, 0)
	for _, qs := range r.sets {
		if qs.KnowledgeBaseID == kbID {
			filtered = append(filtered, qs)
		}
	}
	if page.Page > 1 {
		return types.NewPageResult(int64(len(filtered)), page, []*types.QuestionSet{}), nil
	}
	return types.NewPageResult(int64(len(filtered)), page, filtered), nil
}

type mockQSReprocessor struct {
	mu          sync.Mutex
	calls       []reprocessCall
	callCount   int
	failOnError error
}

type reprocessCall struct {
	KBID  string
	SetID string
	Scope string
}

func (m *mockQSReprocessor) ReprocessQuestionSet(_ context.Context, kbID, setID, scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, reprocessCall{KBID: kbID, SetID: setID, Scope: scope})
	m.callCount++
	if m.failOnError != nil {
		return m.failOnError
	}
	return nil
}

func (m *mockQSReprocessor) getCalls() []reprocessCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]reprocessCall, len(m.calls))
	copy(cp, m.calls)
	return cp
}

func (m *mockQSReprocessor) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// ── Helpers ──

func newKBUpdateService(repo *kbUpdateTestRepo, qRepo *kbUpdateQuestionRepo, reproc *mockQSReprocessor) *knowledgeBaseService {
	return &knowledgeBaseService{
		repo:          repo,
		questionRepo:  qRepo,
		qsReprocessor: reproc,
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

// ── Tests ──

func TestQuestionBankAutoTaggingConfigChanged_NoChange(t *testing.T) {
	old := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	new := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	if questionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected false for identical configs")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_NilEqualsEmpty(t *testing.T) {
	if questionBankAutoTaggingConfigChanged(nil, &types.QuestionBankConfig{}) {
		t.Error("expected false for nil vs empty config")
	}
	if questionBankAutoTaggingConfigChanged(nil, nil) {
		t.Error("expected false for nil vs nil")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_KBIDChanged(t *testing.T) {
	old := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-1"}
	new := &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "kp-2"}
	if !questionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected true when KnowledgePointKnowledgeBaseID changed")
	}
}

func TestQuestionBankAutoTaggingConfigChanged_RerankConfigChanged(t *testing.T) {
	old := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   "kp-1",
		KnowledgePointRerankModelID:     "rm-1",
		KnowledgePointRerankEnabled:     false,
		KnowledgePointRerankTopK:        0,
		KnowledgePointRerankThreshold:   0,
	}
	new := &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID:   "kp-1",
		KnowledgePointRerankModelID:     "rm-2",
		KnowledgePointRerankEnabled:     true,
		KnowledgePointRerankTopK:        5,
		KnowledgePointRerankThreshold:   0.8,
	}
	if !questionBankAutoTaggingConfigChanged(old, new) {
		t.Error("expected true when rerank config changed")
	}
}

func TestUpdateKnowledgeBase_DoesNotTriggerReprocessWhenConfigUnchanged(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	repo := &kbUpdateTestRepo{
		kb:     kb,
		refKBs: map[string]*types.KnowledgeBase{"kp-B": {ID: "kp-B", Type: types.KnowledgeBaseTypeDocument}},
	}
	qRepo := &kbUpdateQuestionRepo{sets: []*types.QuestionSet{
		{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}}
	reproc := &mockQSReprocessor{}
	svc := newKBUpdateService(repo, qRepo, reproc)

	_, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "New Name", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	if reproc.getCallCount() != 0 {
		t.Errorf("expected 0 reprocess calls, got %d", reproc.getCallCount())
	}
}

func TestUpdateKnowledgeBase_DoesNotTriggerForNameOnlyChange(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	qRepo := &kbUpdateQuestionRepo{sets: []*types.QuestionSet{
		{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}}
	reproc := &mockQSReprocessor{}
	svc := newKBUpdateService(repo, qRepo, reproc)

	// Only change name, no questionBankConfig
	_, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Changed Name", "desc", nil, nil)
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	if reproc.getCallCount() != 0 {
		t.Errorf("expected 0 reprocess calls for name-only change, got %d", reproc.getCallCount())
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
	qRepo := &kbUpdateQuestionRepo{sets: []*types.QuestionSet{
		{ID: "set-1", KnowledgeBaseID: "kb-1"},
		{ID: "set-2", KnowledgeBaseID: "kb-1"},
	}}
	reproc := &mockQSReprocessor{}
	svc := newKBUpdateService(repo, qRepo, reproc)

	_, err := svc.UpdateKnowledgeBase(kbUpdateCtx(), "kb-1", "Test Bank", "desc", nil, &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase error: %v", err)
	}
	// Reprocess runs in goroutine; wait briefly for it.
	waitForReprocess(reproc, 2, 1)
	if reproc.getCallCount() != 2 {
		t.Errorf("expected 2 reprocess calls, got %d", reproc.getCallCount())
	}
	calls := reproc.getCalls()
	for _, c := range calls {
		if c.Scope != "auto_tagging" {
			t.Errorf("expected scope=auto_tagging, got %s", c.Scope)
		}
		if c.KBID != "kb-1" {
			t.Errorf("expected KBID=kb-1, got %s", c.KBID)
		}
	}
}

func TestUpdateKnowledgeBase_TriggersWhenRerankConfigChanged(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	qRepo := &kbUpdateQuestionRepo{sets: []*types.QuestionSet{
		{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}}
	reproc := &mockQSReprocessor{}
	svc := newKBUpdateService(repo, qRepo, reproc)

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
	waitForReprocess(reproc, 1, 1)
	if reproc.getCallCount() != 1 {
		t.Errorf("expected 1 reprocess call, got %d", reproc.getCallCount())
	}
}

func TestUpdateKnowledgeBase_PreservesSyllabusIDWhenSavingQuestionBankConfig(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "",
		SyllabusKnowledgeBaseID:       "syl-1",
	})
	repo := &kbUpdateTestRepo{kb: kb}
	qRepo := &kbUpdateQuestionRepo{}
	reproc := &mockQSReprocessor{}
	svc := newKBUpdateService(repo, qRepo, reproc)

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

func TestScheduleKnowledgePointReprocessForKB_UsesAutoTaggingScope(t *testing.T) {
	kb := makeQuestionBankKB("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	qRepo := &kbUpdateQuestionRepo{sets: []*types.QuestionSet{
		{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}}
	reproc := &mockQSReprocessor{}
	svc := &knowledgeBaseService{
		questionRepo:  qRepo,
		qsReprocessor: reproc,
	}

	svc.scheduleKnowledgePointReprocessForKB(kbUpdateCtx(), kb)
	waitForReprocess(reproc, 1, 1)
	calls := reproc.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Scope != "auto_tagging" {
		t.Errorf("expected scope=auto_tagging, got %s", calls[0].Scope)
	}
}

// waitForReprocess polls the mock reprocessor until expectedCalls is reached
// or the timeout (in seconds) expires.
func waitForReprocess(reproc *mockQSReprocessor, expectedCalls int, timeoutSec int) {
	for i := 0; i < timeoutSec*50; i++ {
		if reproc.getCallCount() >= expectedCalls {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stderrors "errors"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ── Mocks ──

type stubKBUpdateService struct {
	interfaces.KnowledgeBaseService
	getByID      func(ctx context.Context, id string) (*types.KnowledgeBase, error)
	updateKB     func(ctx context.Context, id, name, desc string, cfg *types.KnowledgeBaseConfig, qbc *types.QuestionBankConfig) (*types.KnowledgeBase, error)
}

func (s *stubKBUpdateService) GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	return s.getByID(ctx, id)
}

func (s *stubKBUpdateService) UpdateKnowledgeBase(ctx context.Context, id, name, desc string, cfg *types.KnowledgeBaseConfig, qbc *types.QuestionBankConfig) (*types.KnowledgeBase, error) {
	return s.updateKB(ctx, id, name, desc, cfg, qbc)
}

func (s *stubKBUpdateService) FillKnowledgeBaseCounts(ctx context.Context, kb *types.KnowledgeBase) error {
	return nil
}

type stubQuestionServiceForReprocess struct {
	interfaces.QuestionService
	mu            sync.Mutex
	reprocessCalls []stubReprocessCall
	listSetsResult *types.PageResult
	listSetsErr   error
}

type stubReprocessCall struct {
	KBID  string
	SetID string
	Scope string
}

func (s *stubQuestionServiceForReprocess) ReprocessQuestionSet(_ context.Context, kbID, setID, scope string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reprocessCalls = append(s.reprocessCalls, stubReprocessCall{KBID: kbID, SetID: setID, Scope: scope})
	return nil
}

func (s *stubQuestionServiceForReprocess) ListQuestionSets(_ context.Context, kbID string, page *types.Pagination) (*types.PageResult, error) {
	if s.listSetsErr != nil {
		return nil, s.listSetsErr
	}
	if s.listSetsResult != nil {
		return s.listSetsResult, nil
	}
	return types.NewPageResult(0, page, []*types.QuestionSet{}), nil
}

func (s *stubQuestionServiceForReprocess) getReprocessCalls() []stubReprocessCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]stubReprocessCall, len(s.reprocessCalls))
	copy(cp, s.reprocessCalls)
	return cp
}

// ── Helpers ──

func newKBUpdateTestRouter(svc interfaces.KnowledgeBaseService, qs interfaces.QuestionService) (*gin.Engine, *KnowledgeBaseHandler) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc, questionService: qs}
	r.PUT("/knowledge-bases/:id", h.UpdateKnowledgeBase)
	return r, h
}

func makeQuestionBankKBForHandler(id string, qbc *types.QuestionBankConfig) *types.KnowledgeBase {
	return &types.KnowledgeBase{
		ID:                 id,
		TenantID:           1,
		Name:               "Test Bank",
		Type:               types.KnowledgeBaseTypeQuestionBank,
		QuestionBankConfig: qbc,
	}
}

// ── Tests ──

// Test 1: Config changed → ReprocessQuestionSet called with scope=auto_tagging.
func TestUpdateKnowledgeBaseHandler_TriggersReprocessWhenConfigChanged(t *testing.T) {
	oldKB := makeQuestionBankKBForHandler("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	newKB := makeQuestionBankKBForHandler("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	svc := &stubKBUpdateService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return oldKB, nil
		},
		updateKB: func(_ context.Context, _, _, _ string, _ *types.KnowledgeBaseConfig, qbc *types.QuestionBankConfig) (*types.KnowledgeBase, error) {
			return newKB, nil
		},
	}
	qs := &stubQuestionServiceForReprocess{
		listSetsResult: types.NewPageResult(1, &types.Pagination{Page: 1, PageSize: 500},
			[]*types.QuestionSet{{ID: "set-1", KnowledgeBaseID: "kb-1"}}),
	}
	r, _ := newKBUpdateTestRouter(svc, qs)

	body := `{"name":"Test Bank","question_bank_config":{"knowledge_point_knowledge_base_id":"kp-B"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// Reprocess runs in goroutine; wait for it.
	waitForHandlerReprocess(qs, 1, 2)
	calls := qs.getReprocessCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 reprocess call, got %d", len(calls))
	}
	assert.Equal(t, "auto_tagging", calls[0].Scope)
	assert.Equal(t, "kb-1", calls[0].KBID)
	assert.Equal(t, "set-1", calls[0].SetID)
}

// Test 2: Config unchanged → no ReprocessQuestionSet call.
func TestUpdateKnowledgeBaseHandler_DoesNotTriggerReprocessWhenConfigUnchanged(t *testing.T) {
	oldKB := makeQuestionBankKBForHandler("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	newKB := makeQuestionBankKBForHandler("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-B",
	})
	svc := &stubKBUpdateService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return oldKB, nil
		},
		updateKB: func(_ context.Context, _, _, _ string, _ *types.KnowledgeBaseConfig, qbc *types.QuestionBankConfig) (*types.KnowledgeBase, error) {
			return newKB, nil
		},
	}
	qs := &stubQuestionServiceForReprocess{
		listSetsResult: types.NewPageResult(1, &types.Pagination{Page: 1, PageSize: 500},
			[]*types.QuestionSet{{ID: "set-1", KnowledgeBaseID: "kb-1"}}),
	}
	r, _ := newKBUpdateTestRouter(svc, qs)

	body := `{"name":"New Name","question_bank_config":{"knowledge_point_knowledge_base_id":"kp-B"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	// Give goroutine time to potentially fire (it shouldn't).
	waitForHandlerReprocess(qs, 0, 1)
	calls := qs.getReprocessCalls()
	assert.Empty(t, calls, "expected no reprocess calls when config unchanged")
}

// Test 3: Save fails → no ReprocessQuestionSet call.
func TestUpdateKnowledgeBaseHandler_DoesNotTriggerReprocessWhenSaveFails(t *testing.T) {
	oldKB := makeQuestionBankKBForHandler("kb-1", &types.QuestionBankConfig{
		KnowledgePointKnowledgeBaseID: "kp-A",
	})
	svc := &stubKBUpdateService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return oldKB, nil
		},
		updateKB: func(_ context.Context, _, _, _ string, _ *types.KnowledgeBaseConfig, _ *types.QuestionBankConfig) (*types.KnowledgeBase, error) {
			return nil, stderrors.New("db write failed")
		},
	}
	qs := &stubQuestionServiceForReprocess{}
	r, _ := newKBUpdateTestRouter(svc, qs)

	body := `{"name":"Test Bank","question_bank_config":{"knowledge_point_knowledge_base_id":"kp-B"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	calls := qs.getReprocessCalls()
	assert.Empty(t, calls, "expected no reprocess calls when save fails")
}

// ── Utility ──

func waitForHandlerReprocess(qs *stubQuestionServiceForReprocess, expectedCalls int, timeoutSec int) {
	for i := 0; i < timeoutSec*50; i++ {
		if len(qs.getReprocessCalls()) >= expectedCalls {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

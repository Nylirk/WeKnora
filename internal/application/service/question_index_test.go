package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type memoryQuestionVectorIndexRepository struct {
	interfaces.QuestionVectorIndexRepository
	items map[string]*types.QuestionVectorIndex
}

func newMemoryQuestionVectorIndexRepository() *memoryQuestionVectorIndexRepository {
	return &memoryQuestionVectorIndexRepository{items: make(map[string]*types.QuestionVectorIndex)}
}

func questionVectorIndexKey(questionID, modelID string, engine types.RetrieverEngineType, mode string) string {
	return questionID + "\x00" + modelID + "\x00" + string(engine) + "\x00" + mode
}

func (r *memoryQuestionVectorIndexRepository) Get(
	_ context.Context, _ uint64, questionID, modelID string, engine types.RetrieverEngineType, mode string,
) (*types.QuestionVectorIndex, error) {
	return r.items[questionVectorIndexKey(questionID, modelID, engine, mode)], nil
}

func (r *memoryQuestionVectorIndexRepository) Upsert(_ context.Context, index *types.QuestionVectorIndex) error {
	copy := *index
	r.items[questionVectorIndexKey(index.QuestionID, index.EmbeddingModelID, index.RetrieverEngineType, index.IndexMode)] = &copy
	return nil
}

func (r *memoryQuestionVectorIndexRepository) UpdateStatus(
	_ context.Context, _ uint64, questionID, modelID string, engine types.RetrieverEngineType, mode string,
	status types.QuestionVectorIndexStatus, errorMessage, contentHash string, indexedAt *time.Time,
) error {
	item := r.items[questionVectorIndexKey(questionID, modelID, engine, mode)]
	item.Status = status
	item.ErrorMessage = errorMessage
	item.ContentHash = contentHash
	item.IndexedAt = indexedAt
	return nil
}

func (r *memoryQuestionVectorIndexRepository) ListByQuestionIDs(
	_ context.Context, _ uint64, questionIDs []string,
) ([]*types.QuestionVectorIndex, error) {
	wanted := make(map[string]struct{}, len(questionIDs))
	for _, id := range questionIDs {
		wanted[id] = struct{}{}
	}
	var result []*types.QuestionVectorIndex
	for _, item := range r.items {
		if _, ok := wanted[item.QuestionID]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

type questionIndexKBService struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *questionIndexKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type questionIndexModelService struct {
	interfaces.ModelService
	embedder embedding.Embedder
	err      error
}

func (s *questionIndexModelService) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return s.embedder, s.err
}

type questionIndexEmbedder struct{}

func (*questionIndexEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1}, nil
}
func (*questionIndexEmbedder) BatchEmbed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1}}, nil
}
func (*questionIndexEmbedder) BatchEmbedWithPool(context.Context, embedding.Embedder, []string) ([][]float32, error) {
	return [][]float32{{1}}, nil
}
func (*questionIndexEmbedder) GetModelName() string { return "test" }
func (*questionIndexEmbedder) GetDimensions() int   { return 1 }
func (*questionIndexEmbedder) GetModelID() string   { return "model-1" }

type questionIndexEngine struct {
	interfaces.RetrieveEngineService
	batchCalls    int
	deleteCalls   int
	lastIndexes   []*types.IndexInfo
	lastDeletedIDs []string
	batchErr      error
	deleteErr     error
	// callOrder records "delete" or "batch" in the order they were called.
	callOrder []string
}

func (*questionIndexEngine) EngineType() types.RetrieverEngineType {
	return types.PostgresRetrieverEngineType
}
func (*questionIndexEngine) Support() []types.RetrieverType {
	return []types.RetrieverType{types.VectorRetrieverType}
}
func (e *questionIndexEngine) BatchIndex(
	_ context.Context, _ embedding.Embedder, indexes []*types.IndexInfo, _ []types.RetrieverType,
) error {
	e.batchCalls++
	e.lastIndexes = indexes
	e.callOrder = append(e.callOrder, "batch")
	return e.batchErr
}
func (e *questionIndexEngine) DeleteBySourceIDList(_ context.Context, ids []string, _ int, _ string) error {
	e.deleteCalls++
	e.lastDeletedIDs = ids
	e.callOrder = append(e.callOrder, "delete")
	return e.deleteErr
}

type questionIndexRegistry struct {
	interfaces.RetrieveEngineRegistry
	engine interfaces.RetrieveEngineService
}

func (r *questionIndexRegistry) GetRetrieveEngineService(types.RetrieverEngineType) (interfaces.RetrieveEngineService, error) {
	return r.engine, nil
}

type questionIndexVectorStoreRepository struct {
	interfaces.VectorStoreRepository
}

func questionIndexTestContext() context.Context {
	tenant := &types.Tenant{
		ID: 1,
		RetrieverEngines: types.RetrieverEngines{
			Engines: []types.RetrieverEngineParams{{
				RetrieverEngineType: types.PostgresRetrieverEngineType,
				RetrieverType:       types.VectorRetrieverType,
			}},
		},
	}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	return context.WithValue(ctx, types.TenantInfoContextKey, tenant)
}

func newQuestionIndexTestService(
	repository *memoryQuestionVectorIndexRepository,
	engine *questionIndexEngine,
	modelErr error,
) *questionIndexService {
	return newQuestionIndexService(
		repository,
		&questionStatusRepository{},
		&questionIndexKBService{kb: &types.KnowledgeBase{
			ID: "kb-1", TenantID: 1, Type: types.KnowledgeBaseTypeQuestionBank, EmbeddingModelID: "model-1",
		}},
		&questionIndexModelService{embedder: &questionIndexEmbedder{}, err: modelErr},
		&questionIndexRegistry{engine: engine},
		&questionIndexVectorStoreRepository{},
		func(fn func()) { fn() },
	)
}

func TestBuildQuestionIndexContentIncludesOnlyAllowedFields(t *testing.T) {
	question := &types.Question{
		QuestionType:    "single_choice",
		Difficulty:      types.QuestionDifficultyHard,
		StemText:        "题干内容",
		QuestionBody:    types.JSON(`{"options":[{"label":"A","content":"选项一"}],"correct_answer":"A"}`),
		KnowledgePoints: types.JSON(`["代数"]`),
		Tags:            types.JSON(`["期中"]`),
		AnswerText:      "绝密答案",
		AnswerBody:      types.JSON(`{"answer":"绝密结构答案"}`),
		AnalysisText:    "绝密解析",
		GradingRubric:   types.JSON(`{"rubric":"绝密评分"}`),
	}

	content := BuildQuestionIndexContent(question)
	for _, expected := range []string{"single_choice", "hard", "题干内容", "选项一", "代数", "期中"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("content %q does not contain %q", content, expected)
		}
	}
	for _, forbidden := range []string{"绝密答案", "绝密结构答案", "绝密解析", "绝密评分", "correct_answer"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("content %q contains forbidden value %q", content, forbidden)
		}
	}
	if content != BuildQuestionIndexContent(question) {
		t.Fatal("BuildQuestionIndexContent is not stable")
	}
	longQuestion := &types.Question{StemText: strings.Repeat("题", questionIndexMaxChars+100)}
	if got := len([]rune(BuildQuestionIndexContent(longQuestion))); got != questionIndexMaxChars {
		t.Fatalf("long content rune count = %d, want %d", got, questionIndexMaxChars)
	}
}

func TestQuestionIndexServiceIndexesReviewedAndSkipsUnchangedHash(t *testing.T) {
	repository := newMemoryQuestionVectorIndexRepository()
	engine := &questionIndexEngine{}
	service := newQuestionIndexTestService(repository, engine, nil)
	question := &types.Question{
		ID: "q-1", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1",
		QuestionType: "short_answer", Difficulty: types.QuestionDifficultyMedium,
		StemText: "题干", Status: types.QuestionStatusReviewed,
	}

	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("IndexQuestions() error = %v", err)
	}
	if engine.deleteCalls != 1 || engine.batchCalls != 1 || len(engine.lastIndexes) != 1 {
		t.Fatalf("delete=%d batch=%d indexes=%d", engine.deleteCalls, engine.batchCalls, len(engine.lastIndexes))
	}
	if got := engine.callOrder; len(got) != 2 || got[0] != "delete" || got[1] != "batch" {
		t.Fatalf("call order = %v, want [delete batch]", got)
	}
	if engine.lastDeletedIDs == nil || len(engine.lastDeletedIDs) != 1 || engine.lastDeletedIDs[0] != "q-1" {
		t.Fatalf("DeleteBySourceIDList ids = %v, want [q-1]", engine.lastDeletedIDs)
	}
	if !engine.lastIndexes[0].IsEnabled {
		t.Fatal("reviewed question IsEnabled = false")
	}
	if engine.lastIndexes[0].SourceType != types.QuestionSourceType || engine.lastIndexes[0].KnowledgeType != types.KnowledgeTypeQuestion {
		t.Fatalf("unexpected IndexInfo mapping: %+v", engine.lastIndexes[0])
	}
	state := repository.items[questionVectorIndexKey("q-1", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt)]
	if state.Status != types.QuestionVectorIndexStatusIndexed || state.IndexedAt == nil {
		t.Fatalf("state = %+v", state)
	}

	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("second IndexQuestions() error = %v", err)
	}
	if engine.deleteCalls != 1 || engine.batchCalls != 1 {
		t.Fatalf("unchanged content called delete=%d batch=%d", engine.deleteCalls, engine.batchCalls)
	}

	// Transition reviewed -> draft must delete stale enabled vectors and
	// re-index with IsEnabled=false.
	question.Status = types.QuestionStatusDraft
	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("draft IndexQuestions() error = %v", err)
	}
	if engine.deleteCalls != 2 || engine.batchCalls != 2 || engine.lastIndexes[0].IsEnabled {
		t.Fatalf("draft mapping delete=%d batch=%d IsEnabled=%v", engine.deleteCalls, engine.batchCalls, engine.lastIndexes[0].IsEnabled)
	}

	// Transition reviewed -> rejected must also delete stale enabled vectors.
	question.Status = types.QuestionStatusReviewed
	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("reviewed re-IndexQuestions() error = %v", err)
	}
	question.Status = types.QuestionStatusRejected
	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("rejected IndexQuestions() error = %v", err)
	}
	if engine.deleteCalls != 4 || engine.lastIndexes[0].IsEnabled {
		t.Fatalf("rejected mapping delete=%d batch=%d IsEnabled=%v", engine.deleteCalls, engine.batchCalls, engine.lastIndexes[0].IsEnabled)
	}
}

func TestQuestionIndexServiceRecordsFailureAndDeletes(t *testing.T) {
	// BatchIndex failure
	repository := newMemoryQuestionVectorIndexRepository()
	engine := &questionIndexEngine{batchErr: errors.New("embedding unavailable")}
	service := newQuestionIndexTestService(repository, engine, nil)
	question := &types.Question{
		ID: "q-1", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1",
		QuestionType: "short_answer", StemText: "题干", Status: types.QuestionStatusReviewed,
	}

	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("IndexQuestions() scheduling error = %v", err)
	}
	state := repository.items[questionVectorIndexKey("q-1", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt)]
	if state.Status != types.QuestionVectorIndexStatusFailed || state.ErrorMessage != "embedding unavailable" {
		t.Fatalf("failed state = %+v", state)
	}

	engine.batchErr = nil
	if err := service.DeleteQuestionIndexes(questionIndexTestContext(), []string{"q-1"}); err != nil {
		t.Fatalf("DeleteQuestionIndexes() error = %v", err)
	}
	if engine.deleteCalls != 2 || state.Status != types.QuestionVectorIndexStatusDeleted {
		// deleteCalls counts both the pre-index cleanup and the explicit
		// DeleteQuestionIndexes call.
		t.Fatalf("delete calls=%d state=%+v", engine.deleteCalls, state)
	}

	// DeleteBySourceIDList failure during reindex must not call BatchIndex.
	repository2 := newMemoryQuestionVectorIndexRepository()
	engine2 := &questionIndexEngine{deleteErr: errors.New("vector store unavailable")}
	service2 := newQuestionIndexTestService(repository2, engine2, nil)
	question2 := &types.Question{
		ID: "q-2", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1",
		QuestionType: "short_answer", StemText: "题干", Status: types.QuestionStatusReviewed,
	}
	if err := service2.IndexQuestions(questionIndexTestContext(), []*types.Question{question2}); err != nil {
		t.Fatalf("IndexQuestions() scheduling error = %v", err)
	}
	if engine2.batchCalls != 0 {
		t.Fatalf("BatchIndex called %d times after delete failure", engine2.batchCalls)
	}
	state2 := repository2.items[questionVectorIndexKey("q-2", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt)]
	if state2.Status != types.QuestionVectorIndexStatusFailed {
		t.Fatalf("delete-failed state = %+v, want failed", state2)
	}
	if !strings.Contains(state2.ErrorMessage, "delete stale vectors") || !strings.Contains(state2.ErrorMessage, "vector store unavailable") {
		t.Fatalf("delete-failed error message = %q", state2.ErrorMessage)
	}
}

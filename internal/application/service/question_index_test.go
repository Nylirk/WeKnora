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
	batchCalls  int
	deleteCalls int
	lastIndexes []*types.IndexInfo
	batchErr    error
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
	return e.batchErr
}
func (e *questionIndexEngine) DeleteBySourceIDList(context.Context, []string, int, string) error {
	e.deleteCalls++
	return nil
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
	if engine.batchCalls != 1 || len(engine.lastIndexes) != 1 {
		t.Fatalf("BatchIndex calls = %d, indexes = %d", engine.batchCalls, len(engine.lastIndexes))
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
	if engine.batchCalls != 1 {
		t.Fatalf("unchanged content called BatchIndex %d times", engine.batchCalls)
	}

	question.Status = types.QuestionStatusDraft
	if err := service.IndexQuestions(questionIndexTestContext(), []*types.Question{question}); err != nil {
		t.Fatalf("draft IndexQuestions() error = %v", err)
	}
	if engine.batchCalls != 2 || engine.lastIndexes[0].IsEnabled {
		t.Fatalf("draft mapping calls=%d IsEnabled=%v", engine.batchCalls, engine.lastIndexes[0].IsEnabled)
	}
}

func TestQuestionIndexServiceRecordsFailureAndDeletes(t *testing.T) {
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
	if engine.deleteCalls != 1 || state.Status != types.QuestionVectorIndexStatusDeleted {
		t.Fatalf("delete calls=%d state=%+v", engine.deleteCalls, state)
	}
}

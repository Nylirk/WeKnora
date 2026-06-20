package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type QuestionRepository interface {
	CreateQuestionSet(context.Context, *types.QuestionSet) error
	GetQuestionSet(context.Context, uint64, string) (*types.QuestionSet, error)
	GetQuestionSetByKB(context.Context, uint64, string) (*types.QuestionSet, error)
	ListQuestionSets(context.Context, uint64, string, *types.Pagination) (*types.PageResult, error)
	UpdateQuestionSet(context.Context, *types.QuestionSet) error
	UpdateQuestionSetSourceType(context.Context, uint64, string, types.QuestionSetSourceType) error
	DeleteQuestionSet(context.Context, uint64, string) error
	UpdateQuestionCount(context.Context, uint64, string) error

	CreateQuestion(context.Context, *types.Question) error
	CreateQuestions(context.Context, []*types.Question) error
	GetQuestion(context.Context, uint64, string, string) (*types.Question, error)
	GetQuestionByID(context.Context, uint64, string) (*types.Question, error)
	ListQuestions(context.Context, uint64, string, *types.QuestionListFilter, *types.Pagination) (*types.PageResult, error)
	UpdateQuestion(context.Context, *types.Question) error
	DeleteQuestion(context.Context, uint64, string, string) error
	ListQuestionsByKB(context.Context, uint64, string, *types.QuestionListFilter, *types.Pagination) (*types.PageResult, error)
}

type QuestionVectorIndexRepository interface {
	Get(context.Context, uint64, string, string, types.RetrieverEngineType, string) (*types.QuestionVectorIndex, error)
	Upsert(context.Context, *types.QuestionVectorIndex) error
	UpdateStatus(context.Context, uint64, string, string, types.RetrieverEngineType, string, types.QuestionVectorIndexStatus, string, string, *time.Time) error
	ListByQuestionIDs(context.Context, uint64, []string) ([]*types.QuestionVectorIndex, error)
}

type QuestionIndexService interface {
	IndexQuestions(context.Context, []*types.Question) error
	ReindexQuestion(context.Context, string) error
	ReindexQuestionSet(context.Context, string) error
	DeleteQuestionIndexes(context.Context, []string) error
}

type QuestionService interface {
	CreateQuestionSet(context.Context, string, *types.CreateQuestionSetRequest) (*types.QuestionSet, error)
	GetQuestionSet(context.Context, string, string) (*types.QuestionSet, error)
	ListQuestionSets(context.Context, string, *types.Pagination) (*types.PageResult, error)
	UpdateQuestionSet(context.Context, string, string, *types.UpdateQuestionSetRequest) (*types.QuestionSet, error)
	DeleteQuestionSet(context.Context, string, string) error
	GetQuestionSetProcessingStatus(context.Context, string, string) (*types.QuestionSetProcessingStatus, error)

	CreateQuestion(context.Context, string, string, *types.CreateQuestionRequest) (*types.Question, error)
	GetQuestion(context.Context, string, string, string) (*types.Question, error)
	ListQuestions(context.Context, string, string, *types.QuestionListFilter, *types.Pagination) (*types.PageResult, error)
	UpdateQuestion(context.Context, string, string, string, *types.UpdateQuestionRequest) (*types.Question, error)
	DeleteQuestion(context.Context, string, string, string) error
	UpdateQuestionStatus(context.Context, string, string, string, *types.UpdateQuestionStatusRequest) (*types.Question, error)
	ImportQuestions(context.Context, string, string, *types.ImportQuestionsRequest) (*types.ImportQuestionsResult, error)
	PreviewImportQuestionsFromFile(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error)
	PreviewImportBlocks(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.BlockPreviewRequest) (*types.BlockPreviewResponse, error)
	ParseImportedBlocks(ctx context.Context, kbID, setID string, req *types.ParseBlocksRequest) (*types.ImportFilePreviewResponse, error)
	ExportToEvaluationDataset(context.Context, string, string, *types.ExportToEvaluationRequest) (*types.EvaluationDataset, error)
	GenerateQuestions(context.Context, string, *types.GenerateQuestionsRequest) (*types.QuestionSet, error)
}

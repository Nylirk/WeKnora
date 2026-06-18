package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type questionStatusRepository struct {
	interfaces.QuestionRepository
	set              *types.QuestionSet
	question         *types.Question
	createdQuestion  *types.Question
	createdQuestions []*types.Question
	mutationOrder    []string
	fullSetUpdates   int
	sourceUpdateErr  error
	countUpdateErr   error
}

func (r *questionStatusRepository) GetQuestionSet(context.Context, uint64, string) (*types.QuestionSet, error) {
	return r.set, nil
}

func (r *questionStatusRepository) CreateQuestion(_ context.Context, question *types.Question) error {
	r.createdQuestion = question
	return nil
}

func (r *questionStatusRepository) CreateQuestions(_ context.Context, questions []*types.Question) error {
	r.createdQuestions = questions
	return nil
}

func (r *questionStatusRepository) GetQuestion(context.Context, uint64, string, string) (*types.Question, error) {
	return r.question, nil
}

func (r *questionStatusRepository) UpdateQuestion(_ context.Context, question *types.Question) error {
	r.question = question
	return nil
}

func (r *questionStatusRepository) UpdateQuestionSet(_ context.Context, set *types.QuestionSet) error {
	r.set = set
	r.fullSetUpdates++
	r.mutationOrder = append(r.mutationOrder, "full-set")
	return nil
}

func (r *questionStatusRepository) UpdateQuestionSetSourceType(
	_ context.Context,
	_ uint64,
	_ string,
	sourceType types.QuestionSetSourceType,
) error {
	r.mutationOrder = append(r.mutationOrder, "source")
	if r.sourceUpdateErr != nil {
		return r.sourceUpdateErr
	}
	r.set.SourceType = sourceType
	return nil
}

func (r *questionStatusRepository) UpdateQuestionCount(context.Context, uint64, string) error {
	r.mutationOrder = append(r.mutationOrder, "count")
	return r.countUpdateErr
}

type questionStatusKBService struct {
	interfaces.KnowledgeBaseService
}

func (*questionStatusKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{ID: "kb-1", Type: types.KnowledgeBaseTypeQuestionBank}, nil
}

func newQuestionStatusService(repository *questionStatusRepository) *QuestionService {
	return &QuestionService{
		repository:       repository,
		knowledgeBaseSvc: &questionStatusKBService{},
	}
}

func questionStatusContext() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

func TestStructuredQuestionStatusUsesReviewValidation(t *testing.T) {
	tests := []struct {
		name     string
		question *types.Question
		want     types.QuestionStatus
	}{
		{
			name: "valid short answer is reviewed",
			question: &types.Question{
				QuestionType: string(types.QuestionTypeShortAnswer),
				StemText:     "题干",
				AnswerText:   "答案",
			},
			want: types.QuestionStatusReviewed,
		},
		{
			name: "blank short answer remains draft",
			question: &types.Question{
				QuestionType: string(types.QuestionTypeShortAnswer),
				StemText:     "题干",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "choice with answer text but invalid body remains draft",
			question: &types.Question{
				QuestionType: string(types.QuestionTypeSingleChoice),
				StemText:     "题干",
				AnswerText:   "A",
				QuestionBody: types.JSON(`{}`),
				AnswerBody:   types.JSON(`{"selected_index":0}`),
			},
			want: types.QuestionStatusDraft,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := structuredQuestionStatus(tt.question); got != tt.want {
				t.Fatalf("structuredQuestionStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateQuestionUsesReviewValidation(t *testing.T) {
	tests := []struct {
		name       string
		answerText string
		want       types.QuestionStatus
	}{
		{name: "complete manual question", answerText: "答案", want: types.QuestionStatusReviewed},
		{name: "incomplete manual question", answerText: "", want: types.QuestionStatusDraft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
			service := newQuestionStatusService(repository)
			question, err := service.CreateQuestion(questionStatusContext(), "kb-1", "set-1", &types.CreateQuestionRequest{
				QuestionType: string(types.QuestionTypeShortAnswer),
				StemText:     "题干",
				AnswerText:   tt.answerText,
			})
			if err != nil {
				t.Fatalf("CreateQuestion() error = %v", err)
			}
			if question.Status != tt.want {
				t.Fatalf("CreateQuestion() status = %q, want %q", question.Status, tt.want)
			}
		})
	}
}

func TestCreateQuestionKeepsStructurallyInvalidChoiceDraft(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	service := newQuestionStatusService(repository)
	question, err := service.CreateQuestion(questionStatusContext(), "kb-1", "set-1", &types.CreateQuestionRequest{
		QuestionType: string(types.QuestionTypeSingleChoice),
		StemText:     "题干",
		AnswerText:   "A",
		QuestionBody: types.JSON(`{}`),
		AnswerBody:   types.JSON(`{"selected_index":0}`),
	})
	if err != nil {
		t.Fatalf("CreateQuestion() error = %v", err)
	}
	if question.Status != types.QuestionStatusDraft {
		t.Fatalf("CreateQuestion() status = %q, want %q", question.Status, types.QuestionStatusDraft)
	}
}

func TestImportQuestionsUsesReviewValidationAndKeepsCountLast(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	service := newQuestionStatusService(repository)
	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "完整题", AnswerText: "答案"},
			{LineNumber: 2, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "缺答案题"},
			{
				LineNumber: 3, QuestionType: string(types.QuestionTypeSingleChoice), StemText: "结构不完整的选择题", AnswerText: "A",
				QuestionBody: types.JSON(`{}`), AnswerBody: types.JSON(`{"selected_index":0}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	if result.Created != 3 || len(repository.createdQuestions) != 3 {
		t.Fatalf("ImportQuestions() created = %d, stored = %d", result.Created, len(repository.createdQuestions))
	}
	if repository.createdQuestions[0].Status != types.QuestionStatusReviewed {
		t.Fatalf("complete imported question status = %q", repository.createdQuestions[0].Status)
	}
	if repository.createdQuestions[1].Status != types.QuestionStatusDraft {
		t.Fatalf("incomplete imported question status = %q", repository.createdQuestions[1].Status)
	}
	if repository.createdQuestions[2].Status != types.QuestionStatusDraft {
		t.Fatalf("structurally invalid imported question status = %q", repository.createdQuestions[2].Status)
	}
	if repository.fullSetUpdates != 0 {
		t.Fatalf("full question set updates = %d, want 0", repository.fullSetUpdates)
	}
	if got := repository.mutationOrder; len(got) != 2 || got[0] != "source" || got[1] != "count" {
		t.Fatalf("mutation order = %v, want [source count]", got)
	}
}

func TestImportQuestionsPropagatesQuestionSetUpdateErrors(t *testing.T) {
	tests := []struct {
		name            string
		sourceUpdateErr error
		countUpdateErr  error
		wantOrder       []string
	}{
		{name: "source update", sourceUpdateErr: errors.New("source update failed"), wantOrder: []string{"source"}},
		{name: "count update", countUpdateErr: errors.New("count update failed"), wantOrder: []string{"source", "count"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set:             &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
				sourceUpdateErr: tt.sourceUpdateErr,
				countUpdateErr:  tt.countUpdateErr,
			}
			service := newQuestionStatusService(repository)
			_, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
				Items: []types.ImportQuestionItem{{
					LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "题干", AnswerText: "答案",
				}},
			})
			if err == nil {
				t.Fatal("ImportQuestions() error = nil, want update error")
			}
			if len(repository.mutationOrder) != len(tt.wantOrder) {
				t.Fatalf("mutation order = %v, want %v", repository.mutationOrder, tt.wantOrder)
			}
			for i := range tt.wantOrder {
				if repository.mutationOrder[i] != tt.wantOrder[i] {
					t.Fatalf("mutation order = %v, want %v", repository.mutationOrder, tt.wantOrder)
				}
			}
		})
	}
}

func TestUpdateQuestionRecalculatesStatusWithReviewValidation(t *testing.T) {
	tests := []struct {
		name       string
		current    types.QuestionStatus
		answerText string
		want       types.QuestionStatus
	}{
		{name: "complete a draft", current: types.QuestionStatusDraft, answerText: "答案", want: types.QuestionStatusReviewed},
		{name: "remove reviewed answer", current: types.QuestionStatusReviewed, answerText: "", want: types.QuestionStatusDraft},
		{name: "preserve rejected", current: types.QuestionStatusRejected, answerText: "答案", want: types.QuestionStatusRejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				question: &types.Question{
					ID:              "question-1",
					QuestionSetID:   "set-1",
					KnowledgeBaseID: "kb-1",
					QuestionType:    string(types.QuestionTypeShortAnswer),
					StemText:        "题干",
					Status:          tt.current,
				},
			}
			service := newQuestionStatusService(repository)
			question, err := service.UpdateQuestion(questionStatusContext(), "kb-1", "set-1", "question-1", &types.UpdateQuestionRequest{
				AnswerText: &tt.answerText,
			})
			if err != nil {
				t.Fatalf("UpdateQuestion() error = %v", err)
			}
			if question.Status != tt.want {
				t.Fatalf("UpdateQuestion() status = %q, want %q", question.Status, tt.want)
			}
		})
	}
}

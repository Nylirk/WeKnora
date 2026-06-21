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
	deletedQuestion  string
	deletedSetID     string
	allQuestions     []*types.Question
}

func (r *questionStatusRepository) CreateQuestionSet(_ context.Context, set *types.QuestionSet) error {
	r.set = set
	return nil
}

func (r *questionStatusRepository) GetQuestionSet(context.Context, uint64, string) (*types.QuestionSet, error) {
	return r.set, nil
}

func (r *questionStatusRepository) GetQuestionSetByName(context.Context, uint64, string, string) (*types.QuestionSet, error) {
	return nil, nil
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

func (r *questionStatusRepository) DeleteQuestion(_ context.Context, _ uint64, _ string, questionID string) error {
	r.deletedQuestion = questionID
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

func (r *questionStatusRepository) DeleteQuestionSet(_ context.Context, _ uint64, id string) error {
	r.deletedSetID = id
	return nil
}

func (r *questionStatusRepository) ListQuestions(_ context.Context, _ uint64, _ string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	filtered := r.allQuestions
	if filter != nil && filter.Status != "" {
		filtered = nil
		for _, q := range r.allQuestions {
			if string(q.Status) == filter.Status {
				filtered = append(filtered, q)
			}
		}
	}
	if page.Page > 1 {
		return types.NewPageResult(int64(len(filtered)), page, []*types.Question{}), nil
	}
	return types.NewPageResult(int64(len(filtered)), page, filtered), nil
}

type questionStatusKBService struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *questionStatusKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	if s.kb != nil {
		return s.kb, nil
	}
	return &types.KnowledgeBase{ID: "kb-1", Type: types.KnowledgeBaseTypeQuestionBank}, nil
}

type questionStatusIndexService struct {
	interfaces.QuestionIndexService
	indexed    [][]*types.Question
	deleted    [][]string
	indexError error
}

func (s *questionStatusIndexService) IndexQuestions(_ context.Context, questions []*types.Question) error {
	s.indexed = append(s.indexed, questions)
	return s.indexError
}

func (s *questionStatusIndexService) DeleteQuestionIndexes(_ context.Context, questionIDs []string) error {
	s.deleted = append(s.deleted, questionIDs)
	return nil
}

func newQuestionStatusService(repository *questionStatusRepository) *QuestionService {
	return &QuestionService{
		repository:       repository,
		knowledgeBaseSvc: &questionStatusKBService{},
	}
}

func newQuestionStatusServiceWithIndex(
	repository *questionStatusRepository,
	indexService interfaces.QuestionIndexService,
) *QuestionService {
	service := newQuestionStatusService(repository)
	service.questionIndexService = indexService
	return service
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

func TestCreateQuestion_AlwaysDraft(t *testing.T) {
	tests := []struct {
		name       string
		answerText string
	}{
		{name: "complete manual question", answerText: "答案"},
		{name: "incomplete manual question", answerText: ""},
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
			// All manually created questions must be draft regardless of completeness.
			if question.Status != types.QuestionStatusDraft {
				t.Fatalf("CreateQuestion() status = %q, want draft", question.Status)
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
	// All imported questions must be draft regardless of completeness.
	for i, q := range repository.createdQuestions {
		if q.Status != types.QuestionStatusDraft {
			t.Fatalf("imported question[%d] status = %q, want draft", i, q.Status)
		}
	}
	// We now update the question set's processing stage synchronously during import,
	// so full set updates are expected.
	if repository.fullSetUpdates < 1 {
		t.Fatalf("full question set updates = %d, want >= 1 (processing stage update)", repository.fullSetUpdates)
	}
	if got := repository.mutationOrder; len(got) < 2 || got[0] != "source" || got[1] != "count" {
		t.Fatalf("mutation order first two = %v, want [source count ...]", got)
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

func TestUpdateQuestion_DoesNotChangeReviewStatus(t *testing.T) {
	tests := []struct {
		name       string
		current    types.QuestionStatus
		answerText string
	}{
		{name: "draft stays draft after edit", current: types.QuestionStatusDraft, answerText: "答案"},
		{name: "reviewed stays reviewed after edit", current: types.QuestionStatusReviewed, answerText: ""},
		{name: "rejected stays rejected after edit", current: types.QuestionStatusRejected, answerText: "答案"},
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
			if question.Status != tt.current {
				t.Fatalf("UpdateQuestion() status = %q, want %q (must preserve original)", question.Status, tt.current)
			}
		})
	}
}

func TestImportQuestionsRejectsInvalidStatus(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	service := newQuestionStatusService(repository)
	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "题干", AnswerText: "答案", Status: "not_a_real_status"},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	// With forced-draft policy, all items that pass draft validation are created
	// regardless of caller-supplied status. Unknown status values are ignored.
	if result.Created != 1 {
		t.Fatalf("ImportQuestions() created = %d, want 1 (forced draft)", result.Created)
	}
}

func TestImportQuestionsStatusValidation(t *testing.T) {
	tests := []struct {
		name string
		item types.ImportQuestionItem
		want types.QuestionStatus
	}{
		{
			name: "valid question with reviewed forced to draft",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer),
				StemText: "题干", AnswerText: "答案", Status: "reviewed",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "blank answer reviewed stays draft",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer),
				StemText: "题干", AnswerText: "", Status: "reviewed",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "invalid choice with reviewed stays draft",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeSingleChoice),
				StemText: "题干", AnswerText: "A",
				QuestionBody: types.JSON(`{}`), AnswerBody: types.JSON(`{"selected_index":0}`),
				Status: "reviewed",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "explicit draft stays draft",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer),
				StemText: "题干", AnswerText: "答案", Status: "draft",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "explicit rejected forced to draft",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer),
				StemText: "题干", AnswerText: "答案", Status: "rejected",
			},
			want: types.QuestionStatusDraft,
		},
		{
			name: "no status auto-determines draft for valid",
			item: types.ImportQuestionItem{
				LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer),
				StemText: "题干", AnswerText: "答案",
			},
			want: types.QuestionStatusDraft,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
			service := newQuestionStatusService(repository)
			result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
				Items: []types.ImportQuestionItem{tt.item},
			})
			if err != nil {
				t.Fatalf("ImportQuestions() error = %v", err)
			}
			if result.Created != 1 {
				t.Fatalf("ImportQuestions() created = %d, want 1", result.Created)
			}
			if repository.createdQuestions[0].Status != tt.want {
				t.Fatalf("ImportQuestions() status = %q, want %q", repository.createdQuestions[0].Status, tt.want)
			}
		})
	}
}

// TestImportQuestionsAllDraft verifies that ALL imported questions enter draft status,
// regardless of their structural completeness. This is the new behavior where only
// human review can advance questions to reviewed.
func TestImportQuestionsAllDraft(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	service := newQuestionStatusService(repository)
	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "完整题", AnswerText: "答案", Status: "reviewed"},
			{LineNumber: 2, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "缺答案题"},
			{LineNumber: 3, QuestionType: string(types.QuestionTypeSingleChoice), StemText: "结构不完整题", AnswerText: "A", QuestionBody: types.JSON(`{}`), AnswerBody: types.JSON(`{"selected_index":0}`), Status: "reviewed"},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	if result.Created != 3 || len(repository.createdQuestions) != 3 {
		t.Fatalf("ImportQuestions() created = %d, stored = %d", result.Created, len(repository.createdQuestions))
	}
	for i, q := range repository.createdQuestions {
		if q.Status != types.QuestionStatusDraft {
			t.Fatalf("imported question[%d] status = %q, want draft", i, q.Status)
		}
	}
}

// TestImportQuestionsTriggersProcessingPipeline verifies that importing questions
// updates the question set's processing stage to draft_imported.
func TestImportQuestionsTriggersProcessingStage(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	service := newQuestionStatusService(repository)
	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{
			{LineNumber: 1, QuestionType: string(types.QuestionTypeShortAnswer), StemText: "题干", AnswerText: "答案"},
		},
	})
	if err != nil {
		t.Fatalf("ImportQuestions() error = %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("ImportQuestions() created = %d, want 1", result.Created)
	}
	// After synchronous part of import, the set should be in draft_imported stage.
	// The full processing pipeline runs async in a goroutine.
	if repository.set.ProcessingStage != types.QuestionSetProcessingStageDraftImported {
		t.Fatalf("ProcessingStage = %q, want %q", repository.set.ProcessingStage, types.QuestionSetProcessingStageDraftImported)
	}
}

// TestQuestionBankConfigHelper verifies the KB-level QuestionBankConfig helpers.
func TestQuestionBankConfigHelper(t *testing.T) {
	tests := []struct {
		name             string
		knowledgePointID string
		syllabusID       string
		wantAutoTag      bool
		wantSyllabus     bool
	}{
		{name: "both empty", knowledgePointID: "", syllabusID: "", wantAutoTag: false, wantSyllabus: false},
		{name: "knowledge point only", knowledgePointID: "kp-kb-1", syllabusID: "", wantAutoTag: true, wantSyllabus: false},
		{name: "syllabus only", knowledgePointID: "", syllabusID: "syl-kb-1", wantAutoTag: false, wantSyllabus: true},
		{name: "both configured", knowledgePointID: "kp-kb-1", syllabusID: "syl-kb-1", wantAutoTag: true, wantSyllabus: true},
		{name: "nil config", knowledgePointID: "", syllabusID: "", wantAutoTag: false, wantSyllabus: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *types.QuestionBankConfig
			if tt.name != "nil config" {
				cfg = &types.QuestionBankConfig{
					KnowledgePointKnowledgeBaseID: tt.knowledgePointID,
					SyllabusKnowledgeBaseID:       tt.syllabusID,
				}
			}
			if cfg.AutoKnowledgePointEnabled() != tt.wantAutoTag {
				t.Fatalf("AutoKnowledgePointEnabled() = %v, want %v", cfg.AutoKnowledgePointEnabled(), tt.wantAutoTag)
			}
			if cfg.AutoSyllabusCheckEnabled() != tt.wantSyllabus {
				t.Fatalf("AutoSyllabusCheckEnabled() = %v, want %v", cfg.AutoSyllabusCheckEnabled(), tt.wantSyllabus)
			}
		})
	}
}

// TestGetQuestionSetProcessingStatusSkippedReasons verifies skipped reasons
// are returned from the parent KB config.
func TestGetQuestionSetProcessingStatusSkippedReasons(t *testing.T) {
	tests := []struct {
		name                string
		knowledgePointID    string
		syllabusID          string
		wantAutoTagSkipped  bool
		wantSyllabusSkipped bool
	}{
		{
			name:                "empty config shows all skipped reasons",
			wantAutoTagSkipped:  true,
			wantSyllabusSkipped: true,
		},
		{
			name:                "knowledge point configured removes tag skip",
			knowledgePointID:    "kp-kb-1",
			wantAutoTagSkipped:  false,
			wantSyllabusSkipped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &questionStatusRepository{
				set: &types.QuestionSet{
					ID:              "set-1",
					KnowledgeBaseID: "kb-1",
					ProcessingStage: types.QuestionSetProcessingStageIdle,
				},
			}
			kb := &types.KnowledgeBase{
				ID:   "kb-1",
				Type: types.KnowledgeBaseTypeQuestionBank,
				QuestionBankConfig: &types.QuestionBankConfig{
					KnowledgePointKnowledgeBaseID: tt.knowledgePointID,
					SyllabusKnowledgeBaseID:       tt.syllabusID,
				},
			}
			kbService := &questionStatusKBService{kb: kb}
			service := &QuestionService{
				repository:       repository,
				knowledgeBaseSvc: kbService,
			}

			status, err := service.GetQuestionSetProcessingStatus(questionStatusContext(), "kb-1", "set-1")
			if err != nil {
				t.Fatalf("GetQuestionSetProcessingStatus() error = %v", err)
			}
			if tt.wantAutoTagSkipped && status.SkippedAutoTaggingReason == "" {
				t.Fatal("expected skipped auto tagging reason, got empty")
			}
			if !tt.wantAutoTagSkipped && status.SkippedAutoTaggingReason != "" {
				t.Fatalf("expected no skipped auto tagging reason, got %q", status.SkippedAutoTaggingReason)
			}
			if tt.wantSyllabusSkipped && status.SkippedSyllabusReason == "" {
				t.Fatal("expected skipped syllabus reason, got empty")
			}
			if !tt.wantSyllabusSkipped && status.SkippedSyllabusReason != "" {
				t.Fatalf("expected no skipped syllabus reason, got %q", status.SkippedSyllabusReason)
			}
		})
	}
}

// TestSearchDoesNotIncludeDraftByDefault verifies that the existing question search
// (via ListQuestions with status filter) only returns reviewed when requested.
func TestSearchDoesNotIncludeDraftByDefault(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{
			ID:              "set-1",
			KnowledgeBaseID: "kb-1",
		},
		allQuestions: []*types.Question{
			{ID: "q-1", Status: types.QuestionStatusDraft, QuestionSetID: "set-1", KnowledgeBaseID: "kb-1"},
			{ID: "q-2", Status: types.QuestionStatusReviewed, QuestionSetID: "set-1", KnowledgeBaseID: "kb-1"},
		},
	}
	service := newQuestionStatusService(repository)

	// List with reviewed filter should only return reviewed
	filter := &types.QuestionListFilter{Status: string(types.QuestionStatusReviewed)}
	result, err := service.ListQuestions(questionStatusContext(), "kb-1", "set-1", filter, &types.Pagination{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListQuestions() error = %v", err)
	}
	questions := result.Data.([]*types.Question)
	if len(questions) != 1 || questions[0].ID != "q-2" {
		t.Fatalf("expected 1 reviewed question, got %d: %+v", len(questions), questions)
	}

	// List with draft filter should only return draft
	filter = &types.QuestionListFilter{Status: string(types.QuestionStatusDraft)}
	result, err = service.ListQuestions(questionStatusContext(), "kb-1", "set-1", filter, &types.Pagination{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListQuestions() error = %v", err)
	}
	questions = result.Data.([]*types.Question)
	if len(questions) != 1 || questions[0].ID != "q-1" {
		t.Fatalf("expected 1 draft question, got %d: %+v", len(questions), questions)
	}
}

func TestQuestionMutationsScheduleDerivedIndexWithoutChangingSuccess(t *testing.T) {
	repository := &questionStatusRepository{
		set:      &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		question: &types.Question{ID: "q-1", TenantID: 1, QuestionSetID: "set-1", KnowledgeBaseID: "kb-1"},
	}
	indexService := &questionStatusIndexService{indexError: errors.New("index unavailable")}
	service := newQuestionStatusServiceWithIndex(repository, indexService)

	result, err := service.ImportQuestions(questionStatusContext(), "kb-1", "set-1", &types.ImportQuestionsRequest{
		Items: []types.ImportQuestionItem{{
			QuestionType: string(types.QuestionTypeShortAnswer), StemText: "题干", AnswerText: "答案",
		}},
	})
	if err != nil || result.Created != 1 {
		t.Fatalf("ImportQuestions() result=%+v error=%v", result, err)
	}
	if len(indexService.indexed) != 1 || len(indexService.indexed[0]) != 1 {
		t.Fatalf("index calls = %+v", indexService.indexed)
	}

	if err := service.DeleteQuestion(questionStatusContext(), "kb-1", "set-1", "q-1"); err != nil {
		t.Fatalf("DeleteQuestion() error = %v", err)
	}
	if len(indexService.deleted) != 1 || len(indexService.deleted[0]) != 1 || indexService.deleted[0][0] != "q-1" {
		t.Fatalf("delete index calls = %+v", indexService.deleted)
	}
}

func TestCreateAndRelevantUpdateScheduleQuestionIndex(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	indexService := &questionStatusIndexService{indexError: errors.New("index unavailable")}
	service := newQuestionStatusServiceWithIndex(repository, indexService)

	created, err := service.CreateQuestion(questionStatusContext(), "kb-1", "set-1", &types.CreateQuestionRequest{
		QuestionType: string(types.QuestionTypeShortAnswer), StemText: "原题干", AnswerText: "答案",
	})
	if err != nil || created == nil {
		t.Fatalf("CreateQuestion() question=%+v error=%v", created, err)
	}
	if len(indexService.indexed) != 1 {
		t.Fatalf("create index calls = %d", len(indexService.indexed))
	}

	repository.question = &types.Question{
		ID: "q-1", TenantID: 1, QuestionSetID: "set-1", KnowledgeBaseID: "kb-1",
		QuestionType: string(types.QuestionTypeShortAnswer), StemText: "原题干", Status: types.QuestionStatusDraft,
	}
	answer := "新答案"
	if _, err := service.UpdateQuestion(questionStatusContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionRequest{
		AnswerText: &answer,
	}); err != nil {
		t.Fatalf("answer-only UpdateQuestion() error = %v", err)
	}
	// Status stays draft (no auto-review). Answer text is not part of index content.
		// So no index scheduling is triggered by an answer-only update.
		if len(indexService.indexed) != 1 {
			t.Fatalf("answer-only update index calls = %d, want 1 (status unchanged, answer not in index)", len(indexService.indexed))
		}
	secondAnswer := "另一个答案"
	if _, err := service.UpdateQuestion(questionStatusContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionRequest{
		AnswerText: &secondAnswer,
	}); err != nil {
		t.Fatalf("answer-only reviewed UpdateQuestion() error = %v", err)
	}
	if len(indexService.indexed) != 1 {
			t.Fatalf("second answer-only update index calls = %d, want 1", len(indexService.indexed))
		}

	newStem := "新题干"
	if _, err := service.UpdateQuestion(questionStatusContext(), "kb-1", "set-1", "q-1", &types.UpdateQuestionRequest{
		StemText: &newStem,
	}); err != nil {
		t.Fatalf("stem UpdateQuestion() error = %v", err)
	}
	if len(indexService.indexed) != 2 {
			t.Fatalf("stem update index calls = %d, want 2", len(indexService.indexed))
		}
}

func TestDeleteQuestionSetCleansUpVectorIndexes(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
		allQuestions: []*types.Question{
			{ID: "q-1", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1"},
			{ID: "q-2", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1"},
			{ID: "q-3", TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1"},
		},
	}
	indexService := &questionStatusIndexService{}
	service := newQuestionStatusServiceWithIndex(repository, indexService)

	if err := service.DeleteQuestionSet(questionStatusContext(), "kb-1", "set-1"); err != nil {
		t.Fatalf("DeleteQuestionSet() error = %v", err)
	}
	if repository.deletedSetID != "set-1" {
		t.Fatalf("DeleteQuestionSet not called on repository: deletedSetID=%q", repository.deletedSetID)
	}
	if len(indexService.deleted) != 1 {
		t.Fatalf("expected 1 DeleteQuestionIndexes call, got %d", len(indexService.deleted))
	}
	deletedIDs := indexService.deleted[0]
	if len(deletedIDs) != 3 {
		t.Fatalf("expected 3 question IDs deleted, got %d: %v", len(deletedIDs), deletedIDs)
	}
	for _, want := range []string{"q-1", "q-2", "q-3"} {
		found := false
		for _, got := range deletedIDs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected question ID %q in deleted IDs: %v", want, deletedIDs)
		}
	}
	// Only the vector index side-effect should be scheduled; no IndexQuestions
	// call because the questions are already gone from the DB.
	if len(indexService.indexed) != 0 {
		t.Fatalf("unexpected IndexQuestions call: indexed=%d", len(indexService.indexed))
	}
}

func TestDeleteQuestionSetNoQuestionsStillDeletesSet(t *testing.T) {
	repository := &questionStatusRepository{
		set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"},
	}
	indexService := &questionStatusIndexService{}
	service := newQuestionStatusServiceWithIndex(repository, indexService)

	if err := service.DeleteQuestionSet(questionStatusContext(), "kb-1", "set-1"); err != nil {
		t.Fatalf("DeleteQuestionSet() error = %v", err)
	}
	if repository.deletedSetID != "set-1" {
		t.Fatalf("DeleteQuestionSet not called on repository: deletedSetID=%q", repository.deletedSetID)
	}
	// No questions in the set → no DeleteQuestionIndexes call.
	if len(indexService.deleted) != 0 {
		t.Fatalf("expected 0 DeleteQuestionIndexes calls, got %d", len(indexService.deleted))
	}
}

func TestQuestionPreviewAndParseFlowsDoNotScheduleIndexing(t *testing.T) {
	repository := &questionStatusRepository{set: &types.QuestionSet{ID: "set-1", KnowledgeBaseID: "kb-1"}}
	indexService := &questionStatusIndexService{}
	service := newQuestionStatusServiceWithIndex(repository, indexService)

	_, _ = service.PreviewImportQuestionsFromFile(
		questionStatusContext(), "kb-1", "set-1", nil, "questions.txt", &types.ImportFilePreviewRequest{},
	)
	_, _ = service.PreviewImportBlocks(
		questionStatusContext(), "kb-1", "set-1", nil, "questions.txt", &types.BlockPreviewRequest{},
	)
	_, err := service.ParseImportedBlocks(
		questionStatusContext(), "kb-1", "set-1", &types.ParseBlocksRequest{StrategyPreset: "general"},
	)
	if err != nil {
		t.Fatalf("ParseImportedBlocks() error = %v", err)
	}
	if len(indexService.indexed) != 0 || len(indexService.deleted) != 0 {
		t.Fatalf("preview/parse caused index side effects: indexed=%d deleted=%d", len(indexService.indexed), len(indexService.deleted))
	}
}

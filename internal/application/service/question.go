package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type QuestionService struct {
	repository         interfaces.QuestionRepository
	evaluationService  interfaces.EvaluationService
	evaluationRepo     interfaces.EvaluationRepository
	knowledgeBaseSvc   interfaces.KnowledgeBaseService
}

func NewQuestionService(
	repo interfaces.QuestionRepository,
	evalSvc interfaces.EvaluationService,
	evalRepo interfaces.EvaluationRepository,
	kbSvc interfaces.KnowledgeBaseService,
) interfaces.QuestionService {
	return &QuestionService{
		repository:        repo,
		evaluationService: evalSvc,
		evaluationRepo:   evalRepo,
		knowledgeBaseSvc: kbSvc,
	}
}

func (s *QuestionService) CreateQuestionSet(ctx context.Context, req *types.CreateQuestionSetRequest) (*types.QuestionSet, error) {
	if _, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, req.KnowledgeBaseID); err != nil {
		return nil, fmt.Errorf("knowledge base: %w", err)
	}
	qs := &types.QuestionSet{
		TenantID:        tenantID(ctx),
		KnowledgeBaseID: req.KnowledgeBaseID,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		SourceType:      types.QuestionSetSourceManual,
		Status:          types.QuestionSetStatusActive,
		GenerationConfig: normalizeJSONMap(nil),
		Metadata:         normalizeJSONMap(nil),
	}
	if err := s.repository.CreateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) GetQuestionSet(ctx context.Context, id string) (*types.QuestionSet, error) {
	return s.repository.GetQuestionSet(ctx, tenantID(ctx), id)
}

func (s *QuestionService) ListQuestionSets(ctx context.Context, kbID string, page *types.Pagination) (*types.PageResult, error) {
	return s.repository.ListQuestionSets(ctx, tenantID(ctx), kbID, page)
}

func (s *QuestionService) UpdateQuestionSet(ctx context.Context, id string, req *types.UpdateQuestionSetRequest) (*types.QuestionSet, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			return nil, fmt.Errorf("name is required")
		}
		qs.Name = v
	}
	if req.Description != nil {
		qs.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		qs.Status = types.QuestionSetStatus(*req.Status)
	}
	if err := s.repository.UpdateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) DeleteQuestionSet(ctx context.Context, id string) error {
	if _, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), id); err != nil {
		return err
	}
	return s.repository.DeleteQuestionSet(ctx, tenantID(ctx), id)
}

func (s *QuestionService) CreateQuestion(ctx context.Context, setID string, req *types.CreateQuestionRequest) (*types.Question, error) {
	if _, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID); err != nil {
		return nil, err
	}
	q := &types.Question{
		TenantID:          tenantID(ctx),
		QuestionSetID:    setID,
		QuestionType:      req.QuestionType,
		StemText:          strings.TrimSpace(req.StemText),
		QuestionBody:      normalizeJSONObject(req.QuestionBody),
		AnswerText:        strings.TrimSpace(req.AnswerText),
		AnswerBody:        normalizeJSONObject(req.AnswerBody),
		AnalysisText:      strings.TrimSpace(req.AnalysisText),
		GradingRubric:     normalizeJSONObject(req.GradingRubric),
		Difficulty:        types.QuestionDifficulty(req.Difficulty),
		Status:            types.QuestionStatusDraft,
		KnowledgePoints:   normalizeJSONArray(req.KnowledgePoints),
		Tags:              normalizeJSONArray(req.Tags),
		SourceKnowledgeID: req.SourceKnowledgeID,
		EvidenceChunkIDs:  normalizeJSONArray(req.EvidenceChunkIDs),
		SourcePayload:     normalizeJSONMap(nil),
		ExtractionMetadata: normalizeJSONMap(nil),
		SortOrder:         req.SortOrder,
	}
	if q.QuestionType == "" {
		q.QuestionType = string(types.QuestionTypeSingleChoice)
	}
	if q.Difficulty == "" {
		q.Difficulty = types.QuestionDifficultyMedium
	}
	qs := &types.Question{QuestionType: q.QuestionType, StemText: q.StemText, QuestionBody: q.QuestionBody, AnswerBody: q.AnswerBody}
	if errs := types.ValidateQuestionForDraft(qs); len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", errs[0].Message)
	}
	if err := s.repository.CreateQuestion(ctx, q); err != nil {
		return nil, err
	}
	_ = s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID)
	return q, nil
}

func (s *QuestionService) GetQuestion(ctx context.Context, setID, id string) (*types.Question, error) {
	return s.repository.GetQuestion(ctx, tenantID(ctx), setID, id)
}

func (s *QuestionService) ListQuestions(ctx context.Context, setID string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	if _, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID); err != nil {
		return nil, err
	}
	return s.repository.ListQuestions(ctx, tenantID(ctx), setID, filter, page)
}

func (s *QuestionService) UpdateQuestion(ctx context.Context, setID, id string, req *types.UpdateQuestionRequest) (*types.Question, error) {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, id)
	if err != nil {
		return nil, err
	}
	if req.QuestionType != nil {
		q.QuestionType = *req.QuestionType
	}
	if req.StemText != nil {
		q.StemText = strings.TrimSpace(*req.StemText)
	}
	if req.QuestionBody != nil {
		q.QuestionBody = normalizeJSONObject(*req.QuestionBody)
	}
	if req.AnswerText != nil {
		q.AnswerText = strings.TrimSpace(*req.AnswerText)
	}
	if req.AnswerBody != nil {
		q.AnswerBody = normalizeJSONObject(*req.AnswerBody)
	}
	if req.AnalysisText != nil {
		q.AnalysisText = strings.TrimSpace(*req.AnalysisText)
	}
	if req.GradingRubric != nil {
		q.GradingRubric = normalizeJSONObject(*req.GradingRubric)
	}
	if req.Difficulty != nil {
		q.Difficulty = types.QuestionDifficulty(*req.Difficulty)
	}
	if req.KnowledgePoints != nil {
		q.KnowledgePoints = normalizeJSONArray(*req.KnowledgePoints)
	}
	if req.Tags != nil {
		q.Tags = normalizeJSONArray(*req.Tags)
	}
	if req.SourceKnowledgeID != nil {
		q.SourceKnowledgeID = *req.SourceKnowledgeID
	}
	if req.EvidenceChunkIDs != nil {
		q.EvidenceChunkIDs = normalizeJSONArray(*req.EvidenceChunkIDs)
	}
	if req.SortOrder != nil {
		q.SortOrder = *req.SortOrder
	}
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QuestionService) DeleteQuestion(ctx context.Context, setID, id string) error {
	if _, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, id); err != nil {
		return err
	}
	if err := s.repository.DeleteQuestion(ctx, tenantID(ctx), setID, id); err != nil {
		return err
	}
	_ = s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID)
	return nil
}

func (s *QuestionService) UpdateQuestionStatus(ctx context.Context, setID, id string, req *types.UpdateQuestionStatusRequest) (*types.Question, error) {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, id)
	if err != nil {
		return nil, err
	}
	newStatus := types.QuestionStatus(req.Status)
	if newStatus == types.QuestionStatusReviewed {
		errs := types.ValidateQuestionForReview(q)
		if len(errs) > 0 {
			messages := make([]string, 0, len(errs))
			for _, e := range errs {
				messages = append(messages, e.Message)
			}
			return nil, fmt.Errorf("review validation failed: %s", strings.Join(messages, "; "))
		}
	}
	q.Status = newStatus
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QuestionService) ImportQuestions(ctx context.Context, setID string, req *types.ImportQuestionsRequest) (*types.ImportQuestionsResult, error) {
	if _, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID); err != nil {
		return nil, err
	}
	result := &types.ImportQuestionsResult{}
	var created []*types.Question
	for _, item := range req.Items {
		q := &types.Question{
			TenantID:          tenantID(ctx),
			QuestionSetID:    setID,
			QuestionType:      item.QuestionType,
			StemText:          strings.TrimSpace(item.StemText),
			QuestionBody:      normalizeJSONObject(item.QuestionBody),
			AnswerText:        strings.TrimSpace(item.AnswerText),
			AnswerBody:        normalizeJSONObject(item.AnswerBody),
			AnalysisText:      strings.TrimSpace(item.AnalysisText),
			GradingRubric:     normalizeJSONObject(item.GradingRubric),
			Difficulty:        types.QuestionDifficulty(item.Difficulty),
			Status:            types.QuestionStatusDraft,
			KnowledgePoints:   normalizeJSONArray(item.KnowledgePoints),
			Tags:              normalizeJSONArray(item.Tags),
			SourceKnowledgeID: item.SourceKnowledgeID,
			EvidenceChunkIDs:  normalizeJSONArray(item.EvidenceChunkIDs),
			SourcePayload:     normalizeJSONMap(nil),
			ExtractionMetadata: normalizeJSONMap(nil),
		}
		if q.QuestionType == "" {
			q.QuestionType = string(types.QuestionTypeSingleChoice)
		}
		if q.Difficulty == "" {
			q.Difficulty = types.QuestionDifficultyMedium
		}
		draftQ := &types.Question{QuestionType: q.QuestionType, StemText: q.StemText}
		if errs := types.ValidateQuestionForDraft(draftQ); len(errs) > 0 {
			for _, e := range errs {
				result.Errors = append(result.Errors, types.ImportQuestionError{
					LineNumber: item.LineNumber,
					Message:    e.Message,
				})
			}
			continue
		}
		created = append(created, q)
	}
	if len(created) > 0 {
		if err := s.repository.CreateQuestions(ctx, created); err != nil {
			return nil, err
		}
		result.Created = len(created)
	}
	_ = s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID)
	if set, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID); err == nil {
		set.SourceType = types.QuestionSetSourceImported
		_ = s.repository.UpdateQuestionSet(ctx, set)
	}
	return result, nil
}

func (s *QuestionService) ExportToEvaluationDataset(ctx context.Context, setID string, req *types.ExportToEvaluationRequest) (*types.EvaluationDataset, error) {
	if _, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID); err != nil {
		return nil, err
	}
	filter := &types.QuestionListFilter{Status: string(types.QuestionStatusReviewed)}
	pageSize := 1000
	page := &types.Pagination{Page: 1, PageSize: pageSize}
	allQuestions, err := s.listAllReviewedQuestions(ctx, setID, filter, page)
	if err != nil {
		return nil, err
	}
	if len(allQuestions) == 0 {
		return nil, fmt.Errorf("no reviewed questions found for export")
	}
	evalReq := &types.CreateEvaluationDatasetRequest{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	}
	dataset, err := s.evaluationService.CreateDataset(ctx, evalReq)
	if err != nil {
		return nil, err
	}
	samples := make([]*types.EvaluationSample, 0, len(allQuestions))
	for _, q := range allQuestions {
		if errs := types.ValidateQuestionForExport(q); len(errs) > 0 {
			_ = s.evaluationService.DeleteDataset(ctx, dataset.ID)
			return nil, fmt.Errorf("question %s export validation failed: %s", q.ID, errs[0].Message)
		}
		contexts := buildReferenceContexts(q)
		refCtx, err := jsonValue(contexts)
		if err != nil {
			_ = s.evaluationService.DeleteDataset(ctx, dataset.ID)
			return nil, fmt.Errorf("failed to serialize reference contexts: %w", err)
		}
		samples = append(samples, &types.EvaluationSample{
			TenantID:          tenantID(ctx),
			DatasetID:         dataset.ID,
			Question:          q.StemText,
			ReferenceAnswer:   q.AnswerText,
			ReferenceContexts: refCtx,
		})
	}
	if err := s.evaluationRepo.CreateSamples(ctx, samples); err != nil {
		_ = s.evaluationService.DeleteDataset(ctx, dataset.ID)
		return nil, fmt.Errorf("failed to create evaluation samples: %w", err)
	}
	dataset.SampleCount = len(samples)
	return dataset, nil
}

func (s *QuestionService) GenerateQuestions(ctx context.Context, kbID string, req *types.GenerateQuestionsRequest) (*types.QuestionSet, error) {
	if _, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, req.KnowledgeBaseID); err != nil {
		return nil, fmt.Errorf("knowledge base: %w", err)
	}
	genConfig := normalizeJSONObject(req.GenerationConfig)
	qs := &types.QuestionSet{
		TenantID:         tenantID(ctx),
		KnowledgeBaseID: req.KnowledgeBaseID,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		SourceType:      types.QuestionSetSourceGenerated,
		Status:          types.QuestionSetStatusPending,
		GenerationConfig: genConfig,
		Metadata:         normalizeJSONMap(nil),
	}
	if err := s.repository.CreateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) listAllReviewedQuestions(ctx context.Context, setID string, filter *types.QuestionListFilter, page *types.Pagination) ([]*types.Question, error) {
	result, err := s.repository.ListQuestions(ctx, tenantID(ctx), setID, filter, page)
	if err != nil {
		return nil, err
	}
	return result.Data.([]*types.Question), nil
}

func buildReferenceContexts(q *types.Question) []types.EvaluationReferenceContext {
	var contexts []types.EvaluationReferenceContext
	var chunkIDs []string
	_ = json.Unmarshal(q.EvidenceChunkIDs, &chunkIDs)
	for _, chunkID := range chunkIDs {
		contexts = append(contexts, types.EvaluationReferenceContext{
			ChunkID: chunkID,
		})
	}
	if len(contexts) == 0 && q.AnalysisText != "" {
		contexts = append(contexts, types.EvaluationReferenceContext{
			Text: q.AnalysisText,
		})
	}
	if len(contexts) == 0 {
		var answerBody map[string]interface{}
		if err := json.Unmarshal(q.AnswerBody, &answerBody); err == nil {
			if explanation, ok := answerBody["explanation"].(string); ok && explanation != "" {
				contexts = append(contexts, types.EvaluationReferenceContext{
					Text: explanation,
				})
			}
		}
	}
	return contexts
}

func normalizeJSONObject(val types.JSON) types.JSON {
	if len(val) == 0 {
		return types.JSON([]byte("{}"))
	}
	return val
}

func normalizeJSONArray(val types.JSON) types.JSON {
	if len(val) == 0 {
		return types.JSON([]byte("[]"))
	}
	return val
}

func normalizeJSONMap(val map[string]interface{}) types.JSON {
	if val == nil || len(val) == 0 {
		return types.JSON([]byte("{}"))
	}
	data, err := json.Marshal(val)
	if err != nil {
		return types.JSON([]byte("{}"))
	}
	return types.JSON(data)
}


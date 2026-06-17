package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type QuestionService struct {
	repository       interfaces.QuestionRepository
	evaluationService interfaces.EvaluationService
	evaluationRepo   interfaces.EvaluationRepository
	knowledgeBaseSvc interfaces.KnowledgeBaseService
	chunkService     interfaces.ChunkService
	knowledgeService interfaces.KnowledgeService
}

func NewQuestionService(
	repo interfaces.QuestionRepository,
	evalSvc interfaces.EvaluationService,
	evalRepo interfaces.EvaluationRepository,
	kbSvc interfaces.KnowledgeBaseService,
	chunkSvc interfaces.ChunkService,
	knowledgeSvc interfaces.KnowledgeService,
) interfaces.QuestionService {
	return &QuestionService{
		repository:       repo,
		evaluationService: evalSvc,
		evaluationRepo:  evalRepo,
		knowledgeBaseSvc: kbSvc,
		chunkService:     chunkSvc,
		knowledgeService: knowledgeSvc,
	}
}

func (s *QuestionService) CreateQuestionSet(ctx context.Context, kbID string, req *types.CreateQuestionSetRequest) (*types.QuestionSet, error) {
	if _, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, apperrors.NewBadRequestError("knowledge base: " + err.Error())
	}
	qs := &types.QuestionSet{
		TenantID:         tenantID(ctx),
		KnowledgeBaseID:  kbID,
		Name:             strings.TrimSpace(req.Name),
		Description:      strings.TrimSpace(req.Description),
		SourceType:       types.QuestionSetSourceManual,
		Status:           types.QuestionSetStatusActive,
		GenerationConfig: normalizeJSONObject(nil),
		GenerationScope:  normalizeJSONObject(nil),
	}
	if err := s.repository.CreateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) GetQuestionSet(ctx context.Context, kbID, setID string) (*types.QuestionSet, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	return qs, nil
}

func (s *QuestionService) ListQuestionSets(ctx context.Context, kbID string, page *types.Pagination) (*types.PageResult, error) {
	return s.repository.ListQuestionSets(ctx, tenantID(ctx), kbID, page)
}

func (s *QuestionService) UpdateQuestionSet(ctx context.Context, kbID, setID string, req *types.UpdateQuestionSetRequest) (*types.QuestionSet, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			return nil, apperrors.NewBadRequestError("name is required")
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

func (s *QuestionService) DeleteQuestionSet(ctx context.Context, kbID, setID string) error {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return err
	}
	if qs.KnowledgeBaseID != kbID {
		return apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	return s.repository.DeleteQuestionSet(ctx, tenantID(ctx), setID)
}

func (s *QuestionService) CreateQuestion(ctx context.Context, kbID, setID string, req *types.CreateQuestionRequest) (*types.Question, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	q := &types.Question{
		TenantID:          tenantID(ctx),
		QuestionSetID:     setID,
		KnowledgeBaseID:   qs.KnowledgeBaseID,
		QuestionType:      req.QuestionType,
		StemText:          strings.TrimSpace(req.StemText),
		QuestionBody:      normalizeJSONObject(req.QuestionBody),
		AnswerText:        strings.TrimSpace(req.AnswerText),
		AnswerBody:        normalizeJSONObject(req.AnswerBody),
		AnalysisText:      strings.TrimSpace(req.AnalysisText),
		GradingRubric:     normalizeJSONObject(req.GradingRubric),
		Difficulty:         types.QuestionDifficulty(req.Difficulty),
		Status:            types.QuestionStatusDraft,
		KnowledgePoints:   normalizeJSONArray(req.KnowledgePoints),
		Tags:               normalizeJSONArray(req.Tags),
		SourceKnowledgeID: req.SourceKnowledgeID,
		EvidenceChunkIDs:  normalizeJSONArray(req.EvidenceChunkIDs),
		SourcePayload:      normalizeJSONMap(nil),
		ExtractionMetadata: normalizeJSONMap(nil),
		SortOrder:         req.SortOrder,
	}
	if q.QuestionType == "" {
		q.QuestionType = string(types.QuestionTypeSingleChoice)
	}
	if q.Difficulty == "" {
		q.Difficulty = types.QuestionDifficultyMedium
	}
	draftQ := &types.Question{QuestionType: q.QuestionType, StemText: q.StemText, QuestionBody: q.QuestionBody, AnswerBody: q.AnswerBody}
	if errs := types.ValidateQuestionForDraft(draftQ); len(errs) > 0 {
		return nil, apperrors.NewBadRequestError("validation failed: " + errs[0].Message)
	}
	if err := s.validateEvidenceReferences(ctx, qs.KnowledgeBaseID, req.SourceKnowledgeID, req.EvidenceChunkIDs); err != nil {
		return nil, err
	}
	if err := s.repository.CreateQuestion(ctx, q); err != nil {
		return nil, err
	}
	_ = s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID)
	return q, nil
}

func (s *QuestionService) GetQuestion(ctx context.Context, kbID, setID, questionID string) (*types.Question, error) {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, questionID)
	if err != nil {
		return nil, err
	}
	if q.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question does not belong to knowledge base %s", kbID))
	}
	return q, nil
}

func (s *QuestionService) ListQuestions(ctx context.Context, kbID, setID string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	return s.repository.ListQuestions(ctx, tenantID(ctx), setID, filter, page)
}

func (s *QuestionService) UpdateQuestion(ctx context.Context, kbID, setID, questionID string, req *types.UpdateQuestionRequest) (*types.Question, error) {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, questionID)
	if err != nil {
		return nil, err
	}
	if q.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question does not belong to knowledge base %s", kbID))
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
	if err := s.validateEvidenceReferences(ctx, q.KnowledgeBaseID, q.SourceKnowledgeID, q.EvidenceChunkIDs); err != nil {
		return nil, err
	}
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QuestionService) DeleteQuestion(ctx context.Context, kbID, setID, questionID string) error {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, questionID)
	if err != nil {
		return err
	}
	if q.KnowledgeBaseID != kbID {
		return apperrors.NewBadRequestError(fmt.Sprintf("question does not belong to knowledge base %s", kbID))
	}
	if err := s.repository.DeleteQuestion(ctx, tenantID(ctx), setID, questionID); err != nil {
		return err
	}
	_ = s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID)
	return nil
}

func (s *QuestionService) UpdateQuestionStatus(ctx context.Context, kbID, setID, questionID string, req *types.UpdateQuestionStatusRequest) (*types.Question, error) {
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, questionID)
	if err != nil {
		return nil, err
	}
	if q.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question does not belong to knowledge base %s", kbID))
	}
	newStatus := types.QuestionStatus(req.Status)
	if newStatus == types.QuestionStatusReviewed {
		errs := types.ValidateQuestionForReview(q)
		if len(errs) > 0 {
			messages := make([]string, 0, len(errs))
			for _, e := range errs {
				messages = append(messages, e.Message)
			}
			return nil, apperrors.NewBadRequestError("review validation failed: " + strings.Join(messages, "; "))
		}
	}
	q.Status = newStatus
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QuestionService) ImportQuestions(ctx context.Context, kbID, setID string, req *types.ImportQuestionsRequest) (*types.ImportQuestionsResult, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	result := &types.ImportQuestionsResult{}
	var created []*types.Question
	for _, item := range req.Items {
		q := &types.Question{
			TenantID:          tenantID(ctx),
			QuestionSetID:     setID,
			KnowledgeBaseID:   qs.KnowledgeBaseID,
			QuestionType:      item.QuestionType,
			StemText:          strings.TrimSpace(item.StemText),
			QuestionBody:      normalizeJSONObject(item.QuestionBody),
			AnswerText:        strings.TrimSpace(item.AnswerText),
			AnswerBody:        normalizeJSONObject(item.AnswerBody),
			AnalysisText:      strings.TrimSpace(item.AnalysisText),
			GradingRubric:     normalizeJSONObject(item.GradingRubric),
			Difficulty:         types.QuestionDifficulty(item.Difficulty),
			Status:            types.QuestionStatusDraft,
			KnowledgePoints:   normalizeJSONArray(item.KnowledgePoints),
			Tags:               normalizeJSONArray(item.Tags),
			SourceKnowledgeID: item.SourceKnowledgeID,
			EvidenceChunkIDs:  normalizeJSONArray(item.EvidenceChunkIDs),
			SourcePayload:      normalizeJSONMap(nil),
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
		if err := s.validateEvidenceReferences(ctx, qs.KnowledgeBaseID, item.SourceKnowledgeID, item.EvidenceChunkIDs); err != nil {
			result.Errors = append(result.Errors, types.ImportQuestionError{
				LineNumber: item.LineNumber,
				Message:    err.Error(),
			})
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
	qs.SourceType = types.QuestionSetSourceImport
	_ = s.repository.UpdateQuestionSet(ctx, qs)
	return result, nil
}

func (s *QuestionService) ExportToEvaluationDataset(ctx context.Context, kbID, setID string, req *types.ExportToEvaluationRequest) (*types.EvaluationDataset, error) {
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	filter := &types.QuestionListFilter{Status: string(types.QuestionStatusReviewed)}
	pageSize := 1000
	page := &types.Pagination{Page: 1, PageSize: pageSize}
	result, err := s.repository.ListQuestions(ctx, tenantID(ctx), setID, filter, page)
	if err != nil {
		return nil, err
	}
	allQuestions := result.Data.([]*types.Question)
	if len(allQuestions) == 0 {
		return nil, apperrors.NewBadRequestError("no reviewed questions found for export")
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
			return nil, apperrors.NewBadRequestError(fmt.Sprintf("question %s export validation failed: %s", q.ID, errs[0].Message))
		}
		contexts, err := s.buildReferenceContexts(ctx, q)
		if err != nil {
			_ = s.evaluationService.DeleteDataset(ctx, dataset.ID)
			return nil, apperrors.NewBadRequestError(fmt.Sprintf("failed to build reference contexts for question %s: %s", q.ID, err.Error()))
		}
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
	if _, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, apperrors.NewBadRequestError("knowledge base: " + err.Error())
	}
	genConfig := normalizeJSONObject(req.GenerationConfig)
	genScope := normalizeJSONObject(req.GenerationScope)
	qs := &types.QuestionSet{
		TenantID:         tenantID(ctx),
		KnowledgeBaseID:  kbID,
		Name:             strings.TrimSpace(req.Name),
		Description:      strings.TrimSpace(req.Description),
		SourceType:       types.QuestionSetSourceGenerated,
		Status:           types.QuestionSetStatusPending,
		GenerationConfig: genConfig,
		GenerationScope:  genScope,
	}
	if err := s.repository.CreateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) validateEvidenceReferences(ctx context.Context, kbID, sourceKnowledgeID string, evidenceChunkIDs types.JSON) error {
	if sourceKnowledgeID != "" {
		knowledge, err := s.knowledgeService.GetKnowledgeByID(ctx, sourceKnowledgeID)
		if err != nil {
			return apperrors.NewBadRequestError("source_knowledge_id not found: " + err.Error())
		}
		if knowledge.KnowledgeBaseID != kbID {
			return apperrors.NewBadRequestError(fmt.Sprintf("source_knowledge_id does not belong to knowledge base %s", kbID))
		}
	}
	var chunkIDs []string
	if len(evidenceChunkIDs) > 0 {
		if err := json.Unmarshal(evidenceChunkIDs, &chunkIDs); err != nil {
			return apperrors.NewBadRequestError("invalid evidence_chunk_ids: " + err.Error())
		}
	}
	for _, chunkID := range chunkIDs {
		chunk, err := s.chunkService.GetChunkByIDOnly(ctx, chunkID)
		if err != nil {
			return apperrors.NewBadRequestError(fmt.Sprintf("evidence_chunk_id %s not found", chunkID))
		}
		if chunk.KnowledgeBaseID != kbID {
			return apperrors.NewBadRequestError(fmt.Sprintf("evidence_chunk_id %s does not belong to knowledge base %s", chunkID, kbID))
		}
	}
	return nil
}

func (s *QuestionService) buildReferenceContexts(ctx context.Context, q *types.Question) ([]types.EvaluationReferenceContext, error) {
	var contexts []types.EvaluationReferenceContext
	var chunkIDs []string
	if len(q.EvidenceChunkIDs) > 0 {
		_ = json.Unmarshal(q.EvidenceChunkIDs, &chunkIDs)
	}
	for _, chunkID := range chunkIDs {
		chunk, err := s.chunkService.GetChunkByIDOnly(ctx, chunkID)
		if err != nil {
			continue
		}
		contexts = append(contexts, types.EvaluationReferenceContext{
			Text:        chunk.Content,
			KnowledgeID: chunk.KnowledgeID,
			ChunkID:     chunk.ID,
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
	return contexts, nil
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
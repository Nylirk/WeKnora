package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

type QuestionService struct {
	repository           interfaces.QuestionRepository
	evaluationService    interfaces.EvaluationService
	evaluationRepo       interfaces.EvaluationRepository
	knowledgeBaseSvc     interfaces.KnowledgeBaseService
	chunkService         interfaces.ChunkService
	knowledgeService     interfaces.KnowledgeService
	docReader            interfaces.DocumentReader
	extractionService    *QuestionExtractionService
	blockAnalysisService *BlockAnalysisService
	questionIndexService interfaces.QuestionIndexService
}

func NewQuestionService(
	repo interfaces.QuestionRepository,
	evalSvc interfaces.EvaluationService,
	evalRepo interfaces.EvaluationRepository,
	kbSvc interfaces.KnowledgeBaseService,
	chunkSvc interfaces.ChunkService,
	knowledgeSvc interfaces.KnowledgeService,
	docReader interfaces.DocumentReader,
	extractionSvc *QuestionExtractionService,
	blockAnalysisSvc *BlockAnalysisService,
	questionIndexSvc interfaces.QuestionIndexService,
) interfaces.QuestionService {
	return &QuestionService{
		repository:           repo,
		evaluationService:    evalSvc,
		evaluationRepo:       evalRepo,
		knowledgeBaseSvc:     kbSvc,
		chunkService:         chunkSvc,
		knowledgeService:     knowledgeSvc,
		docReader:            docReader,
		extractionService:    extractionSvc,
		blockAnalysisService: blockAnalysisSvc,
		questionIndexService: questionIndexSvc,
	}
}

func structuredQuestionStatus(q *types.Question) types.QuestionStatus {
	if len(types.ValidateQuestionForReview(q)) > 0 {
		return types.QuestionStatusDraft
	}
	return types.QuestionStatusReviewed
}

func statusAfterStructuredEdit(current types.QuestionStatus, q *types.Question) types.QuestionStatus {
	if current == types.QuestionStatusRejected {
		return current
	}
	return structuredQuestionStatus(q)
}

func (s *QuestionService) CreateQuestionSet(ctx context.Context, kbID string, req *types.CreateQuestionSetRequest) (*types.QuestionSet, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
	processingConfig := normalizeJSONObject(nil)
	if req.ProcessingConfig != nil {
		processingConfig = normalizeProcessingConfig(req.ProcessingConfig)
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
		ProcessingConfig: processingConfig,
		ProcessingStage:  types.QuestionSetProcessingStageIdle,
	}
	if err := s.repository.CreateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) ensureQuestionBankKB(ctx context.Context, kbID string) error {
	kb, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return apperrors.NewBadRequestError("knowledge base: " + err.Error())
	}
	if !kb.IsQuestionBank() {
		return apperrors.NewBadRequestError("question bank APIs only support question_bank knowledge bases")
	}
	return nil
}

func (s *QuestionService) getQuestionSetForKB(ctx context.Context, kbID, setID string) (*types.QuestionSet, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
	qs, err := s.repository.GetQuestionSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return nil, err
	}
	if qs.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question set does not belong to knowledge base %s", kbID))
	}
	return qs, nil
}

func (s *QuestionService) GetQuestionSet(ctx context.Context, kbID, setID string) (*types.QuestionSet, error) {
	return s.getQuestionSetForKB(ctx, kbID, setID)
}

func (s *QuestionService) ListQuestionSets(ctx context.Context, kbID string, page *types.Pagination) (*types.PageResult, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
	return s.repository.ListQuestionSets(ctx, tenantID(ctx), kbID, page)
}

func (s *QuestionService) UpdateQuestionSet(ctx context.Context, kbID, setID string, req *types.UpdateQuestionSetRequest) (*types.QuestionSet, error) {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return nil, err
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
	if req.ProcessingConfig != nil {
		qs.ProcessingConfig = normalizeProcessingConfig(req.ProcessingConfig)
		// Reset processing stage when config changes, so a re-import can restart the pipeline.
		if qs.ProcessingStage != types.QuestionSetProcessingStageIdle &&
			qs.ProcessingStage != types.QuestionSetProcessingStageReadyForReview &&
			qs.ProcessingStage != types.QuestionSetProcessingStageFailed {
			// Don't reset if currently processing; the pipeline owns the stage.
		} else {
			qs.ProcessingStage = types.QuestionSetProcessingStageIdle
		}
	}
	if err := s.repository.UpdateQuestionSet(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}

func (s *QuestionService) DeleteQuestionSet(ctx context.Context, kbID, setID string) error {
	if _, err := s.getQuestionSetForKB(ctx, kbID, setID); err != nil {
		return err
	}
	// Collect question IDs before deletion so vector indexes can be
	// cleaned up after the DB transaction commits.
	questionIDs, err := s.listQuestionIDsInSet(ctx, tenantID(ctx), setID)
	if err != nil {
		return err
	}
	if err := s.repository.DeleteQuestionSet(ctx, tenantID(ctx), setID); err != nil {
		return err
	}
	s.scheduleQuestionIndexDelete(ctx, questionIDs)
	return nil
}

func (s *QuestionService) listQuestionIDsInSet(ctx context.Context, tenantID uint64, setID string) ([]string, error) {
	var ids []string
	page := &types.Pagination{Page: 1, PageSize: 500}
	for {
		result, err := s.repository.ListQuestions(ctx, tenantID, setID, nil, page)
		if err != nil {
			return nil, err
		}
		questions, ok := result.Data.([]*types.Question)
		if !ok {
			return nil, fmt.Errorf("unexpected question page data type")
		}
		for _, q := range questions {
			ids = append(ids, q.ID)
		}
		if len(questions) < page.PageSize {
			return ids, nil
		}
		page.Page++
	}
}

func (s *QuestionService) CreateQuestion(ctx context.Context, kbID, setID string, req *types.CreateQuestionRequest) (*types.Question, error) {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return nil, err
	}
	q := &types.Question{
		TenantID:           tenantID(ctx),
		QuestionSetID:      setID,
		KnowledgeBaseID:    qs.KnowledgeBaseID,
		QuestionType:       req.QuestionType,
		StemText:           strings.TrimSpace(req.StemText),
		QuestionBody:       normalizeJSONObject(req.QuestionBody),
		AnswerText:         strings.TrimSpace(req.AnswerText),
		AnswerBody:         normalizeJSONObject(req.AnswerBody),
		AnalysisText:       strings.TrimSpace(req.AnalysisText),
		GradingRubric:      normalizeJSONObject(req.GradingRubric),
		Difficulty:         types.QuestionDifficulty(req.Difficulty),
		Status:             types.QuestionStatusDraft,
		KnowledgePoints:    normalizeJSONArray(req.KnowledgePoints),
		Tags:               normalizeJSONArray(req.Tags),
		SourceKnowledgeID:  req.SourceKnowledgeID,
		EvidenceChunkIDs:   normalizeJSONArray(req.EvidenceChunkIDs),
		SourcePayload:      normalizeJSONMap(nil),
		ExtractionMetadata: normalizeJSONMap(nil),
		SortOrder:          req.SortOrder,
	}
	if q.QuestionType == "" {
		q.QuestionType = string(types.QuestionTypeSingleChoice)
	}
	if q.Difficulty == "" {
		q.Difficulty = types.QuestionDifficultyMedium
	}
	q.Status = structuredQuestionStatus(q)
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
	s.scheduleQuestionIndex(ctx, []*types.Question{q})
	return q, nil
}

func (s *QuestionService) GetQuestion(ctx context.Context, kbID, setID, questionID string) (*types.Question, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
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
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
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
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
	q, err := s.repository.GetQuestion(ctx, tenantID(ctx), setID, questionID)
	if err != nil {
		return nil, err
	}
	if q.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError(fmt.Sprintf("question does not belong to knowledge base %s", kbID))
	}
	before := *q
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
	q.Status = statusAfterStructuredEdit(q.Status, q)
	if err := s.validateEvidenceReferences(ctx, q.KnowledgeBaseID, q.SourceKnowledgeID, q.EvidenceChunkIDs); err != nil {
		return nil, err
	}
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	if questionIndexFieldsChanged(&before, q) {
		s.scheduleQuestionIndex(ctx, []*types.Question{q})
	}
	return q, nil
}

func (s *QuestionService) DeleteQuestion(ctx context.Context, kbID, setID, questionID string) error {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return err
	}
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
	s.scheduleQuestionIndexDelete(ctx, []string{questionID})
	return nil
}

func (s *QuestionService) UpdateQuestionStatus(ctx context.Context, kbID, setID, questionID string, req *types.UpdateQuestionStatusRequest) (*types.Question, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
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
	if newStatus == types.QuestionStatusReviewed {
		now := time.Now()
		q.ReviewedAt = &now
		if userID, ok := types.UserIDFromContext(ctx); ok && userID != "" && !types.IsSyntheticUserID(userID) {
			q.ReviewedBy = userID
		}
	}
	if err := s.repository.UpdateQuestion(ctx, q); err != nil {
		return nil, err
	}
	s.scheduleQuestionIndex(ctx, []*types.Question{q})
	return q, nil
}

func (s *QuestionService) ImportQuestions(ctx context.Context, kbID, setID string, req *types.ImportQuestionsRequest) (*types.ImportQuestionsResult, error) {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return nil, err
	}
	cfg := resolveProcessingConfig(qs.ProcessingConfig)
	result := &types.ImportQuestionsResult{}
	var created []*types.Question
	for _, item := range req.Items {
		q := &types.Question{
			TenantID:           tenantID(ctx),
			QuestionSetID:      setID,
			KnowledgeBaseID:    qs.KnowledgeBaseID,
			QuestionType:       item.QuestionType,
			StemText:           strings.TrimSpace(item.StemText),
			QuestionBody:       normalizeJSONObject(item.QuestionBody),
			AnswerText:         strings.TrimSpace(item.AnswerText),
			AnswerBody:         normalizeJSONObject(item.AnswerBody),
			AnalysisText:       strings.TrimSpace(item.AnalysisText),
			GradingRubric:      normalizeJSONObject(item.GradingRubric),
			Difficulty:         types.QuestionDifficulty(item.Difficulty),
			Status:             types.QuestionStatusDraft,
			KnowledgePoints:    normalizeJSONArray(item.KnowledgePoints),
			Tags:               normalizeJSONArray(item.Tags),
			SourceKnowledgeID:  item.SourceKnowledgeID,
			EvidenceChunkIDs:   normalizeJSONArray(item.EvidenceChunkIDs),
			SourcePayload:      normalizeJSONMap(nil),
			ExtractionMetadata: normalizeJSONMap(nil),
		}
		if q.QuestionType == "" {
			q.QuestionType = string(types.QuestionTypeSingleChoice)
		}
		if q.Difficulty == "" {
			q.Difficulty = types.QuestionDifficultyMedium
		}
		// In this phase, all imported questions MUST enter draft status.
		// Caller-supplied status is intentionally ignored to enforce the
		// draft → review pipeline.  The structuredQuestionStatus logic is
		// also bypassed for imports: even if the question would pass
		// review validation, it stays draft until a human confirms it.
		q.Status = types.QuestionStatusDraft

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
	if len(created) == 0 {
		return result, nil
	}
	if err := s.repository.UpdateQuestionSetSourceType(
		ctx,
		tenantID(ctx),
		setID,
		types.QuestionSetSourceImport,
	); err != nil {
		return nil, err
	}
	if err := s.repository.UpdateQuestionCount(ctx, tenantID(ctx), setID); err != nil {
		return nil, err
	}
	s.scheduleQuestionIndex(ctx, created)

	// Kick off the background processing pipeline.
	s.startProcessingPipeline(ctx, qs, created, cfg)

	return result, nil
}

func questionIndexFieldsChanged(before, after *types.Question) bool {
	if before == nil || after == nil {
		return before != after
	}
	return before.Status != after.Status || BuildQuestionIndexContent(before) != BuildQuestionIndexContent(after)
}

func (s *QuestionService) scheduleQuestionIndex(ctx context.Context, questions []*types.Question) {
	if s.questionIndexService == nil || len(questions) == 0 {
		return
	}
	if err := s.questionIndexService.IndexQuestions(ctx, questions); err != nil {
		logger.Errorf(ctx, "failed to schedule question vector indexing: %v", err)
	}
}

func (s *QuestionService) scheduleQuestionIndexDelete(ctx context.Context, questionIDs []string) {
	if s.questionIndexService == nil || len(questionIDs) == 0 {
		return
	}
	if err := s.questionIndexService.DeleteQuestionIndexes(ctx, questionIDs); err != nil {
		logger.Errorf(ctx, "failed to schedule question vector index deletion: %v", err)
	}
}

func (s *QuestionService) ExportToEvaluationDataset(ctx context.Context, kbID, setID string, req *types.ExportToEvaluationRequest) (*types.EvaluationDataset, error) {
	if _, err := s.getQuestionSetForKB(ctx, kbID, setID); err != nil {
		return nil, err
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
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
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

var validImportFileExtensions = map[string]bool{
	".doc":  true,
	".docx": true,
	".pdf":  true,
}

func isValidImportFileExtension(name string) bool {
	lower := strings.ToLower(name)
	for ext := range validImportFileExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func (s *QuestionExtractionService) fileExtension(name string) string {
	lower := strings.ToLower(name)
	for ext := range validImportFileExtensions {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return ""
}

// PreviewImportQuestionsFromFile extracts text from a document file and parses
// questions from it without persisting to the database.
// It does NOT create knowledge, chunks, or embeddings.
func (s *QuestionService) PreviewImportQuestionsFromFile(
	ctx context.Context,
	kbID, setID string,
	fileData []byte,
	fileName string,
	req *types.ImportFilePreviewRequest,
) (*types.ImportFilePreviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Infof("[import-file preview] started: kb=%s set=%s file=%s size=%d", kbID, setID, fileName, len(fileData))

	// 1. Validate KB is question_bank (this also validates set belongs to kb)
	if _, err := s.getQuestionSetForKB(ctx, kbID, setID); err != nil {
		return nil, err
	}

	// 2. Validate file extension
	if !isValidImportFileExtension(fileName) {
		return nil, apperrors.NewBadRequestError("仅支持 DOC、DOCX、PDF 文件。")
	}

	// 3. Validate file size
	maxSize := secutils.GetMaxFileSize()
	if int64(len(fileData)) > maxSize {
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("文件大小超过限制 (%d MB)", maxSize/(1024*1024)),
		)
	}

	// 4. Determine file type for docreader
	fileType := strings.TrimPrefix(
		strings.ToLower(fileName[strings.LastIndex(fileName, "."):]),
		".",
	)

	// 5. Extract text using docreader with a timeout
	if s.docReader == nil || !s.docReader.IsConnected() {
		return nil, apperrors.NewBadRequestError("文档解析服务不可用，请稍后重试。")
	}

	readCtx, readCancel := context.WithTimeout(ctx, 120*time.Second)
	defer readCancel()

	log.Infof("[import-file preview] docreader read started: file=%s type=%s", fileName, fileType)
	readResp, err := s.docReader.Read(readCtx, &types.ReadRequest{
		FileContent: fileData,
		FileName:    fileName,
		FileType:    fileType,
	})
	if err != nil {
		if readCtx.Err() == context.DeadlineExceeded {
			log.Warnf("[import-file preview] docreader timed out: file=%s", fileName)
			return nil, apperrors.NewBadRequestError("文档解析超时，请尝试拆分文件或使用 JSON/JSONL 导入。")
		}
		log.Errorf("[import-file preview] docreader read failed: file=%s err=%v", fileName, err)
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("文档解析失败: %s", err.Error()),
		)
	}
	if readResp.Error != "" {
		log.Errorf("[import-file preview] docreader returned error: file=%s err=%s", fileName, readResp.Error)
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("文档解析失败: %s", readResp.Error),
		)
	}
	log.Infof("[import-file preview] docreader read finished: file=%s markdown_len=%d", fileName, len(readResp.MarkdownContent))

	extractedText := strings.TrimSpace(readResp.MarkdownContent)

	// 6. Set defaults (needed by both normal and debug-export paths)
	defaultType := req.DefaultQuestionType
	if defaultType == "" {
		defaultType = string(types.QuestionTypeShortAnswer)
	}
	defaultDifficulty := req.DefaultDifficulty
	if defaultDifficulty == "" {
		defaultDifficulty = string(types.QuestionDifficultyMedium)
	}

	var items []types.ImportQuestionItem
	var parseErrors []types.ImportQuestionError
	var parseWarnings []string

	if extractedText == "" {
		parseWarnings = []string{"未能从文件中抽取文本，请确认文件内容可复制，或等待 OCR 支持。"}
		if !req.DebugExport {
			// Normal non-debug: return an empty preview response instead of
			// a hard error so the frontend can show the warning.
			log.Warnf("[import-file preview] empty text extracted: file=%s", fileName)
			return &types.ImportFilePreviewResponse{
				Items:          nil,
				Errors:         nil,
				Warnings:       parseWarnings,
				RawTextPreview: "",
				Stats:          types.ImportFilePreviewStats{},
			}, nil
		}
		// Debug export: continue to generate a debug bundle with empty pipeline
		// intermediates so the caller can inspect what the docreader produced.
		log.Warnf("[import-file preview] empty text extracted, generating debug export anyway: file=%s", fileName)
	} else {
		// Check context cancellation before extraction
		select {
		case <-ctx.Done():
			log.Infof("[import-file preview] cancelled before extraction: file=%s", fileName)
			return nil, apperrors.NewBadRequestError("请求已取消")
		default:
		}

		items, parseErrors, parseWarnings = s.extractionService.Extract(
			ctx, extractedText, defaultType, defaultDifficulty,
		)
		log.Infof("[import-file preview] extraction finished: items=%d errors=%d warnings=%d",
			len(items), len(parseErrors), len(parseWarnings))
	}

	// 6a. Optional debug export (best-effort, non-fatal)
	var debugDir string
	var debugManifest []string
	if req.DebugExport {
		log.Infof("[import-file preview] debug export started: file=%s", fileName)
		var zipPath string
		var exportErr error
		debugDir, zipPath, debugManifest, exportErr = createDebugExport(
			extractedText, defaultType, defaultDifficulty,
			items, parseErrors, parseWarnings,
			fileName, fileType, int64(len(fileData)),
		)
		if exportErr != nil {
			log.Warnf("[import-file preview] debug export failed (continuing): %v", exportErr)
			debugDir = ""
			debugManifest = nil
		} else {
			log.Infof("[import-file preview] debug export ready: dir=%s zip=%s files=%d",
				debugDir, zipPath, len(debugManifest))
		}
	}

	// 7. Build response stats
	withAnswer := 0
	withoutAnswer := 0
	for _, item := range items {
		if strings.TrimSpace(item.AnswerText) != "" {
			withAnswer++
		} else {
			withoutAnswer++
		}
	}

	// Truncate raw text for preview (first 4000 chars is enough to verify content)
	rawText := extractedText
	if len([]rune(rawText)) > 4000 {
		rawText = string([]rune(rawText)[:4000]) + "\n... (truncated)"
	}

	// Normalize nil slices to empty arrays so the JSON never contains null.
	if items == nil {
		items = []types.ImportQuestionItem{}
	}
	if parseErrors == nil {
		parseErrors = []types.ImportQuestionError{}
	}
	if parseWarnings == nil {
		parseWarnings = []string{}
	}

	return &types.ImportFilePreviewResponse{
		Items:          items,
		Errors:         parseErrors,
		Warnings:       parseWarnings,
		RawTextPreview: rawText,
		Stats: types.ImportFilePreviewStats{
			DetectedQuestions: len(items),
			WithAnswer:        withAnswer,
			WithoutAnswer:     withoutAnswer,
		},
		DebugExportPath: debugDir,
		DebugManifest:   debugManifest,
	}, nil
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

func normalizeProcessingConfig(cfg *types.QuestionSetProcessingConfig) types.JSON {
	if cfg == nil {
		return types.JSON([]byte("{}"))
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return types.JSON([]byte("{}"))
	}
	return types.JSON(data)
}

// resolveProcessingConfig unmarshals the ProcessingConfig JSON field into a struct.
func resolveProcessingConfig(raw types.JSON) *types.QuestionSetProcessingConfig {
	cfg := &types.QuestionSetProcessingConfig{}
	if len(raw) == 0 {
		return cfg
	}
	_ = json.Unmarshal(raw, cfg)
	return cfg
}

// GetQuestionSetProcessingStatus returns the current processing status for a question set.
func (s *QuestionService) GetQuestionSetProcessingStatus(ctx context.Context, kbID, setID string) (*types.QuestionSetProcessingStatus, error) {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return nil, err
	}
	cfg := resolveProcessingConfig(qs.ProcessingConfig)
	status := &types.QuestionSetProcessingStatus{
		Stage:                qs.ProcessingStage,
		ErrorMessage:         qs.ErrorMessage,
		AutoTaggingEnabled:   cfg.AutoKnowledgePointEnabled(),
		SyllabusCheckEnabled: cfg.AutoSyllabusCheckEnabled(),
	}
	if !cfg.AutoKnowledgePointEnabled() {
		// Show skip reason regardless of current stage.
		status.SkippedAutoTaggingReason = "未配置知识点知识库，自动知识点关联已禁用"
	}
	if !cfg.AutoSyllabusCheckEnabled() {
		status.SkippedSyllabusReason = "未配置考纲，自动考纲筛选已禁用"
	}
	return status, nil
}

// startProcessingPipeline kicks off the background processing pipeline for imported questions.
// It does NOT block the import response — all work runs in a detached goroutine.
func (s *QuestionService) startProcessingPipeline(
	ctx context.Context,
	qs *types.QuestionSet,
	questions []*types.Question,
	cfg *types.QuestionSetProcessingConfig,
) {
	s.setProcessingStage(ctx, qs, types.QuestionSetProcessingStageDraftImported, "")

	bgCtx := logger.CloneContext(ctx)
	go func() {
		// Stage 1: Indexing — the question index service handles this asynchronously.
		// We poll the vector index status for all imported questions to know when
		// indexing is complete.
		s.setProcessingStage(bgCtx, qs, types.QuestionSetProcessingStageIndexing, "")
		if err := s.waitForIndexing(bgCtx, questions); err != nil {
			logger.Errorf(bgCtx, "question set %s indexing failed: %v", qs.ID, err)
			s.setProcessingStage(bgCtx, qs, types.QuestionSetProcessingStageFailed, truncateError(err))
			return
		}

		// Stage 2: Auto knowledge point tagging (stub — TODO in future phase).
		if cfg.AutoKnowledgePointEnabled() {
			s.setProcessingStage(bgCtx, qs, types.QuestionSetProcessingStageAutoTagging, "")
			// TODO: Implement auto knowledge point matching against the
			// configured knowledge point KB. For now this is a no-op that
			// preserves the extension point.
			logger.Infof(bgCtx, "question set %s: auto knowledge point tagging is enabled (KB=%s) but algorithm is not yet implemented — skipping",
				qs.ID, cfg.KnowledgePointKnowledgeBaseID)
		}

		// Stage 3: Auto syllabus screening (stub — TODO in future phase).
		if cfg.AutoSyllabusCheckEnabled() {
			s.setProcessingStage(bgCtx, qs, types.QuestionSetProcessingStageSyllabusChecking, "")
			// TODO: Implement auto syllabus screening against the configured
			// syllabus KB. For now this is a no-op that preserves the extension point.
			logger.Infof(bgCtx, "question set %s: auto syllabus check is enabled (KB=%s) but algorithm is not yet implemented — skipping",
				qs.ID, cfg.SyllabusKnowledgeBaseID)
		}

		// Stage 4: Ready for human review.
		s.setProcessingStage(bgCtx, qs, types.QuestionSetProcessingStageReadyForReview, "")
	}()
}

// setProcessingStage updates the question set's processing stage and optionally the error message.
func (s *QuestionService) setProcessingStage(ctx context.Context, qs *types.QuestionSet, stage types.QuestionSetProcessingStage, errMsg string) {
	qs.ProcessingStage = stage
	if errMsg != "" {
		qs.ErrorMessage = errMsg
	}
	if err := s.repository.UpdateQuestionSet(ctx, qs); err != nil {
		logger.Errorf(ctx, "failed to update question set processing stage to %s: %v", stage, err)
	}
}

// waitForIndexing polls the question vector index status until all questions are
// indexed or have failed. Returns nil on success (all indexed), or an error if any
// question has permanently failed.
func (s *QuestionService) waitForIndexing(ctx context.Context, questions []*types.Question) error {
	if s.questionIndexService == nil {
		return nil
	}
	// For the MVP, we trust the existing async indexing. The question index
	// service already handles the full lifecycle (pending → indexing → indexed/failed).
	// Future phases may add polling via QuestionVectorIndexRepository.
	_ = questions
	return nil
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 2000 {
		return msg[:2000]
	}
	return msg
}

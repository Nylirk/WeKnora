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
	modelService         interfaces.ModelService
	tenantService        interfaces.TenantService
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
	modelSvc interfaces.ModelService,
	tenantSvc interfaces.TenantService,
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
		modelService:         modelSvc,
		tenantService:        tenantSvc,
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

const maxQuestionSetNameLen = 40

func (s *QuestionService) CreateQuestionSet(ctx context.Context, kbID string, req *types.CreateQuestionSetRequest) (*types.QuestionSet, error) {
	if err := s.ensureQuestionBankKB(ctx, kbID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperrors.NewBadRequestError("分类名称不能为空")
	}
	if len([]rune(name)) > maxQuestionSetNameLen {
		return nil, apperrors.NewBadRequestError("分类名称不能超过 40 个字符")
	}
	// Check for duplicate name within the same knowledge base.
	existing, err := s.repository.GetQuestionSetByName(ctx, tenantID(ctx), kbID, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperrors.NewBadRequestError("当前题库中已存在同名分类")
	}

	qs := &types.QuestionSet{
		TenantID:         tenantID(ctx),
		KnowledgeBaseID:  kbID,
		Name:             name,
		Description:      strings.TrimSpace(req.Description),
		SourceType:       types.QuestionSetSourceManual,
		Status:           types.QuestionSetStatusActive,
		GenerationConfig: normalizeJSONObject(nil),
		GenerationScope:  normalizeJSONObject(nil),
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
			return nil, apperrors.NewBadRequestError("分类名称不能为空")
		}
		if len([]rune(v)) > maxQuestionSetNameLen {
			return nil, apperrors.NewBadRequestError("分类名称不能超过 40 个字符")
		}
		// Check for duplicate name (allow same name if unchanged)
		if v != strings.TrimSpace(qs.Name) {
			existing, lookupErr := s.repository.GetQuestionSetByName(ctx, tenantID(ctx), kbID, v)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if existing != nil {
				return nil, apperrors.NewBadRequestError("当前题库中已存在同名分类")
			}
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
	// Read auto-processing config from parent QuestionBank KnowledgeBase.
	kb, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	var cfg *types.QuestionBankConfig
	if kb.IsQuestionBank() && kb.QuestionBankConfig != nil {
		cfg = kb.QuestionBankConfig
	} else {
		cfg = &types.QuestionBankConfig{}
	}

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
	".doc":     true,
	".docx":    true,
	".pdf":     true,
	".md":      true,
	".markdown": true,
	".xlsx":    true,
	".xls":     true,
}

func isMarkdownFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func isExcelFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls")
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
		return nil, apperrors.NewBadRequestError("仅支持 DOC、DOCX、PDF、MD、Markdown、XLSX、XLS 文件。")
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

		// Route to the appropriate normalizer based on file type.
		// .md / .markdown → markdown question normalizer
		// .xlsx / .xls     → excel question normalizer
		// .doc / .docx / .pdf → existing extraction service (rule-based)
		if isMarkdownFile(fileName) {
			items, parseErrors = normalizeMarkdownQuestions(extractedText)
			log.Infof("[import-file preview] markdown normalization finished: items=%d errors=%d",
				len(items), len(parseErrors))
		} else if isExcelFile(fileName) {
			items, parseErrors = normalizeExcelQuestions(extractedText)
			log.Infof("[import-file preview] excel normalization finished: items=%d errors=%d",
				len(items), len(parseErrors))
		} else {
			items, parseErrors, parseWarnings = s.extractionService.Extract(
				ctx, extractedText, defaultType, defaultDifficulty,
			)
			log.Infof("[import-file preview] extraction finished: items=%d errors=%d warnings=%d",
				len(items), len(parseErrors), len(parseWarnings))
		}
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

// GetQuestionSetProcessingStatus returns the current processing status for a question set.
// Auto-processing enablement is read from the parent question_bank KnowledgeBase.
// ReprocessQuestionSet re-runs semantic matching for all draft questions in a question set.
// scope: "all", "auto_tagging", or "syllabus_checking". Runs in a background goroutine.
func (s *QuestionService) ReprocessQuestionSet(
	ctx context.Context, kbID, setID string, scope string,
) error {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return err
	}
	kb, kberr := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if kberr != nil {
		return kberr
	}
	var cfg *types.QuestionBankConfig
	if kb.IsQuestionBank() && kb.QuestionBankConfig != nil {
		cfg = kb.QuestionBankConfig
	} else {
		cfg = &types.QuestionBankConfig{}
	}

	// Collect all draft questions in this set.
	var draftQuestions []*types.Question
	page := &types.Pagination{Page: 1, PageSize: 500}
	for {
		result, listErr := s.repository.ListQuestions(ctx, tenantID(ctx), setID,
			&types.QuestionListFilter{Status: string(types.QuestionStatusDraft)}, page)
		if listErr != nil {
			return listErr
		}
		questions, ok := result.Data.([]*types.Question)
		if !ok {
			break
		}
		draftQuestions = append(draftQuestions, questions...)
		if len(questions) < page.PageSize {
			break
		}
		page.Page++
	}

	if len(draftQuestions) == 0 {
		return nil
	}

	bgCtx := logger.CloneContext(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf(bgCtx, "panic in reprocess pipeline for set %s: %v", qs.ID, r)
			}
		}()

		runTagging := scope == "all" || scope == "auto_tagging"
		runSyllabus := scope == "all" || scope == "syllabus_checking"

		if runTagging {
			_ = s.updateQuestionSetProcessingStage(bgCtx, qs.ID,
				types.QuestionSetProcessingStageAutoTagging, "", "")
			if err := s.RunKnowledgePointMatching(bgCtx, cfg, draftQuestions); err != nil {
				logger.Warnf(bgCtx, "reprocess auto_tagging error for set %s: %v", qs.ID, err)
			}
		}

		if runSyllabus {
			_ = s.updateQuestionSetProcessingStage(bgCtx, qs.ID,
				types.QuestionSetProcessingStageSyllabusChecking, "", "")
			if err := s.RunSyllabusFiltering(bgCtx, cfg, draftQuestions); err != nil {
				logger.Warnf(bgCtx, "reprocess syllabus_checking error for set %s: %v", qs.ID, err)
			}
		}

		_ = s.updateQuestionSetProcessingStage(bgCtx, qs.ID,
			types.QuestionSetProcessingStageReadyForReview, types.QuestionSetStatusActive, "")
	}()

	return nil
}

func (s *QuestionService) GetQuestionSetProcessingStatus(ctx context.Context, kbID, setID string) (*types.QuestionSetProcessingStatus, error) {
	qs, err := s.getQuestionSetForKB(ctx, kbID, setID)
	if err != nil {
		return nil, err
	}
	// Read auto-processing config from parent QuestionBank KnowledgeBase.
	kb, kberr := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	var cfg *types.QuestionBankConfig
	if kberr == nil && kb.IsQuestionBank() && kb.QuestionBankConfig != nil {
		cfg = kb.QuestionBankConfig
	} else {
		cfg = &types.QuestionBankConfig{}
	}

	status := &types.QuestionSetProcessingStatus{
		Stage:                qs.ProcessingStage,
		ErrorMessage:         qs.ErrorMessage,
		AutoTaggingEnabled:   cfg.AutoKnowledgePointEnabled(),
		SyllabusCheckEnabled: cfg.AutoSyllabusCheckEnabled(),
	}
	if !cfg.AutoKnowledgePointEnabled() {
		status.SkippedAutoTaggingReason = "未配置知识点知识库，自动知识点关联已禁用"
	}
	if !cfg.AutoSyllabusCheckEnabled() {
		status.SkippedSyllabusReason = "未配置考纲，自动考纲筛选已禁用"
	}
	status.Stages = computeProcessingStages(qs.ProcessingStage, cfg)
	return status, nil
}

// computeProcessingStages derives per-stage status from the current processing stage
// and KB config. It is deterministic and safe for polling.
func computeProcessingStages(
	currentStage types.QuestionSetProcessingStage,
	cfg *types.QuestionBankConfig,
) []types.ProcessingStageDetail {
	type stageDef struct {
		key   string
		label string
	}
	pipeline := []stageDef{
		{key: "draft_imported", label: "导入完成"},
		{key: "indexing", label: "索引处理"},
		{key: "auto_tagging", label: "知识点关联"},
		{key: "syllabus_checking", label: "考纲筛选"},
		{key: "ready_for_review", label: "待人工审核"},
	}

	currentKey := string(currentStage)
	currentIdx := -1
	for i, s := range pipeline {
		if s.key == currentKey {
			currentIdx = i
			break
		}
	}

	stages := make([]types.ProcessingStageDetail, len(pipeline))
	for i, s := range pipeline {
		stages[i] = types.ProcessingStageDetail{
			Key:    s.key,
			Label:  s.label,
			Status: "pending",
		}
		if currentIdx < 0 {
			continue
		}
		// Stages before the current one are completed.
		if i < currentIdx {
			stages[i].Status = "completed"
		} else if i == currentIdx {
			switch currentKey {
			case "failed":
				// Mark prior stages as completed; this stage as failed.
				stages[i].Status = "failed"
			case "ready_for_review":
				stages[i].Status = "completed"
			case "":
				stages[i].Status = "pending"
			default:
				stages[i].Status = "running"
			}
		}
	}

	// When the overall status is "failed" we don't know exactly which pipeline
	// stage failed; assume the last stage before the failure marker.
	if currentKey == "failed" && currentIdx < len(stages) {
		// Push the failure onto the last non-completed stage if any.
		lastCompleted := -1
		for i := range stages {
			if stages[i].Status == "completed" {
				lastCompleted = i
			}
		}
		failIdx := lastCompleted + 1
		if failIdx < len(stages) {
			for i := failIdx + 1; i < len(stages); i++ {
				stages[i].Status = "pending"
			}
			stages[failIdx].Status = "failed"
		}
	}

	// Override paused stages from KB config (only when not currently running).
	autoTaggingIdx := 2
	syllabusIdx := 3
	if !cfg.AutoKnowledgePointEnabled() && stages[autoTaggingIdx].Status != "running" {
		stages[autoTaggingIdx].Status = "paused"
		stages[autoTaggingIdx].Reason = "未关联知识点知识库"
	}
	if !cfg.AutoSyllabusCheckEnabled() && stages[syllabusIdx].Status != "running" {
		stages[syllabusIdx].Status = "paused"
		stages[syllabusIdx].Reason = "未配置考纲"
	}

	return stages
}

// startProcessingPipeline kicks off the background processing pipeline for imported questions.
// It does NOT block the import response — all work runs in a detached goroutine.
// The cfg is read from the parent question_bank KnowledgeBase (may be empty).
func (s *QuestionService) startProcessingPipeline(
	ctx context.Context,
	qs *types.QuestionSet,
	questions []*types.Question,
	cfg *types.QuestionBankConfig,
) {
	// Set initial stage synchronously so the API response reflects it.
	if err := s.updateQuestionSetProcessingStage(
		ctx, qs.ID,
		types.QuestionSetProcessingStageDraftImported,
		types.QuestionSetStatusActive,
		"",
	); err != nil {
		logger.Errorf(ctx, "failed to set draft_imported stage for set %s: %v", qs.ID, err)
		return
	}

	bgCtx := logger.CloneContext(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf(bgCtx, "panic in processing pipeline for set %s: %v", qs.ID, r)
				_ = s.updateQuestionSetProcessingStage(
					bgCtx, qs.ID,
					types.QuestionSetProcessingStageFailed,
					types.QuestionSetStatusFailed,
					fmt.Sprintf("panic: %v", r),
				)
			}
		}()

		// Stage 1: Indexing — draft questions are not added to the reviewed-only
		// vector index. We record this as a deliberate skip.
		if err := s.updateQuestionSetProcessingStage(
			bgCtx, qs.ID,
			types.QuestionSetProcessingStageIndexing,
			"",
			"",
		); err != nil {
			logger.Errorf(bgCtx, "failed to set indexing stage for set %s: %v", qs.ID, err)
			_ = s.updateQuestionSetProcessingStage(
				bgCtx, qs.ID,
				types.QuestionSetProcessingStageFailed,
				types.QuestionSetStatusFailed,
				truncateError(err),
			)
			return
		}
		indexingMeta := map[string]any{
			"status": "skipped",
			"reason": "draft questions are not added to reviewed-only index",
		}
		s.writeAutoProcessingMetadataToQuestions(bgCtx, questions, "indexing", indexingMeta)

		// Stage 2: Auto knowledge point tagging via semantic matching.
		if cfg.AutoKnowledgePointEnabled() {
			if err := s.updateQuestionSetProcessingStage(
				bgCtx, qs.ID,
				types.QuestionSetProcessingStageAutoTagging,
				"",
				"",
			); err != nil {
				logger.Errorf(bgCtx, "failed to set auto_tagging stage for set %s: %v", qs.ID, err)
				_ = s.updateQuestionSetProcessingStage(
					bgCtx, qs.ID,
					types.QuestionSetProcessingStageFailed,
					types.QuestionSetStatusFailed,
					truncateError(err),
				)
				return
			}
			if err := s.RunKnowledgePointMatching(bgCtx, cfg, questions); err != nil {
				logger.Warnf(bgCtx, "auto_tagging matching returned error for set %s: %v", qs.ID, err)
			}
		} else {
			_ = s.RunKnowledgePointMatching(bgCtx, cfg, questions)
		}

		// Stage 3: Auto syllabus screening via semantic matching.
		if cfg.AutoSyllabusCheckEnabled() {
			if err := s.updateQuestionSetProcessingStage(
				bgCtx, qs.ID,
				types.QuestionSetProcessingStageSyllabusChecking,
				"",
				"",
			); err != nil {
				logger.Errorf(bgCtx, "failed to set syllabus_checking stage for set %s: %v", qs.ID, err)
				_ = s.updateQuestionSetProcessingStage(
					bgCtx, qs.ID,
					types.QuestionSetProcessingStageFailed,
					types.QuestionSetStatusFailed,
					truncateError(err),
				)
				return
			}
			if err := s.RunSyllabusFiltering(bgCtx, cfg, questions); err != nil {
				logger.Warnf(bgCtx, "syllabus_checking matching returned error for set %s: %v", qs.ID, err)
			}
		} else {
			_ = s.RunSyllabusFiltering(bgCtx, cfg, questions)
		}

		// Stage 4: Ready for human review.
		if err := s.updateQuestionSetProcessingStage(
			bgCtx, qs.ID,
			types.QuestionSetProcessingStageReadyForReview,
			types.QuestionSetStatusActive,
			"",
		); err != nil {
			logger.Errorf(bgCtx, "failed to set ready_for_review stage for set %s: %v", qs.ID, err)
			_ = s.updateQuestionSetProcessingStage(
				bgCtx, qs.ID,
				types.QuestionSetProcessingStageFailed,
				types.QuestionSetStatusFailed,
				truncateError(err),
			)
			return
		}
	}()
}

// truncateError limits an error string to a max length for storage.
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

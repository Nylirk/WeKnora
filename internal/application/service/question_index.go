package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	questionIndexMaxChars = 20000
)

type questionIndexRunner func(func())

type questionIndexService struct {
	indexRepository interfaces.QuestionVectorIndexRepository
	questionRepo    interfaces.QuestionRepository
	kbService       interfaces.KnowledgeBaseService
	modelService    interfaces.ModelService
	engineRegistry  interfaces.RetrieveEngineRegistry
	vectorStoreRepo interfaces.VectorStoreRepository
	run             questionIndexRunner
}

type questionIndexTarget struct {
	engineType types.RetrieverEngineType
	service    interfaces.RetrieveEngineService
	err        error
}

type questionIndexJob struct {
	target           questionIndexTarget
	embeddingModelID string
	indexes          []*types.QuestionVectorIndex
	indexInfos       []*types.IndexInfo
}

func NewQuestionIndexService(
	indexRepository interfaces.QuestionVectorIndexRepository,
	questionRepo interfaces.QuestionRepository,
	kbService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
	engineRegistry interfaces.RetrieveEngineRegistry,
	vectorStoreRepo interfaces.VectorStoreRepository,
) interfaces.QuestionIndexService {
	return newQuestionIndexService(
		indexRepository,
		questionRepo,
		kbService,
		modelService,
		engineRegistry,
		vectorStoreRepo,
		func(fn func()) { go fn() },
	)
}

func newQuestionIndexService(
	indexRepository interfaces.QuestionVectorIndexRepository,
	questionRepo interfaces.QuestionRepository,
	kbService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
	engineRegistry interfaces.RetrieveEngineRegistry,
	vectorStoreRepo interfaces.VectorStoreRepository,
	run questionIndexRunner,
) *questionIndexService {
	return &questionIndexService{
		indexRepository: indexRepository,
		questionRepo:    questionRepo,
		kbService:       kbService,
		modelService:    modelService,
		engineRegistry:  engineRegistry,
		vectorStoreRepo: vectorStoreRepo,
		run:             run,
	}
}

// BuildQuestionIndexContent builds the only embedding input used for a question.
// Answer, analysis and grading fields are intentionally not read here.
func BuildQuestionIndexContent(q *types.Question) string {
	if q == nil {
		return ""
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "question_type", value: strings.TrimSpace(q.QuestionType)},
		{name: "difficulty", value: strings.TrimSpace(string(q.Difficulty))},
		{name: "stem_text", value: strings.TrimSpace(q.StemText)},
		{name: "question_body", value: readableQuestionJSON(q.QuestionBody)},
		{name: "knowledge_points", value: readableQuestionJSON(q.KnowledgePoints)},
		{name: "tags", value: readableQuestionJSON(q.Tags)},
	}

	var builder strings.Builder
	for _, field := range fields {
		if field.value == "" || field.value == "{}" || field.value == "[]" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(field.name)
		builder.WriteString(": ")
		builder.WriteString(field.value)
	}

	content := builder.String()
	if utf8.RuneCountInString(content) <= questionIndexMaxChars {
		return content
	}
	return string([]rune(content)[:questionIndexMaxChars])
}

func readableQuestionJSON(raw types.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return formatQuestionJSONValue(value)
}

func formatQuestionJSONValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool, float64:
		return fmt.Sprint(typed)
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if formatted := formatQuestionJSONValue(item); formatted != "" {
				parts = append(parts, formatted)
			}
		}
		if len(parts) == 0 {
			return "[]"
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if !isQuestionIndexForbiddenKey(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if formatted := formatQuestionJSONValue(typed[key]); formatted != "" {
				parts = append(parts, key+": "+formatted)
			}
		}
		if len(parts) == 0 {
			return "{}"
		}
		return "{" + strings.Join(parts, "; ") + "}"
	default:
		return fmt.Sprint(typed)
	}
}

func isQuestionIndexForbiddenKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	return strings.Contains(normalized, "answer") ||
		strings.Contains(normalized, "analysis") ||
		strings.Contains(normalized, "explanation") ||
		strings.Contains(normalized, "solution") ||
		strings.Contains(normalized, "rubric")
}

func questionIndexPayloadHash(content string, enabled bool) string {
	payload := content + "\x00enabled=false"
	if enabled {
		payload = content + "\x00enabled=true"
	}
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func (s *questionIndexService) IndexQuestions(ctx context.Context, questions []*types.Question) error {
	if len(questions) == 0 {
		return nil
	}

	jobs := make(map[string]*questionIndexJob)
	kbCache := make(map[string]*types.KnowledgeBase)
	targetCache := make(map[string][]questionIndexTarget)
	var preparationErrors []error
	for _, question := range questions {
		if question == nil {
			continue
		}
		kb := kbCache[question.KnowledgeBaseID]
		if kb == nil {
			var err error
			kb, err = s.kbService.GetKnowledgeBaseByID(ctx, question.KnowledgeBaseID)
			if err != nil {
				preparationErrors = append(preparationErrors, fmt.Errorf("get knowledge base %s: %w", question.KnowledgeBaseID, err))
				continue
			}
			kbCache[question.KnowledgeBaseID] = kb
		}
		if kb.TenantID != 0 && kb.TenantID != question.TenantID {
			preparationErrors = append(preparationErrors, fmt.Errorf("knowledge base %s tenant mismatch", kb.ID))
			continue
		}
		targets, targetsCached := targetCache[kb.ID]
		if !targetsCached {
			var err error
			targets, err = s.resolveTargets(ctx, question.TenantID, kb)
			if err != nil {
				preparationErrors = append(preparationErrors, err)
				continue
			}
			targetCache[kb.ID] = targets
		}

		content := BuildQuestionIndexContent(question)
		enabled := question.Status == types.QuestionStatusReviewed
		contentHash := questionIndexPayloadHash(content, enabled)
		for _, target := range targets {
			existing, getErr := s.indexRepository.Get(
				ctx, question.TenantID, question.ID, kb.EmbeddingModelID,
				target.engineType, types.QuestionVectorIndexModePrompt,
			)
			if getErr != nil {
				preparationErrors = append(preparationErrors, getErr)
				continue
			}
			if existing != nil && existing.ContentHash == contentHash &&
				(existing.Status == types.QuestionVectorIndexStatusIndexed ||
					existing.Status == types.QuestionVectorIndexStatusPending ||
					existing.Status == types.QuestionVectorIndexStatusIndexing) {
				continue
			}

			state := &types.QuestionVectorIndex{
				TenantID:            question.TenantID,
				KnowledgeBaseID:     question.KnowledgeBaseID,
				QuestionSetID:       question.QuestionSetID,
				QuestionID:          question.ID,
				EmbeddingModelID:    kb.EmbeddingModelID,
				RetrieverEngineType: target.engineType,
				IndexMode:           types.QuestionVectorIndexModePrompt,
				ContentHash:         contentHash,
				Status:              types.QuestionVectorIndexStatusPending,
			}
			if err := s.indexRepository.Upsert(ctx, state); err != nil {
				preparationErrors = append(preparationErrors, err)
				continue
			}

			jobKey := kb.EmbeddingModelID + "\x00" + string(target.engineType) + "\x00" + targetIdentity(kb)
			job := jobs[jobKey]
			if job == nil {
				job = &questionIndexJob{target: target, embeddingModelID: kb.EmbeddingModelID}
				jobs[jobKey] = job
			}
			job.indexes = append(job.indexes, state)
			job.indexInfos = append(job.indexInfos, &types.IndexInfo{
				Content:         content,
				SourceID:        question.ID,
				SourceType:      types.QuestionSourceType,
				ChunkID:         question.ID,
				KnowledgeID:     question.QuestionSetID,
				KnowledgeBaseID: question.KnowledgeBaseID,
				KnowledgeType:   types.KnowledgeTypeQuestion,
				IsEnabled:       enabled,
			})
		}
	}

	if len(jobs) > 0 {
		background := logger.CloneContext(ctx)
		s.run(func() { s.processIndexJobs(background, jobs) })
	}
	return errors.Join(preparationErrors...)
}

func targetIdentity(kb *types.KnowledgeBase) string {
	if kb.VectorStoreID != nil && strings.TrimSpace(*kb.VectorStoreID) != "" {
		return *kb.VectorStoreID
	}
	return "env"
}

func (s *questionIndexService) resolveTargets(
	ctx context.Context,
	tenantID uint64,
	kb *types.KnowledgeBase,
) ([]questionIndexTarget, error) {
	if kb.VectorStoreID != nil && strings.TrimSpace(*kb.VectorStoreID) != "" {
		store, err := s.vectorStoreRepo.GetByID(ctx, tenantID, *kb.VectorStoreID)
		if err != nil {
			return nil, err
		}
		if store == nil {
			return nil, fmt.Errorf("vector store is not available for knowledge base %s", kb.ID)
		}
		engineService, resolveErr := s.engineRegistry.GetByStoreID(store.ID)
		if resolveErr == nil && !supportsVectorRetrieval(engineService) {
			return nil, nil
		}
		return []questionIndexTarget{{engineType: store.EngineType, service: engineService, err: resolveErr}}, nil
	}

	tenant, ok := types.TenantInfoFromContext(ctx)
	if !ok || tenant.ID != tenantID {
		return nil, fmt.Errorf("tenant info not found for question indexing")
	}
	seen := make(map[types.RetrieverEngineType]struct{})
	var targets []questionIndexTarget
	for _, params := range tenant.GetEffectiveEngines() {
		if params.RetrieverType != types.VectorRetrieverType {
			continue
		}
		if _, exists := seen[params.RetrieverEngineType]; exists {
			continue
		}
		seen[params.RetrieverEngineType] = struct{}{}
		engineService, err := s.engineRegistry.GetRetrieveEngineService(params.RetrieverEngineType)
		targets = append(targets, questionIndexTarget{
			engineType: params.RetrieverEngineType,
			service:    engineService,
			err:        err,
		})
	}
	return targets, nil
}

func supportsVectorRetrieval(engine interfaces.RetrieveEngineService) bool {
	if engine == nil {
		return false
	}
	for _, supported := range engine.Support() {
		if supported == types.VectorRetrieverType {
			return true
		}
	}
	return false
}

func (s *questionIndexService) processIndexJobs(ctx context.Context, jobs map[string]*questionIndexJob) {
	for _, job := range jobs {
		for _, state := range job.indexes {
			s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusIndexing, "", nil)
		}

		var err error
		if job.target.err != nil {
			err = job.target.err
		} else {
			var embedderErr error
			embedder, embedderErr := s.modelService.GetEmbeddingModel(ctx, job.embeddingModelID)
			if embedderErr != nil {
				err = embedderErr
			} else {
				err = job.target.service.BatchIndex(
					ctx, embedder, job.indexInfos, []types.RetrieverType{types.VectorRetrieverType},
				)
			}
		}

		if err != nil {
			for _, state := range job.indexes {
				s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusFailed, truncateQuestionIndexError(err), nil)
			}
			logger.Errorf(ctx, "question vector indexing failed: engine=%s error=%v", job.target.engineType, err)
			continue
		}
		now := time.Now()
		for _, state := range job.indexes {
			s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusIndexed, "", &now)
		}
	}
}

func truncateQuestionIndexError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if utf8.RuneCountInString(message) <= 2000 {
		return message
	}
	return string([]rune(message)[:2000])
}

func (s *questionIndexService) updateIndexStatus(
	ctx context.Context,
	state *types.QuestionVectorIndex,
	status types.QuestionVectorIndexStatus,
	errorMessage string,
	indexedAt *time.Time,
) {
	if err := s.indexRepository.UpdateStatus(
		ctx, state.TenantID, state.QuestionID, state.EmbeddingModelID,
		state.RetrieverEngineType, state.IndexMode, status, errorMessage,
		state.ContentHash, indexedAt,
	); err != nil {
		logger.Errorf(ctx, "update question vector index status failed: question=%s engine=%s error=%v",
			state.QuestionID, state.RetrieverEngineType, err)
	}
}

func (s *questionIndexService) ReindexQuestion(ctx context.Context, questionID string) error {
	question, err := s.questionRepo.GetQuestionByID(ctx, tenantID(ctx), questionID)
	if err != nil {
		return err
	}
	return s.IndexQuestions(ctx, []*types.Question{question})
}

func (s *questionIndexService) ReindexQuestionSet(ctx context.Context, questionSetID string) error {
	page := &types.Pagination{Page: 1, PageSize: 500}
	for {
		result, err := s.questionRepo.ListQuestions(ctx, tenantID(ctx), questionSetID, nil, page)
		if err != nil {
			return err
		}
		questions, ok := result.Data.([]*types.Question)
		if !ok {
			return fmt.Errorf("unexpected question page data type")
		}
		if err := s.IndexQuestions(ctx, questions); err != nil {
			return err
		}
		if len(questions) < page.PageSize {
			return nil
		}
		page.Page++
	}
}

func (s *questionIndexService) DeleteQuestionIndexes(ctx context.Context, questionIDs []string) error {
	if len(questionIDs) == 0 {
		return nil
	}
	states, err := s.indexRepository.ListByQuestionIDs(ctx, tenantID(ctx), questionIDs)
	if err != nil || len(states) == 0 {
		return err
	}
	background := logger.CloneContext(ctx)
	s.run(func() { s.processDeleteJobs(background, states) })
	return nil
}

func (s *questionIndexService) processDeleteJobs(ctx context.Context, states []*types.QuestionVectorIndex) {
	type deleteJob struct {
		states  []*types.QuestionVectorIndex
		service interfaces.RetrieveEngineService
		err     error
	}
	jobs := make(map[string]*deleteJob)
	for _, state := range states {
		kb, err := s.kbService.GetKnowledgeBaseByID(ctx, state.KnowledgeBaseID)
		if err != nil {
			s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusFailed, truncateQuestionIndexError(err), nil)
			continue
		}
		targets, err := s.resolveTargets(ctx, state.TenantID, kb)
		if err != nil {
			s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusFailed, truncateQuestionIndexError(err), nil)
			continue
		}
		var target *questionIndexTarget
		for i := range targets {
			if targets[i].engineType == state.RetrieverEngineType {
				target = &targets[i]
				break
			}
		}
		if target == nil {
			err := fmt.Errorf("retriever engine %s is not configured", state.RetrieverEngineType)
			s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusFailed, truncateQuestionIndexError(err), nil)
			continue
		}
		key := state.KnowledgeBaseID + "\x00" + state.EmbeddingModelID + "\x00" + string(state.RetrieverEngineType)
		job := jobs[key]
		if job == nil {
			job = &deleteJob{service: target.service, err: target.err}
			jobs[key] = job
		}
		job.states = append(job.states, state)
	}

	for _, job := range jobs {
		err := job.err
		if err == nil {
			embedder, modelErr := s.modelService.GetEmbeddingModel(ctx, job.states[0].EmbeddingModelID)
			if modelErr != nil {
				err = modelErr
			} else {
				ids := make([]string, 0, len(job.states))
				for _, state := range job.states {
					ids = append(ids, state.QuestionID)
				}
				err = job.service.DeleteBySourceIDList(ctx, ids, embedder.GetDimensions(), types.KnowledgeTypeQuestion)
			}
		}
		for _, state := range job.states {
			if err != nil {
				s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusFailed, truncateQuestionIndexError(err), nil)
			} else {
				s.updateIndexStatus(ctx, state, types.QuestionVectorIndexStatusDeleted, "", nil)
			}
		}
	}
}

var _ interfaces.QuestionIndexService = (*questionIndexService)(nil)

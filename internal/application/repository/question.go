package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type questionRepository struct {
	db *gorm.DB
}

func NewQuestionRepository(db *gorm.DB) interfaces.QuestionRepository {
	return &questionRepository{db: db}
}

type questionVectorIndexRepository struct {
	db *gorm.DB
}

func NewQuestionVectorIndexRepository(db *gorm.DB) interfaces.QuestionVectorIndexRepository {
	return &questionVectorIndexRepository{db: db}
}

func (r *questionRepository) CreateQuestionSet(ctx context.Context, qs *types.QuestionSet) error {
	return r.db.WithContext(ctx).Create(qs).Error
}

func (r *questionRepository) GetQuestionSet(ctx context.Context, tenantID uint64, id string) (*types.QuestionSet, error) {
	var qs types.QuestionSet
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&qs).Error; err != nil {
		return nil, err
	}
	return &qs, nil
}

func (r *questionRepository) GetQuestionSetByKB(ctx context.Context, tenantID uint64, kbID string) (*types.QuestionSet, error) {
	var qs types.QuestionSet
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).First(&qs).Error; err != nil {
		return nil, err
	}
	return &qs, nil
}

func (r *questionRepository) ListQuestionSets(ctx context.Context, tenantID uint64, kbID string, page *types.Pagination) (*types.PageResult, error) {
	var total int64
	var sets []*types.QuestionSet
	q := r.db.WithContext(ctx).Model(&types.QuestionSet{}).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID)
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("created_at DESC").Offset(page.Offset()).Limit(page.Limit()).Find(&sets).Error; err != nil {
		return nil, err
	}
	return types.NewPageResult(total, page, sets), nil
}

func (r *questionRepository) UpdateQuestionSet(ctx context.Context, qs *types.QuestionSet) error {
	return r.db.WithContext(ctx).Save(qs).Error
}

func (r *questionRepository) UpdateQuestionSetSourceType(
	ctx context.Context,
	tenantID uint64,
	setID string,
	sourceType types.QuestionSetSourceType,
) error {
	return r.db.WithContext(ctx).
		Model(&types.QuestionSet{}).
		Where("tenant_id = ? AND id = ?", tenantID, setID).
		Update("source_type", sourceType).Error
}

func (r *questionRepository) DeleteQuestionSet(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND question_set_id = ?", tenantID, id).Delete(&types.Question{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&types.QuestionSet{}).Error
	})
}

func (r *questionRepository) UpdateQuestionCount(ctx context.Context, tenantID uint64, setID string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&types.Question{}).Where("tenant_id = ? AND question_set_id = ?", tenantID, setID).Count(&count).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&types.QuestionSet{}).Where("tenant_id = ? AND id = ?", tenantID, setID).Update("question_count", count).Error
}

func (r *questionRepository) CreateQuestion(ctx context.Context, q *types.Question) error {
	return r.db.WithContext(ctx).Create(q).Error
}

func (r *questionRepository) CreateQuestions(ctx context.Context, questions []*types.Question) error {
	if len(questions) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&questions).Error
}

func (r *questionRepository) GetQuestion(ctx context.Context, tenantID uint64, setID, id string) (*types.Question, error) {
	var q types.Question
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND question_set_id = ? AND id = ?", tenantID, setID, id).First(&q).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *questionRepository) GetQuestionByID(ctx context.Context, tenantID uint64, id string) (*types.Question, error) {
	var q types.Question
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&q).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *questionRepository) ListQuestions(ctx context.Context, tenantID uint64, setID string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	var total int64
	var questions []*types.Question
	q := r.db.WithContext(ctx).Model(&types.Question{}).Where("tenant_id = ? AND question_set_id = ?", tenantID, setID)
	q = applyQuestionFilters(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("sort_order ASC, created_at ASC").Offset(page.Offset()).Limit(page.Limit()).Find(&questions).Error; err != nil {
		return nil, err
	}
	return types.NewPageResult(total, page, questions), nil
}

func (r *questionRepository) ListQuestionsByKB(ctx context.Context, tenantID uint64, kbID string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	var total int64
	var questions []*types.Question
	q := r.db.WithContext(ctx).Model(&types.Question{}).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID)
	q = applyQuestionFilters(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("sort_order ASC, created_at ASC").Offset(page.Offset()).Limit(page.Limit()).Find(&questions).Error; err != nil {
		return nil, err
	}
	return types.NewPageResult(total, page, questions), nil
}

func (r *questionRepository) UpdateQuestion(ctx context.Context, q *types.Question) error {
	return r.db.WithContext(ctx).Save(q).Error
}

func (r *questionRepository) DeleteQuestion(ctx context.Context, tenantID uint64, setID, id string) error {
	return r.db.WithContext(ctx).Where("tenant_id = ? AND question_set_id = ? AND id = ?", tenantID, setID, id).Delete(&types.Question{}).Error
}

func (r *questionRepository) ListQuestionsByIDs(ctx context.Context, tenantID uint64, questionIDs []string) ([]*types.Question, error) {
	if len(questionIDs) == 0 {
		return []*types.Question{}, nil
	}
	var questions []*types.Question
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, questionIDs).Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

func applyQuestionFilters(q *gorm.DB, filter *types.QuestionListFilter) *gorm.DB {
	if filter == nil {
		return q
	}
	if filter.QuestionType != "" {
		q = q.Where("question_type = ?", filter.QuestionType)
	}
	if filter.Difficulty != "" {
		q = q.Where("difficulty = ?", filter.Difficulty)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.KnowledgePoint != "" {
		b, _ := json.Marshal([]string{filter.KnowledgePoint})
		q = q.Where("knowledge_points @> ?::jsonb", string(b))
	}
	if filter.Tag != "" {
		b, _ := json.Marshal([]string{filter.Tag})
		q = q.Where("tags @> ?::jsonb", string(b))
	}
	if filter.Keyword != "" {
		pattern := "%" + filter.Keyword + "%"
		q = q.Where("stem_text ILIKE ? OR answer_text ILIKE ? OR analysis_text ILIKE ?", pattern, pattern, pattern)
	}
	return q
}

func (r *questionVectorIndexRepository) Get(
	ctx context.Context,
	tenantID uint64,
	questionID, embeddingModelID string,
	engineType types.RetrieverEngineType,
	indexMode string,
) (*types.QuestionVectorIndex, error) {
	var index types.QuestionVectorIndex
	err := r.db.WithContext(ctx).Where(
		"tenant_id = ? AND question_id = ? AND embedding_model_id = ? AND retriever_engine_type = ? AND index_mode = ?",
		tenantID, questionID, embeddingModelID, engineType, indexMode,
	).First(&index).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &index, nil
}

func (r *questionVectorIndexRepository) Upsert(ctx context.Context, index *types.QuestionVectorIndex) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "question_id"},
			{Name: "embedding_model_id"},
			{Name: "retriever_engine_type"},
			{Name: "index_mode"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id", "knowledge_base_id", "question_set_id", "content_hash",
			"status", "error_message", "indexed_at", "updated_at",
		}),
	}).Create(index).Error
}

func (r *questionVectorIndexRepository) UpdateStatus(
	ctx context.Context,
	tenantID uint64,
	questionID, embeddingModelID string,
	engineType types.RetrieverEngineType,
	indexMode string,
	status types.QuestionVectorIndexStatus,
	errorMessage, contentHash string,
	indexedAt *time.Time,
) error {
	updates := map[string]interface{}{
		"status":        status,
		"error_message": errorMessage,
		"content_hash":  contentHash,
		"indexed_at":    indexedAt,
		"updated_at":    time.Now(),
	}
	return r.db.WithContext(ctx).Model(&types.QuestionVectorIndex{}).Where(
		"tenant_id = ? AND question_id = ? AND embedding_model_id = ? AND retriever_engine_type = ? AND index_mode = ?",
		tenantID, questionID, embeddingModelID, engineType, indexMode,
	).Updates(updates).Error
}

func (r *questionVectorIndexRepository) ListByQuestionIDs(
	ctx context.Context,
	tenantID uint64,
	questionIDs []string,
) ([]*types.QuestionVectorIndex, error) {
	if len(questionIDs) == 0 {
		return []*types.QuestionVectorIndex{}, nil
	}
	var indexes []*types.QuestionVectorIndex
	if err := r.db.WithContext(ctx).Where(
		"tenant_id = ? AND question_id IN ?", tenantID, questionIDs,
	).Find(&indexes).Error; err != nil {
		return nil, err
	}
	return indexes, nil
}

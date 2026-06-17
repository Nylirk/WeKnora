package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type questionRepository struct {
	db *gorm.DB
}

func NewQuestionRepository(db *gorm.DB) interfaces.QuestionRepository {
	return &questionRepository{db: db}
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

func (r *questionRepository) ListQuestions(ctx context.Context, tenantID uint64, setID string, filter *types.QuestionListFilter, page *types.Pagination) (*types.PageResult, error) {
	var total int64
	var questions []*types.Question
	q := r.db.WithContext(ctx).Model(&types.Question{}).Where("tenant_id = ? AND question_set_id = ?", tenantID, setID)
	if filter != nil {
		if filter.QuestionType != "" {
			q = q.Where("question_type = ?", filter.QuestionType)
		}
		if filter.Difficulty != "" {
			q = q.Where("difficulty = ?", filter.Difficulty)
		}
		if filter.Status != "" {
			q = q.Where("status = ?", filter.Status)
		}
		if filter.Keyword != "" {
			q = q.Where("stem_text ILIKE ?", "%"+filter.Keyword+"%")
		}
	}
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
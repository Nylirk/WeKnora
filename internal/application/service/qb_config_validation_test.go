package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// qbTestKBRepo implements just the methods needed by validateQuestionBankReferenceConfig
type qbTestKBRepo struct {
	interfaces.KnowledgeBaseRepository
	kbs map[string]*types.KnowledgeBase
}

func (r *qbTestKBRepo) GetKnowledgeBaseByIDAndTenant(_ context.Context, id string, _ uint64) (*types.KnowledgeBase, error) {
	kb := r.kbs[id]
	if kb == nil {
		return nil, errors.New("not found")
	}
	return kb, nil
}

func TestValidateQuestionBankReferenceConfig(t *testing.T) {
	svc := &knowledgeBaseService{repo: &qbTestKBRepo{
		kbs: map[string]*types.KnowledgeBase{
			"doc-ok":  {ID: "doc-ok", Type: types.KnowledgeBaseTypeDocument},
			"wiki-ok": {ID: "wiki-ok", Type: types.KnowledgeBaseTypeWiki},
			"qb-bad":  {ID: "qb-bad", Type: types.KnowledgeBaseTypeQuestionBank},
		},
	}}

	tests := []struct {
		name    string
		cfg     *types.QuestionBankConfig
		wantErr bool
		errMsg  string
	}{
		{name: "nil config", cfg: nil, wantErr: false},
		{name: "empty both", cfg: &types.QuestionBankConfig{}, wantErr: false},
		{name: "valid document", cfg: &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "doc-ok"}, wantErr: false},
		{name: "valid wiki", cfg: &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "wiki-ok"}, wantErr: false},
		{name: "self reference KP", cfg: &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "self"}, wantErr: true,
			errMsg: "题库不能关联自身作为知识点知识库"},
		{name: "self reference syllabus", cfg: &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "self"}, wantErr: true,
			errMsg: "题库不能关联自身作为考纲"},
		{name: "question_bank as KP", cfg: &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "qb-bad"}, wantErr: true,
			errMsg: "知识点知识库不能选择题库型知识库"},
		{name: "question_bank as syllabus", cfg: &types.QuestionBankConfig{SyllabusKnowledgeBaseID: "qb-bad"}, wantErr: true,
			errMsg: "考纲不能选择题库型知识库"},
		{name: "not found", cfg: &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "no-such"}, wantErr: true,
			errMsg: "知识点知识库不存在或无权访问"},
		{name: "cross type invalid", cfg: &types.QuestionBankConfig{KnowledgePointKnowledgeBaseID: "qb-bad", SyllabusKnowledgeBaseID: "doc-ok"}, wantErr: true,
			errMsg: "知识点知识库不能选择题库型知识库"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateQuestionBankReferenceConfig(context.Background(), 1, "self", tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

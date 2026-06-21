package service

import (
	"context"
	"fmt"
	"mime/multipart"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const syllabusKBNameTemplate = "%s-考纲"

// UploadSyllabus uploads a syllabus file for a question bank KB.
// It auto-creates or reuses a hidden system-managed KB, uploads the file
// via the existing docreader pipeline, and updates the parent KB's
// question_bank_config.syllabus_knowledge_base_id.
func (s *QuestionService) UploadSyllabus(
	ctx context.Context, kbID string, fileHeader *multipart.FileHeader,
) (*types.SyllabusUploadResponse, error) {
	// 1. Validate the parent KB is a question bank.
	kb, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, apperrors.NewNotFoundError("题库不存在")
	}
	if !kb.IsQuestionBank() {
		return nil, apperrors.NewBadRequestError("仅题库型知识库支持上传考纲")
	}

	// 2. Look for existing syllabus KB.
	syllabusKB, err := s.findOrCreateSyllabusKB(ctx, kb)
	if err != nil {
		return nil, err
	}

	// 3. Upload the file to the syllabus KB via existing knowledge pipeline.
	knowledge, err := s.knowledgeService.CreateKnowledgeFromFile(
		ctx, syllabusKB.ID, fileHeader, nil, nil, "", "", "", nil,
	)
	if err != nil {
		return nil, apperrors.NewInternalServerError(
			fmt.Sprintf("上传考纲文件失败: %v", err),
		)
	}

	// 4. Persist syllabus KB binding via dedicated update path.
	// This must NOT go through UpdateKnowledgeBase, which explicitly
	// protects SyllabusKnowledgeBaseID from being overwritten.
	if err := s.knowledgeBaseSvc.UpdateQuestionBankSyllabusKnowledgeBaseID(
		ctx, kbID, syllabusKB.ID,
	); err != nil {
		logger.Warnf(ctx, "Failed to update syllabus_knowledge_base_id on parent KB %s: %v", kbID, err)
	}

	logger.Infof(ctx, "Syllabus uploaded for KB %s: file=%s, syllabusKB=%s, knowledge=%s",
		kbID, fileHeader.Filename, syllabusKB.ID, knowledge.ID)

	return &types.SyllabusUploadResponse{
		SyllabusKBID:   syllabusKB.ID,
		FileName:       knowledge.FileName,
		ParseStatus:    knowledge.ParseStatus,
		KnowledgeCount: 0,
		ChunkCount:     0,
		Message:        "考纲上传成功，正在后台解析处理",
	}, nil
}

// GetSyllabus returns syllabus info for a question bank KB.
func (s *QuestionService) GetSyllabus(
	ctx context.Context, kbID string,
) (*types.SyllabusInfo, error) {
	kb, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, apperrors.NewNotFoundError("题库不存在")
	}
	if !kb.IsQuestionBank() {
		return nil, apperrors.NewBadRequestError("仅题库型知识库支持查看考纲")
	}

	syllabusKBID := ""
	if kb.QuestionBankConfig != nil {
		syllabusKBID = kb.QuestionBankConfig.SyllabusKnowledgeBaseID
	}
	if syllabusKBID == "" {
		return nil, nil // No syllabus yet — returns nil data with no error.
	}

	syllabusKB, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, syllabusKBID)
	if err != nil {
		logger.Warnf(ctx, "Syllabus KB %s not found for parent %s: %v", syllabusKBID, kbID, err)
		return nil, nil
	}

	// Enrich with knowledge counts.
	if err := s.knowledgeBaseSvc.FillKnowledgeBaseCounts(ctx, syllabusKB); err != nil {
		logger.Warnf(ctx, "Failed to fill counts for syllabus KB %s: %v", syllabusKBID, err)
	}

	// Look up the most recent knowledge entry to get real file info.
	info := &types.SyllabusInfo{
		SyllabusKBID:   syllabusKB.ID,
		KnowledgeCount: syllabusKB.KnowledgeCount,
		ChunkCount:     syllabusKB.ChunkCount,
		ParseStatus:    "completed",
		CreatedAt:      syllabusKB.CreatedAt,
		UpdatedAt:      syllabusKB.UpdatedAt,
	}

	knowledgeList, kErr := s.knowledgeService.ListKnowledgeByKnowledgeBaseID(ctx, syllabusKBID)
	if kErr != nil {
		logger.Warnf(ctx, "Failed to list knowledge for syllabus KB %s: %v", syllabusKBID, kErr)
	} else {
		// Pick the most recently updated knowledge entry that has a file.
		var latest *types.Knowledge
		for _, k := range knowledgeList {
			if k == nil || k.FileName == "" {
				continue
			}
			if latest == nil || k.UpdatedAt.After(latest.UpdatedAt) {
				latest = k
			}
		}
		if latest != nil {
			info.FileName = latest.FileName
			info.FileSize = latest.FileSize
			info.ParseStatus = latest.ParseStatus
			info.UpdatedAt = latest.UpdatedAt
		}
	}

	if info.FileName == "" {
		if syllabusKB.ProcessingCount > 0 {
			info.ParseStatus = "processing"
		}
		if syllabusKB.KnowledgeCount == 0 && syllabusKB.ProcessingCount == 0 {
			info.ParseStatus = "empty"
		}
	}

	return info, nil
}

// DeleteSyllabus removes the syllabus KB binding and soft-deletes the hidden KB.
func (s *QuestionService) DeleteSyllabus(
	ctx context.Context, kbID string,
) error {
	kb, err := s.knowledgeBaseSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return apperrors.NewNotFoundError("题库不存在")
	}
	if !kb.IsQuestionBank() {
		return apperrors.NewBadRequestError("仅题库型知识库支持删除考纲")
	}

	syllabusKBID := ""
	if kb.QuestionBankConfig != nil {
		syllabusKBID = kb.QuestionBankConfig.SyllabusKnowledgeBaseID
	}
	if syllabusKBID == "" {
		return nil // Nothing to delete.
	}

	// Clear the binding on the parent KB via the dedicated update path.
	if err := s.knowledgeBaseSvc.UpdateQuestionBankSyllabusKnowledgeBaseID(
		ctx, kbID, "",
	); err != nil {
		return apperrors.NewInternalServerError(
			fmt.Sprintf("解除考纲绑定失败: %v", err),
		)
	}

	// Soft-delete the hidden syllabus KB.
	if err := s.knowledgeBaseSvc.DeleteKnowledgeBase(ctx, syllabusKBID); err != nil {
		logger.Warnf(ctx, "Failed to soft-delete syllabus KB %s: %v", syllabusKBID, err)
		return apperrors.NewInternalServerError(
			fmt.Sprintf("删除考纲知识库失败: %v", err),
		)
	}

	logger.Infof(ctx, "Syllabus deleted for KB %s: syllabusKB=%s", kbID, syllabusKBID)
	return nil
}

// findOrCreateSyllabusKB returns an existing or newly created hidden syllabus KB
// for the given question bank parent.
func (s *QuestionService) findOrCreateSyllabusKB(
	ctx context.Context, parentKB *types.KnowledgeBase,
) (*types.KnowledgeBase, error) {
	tenantID := parentKB.TenantID

	// Try to find existing syllabus KB via purpose + parent.
	repo := s.knowledgeBaseSvc.GetRepository()
	existing, err := repo.GetKnowledgeBaseByPurpose(
		ctx, tenantID, types.KBPurposeQuestionBankSyllabus, parentKB.ID,
	)
	if err == nil && existing != nil {
		logger.Infof(ctx, "Reusing existing syllabus KB %s for parent %s", existing.ID, parentKB.ID)
		return existing, nil
	}

	// Create a new hidden syllabus KB.
	syllabusKB := &types.KnowledgeBase{
		Name:                  fmt.Sprintf(syllabusKBNameTemplate, parentKB.Name),
		Type:                  types.KnowledgeBaseTypeDocument,
		Description:           fmt.Sprintf("系统自动创建的考纲知识库，绑定题库：%s", parentKB.Name),
		TenantID:              tenantID,
		Visibility:            types.KBVisibilityHidden,
		SystemManaged:         true,
		ParentKnowledgeBaseID: &parentKB.ID,
		Purpose:               strPtr(types.KBPurposeQuestionBankSyllabus),
		EmbeddingModelID:      parentKB.EmbeddingModelID,
		ChunkingConfig:        parentKB.ChunkingConfig,
	}
	syllabusKB.EnsureDefaults()

	// Inherit storage provider and vector store from parent.
	if parentKB.StorageProviderConfig != nil {
		syllabusKB.StorageProviderConfig = parentKB.StorageProviderConfig
	}
	if parentKB.VectorStoreID != nil {
		id := *parentKB.VectorStoreID
		syllabusKB.VectorStoreID = &id
	}

	syllabusKB.EnsureDefaults()
	// EnsureDefaults sets QuestionBankConfig=nil for non-QuestionBank KBs,
	// but the DB column is NOT NULL DEFAULT '{}'. Force a valid empty config.
	if syllabusKB.QuestionBankConfig == nil {
		syllabusKB.QuestionBankConfig = &types.QuestionBankConfig{}
	}

	if err := repo.CreateKnowledgeBase(ctx, syllabusKB); err != nil {
		return nil, apperrors.NewInternalServerError(
			fmt.Sprintf("创建隐藏考纲知识库失败: %v", err),
		)
	}

	logger.Infof(ctx, "Created hidden syllabus KB %s for parent %s", syllabusKB.ID, parentKB.ID)
	return syllabusKB, nil
}

func strPtr(s string) *string {
	return &s
}

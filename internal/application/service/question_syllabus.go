package service

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
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

	// 3. Defensive check before calling CreateKnowledgeFromFile.
	if syllabusKB == nil || strings.TrimSpace(syllabusKB.ID) == "" {
		return nil, apperrors.NewInternalServerError(
			"上传考纲文件失败: syllabus knowledge_base ID is empty",
		)
	}

	// 4. Record old knowledge IDs before uploading the new file.
	oldKnowledge, listErr := s.knowledgeService.ListKnowledgeByKnowledgeBaseID(ctx, syllabusKB.ID)
	var oldKnowledgeIDs []string
	if listErr != nil {
		logger.Warnf(ctx, "Failed to list old syllabus knowledge (continuing): %v", listErr)
	} else {
		for _, ok := range oldKnowledge {
			if ok != nil && ok.ID != "" {
				oldKnowledgeIDs = append(oldKnowledgeIDs, ok.ID)
			}
		}
	}

	// 5. Upload the new file to the syllabus KB.
	knowledge, err := s.knowledgeService.CreateKnowledgeFromFile(
		ctx, syllabusKB.ID, fileHeader, nil, nil, "", "", "", nil,
	)
	if err != nil {
		return nil, apperrors.NewInternalServerError(
			fmt.Sprintf("上传考纲文件失败: %v", err),
		)
	}

	// 6. Delete old syllabus knowledge, skipping the newly created one.
	if len(oldKnowledgeIDs) > 0 {
		toDelete := make([]string, 0, len(oldKnowledgeIDs))
		for _, id := range oldKnowledgeIDs {
			if id != knowledge.ID {
				toDelete = append(toDelete, id)
			}
		}
		if len(toDelete) > 0 {
			if delErr := s.knowledgeService.DeleteKnowledgeList(ctx, toDelete); delErr != nil {
				return nil, apperrors.NewInternalServerError(
					fmt.Sprintf("考纲文件已上传，但旧考纲清理失败: %v", delErr),
				)
			}
			logger.Infof(ctx, "Cleaned up %d old syllabus knowledge entries for KB %s", len(toDelete), kbID)
		}
	}

	// 7. Persist syllabus KB binding via dedicated update path.
	if err := s.knowledgeBaseSvc.UpdateQuestionBankSyllabusKnowledgeBaseID(
		ctx, kbID, syllabusKB.ID,
	); err != nil {
		logger.Warnf(ctx, "Failed to update syllabus_knowledge_base_id on parent KB %s: %v", kbID, err)
	}

	logger.Infof(ctx, "Syllabus uploaded for KB %s: file=%s, syllabusKB=%s, knowledge=%s",
		kbID, fileHeader.Filename, syllabusKB.ID, knowledge.ID)

	// 8. Schedule syllabus_checking for all draft questions in all question sets
	// under this KB — AFTER the new syllabus config is persisted. Use a fresh
	// background context so the async task is not cancelled by the HTTP request.
	s.scheduleSyllabusReprocessForKB(logger.CloneContext(ctx), kbID)

	return &types.SyllabusUploadResponse{
		SyllabusKBID:   syllabusKB.ID,
		FileName:       knowledge.FileName,
		ParseStatus:    knowledge.ParseStatus,
		KnowledgeCount: 0,
		ChunkCount:     0,
		Message:        "考纲上传成功",
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
		if strings.TrimSpace(existing.ID) == "" {
			repairedID := uuid.NewString()
			rows, repairErr := repo.RepairKnowledgeBaseEmptyIDByPurpose(
				ctx, tenantID, types.KBPurposeQuestionBankSyllabus, parentKB.ID, repairedID,
			)
			if repairErr != nil {
				return nil, apperrors.NewInternalServerError(
					fmt.Sprintf("修复隐藏考纲知识库 ID 失败: %v", repairErr),
				)
			}
			if rows == 0 {
				// The corrupt row may have been deleted by another process.
				// Fall through to create a fresh KB.
				logger.Warnf(ctx,
					"Repair of empty-ID syllabus KB affected 0 rows (parent=%s), creating fresh",
					parentKB.ID)
			} else {
				existing.ID = repairedID
				existing.NormalizeNotNullJSONB()
				logger.Warnf(ctx,
					"Repaired empty ID for hidden syllabus KB: parent=%s repaired_id=%s",
					parentKB.ID, repairedID)
			}
		}
		if strings.TrimSpace(existing.ID) != "" {
			logger.Infof(ctx, "Reusing existing syllabus KB %s for parent %s", existing.ID, parentKB.ID)
			return existing, nil
		}
		// existing.ID still empty after repair (0 rows affected) → create fresh.
	}

	// Create a new hidden syllabus KB.
	syllabusKB := &types.KnowledgeBase{
		ID:                    uuid.NewString(),
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
	if strings.TrimSpace(syllabusKB.ID) == "" {
		return nil, apperrors.NewInternalServerError(
			"创建隐藏考纲知识库失败: generated syllabus knowledge_base ID is empty",
		)
	}

	logger.Infof(ctx, "Created hidden syllabus KB %s for parent %s", syllabusKB.ID, parentKB.ID)
	return syllabusKB, nil
}

// scheduleSyllabusReprocessForKB triggers syllabus_checking for all draft questions
// in all question sets under the given question bank KB. Runs in background goroutines
// and does not block the caller.
func (s *QuestionService) scheduleSyllabusReprocessForKB(ctx context.Context, kbID string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf(ctx, "panic in syllabus reprocess scheduler for KB %s: %v", kbID, r)
			}
		}()

		page := &types.Pagination{Page: 1, PageSize: 100}
		for {
			result, err := s.repository.ListQuestionSets(ctx, tenantID(ctx), kbID, page)
			if err != nil {
				logger.Warnf(ctx, "Failed to list question sets for KB %s during syllabus reprocess: %v", kbID, err)
				return
			}
			sets, ok := result.Data.([]*types.QuestionSet)
			if !ok || len(sets) == 0 {
				return
			}
			for _, qs := range sets {
				if qs == nil {
					continue
				}
				if err := s.ReprocessQuestionSet(ctx, kbID, qs.ID, "syllabus_checking"); err != nil {
					logger.Warnf(ctx, "Failed to schedule syllabus_checking for set %s: %v", qs.ID, err)
				}
			}
			if int64(len(sets)) < result.Total && len(sets) < page.PageSize {
				return
			}
			page.Page++
		}
	}()
}

func strPtr(s string) *string {
	return &s
}

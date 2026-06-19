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
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// PreviewImportBlocks extracts text from a document file, partitions it into
// blocks using the block analysis pipeline, and returns the blocks for review.
// It does NOT persist anything to the database.
func (s *QuestionService) PreviewImportBlocks(
	ctx context.Context,
	kbID, setID string,
	fileData []byte,
	fileName string,
	req *types.BlockPreviewRequest,
) (*types.BlockPreviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Infof("[block-preview] started: kb=%s set=%s file=%s size=%d preset=%s mode=%s",
		kbID, setID, fileName, len(fileData), req.StrategyPreset, req.ImportMode)

	// 1. Validate KB is question_bank (also validates set belongs to kb)
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

	log.Infof("[block-preview] docreader read started: file=%s type=%s", fileName, fileType)
	readResp, err := s.docReader.Read(readCtx, &types.ReadRequest{
		FileContent: fileData,
		FileName:    fileName,
		FileType:    fileType,
	})
	if err != nil {
		if readCtx.Err() == context.DeadlineExceeded {
			log.Warnf("[block-preview] docreader timed out: file=%s", fileName)
			return nil, apperrors.NewBadRequestError("文档解析超时，请尝试拆分文件或使用 JSON/JSONL 导入。")
		}
		log.Errorf("[block-preview] docreader read failed: file=%s err=%v", fileName, err)
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("文档解析失败: %s", err.Error()),
		)
	}
	if readResp.Error != "" {
		log.Errorf("[block-preview] docreader returned error: file=%s err=%s", fileName, readResp.Error)
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("文档解析失败: %s", readResp.Error),
		)
	}
	log.Infof("[block-preview] docreader read finished: file=%s markdown_len=%d", fileName, len(readResp.MarkdownContent))

	extractedText := strings.TrimSpace(readResp.MarkdownContent)

	// 6. Choose strategy
	var strategy types.BlockParseStrategy
	switch req.StrategyPreset {
	case "pdf":
		strategy = types.PDFBlockParseStrategy()
	default:
		strategy = types.GeneralBlockParseStrategy()
	}

	// 7. Analyze blocks
	blocks, summary := s.blockAnalysisService.AnalyzeBlocks(extractedText, strategy)

	log.Infof("[block-preview] analysis complete: blocks=%d anomalies=%d qnums=%d",
		summary.TotalBlocks, summary.BlocksWithAnomalies, summary.QuestionNumbers)

	return &types.BlockPreviewResponse{
		Blocks:  blocks,
		Summary: summary,
	}, nil
}

// ParseImportedBlocks takes user-edited blocks, re-parses them through the
// question extraction service, and returns a preview of the resulting questions.
// It does NOT persist anything to the database.
func (s *QuestionService) ParseImportedBlocks(
	ctx context.Context,
	kbID, setID string,
	req *types.ParseBlocksRequest,
) (*types.ImportFilePreviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Infof("[parse-blocks] started: kb=%s set=%s blocks=%d", kbID, setID, len(req.Blocks))

	// 1. Validate KB is question_bank (also validates set belongs to kb)
	if _, err := s.getQuestionSetForKB(ctx, kbID, setID); err != nil {
		return nil, err
	}

	// 2. Choose strategy
	var strategy types.BlockParseStrategy
	switch req.StrategyPreset {
	case "pdf":
		strategy = types.PDFBlockParseStrategy()
	default:
		strategy = types.GeneralBlockParseStrategy()
	}

	// 3. Default difficulty
	defaultDifficulty := req.DefaultDifficulty
	if defaultDifficulty == "" {
		defaultDifficulty = string(types.QuestionDifficultyMedium)
	}

	// 4. For each block, run question extraction on CurrentText
	var allItems []types.ImportQuestionItem
	var allErrors []types.ImportQuestionError
	var allWarnings []string

	for _, block := range req.Blocks {
		if block.CurrentText == "" {
			continue
		}

		items, parseErrors, warnings := s.extractionService.Extract(
			ctx, block.CurrentText, string(types.QuestionTypeShortAnswer), defaultDifficulty,
		)

		// Propagate block tags to each extracted item
		for i := range items {
			item := &items[i]

			// Merge block tags with existing item tags (dedup)
			if len(block.Tags) > 0 {
				// Parse existing tags from JSON
				var existingTags []string
				if len(item.Tags) > 0 {
					_ = json.Unmarshal(item.Tags, &existingTags)
				}

				tagSet := make(map[string]bool)
				for _, t := range existingTags {
					tagSet[t] = true
				}
				for _, t := range block.Tags {
					if !tagSet[t] {
						existingTags = append(existingTags, t)
						tagSet[t] = true
					}
				}

				// Re-marshal to JSON
				if len(existingTags) > 0 {
					marshaled, _ := json.Marshal(existingTags)
					item.Tags = types.JSON(marshaled)
				}
			}

			// Write block metadata to source_payload for compatibility
			if len(block.Metadata) > 0 {
				metaJSON, _ := json.Marshal(map[string]interface{}{
					"block_id":        block.ID,
					"block_index":     block.Index,
					"block_tags":      block.Tags,
					"block_metadata":  block.Metadata,
					"question_number": block.QuestionNumber,
				})
				item.SourcePayload = types.JSON(metaJSON)
			}
		}

		// Adjust line numbers based on block position
		blockLineBase := block.Index * 1000
		for i := range items {
			items[i].LineNumber += blockLineBase
		}
		for i := range parseErrors {
			parseErrors[i].LineNumber += blockLineBase
		}

		allItems = append(allItems, items...)
		allErrors = append(allErrors, parseErrors...)
		allWarnings = append(allWarnings, warnings...)
	}

	// 5. Build stats
	withAnswer := 0
	withoutAnswer := 0
	for _, item := range allItems {
		if item.AnswerText != "" {
			withAnswer++
		} else {
			withoutAnswer++
		}
	}

	_ = strategy // used implicitly through block analysis

	log.Infof("[parse-blocks] complete: items=%d errors=%d warnings=%d",
		len(allItems), len(allErrors), len(allWarnings))

	return &types.ImportFilePreviewResponse{
		Items:    allItems,
		Errors:   allErrors,
		Warnings: allWarnings,
		Stats: types.ImportFilePreviewStats{
			DetectedQuestions: len(allItems),
			WithAnswer:        withAnswer,
			WithoutAnswer:     withoutAnswer,
		},
	}, nil
}

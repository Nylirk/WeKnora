package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// PreviewImportBlocks handles POST /import-file/block-preview
func (h *QuestionHandler) PreviewImportBlocks(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")

	var req types.BlockPreviewRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		questionBadRequest(c, err)
		return
	}

	if req.ImportMode == "" {
		req.ImportMode = "batch"
	}

	const defaultMaxFileImportBytes = 20 * 1024 * 1024
	maxSize := secutils.GetMaxFileSize()
	if maxSize > defaultMaxFileImportBytes || maxSize < 0 {
		maxSize = defaultMaxFileImportBytes
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") || strings.Contains(err.Error(), "http: request body too large") {
			questionBadRequest(c, apperrors.NewBadRequestError(
				fmt.Sprintf("文件大小超过限制 (%d MB)，请压缩文件或使用 JSON/JSONL 导入。", maxSize/(1024*1024)),
			))
			return
		}
		questionBadRequest(c, apperrors.NewBadRequestError("需要上传文件"))
		return
	}
	defer file.Close()

	if header.Size > maxSize {
		questionBadRequest(c, apperrors.NewBadRequestError(
			fmt.Sprintf("文件大小超过限制 (%d MB)，请压缩文件或使用 JSON/JSONL 导入。", maxSize/(1024*1024)),
		))
		return
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		questionHandleError(c, err)
		return
	}

	result, err := h.questionService.PreviewImportBlocks(
		c.Request.Context(), kbID, setID, fileData, header.Filename, &req,
	)
	if err != nil {
		questionHandleError(c, err)
		return
	}

	questionOK(c, result)
}

// ParseImportedBlocks handles POST /import-file/parse-blocks
func (h *QuestionHandler) ParseImportedBlocks(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")

	var req types.ParseBlocksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}

	result, err := h.questionService.ParseImportedBlocks(
		c.Request.Context(), kbID, setID, &req,
	)
	if err != nil {
		questionHandleError(c, err)
		return
	}

	questionOK(c, result)
}

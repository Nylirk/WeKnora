package handler

import (
	stderrors "errors"
	"io"
	"net/http"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type QuestionHandler struct {
	questionService interfaces.QuestionService
}

func NewQuestionHandler(svc interfaces.QuestionService) *QuestionHandler {
	return &QuestionHandler{questionService: svc}
}

func questionOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
func questionBadRequest(c *gin.Context, err error) {
	_ = c.Error(apperrors.NewBadRequestError(err.Error()))
}
func questionNotFoundError(c *gin.Context, err error) {
	_ = c.Error(apperrors.NewNotFoundError("question resource not found"))
}
func questionHandleError(c *gin.Context, err error) {
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		questionNotFoundError(c, err)
		return
	}
	if appErr, ok := apperrors.IsAppError(err); ok {
		_ = c.Error(appErr)
		return
	}
	_ = c.Error(apperrors.NewInternalServerError(err.Error()))
}

func (h *QuestionHandler) CreateQuestionSet(c *gin.Context) {
	var req types.CreateQuestionSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	result, err := h.questionService.CreateQuestionSet(c.Request.Context(), kbID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) GetQuestionSet(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.GetQuestionSet(c.Request.Context(), kbID, setID)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) ListQuestionSets(c *gin.Context) {
	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	result, err := h.questionService.ListQuestionSets(c.Request.Context(), kbID, &page)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) UpdateQuestionSet(c *gin.Context) {
	var req types.UpdateQuestionSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.UpdateQuestionSet(c.Request.Context(), kbID, setID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) DeleteQuestionSet(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")
	if err := h.questionService.DeleteQuestionSet(c.Request.Context(), kbID, setID); err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, gin.H{})
}

func (h *QuestionHandler) CreateQuestion(c *gin.Context) {
	var req types.CreateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.CreateQuestion(c.Request.Context(), kbID, setID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) GetQuestion(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")
	questionID := c.Param("question_id")
	result, err := h.questionService.GetQuestion(c.Request.Context(), kbID, setID, questionID)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) ListQuestions(c *gin.Context) {
	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		questionBadRequest(c, err)
		return
	}
	var filter types.QuestionListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.ListQuestions(c.Request.Context(), kbID, setID, &filter, &page)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) UpdateQuestion(c *gin.Context) {
	var req types.UpdateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	questionID := c.Param("question_id")
	result, err := h.questionService.UpdateQuestion(c.Request.Context(), kbID, setID, questionID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) DeleteQuestion(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")
	questionID := c.Param("question_id")
	if err := h.questionService.DeleteQuestion(c.Request.Context(), kbID, setID, questionID); err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, gin.H{})
}

func (h *QuestionHandler) UpdateQuestionStatus(c *gin.Context) {
	var req types.UpdateQuestionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	questionID := c.Param("question_id")
	result, err := h.questionService.UpdateQuestionStatus(c.Request.Context(), kbID, setID, questionID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) ImportQuestions(c *gin.Context) {
	var req types.ImportQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.ImportQuestions(c.Request.Context(), kbID, setID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) ExportToEvaluationDataset(c *gin.Context) {
	var req types.ExportToEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	setID := c.Param("set_id")
	result, err := h.questionService.ExportToEvaluationDataset(c.Request.Context(), kbID, setID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) GenerateQuestions(c *gin.Context) {
	var req types.GenerateQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		questionBadRequest(c, err)
		return
	}
	kbID := c.Param("id")
	result, err := h.questionService.GenerateQuestions(c.Request.Context(), kbID, &req)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}

func (h *QuestionHandler) PreviewImportQuestionsFromFile(c *gin.Context) {
	kbID := c.Param("id")
	setID := c.Param("set_id")

	// Parse query params
	var req types.ImportFilePreviewRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		questionBadRequest(c, err)
		return
	}

	// Read uploaded file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		questionBadRequest(c, apperrors.NewBadRequestError("需要上传文件"))
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		questionHandleError(c, err)
		return
	}

	result, err := h.questionService.PreviewImportQuestionsFromFile(
		c.Request.Context(), kbID, setID, fileData, header.Filename, &req,
	)
	if err != nil {
		questionHandleError(c, err)
		return
	}
	questionOK(c, result)
}
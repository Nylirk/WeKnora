package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterQuestionRoutes(r *gin.RouterGroup, h *handler.QuestionHandler, g *rbacGuards) {
	questionSets := r.Group("/knowledge-bases/:id/question-sets")
	{
		questionSets.GET("", g.Viewer(), g.KBAccessRead("id"), h.ListQuestionSets)
		questionSets.POST("", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.CreateQuestionSet)
		questionSets.POST("/generate", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.GenerateQuestions)
	}

	sets := r.Group("/question-sets/:set_id")
	{
		sets.GET("", g.Viewer(), h.GetQuestionSet)
		sets.PUT("", g.Admin(), h.UpdateQuestionSet)
		sets.DELETE("", g.Admin(), h.DeleteQuestionSet)

		questions := sets.Group("/questions")
		{
			questions.GET("", g.Viewer(), h.ListQuestions)
			questions.POST("", g.Admin(), h.CreateQuestion)
			questions.GET("/:question_id", g.Viewer(), h.GetQuestion)
			questions.PUT("/:question_id", g.Admin(), h.UpdateQuestion)
			questions.DELETE("/:question_id", g.Admin(), h.DeleteQuestion)
			questions.PUT("/:question_id/status", g.Admin(), h.UpdateQuestionStatus)
			questions.POST("/import", g.Admin(), h.ImportQuestions)
			questions.POST("/export", g.Admin(), h.ExportToEvaluationDataset)
		}
	}
}
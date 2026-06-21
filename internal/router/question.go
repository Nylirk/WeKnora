package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterQuestionRoutes(r *gin.RouterGroup, h *handler.QuestionHandler, g *rbacGuards) {
	kb := r.Group("/knowledge-bases/:id")
	{
		// Syllabus management (question bank only).
		qb := kb.Group("/question-bank")
		{
			qb.GET("/syllabus", g.Viewer(), g.KBAccessRead("id"), h.GetSyllabus)
			qb.POST("/syllabus", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.UploadSyllabus)
			qb.DELETE("/syllabus", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.DeleteSyllabus)
		}

		qs := kb.Group("/question-sets")
		{
			qs.GET("", g.Viewer(), g.KBAccessRead("id"), h.ListQuestionSets)
			qs.POST("", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.CreateQuestionSet)
			qs.POST("/generate", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.GenerateQuestions)
			qs.GET("/:set_id", g.Viewer(), g.KBAccessRead("id"), h.GetQuestionSet)
			qs.PUT("/:set_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.UpdateQuestionSet)
			qs.DELETE("/:set_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.DeleteQuestionSet)
			qs.GET("/:set_id/processing-status", g.Viewer(), g.KBAccessRead("id"), h.GetQuestionSetProcessingStatus)
		qs.POST("/:set_id/processing/reprocess", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.ReprocessQuestionSet)

			questions := qs.Group("/:set_id/questions")
			{
				questions.GET("", g.Viewer(), g.KBAccessRead("id"), h.ListQuestions)
				questions.POST("", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.CreateQuestion)
				questions.GET("/:question_id", g.Viewer(), g.KBAccessRead("id"), h.GetQuestion)
				questions.PUT("/:question_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.UpdateQuestion)
				questions.DELETE("/:question_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.DeleteQuestion)
				questions.PUT("/:question_id/status", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.UpdateQuestionStatus)
				questions.POST("/import", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.ImportQuestions)
				questions.POST("/import-file/preview", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.PreviewImportQuestionsFromFile)
				questions.POST("/import-file/block-preview", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.PreviewImportBlocks)
				questions.POST("/import-file/parse-blocks", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.ParseImportedBlocks)
				questions.POST("/export", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), h.ExportToEvaluationDataset)
			}
		}
	}
}

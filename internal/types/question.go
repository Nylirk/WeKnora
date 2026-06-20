package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type QuestionVectorIndexStatus string

const (
	QuestionVectorIndexStatusPending  QuestionVectorIndexStatus = "pending"
	QuestionVectorIndexStatusIndexing QuestionVectorIndexStatus = "indexing"
	QuestionVectorIndexStatusIndexed  QuestionVectorIndexStatus = "indexed"
	QuestionVectorIndexStatusFailed   QuestionVectorIndexStatus = "failed"
	QuestionVectorIndexStatusDeleted  QuestionVectorIndexStatus = "deleted"

	QuestionVectorIndexModePrompt = "question_prompt"
)

type QuestionVectorIndex struct {
	ID                  string                    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID            uint64                    `json:"tenant_id" gorm:"not null;index;uniqueIndex:uq_question_vector_index_target"`
	KnowledgeBaseID     string                    `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index"`
	QuestionSetID       string                    `json:"question_set_id" gorm:"type:varchar(36);not null;index"`
	QuestionID          string                    `json:"question_id" gorm:"type:varchar(36);not null;index;uniqueIndex:uq_question_vector_index_target"`
	EmbeddingModelID    string                    `json:"embedding_model_id" gorm:"type:varchar(36);not null;uniqueIndex:uq_question_vector_index_target"`
	RetrieverEngineType RetrieverEngineType       `json:"retriever_engine_type" gorm:"type:varchar(50);not null;uniqueIndex:uq_question_vector_index_target"`
	IndexMode           string                    `json:"index_mode" gorm:"type:varchar(32);not null;default:'question_prompt';uniqueIndex:uq_question_vector_index_target"`
	ContentHash         string                    `json:"content_hash" gorm:"type:varchar(64);not null;default:''"`
	Status              QuestionVectorIndexStatus `json:"status" gorm:"type:varchar(16);not null;default:'pending';index"`
	ErrorMessage        string                    `json:"error_message" gorm:"type:text;not null;default:''"`
	IndexedAt           *time.Time                `json:"indexed_at"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

func (*QuestionVectorIndex) TableName() string { return "question_vector_indexes" }
func (qvi *QuestionVectorIndex) BeforeCreate(*gorm.DB) error {
	if qvi.ID == "" {
		qvi.ID = uuid.NewString()
	}
	return nil
}

type QuestionSetSourceType string

const (
	QuestionSetSourceManual    QuestionSetSourceType = "manual"
	QuestionSetSourceImport    QuestionSetSourceType = "import"
	QuestionSetSourceGenerated QuestionSetSourceType = "generated"
	QuestionSetSourceExamPaper QuestionSetSourceType = "exam_paper"
)

type QuestionSetStatus string

const (
	QuestionSetStatusActive    QuestionSetStatus = "active"
	QuestionSetStatusCompleted QuestionSetStatus = "completed"
	QuestionSetStatusPending   QuestionSetStatus = "pending"
	QuestionSetStatusFailed    QuestionSetStatus = "failed"
)

type QuestionSet struct {
	ID               string                     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID         uint64                     `json:"tenant_id" gorm:"index;not null"`
	KnowledgeBaseID  string                     `json:"knowledge_base_id" gorm:"type:varchar(36);index;not null"`
	Name             string                     `json:"name" gorm:"type:varchar(255);not null"`
	Description      string                     `json:"description" gorm:"type:text;not null;default:''"`
	SourceType       QuestionSetSourceType      `json:"source_type" gorm:"type:varchar(32);not null;default:'manual'"`
	Status           QuestionSetStatus          `json:"status" gorm:"type:varchar(32);not null;default:'active'"`
	QuestionCount    int                        `json:"question_count" gorm:"column:question_count;not null;default:0"`
	GenerationConfig JSON                       `json:"generation_config" gorm:"type:jsonb;not null"`
	GenerationScope  JSON                       `json:"generation_scope" gorm:"type:jsonb;not null"`
	ProcessingStage  QuestionSetProcessingStage `json:"processing_stage" gorm:"type:varchar(32);not null;default:''"`
	ErrorMessage     string                     `json:"error_message" gorm:"type:text;not null;default:''"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
	DeletedAt        gorm.DeletedAt             `json:"-" gorm:"index"`
}

func (*QuestionSet) TableName() string { return "question_sets" }
func (qs *QuestionSet) BeforeCreate(*gorm.DB) error {
	if qs.ID == "" {
		qs.ID = uuid.NewString()
	}
	return nil
}

type QuestionDifficulty string

const (
	QuestionDifficultyEasy   QuestionDifficulty = "easy"
	QuestionDifficultyMedium QuestionDifficulty = "medium"
	QuestionDifficultyHard   QuestionDifficulty = "hard"
)

type QuestionStatus string

const (
	QuestionStatusDraft    QuestionStatus = "draft"
	QuestionStatusReviewed QuestionStatus = "reviewed"
	QuestionStatusRejected QuestionStatus = "rejected"
)

type Question struct {
	ID                 string             `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID           uint64             `json:"tenant_id" gorm:"index;not null"`
	QuestionSetID      string             `json:"question_set_id" gorm:"type:varchar(36);index;not null"`
	KnowledgeBaseID    string             `json:"knowledge_base_id" gorm:"type:varchar(36);index;not null"`
	QuestionType       string             `json:"question_type" gorm:"type:varchar(64);not null;default:'single_choice'"`
	SchemaVersion      string             `json:"schema_version" gorm:"type:varchar(16);not null;default:'v1'"`
	StemText           string             `json:"stem_text" gorm:"type:text;not null;default:''"`
	QuestionBody       JSON               `json:"question_body" gorm:"type:jsonb;not null"`
	AnswerText         string             `json:"answer_text" gorm:"type:text;not null;default:''"`
	AnswerBody         JSON               `json:"answer_body" gorm:"type:jsonb;not null"`
	AnalysisText       string             `json:"analysis_text" gorm:"type:text;not null;default:''"`
	GradingRubric      JSON               `json:"grading_rubric" gorm:"type:jsonb;not null"`
	Difficulty         QuestionDifficulty `json:"difficulty" gorm:"type:varchar(16);not null;default:'medium'"`
	Status             QuestionStatus     `json:"status" gorm:"type:varchar(32);not null;default:'draft'"`
	ReviewedBy         string             `json:"reviewed_by" gorm:"type:varchar(36);not null;default:''"`
	ReviewedAt         *time.Time         `json:"reviewed_at"`
	KnowledgePoints    JSON               `json:"knowledge_points" gorm:"type:jsonb;not null"`
	Tags               JSON               `json:"tags" gorm:"type:jsonb;not null"`
	SourceKnowledgeID  string             `json:"source_knowledge_id" gorm:"type:varchar(36);not null;default:''"`
	EvidenceChunkIDs   JSON               `json:"evidence_chunk_ids" gorm:"type:jsonb;not null"`
	SourcePayload      JSON               `json:"source_payload" gorm:"type:jsonb;not null"`
	ExtractionMetadata JSON               `json:"extraction_metadata" gorm:"type:jsonb;not null"`
	SortOrder          int                `json:"sort_order" gorm:"not null;default:0"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          gorm.DeletedAt     `json:"-" gorm:"index"`
}

func (*Question) TableName() string { return "questions" }
func (q *Question) BeforeCreate(*gorm.DB) error {
	if q.ID == "" {
		q.ID = uuid.NewString()
	}
	return nil
}

type QuestionListFilter struct {
	QuestionType   string `form:"question_type"`
	Difficulty     string `form:"difficulty"`
	Status         string `form:"status"`
	KnowledgePoint string `form:"knowledge_point"`
	Tag            string `form:"tag"`
	Keyword        string `form:"keyword"`
}

// QuestionSetProcessingStage tracks the background processing stage of a question set
// after import. It is only meaningful for sets with source_type=import.
type QuestionSetProcessingStage string

const (
	QuestionSetProcessingStageIdle             QuestionSetProcessingStage = "" // not yet imported or no auto-processing
	QuestionSetProcessingStageDraftImported    QuestionSetProcessingStage = "draft_imported"
	QuestionSetProcessingStageIndexing         QuestionSetProcessingStage = "indexing"
	QuestionSetProcessingStageAutoTagging      QuestionSetProcessingStage = "auto_tagging"
	QuestionSetProcessingStageSyllabusChecking QuestionSetProcessingStage = "syllabus_checking"
	QuestionSetProcessingStageReadyForReview   QuestionSetProcessingStage = "ready_for_review"
	QuestionSetProcessingStageFailed           QuestionSetProcessingStage = "failed"
)

// QuestionSetProcessingStatus is the API response for question set processing status.
type QuestionSetProcessingStatus struct {
	Stage                    QuestionSetProcessingStage `json:"stage"`
	ErrorMessage             string                     `json:"error_message"`
	SkippedAutoTaggingReason string                     `json:"skipped_auto_tagging_reason,omitempty"`
	SkippedSyllabusReason    string                     `json:"skipped_syllabus_reason,omitempty"`
	AutoTaggingEnabled       bool                       `json:"auto_tagging_enabled"`
	SyllabusCheckEnabled     bool                       `json:"syllabus_check_enabled"`
}

type CreateQuestionSetRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateQuestionSetRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type CreateQuestionRequest struct {
	QuestionType      string `json:"question_type" binding:"required"`
	StemText          string `json:"stem_text" binding:"required"`
	QuestionBody      JSON   `json:"question_body"`
	AnswerText        string `json:"answer_text"`
	AnswerBody        JSON   `json:"answer_body"`
	AnalysisText      string `json:"analysis_text"`
	GradingRubric     JSON   `json:"grading_rubric"`
	Difficulty        string `json:"difficulty"`
	KnowledgePoints   JSON   `json:"knowledge_points"`
	Tags              JSON   `json:"tags"`
	SourceKnowledgeID string `json:"source_knowledge_id"`
	EvidenceChunkIDs  JSON   `json:"evidence_chunk_ids"`
	SortOrder         int    `json:"sort_order"`
}

type UpdateQuestionRequest struct {
	QuestionType      *string `json:"question_type"`
	StemText          *string `json:"stem_text"`
	QuestionBody      *JSON   `json:"question_body"`
	AnswerText        *string `json:"answer_text"`
	AnswerBody        *JSON   `json:"answer_body"`
	AnalysisText      *string `json:"analysis_text"`
	GradingRubric     *JSON   `json:"grading_rubric"`
	Difficulty        *string `json:"difficulty"`
	KnowledgePoints   *JSON   `json:"knowledge_points"`
	Tags              *JSON   `json:"tags"`
	SourceKnowledgeID *string `json:"source_knowledge_id"`
	EvidenceChunkIDs  *JSON   `json:"evidence_chunk_ids"`
	SortOrder         *int    `json:"sort_order"`
}

type UpdateQuestionStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type ImportQuestionItem struct {
	LineNumber        int    `json:"line_number"`
	QuestionType      string `json:"question_type"`
	StemText          string `json:"stem_text"`
	QuestionBody      JSON   `json:"question_body"`
	AnswerText        string `json:"answer_text"`
	AnswerBody        JSON   `json:"answer_body"`
	AnalysisText      string `json:"analysis_text"`
	GradingRubric     JSON   `json:"grading_rubric"`
	Difficulty        string `json:"difficulty"`
	KnowledgePoints   JSON   `json:"knowledge_points"`
	Tags              JSON   `json:"tags"`
	SourceKnowledgeID string `json:"source_knowledge_id"`
	EvidenceChunkIDs  JSON   `json:"evidence_chunk_ids"`
	Status            string `json:"status,omitempty"`
	RawText           string `json:"raw_text,omitempty"`
	SourcePayload     JSON   `json:"source_payload,omitempty"`
}

type ImportQuestionError struct {
	LineNumber int    `json:"line_number"`
	Message    string `json:"message"`
}

type ImportQuestionsRequest struct {
	Items []ImportQuestionItem `json:"items" binding:"required,dive"`
}

type ImportQuestionsResult struct {
	Created int                   `json:"created"`
	Errors  []ImportQuestionError `json:"errors"`
}

type ExportToEvaluationRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type GenerateQuestionsRequest struct {
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	GenerationConfig JSON   `json:"generation_config"`
	GenerationScope  JSON   `json:"generation_scope"`
}

type ImportFilePreviewRequest struct {
	DefaultQuestionType string `form:"default_question_type"`
	DefaultDifficulty   string `form:"default_difficulty"`
	Mode                string `form:"mode"`
	DebugExport         bool   `form:"debug_export"`
}

type ImportFilePreviewStats struct {
	DetectedQuestions int `json:"detected_questions"`
	WithAnswer        int `json:"with_answer"`
	WithoutAnswer     int `json:"without_answer"`
}

type ImportFilePreviewResponse struct {
	Items           []ImportQuestionItem   `json:"items"`
	Errors          []ImportQuestionError  `json:"errors"`
	Warnings        []string               `json:"warnings"`
	RawTextPreview  string                 `json:"raw_text_preview"`
	Stats           ImportFilePreviewStats `json:"stats"`
	DebugExportPath string                 `json:"debug_export_path,omitempty"`
	DebugManifest   []string               `json:"-"`
}

// SyllabusInfo describes the syllabus knowledge base bound to a question bank.
type SyllabusInfo struct {
	SyllabusKBID   string    `json:"syllabus_kb_id"`
	FileName       string    `json:"file_name"`
	FileSize       int64     `json:"file_size"`
	ParseStatus    string    `json:"parse_status"`
	KnowledgeCount int64     `json:"knowledge_count"`
	ChunkCount     int64     `json:"chunk_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SyllabusUploadResponse is returned after uploading a syllabus file.
type SyllabusUploadResponse struct {
	SyllabusKBID   string `json:"syllabus_kb_id"`
	FileName       string `json:"file_name"`
	ParseStatus    string `json:"parse_status"`
	KnowledgeCount int64  `json:"knowledge_count"`
	ChunkCount     int64  `json:"chunk_count"`
	Message        string `json:"message"`
}

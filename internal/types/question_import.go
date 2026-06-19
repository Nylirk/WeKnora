package types

// ImportBlock represents a single block parsed from the uploaded file
// during the block-preview stage of question import.
type ImportBlock struct {
	ID             string                 `json:"id"`
	Index          int                    `json:"index"`
	OriginalText   string                 `json:"original_text"`
	CurrentText    string                 `json:"current_text"`
	QuestionNumber *int                   `json:"question_number"`
	Tags           []string               `json:"tags"`
	Metadata       map[string]interface{} `json:"metadata"`
	Anomalies      []ImportBlockAnomaly   `json:"anomalies"`
}

// ImportBlockAnomaly describes a detected issue with a block.
type ImportBlockAnomaly struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

// Anomaly type constants.
const (
	AnomalyMultipleQuestionMarkers   = "MULTIPLE_QUESTION_MARKERS"
	AnomalyNonMonotonicQuestionNum   = "NON_MONOTONIC_QUESTION_NUMBER"
	AnomalyQuestionNumberGap         = "QUESTION_NUMBER_GAP"
	AnomalyDuplicateQuestionNumber   = "DUPLICATE_QUESTION_NUMBER"
	AnomalyMissingQuestionNumber     = "MISSING_QUESTION_NUMBER"
	AnomalyOptionOnlyBlock           = "OPTION_ONLY_BLOCK"
	AnomalyOptionSequenceRestart     = "OPTION_SEQUENCE_RESTART"
	AnomalyPageNoiseDetected         = "PAGE_NOISE_DETECTED"
	AnomalySectionHeadingInStem      = "SECTION_HEADING_IN_STEM"
	AnomalyQuestionTypeHeadingInStem = "QUESTION_TYPE_HEADING_IN_STEM"
	AnomalyMissingAnswer             = "MISSING_ANSWER"
	AnomalyStemTooShort              = "STEM_TOO_SHORT"
	AnomalyStemTooLong               = "STEM_TOO_LONG"
	AnomalyAnswerOutOfOptions        = "ANSWER_OUT_OF_OPTIONS"
	AnomalyAnswerAnalysisMixed       = "ANSWER_ANALYSIS_MIXED"
)

// BlockParseStrategy defines how the block analysis pipeline should behave.
type BlockParseStrategy struct {
	SplitEmbeddedQuestionNumbers       bool `json:"split_embedded_question_numbers"`
	AllowBareQuestionNumber            bool `json:"allow_bare_question_number"`
	RemovePageNumbers                  bool `json:"remove_page_numbers"`
	DetectSectionHeadings              bool `json:"detect_section_headings"`
	DetectQuestionTypeHeadings         bool `json:"detect_question_type_headings"`
	ExtractSectionTags                 bool `json:"extract_section_tags"`
	ExtractQuestionTypeTags            bool `json:"extract_question_type_tags"`
	SortBlocksByQuestionNumber         bool `json:"sort_blocks_by_question_number"`
	DetectInterleavedTwoColumnSequence bool `json:"detect_interleaved_two_column_sequence"`
}

// GeneralBlockParseStrategy returns the "general" preset strategy.
func GeneralBlockParseStrategy() BlockParseStrategy {
	return BlockParseStrategy{
		SplitEmbeddedQuestionNumbers:       true,
		AllowBareQuestionNumber:            false,
		RemovePageNumbers:                  true,
		DetectSectionHeadings:              true,
		DetectQuestionTypeHeadings:         true,
		ExtractSectionTags:                 true,
		ExtractQuestionTypeTags:            true,
		SortBlocksByQuestionNumber:         false,
		DetectInterleavedTwoColumnSequence: false,
	}
}

// PDFBlockParseStrategy returns the "pdf" preset strategy.
func PDFBlockParseStrategy() BlockParseStrategy {
	return BlockParseStrategy{
		SplitEmbeddedQuestionNumbers:       true,
		AllowBareQuestionNumber:            true,
		RemovePageNumbers:                  true,
		DetectSectionHeadings:              true,
		DetectQuestionTypeHeadings:         true,
		ExtractSectionTags:                 true,
		ExtractQuestionTypeTags:            true,
		SortBlocksByQuestionNumber:         true,
		DetectInterleavedTwoColumnSequence: true,
	}
}

// BlockPreviewRequest is the query-param request for the block-preview endpoint.
type BlockPreviewRequest struct {
	DefaultDifficulty string `form:"default_difficulty"`
	StrategyPreset    string `form:"strategy_preset"` // "general" | "pdf"
	ImportMode        string `form:"import_mode"`     // "single" | "batch"
}

// BlockPreviewSummary provides an overview of the block analysis results.
type BlockPreviewSummary struct {
	TotalBlocks         int            `json:"total_blocks"`
	BlocksWithAnomalies int            `json:"blocks_with_anomalies"`
	QuestionNumbers     int            `json:"question_numbers"`
	AnomalyBreakdown    map[string]int `json:"anomaly_breakdown"`
}

// BlockPreviewResponse is returned by the block-preview endpoint.
type BlockPreviewResponse struct {
	Blocks  []ImportBlock       `json:"blocks"`
	Summary BlockPreviewSummary `json:"summary"`
}

// ParseBlocksRequest is the JSON body for the parse-blocks endpoint.
type ParseBlocksRequest struct {
	Blocks            []ImportBlock `json:"blocks" binding:"required"`
	DefaultDifficulty string        `json:"default_difficulty"`
	StrategyPreset    string        `json:"strategy_preset"`
}

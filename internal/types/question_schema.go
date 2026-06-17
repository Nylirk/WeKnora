package types

import (
	"encoding/json"
	"strings"
)

type QuestionType string

const (
	QuestionTypeSingleChoice   QuestionType = "single_choice"
	QuestionTypeMultipleChoice QuestionType = "multiple_choice"
	QuestionTypeTrueFalse      QuestionType = "true_false"
	QuestionTypeFillBlank      QuestionType = "fill_blank"
	QuestionTypeShortAnswer    QuestionType = "short_answer"
	QuestionTypeEssay          QuestionType = "essay"
	QuestionTypeComposite      QuestionType = "composite"
)

var questionTypeRegistry = map[QuestionType]bool{
	QuestionTypeSingleChoice:   true,
	QuestionTypeMultipleChoice: true,
	QuestionTypeTrueFalse:      true,
	QuestionTypeFillBlank:      true,
	QuestionTypeShortAnswer:    true,
	QuestionTypeEssay:          true,
	QuestionTypeComposite:     true,
}

func IsValidQuestionType(qt string) bool {
	return questionTypeRegistry[QuestionType(qt)]
}

func RegisteredQuestionTypes() []QuestionType {
	types := make([]QuestionType, 0, len(questionTypeRegistry))
	for qt := range questionTypeRegistry {
		types = append(types, qt)
	}
	return types
}

type QuestionOption struct {
	Label   string `json:"label"`
	Content string `json:"content"`
}

type ChoiceQuestionBody struct {
	Options   []QuestionOption `json:"options"`
	MinSelect int              `json:"min_select,omitempty"`
	MaxSelect int              `json:"max_select,omitempty"`
}

type CompositeSubQuestion struct {
	QuestionType string `json:"question_type"`
	StemText     string `json:"stem_text"`
	QuestionBody JSON   `json:"question_body"`
	AnswerBody   JSON   `json:"answer_body"`
	Points       int    `json:"points,omitempty"`
}

type SingleChoiceAnswer struct {
	SelectedIndex int    `json:"selected_index"`
	Explanation   string `json:"explanation,omitempty"`
}

type MultipleChoiceAnswer struct {
	SelectedIndices []int `json:"selected_indices"`
	Explanation     string `json:"explanation,omitempty"`
}

type TrueFalseAnswer struct {
	IsTrue      bool   `json:"is_true"`
	Explanation string `json:"explanation,omitempty"`
}

type FillBlankAnswer struct {
	BlankAnswers []string `json:"blank_answers"`
}

type ShortAnswerAnswer struct {
	Keywords    []string `json:"keywords,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
}

type EssayAnswer struct {
	Explanation string `json:"explanation,omitempty"`
}

type CompositeAnswer struct {
	SubAnswers []JSON `json:"sub_answers"`
}

type ValidateForReviewError struct {
	Field   string
	Message string
}

func ValidateQuestionForDraft(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	if !IsValidQuestionType(q.QuestionType) {
		errs = append(errs, ValidateForReviewError{Field: "question_type", Message: "unsupported question type: " + q.QuestionType})
	}
	if strings.TrimSpace(q.StemText) == "" {
		errs = append(errs, ValidateForReviewError{Field: "stem_text", Message: "stem_text is required"})
	}
	return errs
}

func ValidateQuestionForReview(q *Question) []ValidateForReviewError {
	errs := ValidateQuestionForDraft(q)
	if len(errs) > 0 {
		return errs
	}

	switch QuestionType(q.QuestionType) {
	case QuestionTypeSingleChoice:
		errs = append(errs, validateChoiceBody(q)...)
		errs = append(errs, validateSingleChoiceAnswer(q)...)
	case QuestionTypeMultipleChoice:
		errs = append(errs, validateChoiceBody(q)...)
		errs = append(errs, validateMultipleChoiceAnswer(q)...)
	case QuestionTypeTrueFalse:
		errs = append(errs, validateTrueFalseAnswer(q)...)
	case QuestionTypeFillBlank:
		errs = append(errs, validateFillBlankAnswer(q)...)
	case QuestionTypeShortAnswer:
		if strings.TrimSpace(q.AnswerText) == "" {
			errs = append(errs, ValidateForReviewError{Field: "answer_text", Message: "answer_text is required for short_answer"})
		}
	case QuestionTypeEssay:
	case QuestionTypeComposite:
		errs = append(errs, validateCompositeBody(q)...)
	}
	return errs
}

func ValidateQuestionForExport(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	if strings.TrimSpace(q.StemText) == "" {
		errs = append(errs, ValidateForReviewError{Field: "stem_text", Message: "stem_text is required"})
	}
	if strings.TrimSpace(q.AnswerText) == "" {
		errs = append(errs, ValidateForReviewError{Field: "answer_text", Message: "answer_text is required"})
	}
	return errs
}

func validateChoiceBody(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var body ChoiceQuestionBody
	if err := jsonUnmarshal(q.QuestionBody, &body); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "invalid question_body: expected {\"options\": [...]}: " + err.Error()})
		return errs
	}
	if len(body.Options) < 2 {
		errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "choice questions must have at least 2 options"})
		return errs
	}
	for i, opt := range body.Options {
		if strings.TrimSpace(opt.Label) == "" {
			errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "option label cannot be empty"})
		}
		if strings.TrimSpace(opt.Content) == "" {
			errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "option content cannot be empty"})
		}
		_ = i
	}
	seen := make(map[string]bool)
	for _, opt := range body.Options {
		if seen[opt.Label] {
			errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "duplicate option label: " + opt.Label})
			break
		}
		seen[opt.Label] = true
	}
	return errs
}

func validateSingleChoiceAnswer(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var ans SingleChoiceAnswer
	if err := jsonUnmarshal(q.AnswerBody, &ans); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "invalid answer_body: " + err.Error()})
		return errs
	}
	if strings.TrimSpace(q.AnswerText) == "" {
		errs = append(errs, ValidateForReviewError{Field: "answer_text", Message: "answer_text is required for single_choice"})
	}
	var body ChoiceQuestionBody
	_ = jsonUnmarshal(q.QuestionBody, &body)
	if ans.SelectedIndex < 0 || (len(body.Options) > 0 && ans.SelectedIndex >= len(body.Options)) {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "selected_index out of range"})
	}
	return errs
}

func validateMultipleChoiceAnswer(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var ans MultipleChoiceAnswer
	if err := jsonUnmarshal(q.AnswerBody, &ans); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "invalid answer_body: " + err.Error()})
		return errs
	}
	if len(ans.SelectedIndices) == 0 {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "multiple_choice must have at least one selected index"})
	}
	var body ChoiceQuestionBody
	_ = jsonUnmarshal(q.QuestionBody, &body)
	for _, idx := range ans.SelectedIndices {
		if idx < 0 || (len(body.Options) > 0 && idx >= len(body.Options)) {
			errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "selected_indices out of range"})
			break
		}
	}
	return errs
}

func validateTrueFalseAnswer(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var ans TrueFalseAnswer
	if err := jsonUnmarshal(q.AnswerBody, &ans); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "invalid answer_body: " + err.Error()})
		return errs
	}
	if strings.TrimSpace(q.AnswerText) == "" {
		errs = append(errs, ValidateForReviewError{Field: "answer_text", Message: "answer_text is required for true_false"})
	}
	return errs
}

func validateFillBlankAnswer(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var ans FillBlankAnswer
	if err := jsonUnmarshal(q.AnswerBody, &ans); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "invalid answer_body: " + err.Error()})
		return errs
	}
	if len(ans.BlankAnswers) == 0 {
		errs = append(errs, ValidateForReviewError{Field: "answer_body", Message: "fill_blank must have at least one blank_answer"})
	}
	return errs
}

func validateCompositeBody(q *Question) []ValidateForReviewError {
	var errs []ValidateForReviewError
	var subs []CompositeSubQuestion
	if err := jsonUnmarshal(q.QuestionBody, &subs); err != nil {
		errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "invalid question_body for composite: " + err.Error()})
		return errs
	}
	if len(subs) == 0 {
		errs = append(errs, ValidateForReviewError{Field: "question_body", Message: "composite question must have at least one sub_question"})
	}
	return errs
}

func jsonUnmarshal(data JSON, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
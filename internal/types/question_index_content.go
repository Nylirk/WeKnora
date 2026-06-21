package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const questionIndexMaxChars = 20000

// BuildQuestionIndexContent builds the only embedding input used for a question.
// Answer, analysis, and grading fields are intentionally not read here.
func BuildQuestionIndexContent(q *Question) string {
	if q == nil {
		return ""
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "question_type", value: strings.TrimSpace(q.QuestionType)},
		{name: "difficulty", value: strings.TrimSpace(string(q.Difficulty))},
		{name: "stem_text", value: strings.TrimSpace(q.StemText)},
		{name: "question_body", value: readableQuestionJSON(q.QuestionBody)},
		{name: "knowledge_points", value: readableQuestionJSON(q.KnowledgePoints)},
		{name: "tags", value: readableQuestionJSON(q.Tags)},
	}

	var builder strings.Builder
	for _, field := range fields {
		if field.value == "" || field.value == "{}" || field.value == "[]" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(field.name)
		builder.WriteString(": ")
		builder.WriteString(field.value)
	}

	content := builder.String()
	if utf8.RuneCountInString(content) <= questionIndexMaxChars {
		return content
	}
	return string([]rune(content)[:questionIndexMaxChars])
}

func readableQuestionJSON(raw JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return formatQuestionJSONValue(value)
}

func formatQuestionJSONValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool, float64:
		return fmt.Sprint(typed)
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if formatted := formatQuestionJSONValue(item); formatted != "" {
				parts = append(parts, formatted)
			}
		}
		if len(parts) == 0 {
			return "[]"
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if !isQuestionIndexForbiddenKey(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if formatted := formatQuestionJSONValue(typed[key]); formatted != "" {
				parts = append(parts, key+": "+formatted)
			}
		}
		if len(parts) == 0 {
			return "{}"
		}
		return "{" + strings.Join(parts, "; ") + "}"
	default:
		return fmt.Sprint(typed)
	}
}

// BuildQuestionSemanticQuery builds a query string for semantic matching (knowledge point
// tagging and syllabus filtering). It intentionally excludes answer, analysis, and grading
// fields to prevent answer/analysis text from polluting match results.
func BuildQuestionSemanticQuery(q *Question) string {
	if q == nil {
		return ""
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "question_type", value: strings.TrimSpace(q.QuestionType)},
		{name: "difficulty", value: strings.TrimSpace(string(q.Difficulty))},
		{name: "stem_text", value: strings.TrimSpace(q.StemText)},
		{name: "question_body", value: readableQuestionJSON(q.QuestionBody)},
	}

	var builder strings.Builder
	for _, field := range fields {
		if field.value == "" || field.value == "{}" || field.value == "[]" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(field.name)
		builder.WriteString(": ")
		builder.WriteString(field.value)
	}
	return builder.String()
}

// BuildKnowledgePointMatchingQuery builds a query string dedicated to
// knowledge-point semantic matching against a linked knowledge point KB.
// It intentionally excludes answer, analysis, and grading fields to prevent
// answer text from polluting knowledge-point matches. This is kept separate
// from BuildQuestionSemanticQuery so the knowledge-point matching path can
// evolve independently from the syllabus filtering path.
func BuildKnowledgePointMatchingQuery(q *Question) string {
	if q == nil {
		return ""
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "question_type", value: strings.TrimSpace(q.QuestionType)},
		{name: "difficulty", value: strings.TrimSpace(string(q.Difficulty))},
		{name: "stem_text", value: strings.TrimSpace(q.StemText)},
		{name: "question_body", value: readableQuestionJSON(q.QuestionBody)},
	}

	var builder strings.Builder
	for _, field := range fields {
		if field.value == "" || field.value == "{}" || field.value == "[]" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(field.name)
		builder.WriteString(": ")
		builder.WriteString(field.value)
	}
	return builder.String()
}

func isQuestionIndexForbiddenKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	return strings.Contains(normalized, "answer") ||
		strings.Contains(normalized, "analysis") ||
		strings.Contains(normalized, "explanation") ||
		strings.Contains(normalized, "solution") ||
		strings.Contains(normalized, "rubric")
}

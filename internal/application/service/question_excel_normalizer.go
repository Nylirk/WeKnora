package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// normalizeExcelQuestions parses Excel table content (from docreader output)
// into ImportQuestionItem drafts. The docreader Excel parser outputs each row
// as "col1: val1, col2: val2, ..." format.
//
// It does NOT read Excel files — file reading must be done by the existing
// docreader pipeline before this normalizer runs.
//
// Expected columns (case-insensitive header matching):
//
//	stem_text (required)
//	question_type (optional, defaults to short_answer)
//	option_a, option_b, option_c, option_d, ... (optional)
//	answer (required for choice questions)
//	analysis (optional)
//	knowledge_points (optional, comma-separated)
//	tags (optional, comma-separated)
//	difficulty (optional, defaults to medium)
//	source_note (optional)
func normalizeExcelQuestions(tableText string) ([]types.ImportQuestionItem, []types.ImportQuestionError) {
	lines := strings.Split(tableText, "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	// Parse the header line to map column names to indices.
	headers := parseExcelRow(lines[0])
	if len(headers) == 0 {
		return nil, []types.ImportQuestionError{{
			LineNumber: 1,
			Message:    "无法解析表头",
		}}
	}

	colIndex := buildColumnIndex(headers)

	var items []types.ImportQuestionItem
	var errors []types.ImportQuestionError

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		values := parseExcelRow(line)
		lineNum := i + 1

		stemText := getColValue(values, colIndex, "stem_text")
		if stemText == "" {
			errors = append(errors, types.ImportQuestionError{
				LineNumber: lineNum,
				Message:    fmt.Sprintf("第 %d 行题干为空", i),
			})
			continue
		}

		qType := strings.ToLower(getColValue(values, colIndex, "question_type"))
		if qType == "" {
			qType = string(types.QuestionTypeShortAnswer)
		}
		if !types.IsValidQuestionType(qType) {
			qType = string(types.QuestionTypeShortAnswer)
		}

		answerText := getColValue(values, colIndex, "answer")

		difficulty := getColValue(values, colIndex, "difficulty")
		if difficulty == "" {
			difficulty = string(types.QuestionDifficultyMedium)
		}

		item := types.ImportQuestionItem{
			LineNumber:   lineNum,
			QuestionType: qType,
			StemText:     stemText,
			AnswerText:   answerText,
			AnalysisText: getColValue(values, colIndex, "analysis"),
			Difficulty:   normalizeDifficulty(difficulty),
		}

		// Parse options (option_a, option_b, ...)
		options := parseOptions(values, colIndex)
		if len(options) > 0 && (qType == string(types.QuestionTypeSingleChoice) || qType == string(types.QuestionTypeMultipleChoice)) {
			body := types.ChoiceQuestionBody{Options: options}
			if bodyBytes, err := json.Marshal(body); err == nil {
				item.QuestionBody = types.JSON(bodyBytes)
			}
		}

		// Knowledge points and tags
		if kp := getColValue(values, colIndex, "knowledge_points"); kp != "" {
			kpList := splitAndTrim(kp, ",，;；")
			if kpBytes, err := json.Marshal(kpList); err == nil {
				item.KnowledgePoints = types.JSON(kpBytes)
			}
		}
		if tags := getColValue(values, colIndex, "tags"); tags != "" {
			tagList := splitAndTrim(tags, ",，;；")
			if tagBytes, err := json.Marshal(tagList); err == nil {
				item.Tags = types.JSON(tagBytes)
			}
		}

		// Source note
		if src := getColValue(values, colIndex, "source_note"); src != "" {
			if srcBytes, err := json.Marshal(map[string]string{"note": src}); err == nil {
				item.SourcePayload = types.JSON(srcBytes)
			}
		}

		items = append(items, item)
	}

	return items, errors
}

// parseExcelRow parses a single row from the docreader Excel output format.
// The format is: "col1: val1, col2: val2, ..."
// It handles values that may contain commas by being lenient with splitting.
func parseExcelRow(line string) []string {
	// Split on ", " pattern which is what docreader uses.
	parts := strings.Split(line, ", ")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		// Extract value after ": "
		idx := strings.Index(p, ": ")
		if idx >= 0 {
			result = append(result, strings.TrimSpace(p[idx+2:]))
		} else {
			result = append(result, strings.TrimSpace(p))
		}
	}
	return result
}

// columnIndex maps normalized column names to their positional index.
type columnIndex map[string]int

// buildColumnIndex creates a column name → index mapping from header values.
func buildColumnIndex(headers []string) columnIndex {
	idx := make(columnIndex)
	for i, h := range headers {
		normalized := normalizeColumnName(h)
		idx[normalized] = i
	}
	return idx
}

// normalizeColumnName normalizes a column header for matching.
// Handles common variations:
//
//	"题干" / "stem_text" → "stem_text"
//	"题目类型" / "question_type" → "question_type"
//	"选项A" / "option_a" / "A" → "option_a"
//	"答案" / "answer" → "answer"
func normalizeColumnName(name string) string {
	n := strings.TrimSpace(name)

	// Chinese → English mappings
	cnToEn := map[string]string{
		"题干":   "stem_text",
		"题目":   "stem_text",
		"题型":   "question_type",
		"题目类型": "question_type",
		"选项a":  "option_a",
		"选项b":  "option_b",
		"选项c":  "option_c",
		"选项d":  "option_d",
		"选项e":  "option_e",
		"选项f":  "option_f",
		"选项g":  "option_g",
		"选项h":  "option_h",
		"答案":   "answer",
		"正确答案": "answer",
		"解析":   "analysis",
		"题目解析": "analysis",
		"知识点":  "knowledge_points",
		"标签":   "tags",
		"难度":   "difficulty",
		"来源":   "source_note",
		"备注":   "source_note",
	}

	lower := strings.ToLower(n)
	if mapped, ok := cnToEn[lower]; ok {
		return mapped
	}

	// Single letter option (A, B, C, D) → option_a, option_b, ...
	if len(lower) == 1 && lower >= "a" && lower <= "h" {
		return "option_" + lower
	}

	// Already English: stem_text, question_type, etc.
	return lower
}

// getColValue returns the value for a given column name, or empty string if not found.
func getColValue(values []string, idx columnIndex, col string) string {
	pos, ok := idx[col]
	if !ok || pos >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[pos])
}

// parseOptions extracts choice options from the row values based on column headers.
func parseOptions(values []string, idx columnIndex) []types.QuestionOption {
	var options []types.QuestionOption
	optionLabels := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, label := range optionLabels {
		colName := "option_" + label
		content := getColValue(values, idx, colName)
		if content == "" {
			continue
		}
		options = append(options, types.QuestionOption{
			Label:   strings.ToUpper(label),
			Content: content,
		})
	}
	return options
}

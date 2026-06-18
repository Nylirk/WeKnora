package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// QuestionExtractionService extracts structured questions from plain text.
// MVP: rule-based extraction using question numbering, option markers, and answer sections.
type QuestionExtractionService struct{}

// NewQuestionExtractionService creates a new question extraction service.
func NewQuestionExtractionService() *QuestionExtractionService {
	return &QuestionExtractionService{}
}

type extractionContext struct {
	defaultType       string
	defaultDifficulty string
	lineCount         int
}

var questionNumPattern = regexp.MustCompile(
	`^[\s]*((?:\d+)[\.\)、]|(?:\d+)\s*[\.\)、\s]|（\s*\d+\s*）|[（\(]\s*\d+\s*[）\)]|[一二三四五六七八九十]+[、.）\)]|(?:Question\s*\d+))\s*`,
)

var optionLabelPattern = regexp.MustCompile(`^[\s]*([A-Da-d])[\.\)、]\s*`)

var answerLabelPattern = regexp.MustCompile(
	`(?i)^[\s]*(?:答案|参考答案|答案解析|答案部分|答案解析部分|Answer\s*(?:Key)?|Explanation)[：:]\s*`,
)

var answerSectionPattern = regexp.MustCompile(
	`(?i)参考答案|答案解析|答案部分|答案解析部分|Answer\s*Key|^\s*Explanation\s*$`,
)

var analysisLabelPattern = regexp.MustCompile(`(?i)^[\s]*(?:解析|分析|答案解析)[：:]\s*`)

var blankMarkerPattern = regexp.MustCompile(`[（(]\s*[）)]|___+|_{3,}|\.{3,}`)

// Extract extracts question items from plain text using rule-based heuristics.
func (s *QuestionExtractionService) Extract(text string, defaultQuestionType string, defaultDifficulty string) (
	items []types.ImportQuestionItem, errors []types.ImportQuestionError, warnings []string,
) {
	ctx := &extractionContext{
		defaultType:       defaultQuestionType,
		defaultDifficulty: defaultDifficulty,
	}

	normalized := normalizeText(text)
	lines := splitAndCleanLines(normalized)
	ctx.lineCount = len(lines)

	if len(lines) == 0 {
		return nil, nil, []string{"未能从文件中抽取文本，请确认文件内容可复制，或等待 OCR 支持。"}
	}

	blocks := partitionIntoBlocks(lines)
	if len(blocks) == 0 {
		return nil, nil, []string{"未识别到题目，请检查文档题号格式，或使用 JSON/JSONL 导入。"}
	}

	for i, block := range blocks {
		item := s.parseBlock(block, i+1, ctx)
		if item != nil {
			items = append(items, *item)
		}
	}

	if len(items) == 0 {
		warnings = append(warnings, "未识别到题目，请检查文档题号格式，或使用 JSON/JSONL 导入。")
	}

	return items, errors, warnings
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func splitAndCleanLines(text string) []string {
	raw := strings.Split(text, "\n")
	result := make([]string, 0, len(raw))
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		result = append(result, trimmed)
	}
	return result
}

func partitionIntoBlocks(lines []string) [][]string {
	var blocks [][]string
	var currentBlock []string

	for _, line := range lines {
		if questionNumPattern.MatchString(line) {
			if len(currentBlock) > 0 {
				blocks = append(blocks, currentBlock)
			}
			currentBlock = []string{line}
		} else if len(currentBlock) > 0 {
			currentBlock = append(currentBlock, line)
		}
	}
	if len(currentBlock) > 0 {
		blocks = append(blocks, currentBlock)
	}
	return blocks
}

func (s *QuestionExtractionService) parseBlock(lines []string, blockIndex int, ctx *extractionContext) *types.ImportQuestionItem {
	if len(lines) == 0 {
		return nil
	}

	stemLine := questionNumPattern.ReplaceAllString(lines[0], "")
	stemLines := []string{stemLine}

	var optionLines []string
	var answerLines []string
	var analysisLines []string
	inAnswer := false
	inAnalysis := false
	hasOptionMarkers := false

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Detect answer section start
		if !inAnswer && !inAnalysis && answerLabelPattern.MatchString(line) {
			inAnswer = true
			content := answerLabelPattern.ReplaceAllString(line, "")
			if content != "" {
				answerLines = append(answerLines, content)
			}
			continue
		}

		// Detect analysis section start
		if !inAnswer && !inAnalysis && analysisLabelPattern.MatchString(line) {
			inAnalysis = true
			content := analysisLabelPattern.ReplaceAllString(line, "")
			if content != "" {
				analysisLines = append(analysisLines, content)
			}
			continue
		}

		// Detect option line (A./B./C./D. markers)
		if !inAnswer && !inAnalysis && optionLabelPattern.MatchString(line) {
			hasOptionMarkers = true
			optionLines = append(optionLines, line)
			continue
		}

		if inAnalysis {
			if questionNumPattern.MatchString(line) {
				break
			}
			analysisLines = append(analysisLines, line)
			continue
		}

		if inAnswer {
			if questionNumPattern.MatchString(line) {
				break
			}
			if analysisLabelPattern.MatchString(line) {
				inAnalysis = true
				inAnswer = false
				content := analysisLabelPattern.ReplaceAllString(line, "")
				if content != "" {
					analysisLines = append(analysisLines, content)
				}
				continue
			}
			answerLines = append(answerLines, line)
			continue
		}

		// Remaining content: part of stem
		stemLines = append(stemLines, line)
	}

	stemText := strings.TrimSpace(strings.Join(filterEmpty(stemLines), "\n"))
	if stemText == "" {
		return &types.ImportQuestionItem{
			LineNumber:        blockIndex,
			QuestionType:      ctx.defaultType,
			Difficulty:        ctx.defaultDifficulty,
			QuestionBody:      normalizeJSONObject(nil),
			AnswerBody:        normalizeJSONObject(nil),
			GradingRubric:     normalizeJSONObject(nil),
			KnowledgePoints:   normalizeJSONArray(nil),
			Tags:              normalizeJSONArray(nil),
			EvidenceChunkIDs:  normalizeJSONArray(nil),
		}
	}

	answerText := strings.TrimSpace(strings.Join(filterEmpty(answerLines), "\n"))
	analysisText := strings.TrimSpace(strings.Join(filterEmpty(analysisLines), "\n"))

	// Infer question type
	questionType := s.inferQuestionType(stemText, hasOptionMarkers, optionLines, answerText, ctx)

	// Build question body for choice questions
	questionBody := types.JSON(nil)
	if hasOptionMarkers && (questionType == string(types.QuestionTypeSingleChoice) || questionType == string(types.QuestionTypeMultipleChoice)) {
		options := parseOptions(optionLines)
		if len(options) > 0 {
			body := types.ChoiceQuestionBody{Options: options}
			b, err := json.Marshal(body)
			if err == nil {
				questionBody = b
			}
		}
	}

	difficulty := ctx.defaultDifficulty
	if difficulty == "" {
		difficulty = string(types.QuestionDifficultyMedium)
	}
	qtype := questionType
	if qtype == "" {
		qtype = string(types.QuestionTypeShortAnswer)
	}

	item := &types.ImportQuestionItem{
		LineNumber:    blockIndex,
		QuestionType:  qtype,
		StemText:      stemText,
		QuestionBody:  normalizeJSONObject(questionBody),
		AnswerText:    answerText,
		AnswerBody:    normalizeJSONObject(nil),
		AnalysisText:  analysisText,
		GradingRubric: normalizeJSONObject(nil),
		Difficulty:    difficulty,
	}

	return item
}

func (s *QuestionExtractionService) inferQuestionType(
	stemText string, hasOptionMarkers bool, optionLines []string, answerText string, ctx *extractionContext,
) string {
	defaultType := ctx.defaultType
	if defaultType == "" {
		defaultType = string(types.QuestionTypeShortAnswer)
	}

	// Check for blank fill markers in stem
	if blankMarkerPattern.MatchString(stemText) {
		return string(types.QuestionTypeFillBlank)
	}

	// Check for true/false answer patterns
	lowerAnswer := strings.TrimSpace(strings.ToLower(answerText))
	switch lowerAnswer {
	case "对", "错", "正确", "错误", "是", "否", "true", "false", "t", "f", "√", "×":
		return string(types.QuestionTypeTrueFalse)
	}

	if hasOptionMarkers && len(optionLines) >= 2 {
		cleanAnswer := strings.ToUpper(strings.TrimSpace(answerText))
		if len(cleanAnswer) >= 2 && isMultiChoiceAnswer(cleanAnswer) && len(optionLines) >= 4 {
			return string(types.QuestionTypeMultipleChoice)
		}
		return string(types.QuestionTypeSingleChoice)
	}

	return defaultType
}

func isMultiChoiceAnswer(answer string) bool {
	if match, _ := regexp.MatchString(`^[A-Da-d]{2,}$`, answer); match {
		return true
	}
	if match, _ := regexp.MatchString(`^[A-Da-d][,，\s、]+[A-Da-d]`, answer); match {
		return true
	}
	return false
}

func parseOptions(lines []string) []types.QuestionOption {
	var options []types.QuestionOption
	for _, line := range lines {
		matches := optionLabelPattern.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		label := strings.ToUpper(matches[1])
		content := optionLabelPattern.ReplaceAllString(line, "")
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		options = append(options, types.QuestionOption{Label: label, Content: content})
	}
	return options
}

func filterEmpty(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}

// normalizeJSONObject and normalizeJSONArray are defined in question.go (same package).

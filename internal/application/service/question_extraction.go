package service

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

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
	errors            []types.ImportQuestionError
}

var questionNumPattern = regexp.MustCompile(
	`^[\s]*((?:\d+)[\.\)、]|(?:\d+)\s*[\.\)、\s]|（\s*\d+\s*）|[（\(]\s*\d+\s*[）\)]|[一二三四五六七八九十]+[、.）\)]|(?:Question\s*\d+))\s*`,
)

// optionLabelPattern matches a single option label at line start: A. / a) / B．/ etc.
// Supports A-Z (case-insensitive), fullwidth/halfwidth markers.
// Note: fullwidth characters like ．）、：must NOT be backslash-escaped in Go regexp.
var optionLabelPattern = regexp.MustCompile(`^\s*([A-Za-z])[.．)、）：:]\s*`)

// inlineOptionPattern matches option markers anywhere in a line (not just start).
// Requires whitespace or line-start before the marker so it doesn't match
// ordinary English punctuation like "Node.js", "Go.", "e.g.", "U.S.".
// Used to split inline options like "A. foo B. bar C. baz".
var inlineOptionPattern = regexp.MustCompile(`(?:^|[\s　]+)([A-Za-z])[.．)、）：:]\s*`)

// bracketAnswerPattern matches answer letters in parentheses at end of stem.
// Examples: （B）, (E), （A、C、E）, (A,C,E), （a、c、e）
var bracketAnswerPattern = regexp.MustCompile(
	`[（(]\s*([A-Za-z](?:\s*[,，、\s]\s*[A-Za-z])*)\s*[）)]`,
)

var answerLabelPattern = regexp.MustCompile(
	`(?i)^[\s]*(?:答案|参考答案|答案解析|答案部分|答案解析部分|Answer\s*(?:Key)?|Explanation)[：:]\s*`,
)

var analysisLabelPattern = regexp.MustCompile(`(?i)^[\s]*(?:解析|分析|答案解析)[：:]\s*`)

var blankMarkerPattern = regexp.MustCompile(`[（(]\s*[）)]|___+|_{3,}|\.{3,}`)

// Extract extracts question items from plain text using rule-based heuristics.
func (s *QuestionExtractionService) Extract(ctx context.Context, text string, defaultQuestionType string, defaultDifficulty string) (
	items []types.ImportQuestionItem, errors []types.ImportQuestionError, warnings []string,
) {
	ext := &extractionContext{
		defaultType:       defaultQuestionType,
		defaultDifficulty: defaultDifficulty,
	}

	normalized := normalizeText(text)
	lines := splitAndCleanLines(normalized)
	ext.lineCount = len(lines)

	if len(lines) == 0 {
		return nil, nil, []string{"未能从文件中抽取文本，请确认文件内容可复制，或等待 OCR 支持。"}
	}

	blocks := partitionIntoBlocks(lines)
	if len(blocks) == 0 {
		return nil, nil, []string{"未识别到题目，请检查文档题号格式，或使用 JSON/JSONL 导入。"}
	}

	for i, block := range blocks {
		if i%10 == 0 {
			select {
			case <-ctx.Done():
				return items, ext.errors, append(warnings, "请求已取消")
			default:
			}
		}
		item, parseErr := s.parseBlock(block, i+1, ext)
		if parseErr != nil {
			ext.errors = append(ext.errors, *parseErr)
			continue
		}
		if item != nil {
			items = append(items, *item)
		}
	}

	if len(items) == 0 && len(ext.errors) == 0 {
		warnings = append(warnings, "未识别到题目，请检查文档题号格式，或使用 JSON/JSONL 导入。")
	}

	return items, ext.errors, warnings
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

func (s *QuestionExtractionService) parseBlock(lines []string, blockIndex int, ctx *extractionContext) (*types.ImportQuestionItem, *types.ImportQuestionError) {
	if len(lines) == 0 {
		return nil, nil
	}

	stemLine := questionNumPattern.ReplaceAllString(lines[0], "")
	stemLines := []string{stemLine}

	var optionLines []string
	var answerLines []string
	var analysisLines []string
	inAnswer := false
	inAnalysis := false
	hasOptionMarkers := false

	// Check if the first line (stem line) contains inline options after the stem.
	// e.g. "1. 以下哪个是注册器？（E） A. RegistryObject B. EventBus ..."
	// The stem prefix stays as stem; the option markers go into optionLines.
	if stemPrefix, optionSource, ok := splitStemInlineOptions(stemLine); ok {
		stemLines = []string{stemPrefix}
		optionLines = append(optionLines, optionSource)
		hasOptionMarkers = true
	}

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

		// Detect option line (A./B./C./... markers, A-Z)
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

		// Option continuation: if we've seen options and this line is not a question marker,
		// not an answer section, not an analysis section, treat it as continuation of the
		// last option's content (or the stem before options began).
		if hasOptionMarkers {
			optionLines = append(optionLines, line)
			continue
		}

		// Remaining content: part of stem
		stemLines = append(stemLines, line)
	}

	stemText := strings.TrimSpace(strings.Join(filterEmpty(stemLines), "\n"))
	if stemText == "" {
		return nil, &types.ImportQuestionError{
			LineNumber: blockIndex,
			Message:    "未识别到题干，请检查题号后的内容。",
		}
	}

	answerText := strings.TrimSpace(strings.Join(filterEmpty(answerLines), "\n"))
	analysisText := strings.TrimSpace(strings.Join(filterEmpty(analysisLines), "\n"))

	// Parse options (with inline splitting and continuation merging)
	var options []types.QuestionOption
	if hasOptionMarkers {
		options = parseOptions(optionLines)
	}

	// If we have options but no explicit answer section, try extracting the answer
	// from bracket notation in the stem: 下列正确的是（B）, (E), etc.
	if len(options) >= 2 && answerText == "" {
		optionLabelSet := make(map[string]bool, len(options))
		for _, opt := range options {
			optionLabelSet[opt.Label] = true
		}
		cleanStem, extractedAnswer, ok := extractChoiceAnswerFromStem(stemText, optionLabelSet)
		if ok {
			stemText = cleanStem
			answerText = extractedAnswer
		}
	}

	// Infer question type
	questionType := s.inferQuestionType(stemText, options, answerText, ctx)

	// Build question body for choice questions
	questionBody := types.JSON(nil)
	if len(options) >= 2 && (questionType == string(types.QuestionTypeSingleChoice) || questionType == string(types.QuestionTypeMultipleChoice)) {
		body := types.ChoiceQuestionBody{Options: options}
		b, err := json.Marshal(body)
		if err == nil {
			questionBody = b
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

	// For choice questions, build display-friendly stem_text and answer_text.
	displayStem := stemText
	displayAnswer := answerText
	if len(options) >= 2 && (qtype == string(types.QuestionTypeSingleChoice) || qtype == string(types.QuestionTypeMultipleChoice)) {
		displayStem = appendOptionsToStem(stemText, options)
		displayAnswer = expandChoiceAnswerText(answerText, options)
	}

	rawText := strings.TrimSpace(strings.Join(lines, "\n"))

	item := &types.ImportQuestionItem{
		LineNumber:    blockIndex,
		QuestionType:  qtype,
		StemText:      displayStem,
		QuestionBody:  normalizeJSONObject(questionBody),
		AnswerText:    displayAnswer,
		AnswerBody:    normalizeJSONObject(nil),
		AnalysisText:  analysisText,
		GradingRubric: normalizeJSONObject(nil),
		Difficulty:    difficulty,
		RawText:       rawText,
	}

	return item, nil
}

func (s *QuestionExtractionService) inferQuestionType(
	stemText string, options []types.QuestionOption, answerText string, ctx *extractionContext,
) string {
	defaultType := ctx.defaultType
	if defaultType == "" {
		defaultType = string(types.QuestionTypeShortAnswer)
	}

	// Check for blank fill markers in stem
	if blankMarkerPattern.MatchString(stemText) {
		return string(types.QuestionTypeFillBlank)
	}

	// Options take precedence over true/false shorthand. In a choice question,
	// answer "F" refers to option F rather than the false marker.
	if len(options) >= 2 {
		cleanAnswer := normalizeMultiChoiceAnswer(answerText)
		if len(cleanAnswer) >= 2 && isMultiChoiceAnswer(answerText) {
			return string(types.QuestionTypeMultipleChoice)
		}
		return string(types.QuestionTypeSingleChoice)
	}

	// Check for true/false answer patterns
	lowerAnswer := strings.TrimSpace(strings.ToLower(answerText))
	switch lowerAnswer {
	case "对", "错", "正确", "错误", "是", "否", "true", "false", "t", "f", "√", "×":
		return string(types.QuestionTypeTrueFalse)
	}

	return defaultType
}

func isMultiChoiceAnswer(answer string) bool {
	normalized := normalizeMultiChoiceAnswer(answer)
	if len(normalized) < 2 {
		return false
	}
	// All characters must be A-Z
	for _, r := range normalized {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// normalizeMultiChoiceAnswer converts multi-choice answers like "A、C、E" or "A,C,E" to "ACE".
func normalizeMultiChoiceAnswer(answer string) string {
	a := strings.ToUpper(strings.TrimSpace(answer))
	var b strings.Builder
	for _, r := range a {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseOptions extracts option labels and content from lines.
// Handles inline options on a single line (e.g. "A. foo B. bar C. baz"),
// multi-line options, and continuation lines.
func parseOptions(lines []string) []types.QuestionOption {
	var options []types.QuestionOption
	expected := rune('A')
	invalidSequence := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// First handle multiple options on the same line. The scanner also
		// reports a broken sequence while preserving the valid prefix.
		parts, invalid := scanInlineOptionsFromExpected(line, expected)
		if len(parts) >= 2 {
			for _, p := range parts {
				content := strings.TrimSpace(p.Content)
				label := strings.ToUpper(p.Label)
				if label == "" || content == "" || []rune(label)[0] != expected {
					invalidSequence = true
					break
				}
				options = append(options, types.QuestionOption{Label: label, Content: content})
				expected++
			}
			if invalid || invalidSequence {
				invalidSequence = true
				break
			}
			continue
		}

		// Then handle the common one-option-per-line form.
		if label, content, ok := parseLeadingOptionLine(line); ok {
			labelRune := []rune(label)[0]
			if labelRune != expected || content == "" {
				invalidSequence = true
				break
			}
			options = append(options, types.QuestionOption{Label: label, Content: content})
			expected++
			continue
		}

		// Otherwise this is a continuation of the last parsed option.
		if len(options) > 0 {
			last := &options[len(options)-1]
			if last.Content != "" {
				last.Content += "\n" + line
			} else {
				last.Content = line
			}
		}
	}

	if len(options) < 2 {
		return nil
	}
	return options
}

type optionPart struct {
	Label   string
	Content string
}

// splitInlineOptions splits a line at option marker positions.
// "A. foo B. bar C. baz" → [{A, foo}, {B, bar}, {C, baz}]
// Only accepts markers in strict sequential order (A, B, C, D, E, ...).
// Non-sequential markers (e.g. "e." in "e.g.") are skipped and treated
// as part of the current option's content.
func splitInlineOptions(line string) []optionPart {
	return splitInlineOptionsFromExpected(line, 'A')
}

// parseLeadingOptionLine parses one option marker at the start of a line.
// It intentionally does not require another marker on the same line.
func parseLeadingOptionLine(line string) (label string, content string, ok bool) {
	match := optionLabelPattern.FindStringSubmatchIndex(line)
	if match == nil {
		return "", "", false
	}

	label = strings.ToUpper(line[match[2]:match[3]])
	if len(label) != 1 || label[0] < 'A' || label[0] > 'Z' {
		return "", "", false
	}
	return label, strings.TrimSpace(line[match[1]:]), true
}

// splitInlineOptionsFromExpected extracts two or more sequential inline
// options beginning at expectedStart.
func splitInlineOptionsFromExpected(line string, expectedStart rune) []optionPart {
	parts, _ := scanInlineOptionsFromExpected(line, expectedStart)
	if len(parts) < 2 {
		return nil
	}
	return parts
}

// scanInlineOptionsFromExpected returns the valid sequential prefix and
// reports whether a later marker breaks the sequence.
func scanInlineOptionsFromExpected(line string, expectedStart rune) ([]optionPart, bool) {
	candidates := inlineOptionPattern.FindAllStringSubmatchIndex(line, -1)
	if len(candidates) == 0 {
		return nil, false
	}

	// Filter candidates: only accept those whose label matches the expected
	// next letter in sequence (A, B, C, ...).
	type acceptedMarker struct {
		labelStart int // submatch group 1 start
		fullEnd    int // full match end
		label      string
	}
	var accepted []acceptedMarker
	expected := expectedStart
	invalid := false
	contentEnd := len(line)
	for i := range candidates {
		m := candidates[i]
		labelStart := m[2] // submatch group 1 → (A-Z)
		labelEnd := m[3]
		fullEnd := m[1] // full match end (marker + delimiter)
		label := strings.ToUpper(line[labelStart:labelEnd])
		if len(label) != 1 {
			continue
		}
		if rune(label[0]) != expected {
			if len(accepted) == 0 || isInlineAbbreviationMarker(line, m) {
				continue
			}
			invalid = true
			contentEnd = labelStart
			break
		}
		accepted = append(accepted, acceptedMarker{
			labelStart: labelStart,
			fullEnd:    fullEnd,
			label:      label,
		})
		expected++
	}

	if len(accepted) == 0 {
		return nil, invalid
	}

	// Build parts using accepted marker boundaries
	var parts []optionPart
	for i, am := range accepted {
		contentStart := am.fullEnd
		var content string
		if i+1 < len(accepted) {
			content = line[contentStart:accepted[i+1].labelStart]
		} else {
			content = line[contentStart:contentEnd]
		}
		parts = append(parts, optionPart{
			Label:   am.label,
			Content: strings.TrimSpace(content),
		})
	}
	return parts, invalid
}

// isInlineAbbreviationMarker recognises the leading "e." in text such as
// "e.g." so it remains option content instead of breaking label sequencing.
func isInlineAbbreviationMarker(line string, match []int) bool {
	if match[2] < 0 || match[3] < 0 || match[1] >= len(line) {
		return false
	}
	rawLabel := line[match[2]:match[3]]
	if rawLabel == strings.ToUpper(rawLabel) {
		return false
	}
	rest := line[match[1]:]
	return len(rest) >= 2 && ((rest[0] >= 'A' && rest[0] <= 'Z') || (rest[0] >= 'a' && rest[0] <= 'z')) && rest[1] == '.'
}

// extractChoiceAnswerFromStem extracts answer letters from bracket notation in a stem,
// e.g., "下列说法正确的是（B）" → cleanStem="下列说法正确的是", answer="B"
// "下列选项正确的是（A、C、E）" → cleanStem="下列选项正确的是", answer="ACE"
// Only extracts if all letters exist in optionLabels.
func extractChoiceAnswerFromStem(stem string, optionLabels map[string]bool) (cleanStem string, answer string, ok bool) {
	loc := bracketAnswerPattern.FindStringIndex(stem)
	if loc == nil {
		return stem, "", false
	}

	// Extract the bracket content
	bracketContent := stem[loc[0]:loc[1]]

	// Parse letters from the bracket
	var letters []rune
	for _, r := range bracketContent {
		upper := unicode.ToUpper(r)
		if upper >= 'A' && upper <= 'Z' {
			letters = append(letters, upper)
		}
	}

	if len(letters) == 0 {
		return stem, "", false
	}

	// Validate: all extracted letters must exist in the parsed options
	seen := make(map[rune]bool, len(letters))
	var answerLetters []rune
	for _, l := range letters {
		label := string(l)
		if !optionLabels[label] {
			// A letter in the bracket doesn't match any option → don't extract
			return stem, "", false
		}
		if !seen[l] {
			seen[l] = true
			answerLetters = append(answerLetters, l)
		}
	}

	// Build answer string: "B" for single, "ACE" for multi
	var answerBuilder strings.Builder
	for _, l := range answerLetters {
		answerBuilder.WriteRune(l)
	}
	answer = answerBuilder.String()

	// Remove the bracket from the stem
	cleanStem = strings.TrimSpace(stem[:loc[0]] + stem[loc[1]:])
	// Also clean up any trailing space before the bracket
	cleanStem = strings.TrimSpace(cleanStem)

	return cleanStem, answer, true
}

// splitStemInlineOptions detects inline option markers on the stem line.
// When a single line contains both the stem and multiple choice options
// (e.g. "以下哪个是注册器？（E） A. A选项 B. B选项 C. C选项 D. D选项 E. E选项"),
// it splits the stem prefix from the option source so that parseOptions can
// correctly extract every option instead of treating the whole line as stem.
//
// The required whitespace/line-start boundary prevents bracket answers like
// （E） from being confused with option markers.
//
// Returns (stem, optionSource, true) when at least 2 option markers are found;
// otherwise ("", "", false).
func splitStemInlineOptions(line string) (stem string, optionSource string, ok bool) {
	candidates := inlineOptionPattern.FindAllStringSubmatchIndex(line, -1)
	if len(candidates) < 2 {
		return "", "", false
	}

	// Filter: only accept sequential labels starting from A.
	// Non-sequential markers (e.g. bracket answers, English punctuation,
	// out-of-order letters) are ignored so the stem boundary is correct.
	var firstAccepted *struct{ labelStart int }
	expected := rune('A')
	acceptedCount := 0
	for _, m := range candidates {
		label := strings.ToUpper(line[m[2]:m[3]])
		if len(label) != 1 || rune(label[0]) != expected {
			continue
		}
		if acceptedCount == 0 {
			firstAccepted = &struct{ labelStart int }{m[2]}
		}
		acceptedCount++
		expected++
	}

	if acceptedCount < 2 || firstAccepted == nil {
		return "", "", false
	}

	stem = strings.TrimSpace(line[:firstAccepted.labelStart])
	optionSource = strings.TrimSpace(line[firstAccepted.labelStart:])
	return stem, optionSource, true
}

// appendOptionsToStem appends the options list to the stem for display purposes.
// e.g. "题干\nA. 选项一\nB. 选项二"
func appendOptionsToStem(stem string, options []types.QuestionOption) string {
	stem = strings.TrimSpace(stem)
	if len(options) == 0 {
		return stem
	}

	var lines []string
	if stem != "" {
		lines = append(lines, stem)
	}
	for _, opt := range options {
		label := strings.ToUpper(strings.TrimSpace(opt.Label))
		content := strings.TrimSpace(opt.Content)
		if label == "" || content == "" {
			continue
		}
		lines = append(lines, label+". "+content)
	}
	return strings.Join(lines, "\n")
}

// expandChoiceAnswerText expands single-letter answers into full option text.
// e.g. "B" → "B. 整体观念与辨证论治"
// e.g. "ACE" → "A. xxx\nC. yyy\nE. zzz"
func expandChoiceAnswerText(answer string, options []types.QuestionOption) string {
	normalized := normalizeMultiChoiceAnswer(answer)
	if normalized == "" || len(options) == 0 {
		return strings.TrimSpace(answer)
	}

	optionMap := make(map[string]string, len(options))
	for _, opt := range options {
		label := strings.ToUpper(strings.TrimSpace(opt.Label))
		content := strings.TrimSpace(opt.Content)
		if label != "" && content != "" {
			optionMap[label] = content
		}
	}

	var lines []string
	for _, r := range normalized {
		label := string(r)
		content, ok := optionMap[label]
		if !ok {
			return strings.TrimSpace(answer)
		}
		lines = append(lines, label+". "+content)
	}

	if len(lines) == 0 {
		return strings.TrimSpace(answer)
	}
	return strings.Join(lines, "\n")
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

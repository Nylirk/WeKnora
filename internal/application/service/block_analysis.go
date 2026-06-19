package service

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// BlockAnalysisService analyzes extracted document text and partitions it
// into importable blocks for the two-stage question import workbench.
type BlockAnalysisService struct{}

// NewBlockAnalysisService creates a new block analysis service.
func NewBlockAnalysisService() *BlockAnalysisService {
	return &BlockAnalysisService{}
}

// ---------------------------------------------------------------------------
// Patterns (kept at package level for test visibility)
// ---------------------------------------------------------------------------

// questionNumPattern matches question number markers at line start.
// Supports: 1. 1) 1、 (1) （1） 一、 Question 1
var blockQuestionNumPattern = regexp.MustCompile(
	`^[\s]*((?:\d+)[\.\)、]|(?:\d+)\s*[\.\)、\s]|（\s*\d+\s*）|[（(]\s*\d+\s*[）)]|[一二三四五六七八九十]+[、.）\)]|(?:Question\s*\d+))\s*`,
)

// embeddedQuestionNumPattern matches a question number embedded mid-line.
// Non-capturing groups for separators so the number is group 1.
var embeddedQuestionNumPattern = regexp.MustCompile(
	`(?:^|[\s　]+)(\d+)[\.\)、](?:\s+|$)`,
)

// bareQuestionNumPattern matches a bare number line (pdf-only preset).
// A line that starts with a number followed by Chinese/CJK text.
var bareQuestionNumPattern = regexp.MustCompile(
	`^[\s]*(\d+)\s+[\p{Han}\p{Hiragana}\p{Katakana}]`,
)

// pageNumPatterns are patterns for pure page number lines that should be removed.
var pageNumPatterns = []*regexp.Regexp{
	// 第20页，共46页 / 第20页 共46页
	regexp.MustCompile(`^[\s]*第\s*\d+\s*页[，,\s]*共\s*\d+\s*页[\s]*$`),
	// 第20页
	regexp.MustCompile(`^[\s]*第\s*\d+\s*页[\s]*$`),
	// Page 20 of 46
	regexp.MustCompile(`(?i)^[\s]*page\s+\d+\s+of\s+\d+[\s]*$`),
	// Page 20
	regexp.MustCompile(`(?i)^[\s]*page\s+\d+[\s]*$`),
}

// barePageNumPattern matches bare page number lines (e.g., "20", "46")
// when isolated — only applied when adjacent lines are non-numeric.
var barePageNumPattern = regexp.MustCompile(`^[\s]*\d{1,4}[\s]*$`)

// sectionHeadingPattern matches chapter/section/unit headings.
var sectionHeadingPattern = regexp.MustCompile(
	`[\s]*(第[一二三四五六七八九十百千\d]+(?:章|节|单元|篇|部分)|(?:Unit|Chapter|Section|Part)\s*\d+)\s*(.*)`,
)

// questionTypeHeadingPattern matches medical question type headings.
var questionTypeHeadingPattern = regexp.MustCompile(
	`[\s]*(?:【?)(A[12]型题|A1型题|A2型题|A3型题|A4型题|B型题|B1型题|C型题|X型题)(?:】?)[\s]*`,
)

// optionLabelPattern matches a single option label at line start.
var blockOptionLabelPattern = regexp.MustCompile(`^\s*([A-Za-z])[.．)、）：:]\s*`)

// answerLabelPattern matches answer section labels.
var blockAnswerLabelPattern = regexp.MustCompile(
	`(?i)^[\s]*(?:答案|参考答案|答案解析|答案部分|答案解析部分|Answer\s*(?:Key)?|Explanation)[：:]\s*`,
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// AnalyzeBlocks partitions extracted markdown text into ImportBlocks using
// the given strategy preset.
func (s *BlockAnalysisService) AnalyzeBlocks(
	text string,
	strategy types.BlockParseStrategy,
) ([]types.ImportBlock, types.BlockPreviewSummary) {
	if strings.TrimSpace(text) == "" {
		return nil, types.BlockPreviewSummary{}
	}

	// Pipeline: text → lines → [][]string → []ImportBlock
	normalized := s.normalizeText(text)
	normalized = s.normalizeQuestionMarkers(normalized)
	lines := s.splitAndCleanLines(normalized, strategy)
	lines = s.splitLinesAtEmbeddedQuestionStarts(lines, strategy)
	lines = s.repairBareQuestionNumberLines(lines, strategy)
	lines = s.detectInterleavedTwoColumn(lines, strategy)
	rawBlocks := s.partitionIntoBlocks(lines)
	rawBlocks = s.sanitizeBlocks(rawBlocks)
	rawBlocks = s.splitBlocksAtEmbeddedQuestionStarts(rawBlocks, strategy)

	// Convert to ImportBlock (this also does tag extraction)
	blocks := s.rawBlocksToImportBlocks(rawBlocks, strategy)
	blocks = s.validateBlockAnomalies(blocks)

	if strategy.SortBlocksByQuestionNumber {
		blocks = s.sortBlocksByQuestionNumber(blocks)
	}

	// Build summary
	summary := s.buildSummary(blocks)

	return blocks, summary
}

// ---------------------------------------------------------------------------
// Step 1: normalizeText
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	// Collapse 3+ newlines into 2
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")

	// NFKC-like normalization for fullwidth numbers
	// Fullwidth digits ０-９ → 0-9
	fullwidth := map[rune]rune{
		'０': '0', '１': '1', '２': '2', '３': '3', '４': '4',
		'５': '5', '６': '6', '７': '7', '８': '8', '９': '9',
		'（': '(', '）': ')', '．': '.', '、': '、',
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if v, ok := fullwidth[r]; ok {
			b.WriteRune(v)
		} else {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ---------------------------------------------------------------------------
// Step 2: normalizeQuestionMarkers
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) normalizeQuestionMarkers(text string) string {
	// Normalize （1）→ (1) — already done in normalizeText
	// Normalize 一、→ keep as-is (already matched by questionNumPattern)
	// Normalize Question 1 → keep as-is
	return text
}

// ---------------------------------------------------------------------------
// Step 3: splitAndCleanLines
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) splitAndCleanLines(text string, strategy types.BlockParseStrategy) []string {
	raw := strings.Split(text, "\n")
	result := make([]string, 0, len(raw))

	for i, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Skip consecutive empty lines
			if len(result) > 0 && result[len(result)-1] == "" {
				continue
			}
			result = append(result, "")
			continue
		}

		// Remove page numbers
		if strategy.RemovePageNumbers && s.isPageNumberLine(trimmed, i, raw) {
			continue
		}

		result = append(result, trimmed)
	}

	return result
}

// isPageNumberLine checks if a line is a pure page number marker.
func (s *BlockAnalysisService) isPageNumberLine(line string, index int, allLines []string) bool {
	for _, pat := range pageNumPatterns {
		if pat.MatchString(line) {
			return true
		}
	}

	// Bare number: "20" or "46" — only if context suggests page number
	if barePageNumPattern.MatchString(line) {
		num, _ := strconv.Atoi(strings.TrimSpace(line))
		if num >= 1 && num <= 9999 {
			// Check context: if surrounding lines are also numbers or empty, likely page nums
			prevIsNumOrEmpty := index == 0 || s.isBareNumOrEmpty(allLines[max(0, index-1)])
			nextIsNumOrEmpty := index >= len(allLines)-1 || s.isBareNumOrEmpty(allLines[min(len(allLines)-1, index+1)])
			if prevIsNumOrEmpty || nextIsNumOrEmpty {
				return true
			}
		}
	}

	return false
}

func (s *BlockAnalysisService) isBareNumOrEmpty(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || barePageNumPattern.MatchString(trimmed)
}

// ---------------------------------------------------------------------------
// Step 4: splitLinesAtEmbeddedQuestionStarts
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) splitLinesAtEmbeddedQuestionStarts(lines []string, strategy types.BlockParseStrategy) []string {
	if !strategy.SplitEmbeddedQuestionNumbers {
		return lines
	}

	var result []string
	for _, line := range lines {
		if line == "" {
			result = append(result, line)
			continue
		}

		// Find all embedded question number match positions
		matches := embeddedQuestionNumPattern.FindAllStringSubmatchIndex(line, -1)
		if len(matches) <= 1 {
			result = append(result, line)
			continue
		}

		// Collect split positions (start of each embedded question number AFTER the first)
		splitPositions := []int{}
		for _, m := range matches {
			if m[0] > 0 {
				splitPositions = append(splitPositions, m[0])
			}
		}

		if len(splitPositions) == 0 {
			result = append(result, line)
			continue
		}

		// Split the line at each position
		prev := 0
		for _, pos := range splitPositions {
			part := strings.TrimSpace(line[prev:pos])
			if part != "" {
				result = append(result, part)
			}
			prev = pos
		}
		// Last part
		part := strings.TrimSpace(line[prev:])
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Step 5: repairBareQuestionNumberLines (pdf only)
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) repairBareQuestionNumberLines(lines []string, strategy types.BlockParseStrategy) []string {
	if !strategy.AllowBareQuestionNumber {
		return lines
	}

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			result = append(result, line)
			continue
		}

		// If it starts with bare number + Chinese text, prepend a marker
		if bareQuestionNumPattern.MatchString(line) && !blockQuestionNumPattern.MatchString(line) {
			// Extract number and add a period to make it a proper question marker
			matches := bareQuestionNumPattern.FindStringSubmatch(line)
			if len(matches) >= 2 {
				num := matches[1]
				rest := strings.TrimPrefix(line, num)
				rest = strings.TrimSpace(rest)
				result = append(result, num+". "+rest)
				continue
			}
		}
		result = append(result, line)
	}
	return result
}

// ---------------------------------------------------------------------------
// Step 6: detectInterleavedTwoColumn (pdf only)
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) detectInterleavedTwoColumn(lines []string, strategy types.BlockParseStrategy) []string {
	if !strategy.DetectInterleavedTwoColumnSequence {
		return lines
	}
	// For v1, we handle interleaved two-column implicitly via
	// bare question number repair + sort. The two-column case
	// mainly manifests as odd-numbered questions on left and
	// even-numbered on right, which the sort handles.
	return lines
}

// ---------------------------------------------------------------------------
// Step 7: partitionIntoBlocks
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) partitionIntoBlocks(lines []string) [][]string {
	var blocks [][]string
	var current []string

	for _, line := range lines {
		if line == "" {
			if len(current) > 0 {
				current = append(current, line)
			}
			continue
		}

		if blockQuestionNumPattern.MatchString(line) {
			if len(current) > 0 {
				blocks = append(blocks, current)
			}
			current = []string{line}
		} else if len(current) > 0 {
			current = append(current, line)
		} else {
			// Lines before the first question number — accumulate
			// as potential noise. Will be cleaned in sanitizeBlocks.
			current = []string{line}
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

// ---------------------------------------------------------------------------
// Step 8: sanitizeBlocks
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) sanitizeBlocks(blocks [][]string) [][]string {
	if len(blocks) == 0 {
		return blocks
	}

	var result [][]string

	for i, block := range blocks {
		if len(block) == 0 {
			continue
		}

		// If first block has no question number marker, try to strip leading
		// noise that looks like option debris from a previous page.
		if i == 0 && !blockQuestionNumPattern.MatchString(block[0]) {
			// Check if this block contains a question start somewhere
			cutIndex := -1
			for j, line := range block {
				if blockQuestionNumPattern.MatchString(line) {
					cutIndex = j
					break
				}
			}

			if cutIndex > 0 {
				// Lines before the first question number may be noise
				// (e.g., D/E option debris from previous page). Strip them.
				precedingLines := block[:cutIndex]
				allOptions := true
				for _, line := range precedingLines {
					if !blockOptionLabelPattern.MatchString(line) {
						allOptions = false
						break
					}
				}
				if allOptions {
					block = block[cutIndex:]
				}
			} else if cutIndex < 0 {
				// No question number in this block. Check if it's pure options
				// (debris split into its own block by partitionIntoBlocks).
				allOptions := true
				for _, line := range block {
					if !blockOptionLabelPattern.MatchString(line) {
						allOptions = false
						break
					}
				}
				if allOptions && len(blocks) > 1 {
					// This block is pure options AND there are more blocks
					// following. Likely debris from a previous page — skip it.
					continue
				}
				// Otherwise keep it (standalone option block, heading, or only block)
			}
		}

		// Clean trailing empty lines
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}

		// Clean leading empty lines
		for len(block) > 0 && strings.TrimSpace(block[0]) == "" {
			block = block[1:]
		}

		if len(block) > 0 {
			result = append(result, block)
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Step 9: splitBlocksAtEmbeddedQuestionStarts
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) splitBlocksAtEmbeddedQuestionStarts(
	blocks [][]string,
	strategy types.BlockParseStrategy,
) [][]string {
	if !strategy.SplitEmbeddedQuestionNumbers {
		return blocks
	}

	var result [][]string
	for _, block := range blocks {
		parts := s.splitBlockAtInternalQuestionMarkers(block)
		result = append(result, parts...)
	}
	return result
}

func (s *BlockAnalysisService) splitBlockAtInternalQuestionMarkers(block []string) [][]string {
	if len(block) == 0 {
		return nil
	}

	var result [][]string
	var current []string

	for _, line := range block {
		if blockQuestionNumPattern.MatchString(line) {
			if len(current) > 0 {
				result = append(result, current)
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}

	if len(current) > 0 {
		result = append(result, current)
	}
	return result
}

// ---------------------------------------------------------------------------
// Step 10: rawBlocksToImportBlocks — convert [][]string to []ImportBlock + extract tags
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) rawBlocksToImportBlocks(
	blocks [][]string,
	strategy types.BlockParseStrategy,
) []types.ImportBlock {
	result := make([]types.ImportBlock, 0, len(blocks))
	var sectionTags []string
	var questionTypeTags []string

	for _, rawBlock := range blocks {
		// Check if this block consists entirely of heading lines (section/type)
		allHeadings := true
		hasAnyHeading := false
		for _, line := range rawBlock {
			if line == "" {
				continue
			}
			isSection := strategy.DetectSectionHeadings && sectionHeadingPattern.MatchString(line)
			isQType := strategy.DetectQuestionTypeHeadings && questionTypeHeadingPattern.MatchString(line)
			if isSection || isQType {
				hasAnyHeading = true
			} else {
				allHeadings = false
			}
		}

		// Extract tags from heading lines
		var cleanedLines []string
		for _, line := range rawBlock {
			if line == "" {
				cleanedLines = append(cleanedLines, line)
				continue
			}

			handled := false

			// Detect section headings
			if strategy.DetectSectionHeadings && sectionHeadingPattern.MatchString(line) {
				if strategy.ExtractSectionTags {
					tag := strings.TrimSpace(line)
					tag = strings.TrimPrefix(tag, "【")
					tag = strings.TrimSuffix(tag, "】")
					sectionTags = append(sectionTags, tag)
				}
				handled = true
			}

			// Detect question type headings
			if strategy.DetectQuestionTypeHeadings && questionTypeHeadingPattern.MatchString(line) {
				if strategy.ExtractQuestionTypeTags {
					tag := strings.TrimSpace(line)
					tag = strings.TrimPrefix(tag, "【")
					tag = strings.TrimSuffix(tag, "】")
					questionTypeTags = append(questionTypeTags, tag)
				}
				handled = true
			}

			if !handled {
				cleanedLines = append(cleanedLines, line)
			}
		}

		// If this block is purely headings, extract tags but don't create an ImportBlock
		if hasAnyHeading && allHeadings && len(cleanedLines) == 0 {
			continue
		}

		// If no content lines remain after heading extraction, skip
		hasContent := false
		for _, line := range cleanedLines {
			if strings.TrimSpace(line) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		blk := s.linesToImportBlock(rawBlock, len(result))

		// Rebuild CurrentText from cleaned lines
		textLines := make([]string, 0, len(cleanedLines))
		for _, line := range cleanedLines {
			if strings.TrimSpace(line) != "" {
				textLines = append(textLines, line)
			}
		}
		blk.CurrentText = strings.Join(textLines, "\n")

		// Accumulate tags
		allTags := make([]string, 0, len(sectionTags)+len(questionTypeTags))
		allTags = append(allTags, sectionTags...)
		allTags = append(allTags, questionTypeTags...)
		blk.Tags = allTags

		result = append(result, blk)
	}

	return result
}

// linesToImportBlock converts raw block lines to an ImportBlock.
func (s *BlockAnalysisService) linesToImportBlock(lines []string, index int) types.ImportBlock {
	// Extract text (non-empty lines)
	textLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			textLines = append(textLines, line)
		}
	}
	text := strings.Join(textLines, "\n")

	// Extract question number from first line
	var qNum *int
	if len(lines) > 0 {
		firstLine := lines[0]
		num := s.extractQuestionNumber(firstLine)
		if num != nil {
			qNum = num
		}
	}

	return types.ImportBlock{
		ID:           uuid.NewString(),
		Index:        index,
		OriginalText: text,
		CurrentText:  text,
		QuestionNumber: qNum,
		Tags:         nil,
		Metadata:     make(map[string]interface{}),
		Anomalies:    nil,
	}
}

func (s *BlockAnalysisService) extractQuestionNumber(line string) *int {
	// Try to match question number patterns
	if m := blockQuestionNumPattern.FindStringSubmatch(line); m != nil {
		full := m[0]
		// Try Arabic numeral
		numRe := regexp.MustCompile(`\d+`)
		if n := numRe.FindString(full); n != "" {
			v, _ := strconv.Atoi(n)
			return &v
		}
		// Try Chinese numeral
		if cn := s.chineseToInt(full); cn > 0 {
			return &cn
		}
	}
	return nil
}

// chineseToInt converts Chinese numerals to integer. Returns 0 on failure.
func (s *BlockAnalysisService) chineseToInt(s2 string) int {
	// Simple mapping for 一～十
	cn := map[rune]int{
		'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
		'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
	}
	for _, r := range s2 {
		if v, ok := cn[r]; ok {
			return v
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Step 11: validateBlockAnomalies
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) validateBlockAnomalies(blocks []types.ImportBlock) []types.ImportBlock {
	var prevNum *int
	var firstNum *int
	seenNums := make(map[int]bool)

	for i := range blocks {
		blk := &blocks[i]
		num := blk.QuestionNumber

		// Track first question number for initial gap detection
		if num != nil && firstNum == nil {
			firstNum = num
			if *num > 1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyQuestionNumberGap,
					Message: fmt.Sprintf("首个题号为 %d，前面可能遗漏了题目", *num),
				})
			}
		}

		// MISSING_QUESTION_NUMBER
		if num == nil {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyMissingQuestionNumber,
				Message: "未检测到题号",
			})
		} else {
			n := *num

			// MULTIPLE_QUESTION_MARKERS (check if text has additional question patterns)
			if s.countQuestionMarkers(blk.CurrentText) > 1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyMultipleQuestionMarkers,
					Message: fmt.Sprintf("block 中包含多个题号标记"),
				})
			}

			// DUPLICATE_QUESTION_NUMBER
			if seenNums[n] {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyDuplicateQuestionNumber,
					Message: fmt.Sprintf("题号 %d 重复出现", n),
				})
			}
			seenNums[n] = true

			// NON_MONOTONIC_QUESTION_NUMBER
			if prevNum != nil && n < *prevNum {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyNonMonotonicQuestionNum,
					Message: fmt.Sprintf("题号 %d 小于前一个题号 %d", n, *prevNum),
				})
			}

			// QUESTION_NUMBER_GAP
			if prevNum != nil && n > *prevNum+1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyQuestionNumberGap,
					Message: fmt.Sprintf("题号从 %d 跳至 %d，中间可能存在遗漏", *prevNum, n),
				})
			}

			prevNum = num
		}

		// OPTION_ONLY_BLOCK: every non-empty line is an option marker
		if s.isOptionOnlyBlock(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyOptionOnlyBlock,
				Message: "block 仅包含选项标记，可能是前一题选项的延续",
			})
		}

		// OPTION_SEQUENCE_RESTART
		if s.hasOptionRestart(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyOptionSequenceRestart,
				Message: "选项序列在 block 内重新开始",
			})
		}

		// STEM_TOO_SHORT
		if len(strings.TrimSpace(blk.CurrentText)) < 10 {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyStemTooShort,
				Message: "题干文本过短，可能不完整",
			})
		}

		// STEM_TOO_LONG
		if len(strings.TrimSpace(blk.CurrentText)) > 5000 {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyStemTooLong,
				Message: "题干文本过长，可能包含多道题目",
			})
		}

		// PAGE_NOISE_DETECTED
		if s.containsPageNoise(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyPageNoiseDetected,
				Message: "block 中检测到页码噪声",
			})
		}

		// SECTION_HEADING_IN_STEM
		if sectionHeadingPattern.MatchString(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalySectionHeadingInStem,
				Message: "题干中包含章节标题文本",
			})
		}

		// QUESTION_TYPE_HEADING_IN_STEM
		if questionTypeHeadingPattern.MatchString(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Type:    types.AnomalyQuestionTypeHeadingInStem,
				Message: "题干中包含题型标题文本",
			})
		}

		// MISSING_ANSWER: no answer section after this block (only on last block)
		if i == len(blocks)-1 {
			lastText := strings.ToLower(blk.CurrentText)
			lines := strings.Split(lastText, "\n")
			hasAnswer := false
			for _, line := range lines {
				if blockAnswerLabelPattern.MatchString(strings.TrimSpace(line)) {
					hasAnswer = true
					break
				}
			}
			if !hasAnswer {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Type:    types.AnomalyMissingAnswer,
					Message: "最后一个 block 未检测到答案部分",
				})
			}
		}
	}

	return blocks
}

func (s *BlockAnalysisService) countQuestionMarkers(text string) int {
	matches := blockQuestionNumPattern.FindAllString(text, -1)
	return len(matches)
}

func (s *BlockAnalysisService) isOptionOnlyBlock(text string) bool {
	lines := strings.Split(text, "\n")
	nonEmpty := 0
	optionLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if blockOptionLabelPattern.MatchString(trimmed) {
			optionLines++
		}
	}
	return nonEmpty > 0 && optionLines == nonEmpty
}

func (s *BlockAnalysisService) hasOptionRestart(text string) bool {
	lines := strings.Split(text, "\n")
	var lastLetter rune
	for _, line := range lines {
		m := blockOptionLabelPattern.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			letter := rune(strings.ToUpper(m[1])[0])
			if letter < lastLetter {
				return true
			}
			lastLetter = letter
		}
	}
	return false
}

func (s *BlockAnalysisService) containsPageNoise(text string) bool {
	for _, pat := range pageNumPatterns {
		if pat.MatchString(text) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Step 12: sortBlocksByQuestionNumber
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) sortBlocksByQuestionNumber(blocks []types.ImportBlock) []types.ImportBlock {
	// Separate blocks with and without question numbers
	var numbered []types.ImportBlock
	var unnumbered []types.ImportBlock
	for _, b := range blocks {
		if b.QuestionNumber != nil {
			numbered = append(numbered, b)
		} else {
			unnumbered = append(unnumbered, b)
		}
	}

	sort.SliceStable(numbered, func(i, j int) bool {
		return *numbered[i].QuestionNumber < *numbered[j].QuestionNumber
	})

	// Append unnumbered at the end
	result := append(numbered, unnumbered...)

	// Re-index
	for i := range result {
		result[i].Index = i
	}

	return result
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) buildSummary(blocks []types.ImportBlock) types.BlockPreviewSummary {
	summary := types.BlockPreviewSummary{
		TotalBlocks:      len(blocks),
		AnomalyBreakdown: make(map[string]int),
	}

	qNumCount := 0
	for _, b := range blocks {
		if b.QuestionNumber != nil {
			qNumCount++
		}
		if len(b.Anomalies) > 0 {
			summary.BlocksWithAnomalies++
			for _, a := range b.Anomalies {
				summary.AnomalyBreakdown[a.Type]++
			}
		}
	}
	summary.QuestionNumbers = qNumCount
	return summary
}

// ---------------------------------------------------------------------------
// Utility: convert Chinese number prefix to section tag helper
// ---------------------------------------------------------------------------

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure unicode is imported (no unused import error)
var _ = unicode.Han

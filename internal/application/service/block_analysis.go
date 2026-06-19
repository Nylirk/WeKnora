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
// Patterns
// ---------------------------------------------------------------------------

// strongQuestionNumPattern matches explicit question number markers at line start.
// Supports: 1. 1) 1、 (1) （1） 一、 Question 1
// Does NOT match bare "249 题干" — that is handled by bareQuestionPattern (PDF only).
var strongQuestionNumPattern = regexp.MustCompile(
	`^[\s]*((?:\d+)[\.\)、]|（\s*\d+\s*）|[（(]\s*\d+\s*[）)]|[一二三四五六七八九十]+[、.）\)]|(?:Question\s*\d+))\s*`,
)

// bareQuestionPattern matches a bare number followed by CJK text at line start.
// PDF-only — must be explicitly enabled via AllowBareQuestionNumber.
var bareQuestionPattern = regexp.MustCompile(
	`^[\s]*(\d{1,4})\s+[\p{Han}\p{Hiragana}\p{Katakana}]`,
)

// embeddedStrongMarkerPattern matches an embedded question number with
// explicit marker (number + .  /  ) / 、) mid-line.
// Left boundary: line-start or whitespace.
// Right boundary: whitespace, end-of-string, or CJK (e.g. "249.津液...").
var embeddedStrongMarkerPattern = regexp.MustCompile(
	`(?:^|\s)(\d+)[\.\)、](?:\s+|$|[\p{Han}\p{Hiragana}\p{Katakana}])`,
)

// embeddedBareQuestionPattern matches an embedded bare question number
// (PDF only). Strict left boundary, 1-4 digit number, then CJK text.
// Left boundary: line-start, tab, 2+ spaces, fullwidth space, or
// closing parenthesis + space (common PDF column gap pattern).
var embeddedBareQuestionPattern = regexp.MustCompile(
	`(?:^|[\t]|\s{2,}|[\x{3000}]|[)）]\s+)(\d{1,4})\s*[\p{Han}\p{Hiragana}\p{Katakana}]`,
)

// pageNumPatterns are patterns for pure page number lines that should be removed.
var pageNumPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[\s]*第\s*\d+\s*页[，,\s]*共\s*\d+\s*页[\s]*$`),
	regexp.MustCompile(`^[\s]*第\s*\d+\s*页[\s]*$`),
	regexp.MustCompile(`(?i)^[\s]*page\s+\d+\s+of\s+\d+[\s]*$`),
	regexp.MustCompile(`(?i)^[\s]*page\s+\d+[\s]*$`),
}

// barePageNumPattern matches bare page number lines (e.g., "20", "46")
var barePageNumPattern = regexp.MustCompile(`^[\s]*\d{1,4}[\s]*$`)

// sectionHeadingPattern matches chapter/section/unit headings.
// Anchored to line start — must not match within option lines.
var sectionHeadingPattern = regexp.MustCompile(
	`^[\s]*(第[一二三四五六七八九十百千\d]+(?:章|节|单元|篇|部分)|(?:Unit|Chapter|Section|Part)\s*\d+)\s*(.*)$`,
)

// questionTypeHeadingPattern matches medical question type headings.
// Anchored to line start.
var questionTypeHeadingPattern = regexp.MustCompile(
	`^[\s]*(?:【?)(A[12]型题|A1型题|A2型题|A3型题|A4型题|B型题|B1型题|C型题|X型题)(?:】?)[\s]*$`,
)

// optionLabelPattern matches a single option label at line start.
var blockOptionLabelPattern = regexp.MustCompile(`^\s*([A-Za-z])[.．)、）：:]\s*`)

// answerLabelPattern matches answer section labels.
var blockAnswerLabelPattern = regexp.MustCompile(
	`(?i)^[\s]*(?:答案|参考答案|答案解析|答案部分|答案解析部分|Answer\s*(?:Key)?|Explanation)[：:]\s*`,
)

// Note: bracketAnswerPattern is declared in question_extraction.go — reuse that.


// falsePositiveNumberPatterns — numbers that look like question markers but aren't.
var falsePositivePrePatterns = []*regexp.Regexp{
	regexp.MustCompile(`第\d+章`),  // 第2章
	regexp.MustCompile(`第\d+页`),  // 第20页
	regexp.MustCompile(`\d+\.\d+`), // 1.5 (decimal)
}

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

	normalized := s.normalizeText(text)
	normalized = s.normalizeQuestionMarkers(normalized)
	lines := s.splitAndCleanLines(normalized, strategy)
	lines = s.splitLinesAtEmbeddedQuestionStarts(lines, strategy)
	lines = s.repairBareQuestionNumberLines(lines, strategy)
	lines = s.detectInterleavedTwoColumn(lines, strategy)
	rawBlocks := s.partitionIntoBlocks(lines, strategy)
	rawBlocks = s.sanitizeBlocks(rawBlocks, strategy)
	rawBlocks = s.splitBlocksAtEmbeddedQuestionStarts(rawBlocks, strategy)

	blocks := s.rawBlocksToImportBlocks(rawBlocks, strategy)
	blocks = s.validateBlockAnomalies(blocks)

	if strategy.SortBlocksByQuestionNumber {
		blocks = s.sortBlocksByQuestionNumber(blocks)
	}

	summary := s.buildSummary(blocks)
	return blocks, summary
}

// ---------------------------------------------------------------------------
// Step 1: normalizeText
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")

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
			if len(result) > 0 && result[len(result)-1] == "" {
				continue
			}
			result = append(result, "")
			continue
		}

		if strategy.RemovePageNumbers && s.isPageNumberLine(trimmed, i, raw) {
			continue
		}

		result = append(result, trimmed)
	}

	return result
}

func (s *BlockAnalysisService) isPageNumberLine(line string, index int, allLines []string) bool {
	for _, pat := range pageNumPatterns {
		if pat.MatchString(line) {
			return true
		}
	}

	if barePageNumPattern.MatchString(line) {
		num, _ := strconv.Atoi(strings.TrimSpace(line))
		if num >= 1 && num <= 9999 {
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

		// Collect split positions from strong markers + (if pdf) bare markers
		splitPositions := s.collectEmbeddedSplitPositions(line, strategy)
		if len(splitPositions) == 0 {
			result = append(result, line)
			continue
		}

		prev := 0
		for _, pos := range splitPositions {
			part := strings.TrimSpace(line[prev:pos])
			if part != "" {
				result = append(result, part)
			}
			prev = pos
		}
		part := strings.TrimSpace(line[prev:])
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (s *BlockAnalysisService) collectEmbeddedSplitPositions(line string, strategy types.BlockParseStrategy) []int {
	var positions []int

	// Strong markers (always checked)
	strongMatches := embeddedStrongMarkerPattern.FindAllStringSubmatchIndex(line, -1)
	for _, m := range strongMatches {
		if m[0] == 0 {
			continue // first match at start is the primary question, not embedded
		}
		// False positive check
		numStart := m[2] // group 1 start position
		numEnd := m[3]   // group 1 end position
		numStr := line[numStart:numEnd]
		if s.isFalsePositiveNumber(line, m[0], numStr) {
			continue
		}
		positions = append(positions, m[0])
	}

	// Bare markers (PDF only)
	if strategy.AllowBareQuestionNumber {
		bareMatches := embeddedBareQuestionPattern.FindAllStringSubmatchIndex(line, -1)
		for _, m := range bareMatches {
			if m[0] == 0 {
				continue
			}
			numStart := m[2]
			numEnd := m[3]
			numStr := line[numStart:numEnd]
			// Pass numStart so false-positive check looks at character
			// before the number itself, not before the left boundary.
			if s.isFalsePositiveNumber(line, numStart, numStr) {
				continue
			}
			positions = append(positions, numStart)
		}
	}

	// Sort and dedup
	sort.Ints(positions)
	deduped := make([]int, 0, len(positions))
	for _, p := range positions {
		if len(deduped) == 0 || p != deduped[len(deduped)-1] {
			deduped = append(deduped, p)
		}
	}
	return deduped
}

func (s *BlockAnalysisService) isFalsePositiveNumber(line string, matchStart int, numStr string) bool {
	if matchStart > 0 {
		prefixStart := max(0, matchStart-3)
		prefix := line[prefixStart:matchStart]
		for _, pat := range falsePositivePrePatterns {
			if pat.MatchString(prefix + numStr) {
				return true
			}
		}
		// Check preceding character: if it's a letter or digit without
		// a column-gap separator, it's likely false positive (e.g. "10mg",
		// "E249"). But if the boundary is ")" or "）" (column gap), allow it.
		prevByte := line[matchStart-1]
		// ASCII closing paren
		if prevByte == ')' {
			return false
		}
		// Fullwidth closing paren (U+FF09) — check the 3-byte UTF-8 sequence
		if matchStart >= 3 && line[matchStart-3:matchStart] == "）" {
			return false
		}
		if (prevByte >= '0' && prevByte <= '9') || (prevByte >= 'a' && prevByte <= 'z') || (prevByte >= 'A' && prevByte <= 'Z') {
			return true
		}
	}
	return false
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

		// If line matches bareQuestionPattern but NOT strongQuestionNumPattern,
		// convert it to a proper question marker.
		if bareQuestionPattern.MatchString(line) && !strongQuestionNumPattern.MatchString(line) {
			matches := bareQuestionPattern.FindStringSubmatch(line)
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
	return lines
}

// ---------------------------------------------------------------------------
// Step 7: partitionIntoBlocks
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) partitionIntoBlocks(lines []string, strategy types.BlockParseStrategy) [][]string {
	var blocks [][]string
	var current []string

	isQuestionStart := func(line string) bool {
		if strongQuestionNumPattern.MatchString(line) {
			return true
		}
		if strategy.AllowBareQuestionNumber && bareQuestionPattern.MatchString(line) {
			return true
		}
		return false
	}

	for _, line := range lines {
		if line == "" {
			if len(current) > 0 {
				current = append(current, line)
			}
			continue
		}

		if isQuestionStart(line) {
			if len(current) > 0 {
				blocks = append(blocks, current)
			}
			current = []string{line}
		} else if len(current) > 0 {
			current = append(current, line)
		} else {
			current = []string{line}
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

// ---------------------------------------------------------------------------
// Step 8: sanitizeBlocks — apply to ALL blocks
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) sanitizeBlocks(blocks [][]string, strategy types.BlockParseStrategy) [][]string {
	if len(blocks) == 0 {
		return blocks
	}

	var result [][]string

	for i, block := range blocks {
		if len(block) == 0 {
			continue
		}

		// Cross-block: if this block has no question start AND consists entirely
		// of option lines AND the next block has a question start, skip it as debris.
		if !s.blockHasQuestionStart(block, strategy) && s.isPureOptionBlock(block) && i+1 < len(blocks) {
			if s.blockHasQuestionStart(blocks[i+1], strategy) {
				continue
			}
		}

		block = s.sanitizeSingleBlock(block, strategy)

		// Clean trailing/leading empty lines
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		for len(block) > 0 && strings.TrimSpace(block[0]) == "" {
			block = block[1:]
		}

		if len(block) > 0 {
			result = append(result, block)
		}
	}

	return result
}

func (s *BlockAnalysisService) blockHasQuestionStart(block []string, strategy types.BlockParseStrategy) bool {
	for _, line := range block {
		if s.isQuestionStart(line, strategy) {
			return true
		}
	}
	return false
}

func (s *BlockAnalysisService) isPureOptionBlock(block []string) bool {
	for _, line := range block {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !blockOptionLabelPattern.MatchString(trimmed) {
			return false
		}
	}
	return true
}

func (s *BlockAnalysisService) sanitizeSingleBlock(block []string, strategy types.BlockParseStrategy) []string {
	if len(block) == 0 {
		return block
	}

	// 1. Remove pure page number lines from within the block
	if strategy.RemovePageNumbers {
		cleaned := make([]string, 0, len(block))
		for _, line := range block {
			if !s.isPageNumberLine(line, 0, nil) {
				cleaned = append(cleaned, line)
			}
		}
		block = cleaned
	}

	// 2. If first line is not a question start, strip leading debris
	if len(block) > 0 && !s.isQuestionStart(block[0], strategy) {
		cutIndex := -1
		for j, line := range block {
			if s.isQuestionStart(line, strategy) {
				cutIndex = j
				break
			}
		}

		if cutIndex > 0 {
			// Strip only if preceding lines are ALL option labels
			precedingLines := block[:cutIndex]
			allOptions := true
			for _, line := range precedingLines {
				if line == "" {
					continue
				}
				if !blockOptionLabelPattern.MatchString(line) {
					allOptions = false
					break
				}
			}
			if allOptions {
				block = block[cutIndex:]
			}
		}
	}

	return block
}

func (s *BlockAnalysisService) isQuestionStart(line string, strategy types.BlockParseStrategy) bool {
	if strongQuestionNumPattern.MatchString(line) {
		return true
	}
	if strategy.AllowBareQuestionNumber && bareQuestionPattern.MatchString(line) {
		return true
	}
	return false
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
		parts := s.splitBlockAtInternalQuestionMarkers(block, strategy)
		result = append(result, parts...)
	}
	return result
}

func (s *BlockAnalysisService) splitBlockAtInternalQuestionMarkers(block []string, strategy types.BlockParseStrategy) [][]string {
	if len(block) == 0 {
		return nil
	}

	var result [][]string
	var current []string

	for _, line := range block {
		if s.isQuestionStart(line, strategy) {
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
// Step 10: rawBlocksToImportBlocks
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) rawBlocksToImportBlocks(
	blocks [][]string,
	strategy types.BlockParseStrategy,
) []types.ImportBlock {
	result := make([]types.ImportBlock, 0, len(blocks))
	var sectionTags []string
	var questionTypeTags []string

	for _, rawBlock := range blocks {
		// Check if block is purely headings
		allHeadings, hasAnyHeading := s.classifyBlockLines(rawBlock, strategy)

		// Extract tags from standalone heading lines
		var cleanedLines []string
		for _, line := range rawBlock {
			if line == "" {
				cleanedLines = append(cleanedLines, line)
				continue
			}

			// Only classify as heading if line is standalone (not an option line)
			handled := false

			if strategy.DetectSectionHeadings && s.isStandaloneSectionHeading(line) {
				if strategy.ExtractSectionTags {
					tag := strings.TrimSpace(line)
					tag = strings.TrimPrefix(tag, "【")
					tag = strings.TrimSuffix(tag, "】")
					sectionTags = append(sectionTags, tag)
				}
				handled = true
			}

			if strategy.DetectQuestionTypeHeadings && s.isStandaloneQuestionTypeHeading(line) {
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

		// If purely headings, skip creating a block
		if hasAnyHeading && allHeadings && len(cleanedLines) == 0 {
			continue
		}

		// If no content after heading extraction
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

		// If at least one heading was extracted AND all remaining lines
		// are option lines, this is option debris with a heading annotation.
		// Skip creating ImportBlock — accumulated tags carry forward.
		if hasAnyHeading {
			allRemainingAreOptions := true
			for _, line := range cleanedLines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				if !blockOptionLabelPattern.MatchString(trimmed) {
					allRemainingAreOptions = false
					break
				}
			}
			if allRemainingAreOptions {
				continue
			}
		}

		blk := s.linesToImportBlock(rawBlock, len(result), strategy)

		textLines := make([]string, 0, len(cleanedLines))
		for _, line := range cleanedLines {
			if strings.TrimSpace(line) != "" {
				textLines = append(textLines, line)
			}
		}
		blk.CurrentText = strings.Join(textLines, "\n")

		allTags := make([]string, 0, len(sectionTags)+len(questionTypeTags))
		allTags = append(allTags, sectionTags...)
		allTags = append(allTags, questionTypeTags...)
		blk.Tags = allTags

		result = append(result, blk)
	}

	return result
}

func (s *BlockAnalysisService) classifyBlockLines(block []string, strategy types.BlockParseStrategy) (allHeadings bool, hasAnyHeading bool) {
	allHeadings = true
	for _, line := range block {
		if line == "" {
			continue
		}
		isSection := strategy.DetectSectionHeadings && s.isStandaloneSectionHeading(line)
		isQType := strategy.DetectQuestionTypeHeadings && s.isStandaloneQuestionTypeHeading(line)
		if isSection || isQType {
			hasAnyHeading = true
		} else {
			allHeadings = false
		}
	}
	return
}

// isStandaloneSectionHeading checks if a line is a standalone section heading.
// Must NOT match option lines (A. B. C. D. etc.).
func (s *BlockAnalysisService) isStandaloneSectionHeading(line string) bool {
	if blockOptionLabelPattern.MatchString(strings.TrimSpace(line)) {
		return false
	}
	return sectionHeadingPattern.MatchString(line)
}

// isStandaloneQuestionTypeHeading checks if a line is a standalone question type heading.
// Must NOT match option lines (A. B. C. D. etc.).
func (s *BlockAnalysisService) isStandaloneQuestionTypeHeading(line string) bool {
	if blockOptionLabelPattern.MatchString(strings.TrimSpace(line)) {
		return false
	}
	return questionTypeHeadingPattern.MatchString(line)
}

// linesToImportBlock converts raw block lines to an ImportBlock.
func (s *BlockAnalysisService) linesToImportBlock(lines []string, index int, strategy types.BlockParseStrategy) types.ImportBlock {
	textLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			textLines = append(textLines, line)
		}
	}
	text := strings.Join(textLines, "\n")

	var qNum *int
	if len(lines) > 0 {
		qNum = s.extractQuestionNumber(lines[0], strategy)
	}

	return types.ImportBlock{
		ID:             uuid.NewString(),
		Index:          index,
		OriginalText:   text,
		CurrentText:    text,
		QuestionNumber: qNum,
		Tags:           nil,
		Metadata:       make(map[string]interface{}),
		Anomalies:      nil,
	}
}

func (s *BlockAnalysisService) extractQuestionNumber(line string, strategy types.BlockParseStrategy) *int {
	// Try strong markers first
	if m := strongQuestionNumPattern.FindStringSubmatch(line); m != nil {
		full := m[0]
		numRe := regexp.MustCompile(`\d+`)
		if n := numRe.FindString(full); n != "" {
			v, _ := strconv.Atoi(n)
			return &v
		}
		if cn := s.chineseToInt(full); cn > 0 {
			return &cn
		}
	}

	// PDF: also try bare number
	if strategy.AllowBareQuestionNumber {
		if m := bareQuestionPattern.FindStringSubmatch(line); m != nil {
			if len(m) >= 2 {
				v, err := strconv.Atoi(m[1])
				if err == nil {
					return &v
				}
			}
		}
	}

	return nil
}

func (s *BlockAnalysisService) chineseToInt(s2 string) int {
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
// Step 11: validateBlockAnomalies — with severity, per-block MISSING_ANSWER
// ---------------------------------------------------------------------------

func (s *BlockAnalysisService) validateBlockAnomalies(blocks []types.ImportBlock) []types.ImportBlock {
	var prevNum *int
	var firstNum *int
	seenNums := make(map[int]bool)

	for i := range blocks {
		blk := &blocks[i]
		num := blk.QuestionNumber

		// Initial gap
		if num != nil && firstNum == nil {
			firstNum = num
			if *num > 1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Code:     types.AnomalyQuestionNumberGap,
					Severity: types.SeverityWarning,
					Message:  fmt.Sprintf("首个题号为 %d，前面可能遗漏了题目", *num),
				})
			}
		}

		if num == nil {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyMissingQuestionNumber,
				Severity: types.SeverityWarning,
				Message:  "未检测到题号",
			})
		} else {
			n := *num

			if s.countQuestionMarkers(blk.CurrentText) > 1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Code:     types.AnomalyMultipleQuestionMarkers,
					Severity: types.SeverityError,
					Message:  "block 中包含多个题号标记",
				})
			}

			if seenNums[n] {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Code:     types.AnomalyDuplicateQuestionNumber,
					Severity: types.SeverityError,
					Message:  fmt.Sprintf("题号 %d 重复出现", n),
				})
			}
			seenNums[n] = true

			if prevNum != nil && n < *prevNum {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Code:     types.AnomalyNonMonotonicQuestionNum,
					Severity: types.SeverityWarning,
					Message:  fmt.Sprintf("题号 %d 小于前一个题号 %d", n, *prevNum),
				})
			}

			if prevNum != nil && n > *prevNum+1 {
				blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
					Code:     types.AnomalyQuestionNumberGap,
					Severity: types.SeverityWarning,
					Message:  fmt.Sprintf("题号从 %d 跳至 %d，中间可能存在遗漏", *prevNum, n),
				})
			}

			prevNum = num
		}

		// OPTION_ONLY_BLOCK
		if s.isOptionOnlyBlock(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyOptionOnlyBlock,
				Severity: types.SeverityError,
				Message:  "block 仅包含选项标记，可能是前一题选项的延续",
			})
		}

		// OPTION_SEQUENCE_RESTART
		if s.hasOptionRestart(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyOptionSequenceRestart,
				Severity: types.SeverityWarning,
				Message:  "选项序列在 block 内重新开始",
			})
		}

		// STEM_TOO_SHORT
		if len(strings.TrimSpace(blk.CurrentText)) < 10 {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyStemTooShort,
				Severity: types.SeverityWarning,
				Message:  "题干文本过短，可能不完整",
			})
		}

		// STEM_TOO_LONG
		if len(strings.TrimSpace(blk.CurrentText)) > 5000 {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyStemTooLong,
				Severity: types.SeverityWarning,
				Message:  "题干文本过长，可能包含多道题目",
			})
		}

		// PAGE_NOISE_DETECTED
		if s.containsPageNoise(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyPageNoiseDetected,
				Severity: types.SeverityWarning,
				Message:  "block 中检测到页码噪声",
			})
		}

		// SECTION_HEADING_IN_STEM
		if sectionHeadingPattern.MatchString(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalySectionHeadingInStem,
				Severity: types.SeverityWarning,
				Message:  "题干中包含章节标题文本",
			})
		}

		// QUESTION_TYPE_HEADING_IN_STEM
		if questionTypeHeadingPattern.MatchString(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyQuestionTypeHeadingInStem,
				Severity: types.SeverityWarning,
				Message:  "题干中包含题型标题文本",
			})
		}

		// MISSING_ANSWER: check every block, not just the last
		if !s.blockHasAnswer(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyMissingAnswer,
				Severity: types.SeverityWarning,
				Message:  "block 未检测到答案部分",
			})
		}

		// ANSWER_OUT_OF_OPTIONS: if block has options and an answer in brackets,
		// verify the answer letter is within option range.
		if s.answerOutOfOptions(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyAnswerOutOfOptions,
				Severity: types.SeverityError,
				Message:  "答案选项超出选项范围",
			})
		}

		// ANSWER_ANALYSIS_MIXED: answer section contains analysis-like text
		if s.answerAnalysisMixed(blk.CurrentText) {
			blk.Anomalies = append(blk.Anomalies, types.ImportBlockAnomaly{
				Code:     types.AnomalyAnswerAnalysisMixed,
				Severity: types.SeverityWarning,
				Message:  "答案部分可能混入了解析文本",
			})
		}
	}

	return blocks
}

func (s *BlockAnalysisService) blockHasAnswer(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if blockAnswerLabelPattern.MatchString(strings.TrimSpace(line)) {
			return true
		}
		// Also check for bracket answer patterns like （D） (E)
		if bracketAnswerPattern.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

func (s *BlockAnalysisService) answerOutOfOptions(text string) bool {
	// Find bracket answer
	ansMatch := bracketAnswerPattern.FindStringSubmatch(text)
	if ansMatch == nil {
		return false
	}
	answerLetter := strings.ToUpper(strings.TrimSpace(ansMatch[1]))

	// Find all option labels
	lines := strings.Split(text, "\n")
	maxOption := byte('A' - 1)
	for _, line := range lines {
		m := blockOptionLabelPattern.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			letter := strings.ToUpper(m[1])[0]
			if letter > maxOption {
				maxOption = letter
			}
		}
	}

	if maxOption < 'A' {
		return false // no options found
	}

	// Check each answer letter (answerLetter may have multiple chars A-E)
	for _, ch := range answerLetter {
		if ch < 'A' || ch > 'Z' {
			continue
		}
		if byte(ch) > maxOption {
			return true
		}
	}
	return false
}

func (s *BlockAnalysisService) answerAnalysisMixed(text string) bool {
	lines := strings.Split(text, "\n")
	inAnswer := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if blockAnswerLabelPattern.MatchString(trimmed) {
			inAnswer = true
			continue
		}
		if inAnswer {
			// If answer section spans many lines (3+) with substantial analysis content
			if len(trimmed) > 100 {
				return true
			}
		}
	}
	return false
}

func (s *BlockAnalysisService) countQuestionMarkers(text string) int {
	matches := strongQuestionNumPattern.FindAllString(text, -1)
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

	result := append(numbered, unnumbered...)
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
				summary.AnomalyBreakdown[a.Code]++
			}
		}
	}
	summary.QuestionNumbers = qNumCount
	return summary
}

// ---------------------------------------------------------------------------
// Utility
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

var _ = unicode.Han

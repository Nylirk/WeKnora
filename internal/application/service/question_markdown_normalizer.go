package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// questionMarkdownNormalizer parses markdown content (from docreader output)
// into ImportQuestionItem drafts. It does NOT read files — file reading must
// be done by the existing docreader pipeline before this normalizer runs.
//
// Supported format:
//
//	### 1. 题目内容
//	A. 选项A
//	B. 选项B
//	C. 选项C
//	D. 选项D
//	答案：B
//	解析：...
//	知识点：...
//	难度：easy
//
// The question number marker (e.g., "### 1.") starts a new question block.
var (
	mdQuestionStartPattern  = regexp.MustCompile(`(?m)^(?:#{1,4}\s*)?(\d+)[\.．、\)]\s*(.+)`)
	mdOptionPattern         = regexp.MustCompile(`(?i)^([A-H])[\.．、\)]\s*(.+)`)
	mdAnswerPattern         = regexp.MustCompile(`(?i)^答案[：:]\s*(.+)`)
	mdAnalysisPattern       = regexp.MustCompile(`(?i)^解析[：:]\s*(.+)`)
	mdKnowledgePointPattern = regexp.MustCompile(`(?i)^知识点[：:]\s*(.+)`)
	mdTagPattern            = regexp.MustCompile(`(?i)^标签[：:]\s*(.+)`)
	mdDifficultyPattern     = regexp.MustCompile(`(?i)^难度[：:]\s*(.+)`)
	mdSourcePattern         = regexp.MustCompile(`(?i)^来源[：:]\s*(.+)`)
)

// normalizeMarkdownQuestions parses markdown text into ImportQuestionItem drafts.
// It does NOT read files — it operates on the markdown text already extracted
// by the docreader.
func normalizeMarkdownQuestions(markdownText string) ([]types.ImportQuestionItem, []types.ImportQuestionError) {
	lines := strings.Split(markdownText, "\n")
	var items []types.ImportQuestionItem
	var errors []types.ImportQuestionError

	type currentQuestion struct {
		number          int
		stemLines       []string
		options         []types.QuestionOption
		answer          string
		analysis        string
		knowledgePoints []string
		tags            []string
		difficulty      string
		sourceNote      string
		startLine       int
	}

	var cur *currentQuestion
	flushCurrent := func() {
		if cur == nil {
			return
		}
		if strings.TrimSpace(strings.Join(cur.stemLines, "")) == "" {
			errors = append(errors, types.ImportQuestionError{
				LineNumber: cur.startLine,
				Message:    fmt.Sprintf("第 %d 题题干为空", cur.number),
			})
			cur = nil
			return
		}

		stemText := strings.TrimSpace(strings.Join(cur.stemLines, "\n"))
		item := types.ImportQuestionItem{
			StemText:     stemText,
			Difficulty:   normalizeDifficulty(cur.difficulty),
			AnalysisText: cur.analysis,
		}

		if len(cur.options) > 0 {
			item.QuestionType = string(types.QuestionTypeSingleChoice)
			if len(cur.answer) > 1 && strings.ContainsAny(cur.answer, ",，") {
				item.QuestionType = string(types.QuestionTypeMultipleChoice)
			}
			body := types.ChoiceQuestionBody{Options: cur.options}
			if bodyBytes, err := json.Marshal(body); err == nil {
				item.QuestionBody = types.JSON(bodyBytes)
			}
		} else {
			item.QuestionType = string(types.QuestionTypeShortAnswer)
		}

		item.AnswerText = strings.TrimSpace(cur.answer)

		if len(cur.knowledgePoints) > 0 {
			if kpBytes, err := json.Marshal(cur.knowledgePoints); err == nil {
				item.KnowledgePoints = types.JSON(kpBytes)
			}
		}
		if len(cur.tags) > 0 {
			if tagBytes, err := json.Marshal(cur.tags); err == nil {
				item.Tags = types.JSON(tagBytes)
			}
		}

		if cur.sourceNote != "" {
			if srcBytes, err := json.Marshal(map[string]string{"note": cur.sourceNote}); err == nil {
				item.SourcePayload = types.JSON(srcBytes)
			}
		}

		items = append(items, item)
		cur = nil
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := i + 1

		// Check for question start
		if matches := mdQuestionStartPattern.FindStringSubmatch(trimmed); matches != nil {
			flushCurrent()
			num := 0
			fmt.Sscanf(matches[1], "%d", &num)
			cur = &currentQuestion{
				number:    num,
				stemLines: []string{matches[2]},
				startLine: lineNum,
			}
			continue
		}

		// If no current question, skip
		if cur == nil {
			continue
		}

		// Check for metadata fields
		switch {
		case mdAnswerPattern.MatchString(trimmed):
			cur.answer = mdAnswerPattern.FindStringSubmatch(trimmed)[1]
		case mdAnalysisPattern.MatchString(trimmed):
			cur.analysis = mdAnalysisPattern.FindStringSubmatch(trimmed)[1]
		case mdKnowledgePointPattern.MatchString(trimmed):
			kp := mdKnowledgePointPattern.FindStringSubmatch(trimmed)[1]
			cur.knowledgePoints = splitAndTrim(kp, ",，;；")
		case mdTagPattern.MatchString(trimmed):
			tag := mdTagPattern.FindStringSubmatch(trimmed)[1]
			cur.tags = splitAndTrim(tag, ",，;；")
		case mdDifficultyPattern.MatchString(trimmed):
			cur.difficulty = strings.TrimSpace(mdDifficultyPattern.FindStringSubmatch(trimmed)[1])
		case mdSourcePattern.MatchString(trimmed):
			cur.sourceNote = strings.TrimSpace(mdSourcePattern.FindStringSubmatch(trimmed)[1])
		case mdOptionPattern.MatchString(trimmed):
			m := mdOptionPattern.FindStringSubmatch(trimmed)
			cur.options = append(cur.options, types.QuestionOption{
				Label:   strings.ToUpper(m[1]),
				Content: strings.TrimSpace(m[2]),
			})
		default:
			// Append to stem text (multi-line stem support)
			if trimmed != "" {
				cur.stemLines = append(cur.stemLines, trimmed)
			}
		}
	}
	flushCurrent()

	return items, errors
}

// splitAndTrim splits a string by any of the given separators and trims each part.
func splitAndTrim(s string, seps string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(seps, r)
	})
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// normalizeDifficulty maps Chinese difficulty labels to standard values.
func normalizeDifficulty(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "easy", "简单", "容易":
		return string(types.QuestionDifficultyEasy)
	case "medium", "中等", "一般":
		return string(types.QuestionDifficultyMedium)
	case "hard", "困难", "难":
		return string(types.QuestionDifficultyHard)
	default:
		return string(types.QuestionDifficultyMedium)
	}
}

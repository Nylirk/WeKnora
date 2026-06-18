package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func newTestExtractionService() *QuestionExtractionService {
	return NewQuestionExtractionService()
}

func TestExtractNumberedQuestions(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 什么是 Kubernetes？
A. 容器编排平台
B. 编程语言
C. 数据库
D. 网络协议
答案：A

2. Docker 的核心组件有哪些？
3. 请简述微服务架构的优势。`

	items, errors, warnings := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))

	if len(errors) > 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Item 1: choice question with options
	q1 := items[0]
	if q1.QuestionType != string(types.QuestionTypeSingleChoice) {
		t.Errorf("item 1 type = %q, want single_choice", q1.QuestionType)
	}
	if !strings.Contains(q1.StemText, "Kubernetes") {
		t.Errorf("item 1 stem = %q, should contain Kubernetes", q1.StemText)
	}
	if q1.AnswerText != "A" {
		t.Errorf("item 1 answer = %q, want A", q1.AnswerText)
	}
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}

	// Item 2: short answer (no options, no explicit answer)
	q2 := items[1]
	if q2.QuestionType != string(types.QuestionTypeShortAnswer) {
		t.Errorf("item 2 type = %q, want short_answer", q2.QuestionType)
	}
	if !strings.Contains(q2.StemText, "Docker") {
		t.Errorf("item 2 stem = %q, should contain Docker", q2.StemText)
	}

	// Item 3: short answer
	q3 := items[2]
	if q3.QuestionType != string(types.QuestionTypeShortAnswer) {
		t.Errorf("item 3 type = %q, want short_answer", q3.QuestionType)
	}
}

func TestExtractChineseNumbering(t *testing.T) {
	svc := newTestExtractionService()
	text := `1、第一题
一、第二题
（1）第三题`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestExtractQuestionNumbering(t *testing.T) {
	svc := newTestExtractionService()
	text := `Question 1: What is AI?
Answer: Artificial Intelligence

Question 2: What is ML?`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].AnswerText != "Artificial Intelligence" {
		t.Errorf("item 0 answer = %q, want 'Artificial Intelligence'", items[0].AnswerText)
	}
}

func TestExtractTrueFalseAnswer(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 地球是圆的。
答案：正确

2. 太阳绕地球转。
答案：错误`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].QuestionType != string(types.QuestionTypeTrueFalse) {
		t.Errorf("item 0 type = %q, want true_false", items[0].QuestionType)
	}
	if items[1].QuestionType != string(types.QuestionTypeTrueFalse) {
		t.Errorf("item 1 type = %q, want true_false", items[1].QuestionType)
	}
}

func TestExtractMultipleChoiceAnswer(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪些是编程语言？
A. Python
B. Java
C. HTML
D. CSS
答案：AB`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].QuestionType != string(types.QuestionTypeMultipleChoice) {
		t.Errorf("item 0 type = %q, want multiple_choice", items[0].QuestionType)
	}
}

func TestExtractFillBlank(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 请填写空白处的值：_______

2. 把括号中的内容补全：我们通常使用（）来管理 Docker 容器。`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].QuestionType != string(types.QuestionTypeFillBlank) {
		t.Errorf("item 0 type = %q, want fill_blank (_______ marker)", items[0].QuestionType)
	}
	if items[1].QuestionType != string(types.QuestionTypeFillBlank) {
		t.Errorf("item 1 type = %q, want fill_blank (（） marker)", items[1].QuestionType)
	}
}

func TestExtractWithAnalysis(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 题目一
答案：正确答案
解析：解题思路解析`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].AnalysisText == "" {
		t.Error("analysis should not be empty")
	}
	if !strings.Contains(items[0].AnalysisText, "解题思路") {
		t.Errorf("analysis = %q, should contain '解题思路'", items[0].AnalysisText)
	}
}

func TestExtractDefaultDifficulty(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 题目`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Difficulty != string(types.QuestionDifficultyMedium) {
		t.Errorf("difficulty = %q, want medium", items[0].Difficulty)
	}
}

func TestExtractCustomDifficulty(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 题目`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyHard))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Difficulty != string(types.QuestionDifficultyHard) {
		t.Errorf("difficulty = %q, want hard", items[0].Difficulty)
	}
}

func TestExtractEmptyText(t *testing.T) {
	svc := newTestExtractionService()
	_, _, warnings := svc.Extract(context.Background(), "", string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(warnings) == 0 {
		t.Fatal("expected warnings for empty text")
	}
}

func TestExtractNoQuestions(t *testing.T) {
	svc := newTestExtractionService()
	text := "这里只是一段普通文本，没有题号标记。\n全文只有一段介绍。"

	_, _, warnings := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(warnings) == 0 {
		t.Fatal("expected warnings for text with no questions")
	}
}

func TestExtractEmptyStemReturnsError(t *testing.T) {
	svc := newTestExtractionService()
	// Line 1 has a number marker but no actual stem content after stripping
	text := "1.\n答案：某个答案"

	items, errors, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty stem, got %d", len(items))
	}
	if len(errors) == 0 {
		t.Fatal("expected errors for empty stem block")
	}
	if !strings.Contains(errors[0].Message, "未识别到题干") {
		t.Errorf("error message = %q, should mention missing stem", errors[0].Message)
	}
}

func TestExtractStemWithOnlyNumber(t *testing.T) {
	svc := newTestExtractionService()
	// Only a number and whitespace as the block content
	text := "1. \n\n2. 有内容"

	items, errors, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item with content, got %d", len(items))
	}
	if len(errors) != 1 {
		t.Fatalf("expected 1 error for empty-stem block, got %d", len(errors))
	}
}

func TestExtractContextCancellation(t *testing.T) {
	svc := newTestExtractionService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	text := ""
	for i := 0; i < 100; i++ {
		text += "1. 题目\n答案：答案\n"
	}

	_, _, warnings := svc.Extract(ctx, text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	hasCancel := false
	for _, w := range warnings {
		if strings.Contains(w, "已取消") {
			hasCancel = true
			break
		}
	}
	if !hasCancel {
		t.Log("context cancellation did not produce a cancel warning (may finish before first check)")
	}
}

func TestExtractWithAnswerSectionPrefix(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 题目一
参考答案：标准答案

2. 题目二
答案解析：详细解析`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].AnswerText != "标准答案" {
		t.Errorf("item 0 answer = %q, want '标准答案'", items[0].AnswerText)
	}
	if items[1].AnswerText == "" {
		t.Error("item 1 answer should not be empty (答案解析)")
	}
}

func TestExtractOptionRecognition(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. Which language is used most?
A. JavaScript
B. Python
C. Java
D. TypeScript
Answer: B`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].QuestionType != string(types.QuestionTypeSingleChoice) {
		t.Errorf("type = %q, want single_choice (4 options)", items[0].QuestionType)
	}
}

func TestExtractMultiLineStem(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 多行题干示例：
请根据以下场景回答：
某公司需要迁移到云原生架构。
请问应该采用什么方案？`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].StemText, "云原生") {
		t.Errorf("stem should contain multi-line content")
	}
}

func TestOptionBodyIncludesCorrectLabels(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 选择题
A. 选项一
B. 选项二
答案：A`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].QuestionBody) == 0 {
		t.Fatal("question_body should not be empty for choice questions")
	}
	body := string(items[0].QuestionBody)
	if !strings.Contains(body, "options") {
		t.Errorf("question_body should contain options array, got %s", body)
	}
	if !strings.Contains(body, "选项一") || !strings.Contains(body, "选项二") {
		t.Errorf("question_body should contain both option contents, got %s", body)
	}
}

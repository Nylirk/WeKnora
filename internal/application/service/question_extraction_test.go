package service

import (
	"context"
	"encoding/json"
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

	q2 := items[1]
	if q2.QuestionType != string(types.QuestionTypeShortAnswer) {
		t.Errorf("item 2 type = %q, want short_answer", q2.QuestionType)
	}
	if !strings.Contains(q2.StemText, "Docker") {
		t.Errorf("item 2 stem = %q, should contain Docker", q2.StemText)
	}

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
		t.Errorf("item 0 type = %q, want multiple_choice (got %q)", items[0].QuestionType, items[0].AnswerText)
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
		t.Errorf("item 0 type = %q, want fill_blank", items[0].QuestionType)
	}
	if items[1].QuestionType != string(types.QuestionTypeFillBlank) {
		t.Errorf("item 1 type = %q, want fill_blank", items[1].QuestionType)
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
	cancel()

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
		t.Error("item 1 answer should not be empty")
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
		t.Errorf("type = %q, want single_choice", items[0].QuestionType)
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

// --- New tests for expanded option support ---

func TestExtractFiveOptionsOnePerLineWithAnswerInStem(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个是注册器？（E）
A. RegistryObject
B. EventBus
C. ModContainer
D. ItemStack
E. DeferredRegister`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.QuestionType != string(types.QuestionTypeSingleChoice) {
		t.Errorf("type = %q, want single_choice", q.QuestionType)
	}
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E", q.AnswerText)
	}
	if strings.Contains(q.StemText, "（E）") || strings.Contains(q.StemText, "(E)") {
		t.Errorf("stem should not contain answer bracket: %q", q.StemText)
	}

	// Verify all 5 options exist with correct labels
	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}
	assertOption(t, options, "A", "RegistryObject")
	assertOption(t, options, "B", "EventBus")
	assertOption(t, options, "C", "ModContainer")
	assertOption(t, options, "D", "ItemStack")
	assertOption(t, options, "E", "DeferredRegister")
}

func TestExtractFiveInlineOptionsWithAnswerInStem(t *testing.T) {
	svc := newTestExtractionService()
	text := "1. 以下哪个是注册器？（E） A. RegistryObject B. EventBus C. ModContainer D. ItemStack E. DeferredRegister"

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}
	assertOption(t, options, "A", "RegistryObject")
	assertOption(t, options, "B", "EventBus")
	assertOption(t, options, "C", "ModContainer")
	assertOption(t, options, "D", "ItemStack")
	assertOption(t, options, "E", "DeferredRegister")

	// The issue was "only E remains" — verify E is not the only option
	assertOption(t, options, "A", "RegistryObject")
}

func TestExtractSixInlineOptions(t *testing.T) {
	svc := newTestExtractionService()
	text := "1. 以下哪个选项正确？（F） A. A选项 B. B选项 C. C选项 D. D选项 E. E选项 F. F选项"

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "F" {
		t.Errorf("answer = %q, want F", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 6 {
		t.Fatalf("expected 6 options, got %d: %v", len(options), optionLabels(options))
	}
	assertOption(t, options, "F", "F选项")
}

func TestExtractMultipleChoiceAnswerBeyondD(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪些属于注册相关对象？（A、C、E）
A. RegistryObject
B. ItemStack
C. DeferredRegister
D. Level
E. RegisterEvent`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.QuestionType != string(types.QuestionTypeMultipleChoice) {
		t.Errorf("type = %q, want multiple_choice, answer=%q", q.QuestionType, q.AnswerText)
	}
	if q.AnswerText != "ACE" {
		t.Errorf("answer = %q, want ACE", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d", len(options))
	}
}

func TestExtractFullWidthOptionMarkersWithE(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个是注册器？（E）
A．RegistryObject
B．EventBus
C．ModContainer
D．ItemStack
E．DeferredRegister`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}
}

func TestExtractChineseOptionMarkersWithE(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个是注册器？（E）
A、RegistryObject
B、EventBus
C、ModContainer
D、ItemStack
E、DeferredRegister`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}
}

func TestExtractOptionContinuationDoesNotOverwritePreviousOption(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个是注册器？（E）
A. RegistryObject
用于持有注册结果
B. EventBus
C. ModContainer
D. ItemStack
E. DeferredRegister
用于延迟注册`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}

	// A's content should contain the continuation line
	aOpt := getOption(options, "A")
	if aOpt == nil {
		t.Fatal("option A not found")
	}
	if !strings.Contains(aOpt.Content, "用于持有注册结果") {
		t.Errorf("A content = %q, should contain continuation '用于持有注册结果'", aOpt.Content)
	}

	// E's content should contain the continuation line
	eOpt := getOption(options, "E")
	if eOpt == nil {
		t.Fatal("option E not found")
	}
	if !strings.Contains(eOpt.Content, "用于延迟注册") {
		t.Errorf("E content = %q, should contain continuation '用于延迟注册'", eOpt.Content)
	}
}

func TestDoesNotExtractParenthesesAnswerWithoutOptions(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. DeferredRegister（延迟注册器）是什么？
答案：它用于延迟注册对象。`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.QuestionType != string(types.QuestionTypeShortAnswer) {
		t.Errorf("type = %q, want short_answer", q.QuestionType)
	}
	if !strings.Contains(q.StemText, "延迟注册器") {
		t.Errorf("stem should preserve '（延迟注册器）', got %q", q.StemText)
	}
	if q.AnswerText != "它用于延迟注册对象。" {
		t.Errorf("answer = %q, want '它用于延迟注册对象。'", q.AnswerText)
	}
}

func TestExtractTwoOptionsStillRecognizedAsChoice(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 判断题？（B）
A. 正确
B. 错误`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	// 2 options should be recognized as single_choice
	if q.QuestionType != string(types.QuestionTypeSingleChoice) {
		t.Errorf("type = %q, want single_choice (2 options)", q.QuestionType)
	}
	if q.AnswerText != "B" {
		t.Errorf("answer = %q, want B (bracket extraction)", q.AnswerText)
	}
}

func TestExtractBracketAnswerWithParenthesesVar(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. Which statement is correct? (E)
A. Option one
B. Option two
C. Option three
D. Option four
E. Option five`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E (half-width parentheses)", q.AnswerText)
	}
}

func TestExtractBracketAnswerWithComma(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 下列选项正确的是（A,C,E）
A. a选项
B. b选项
C. c选项
D. d选项
E. e选项`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "ACE" {
		t.Errorf("answer = %q, want ACE (comma separated)", q.AnswerText)
	}
}

func TestExtractBracketAnswerWithSpace(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 下列选项正确的是（A C E）
A. a选项
B. b选项
C. c选项
D. d选项
E. e选项`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "ACE" {
		t.Errorf("answer = %q, want ACE (space separated)", q.AnswerText)
	}
}

func TestExtractLowercaseOptionLabels(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个是注册器？（E）
a. RegistryObject
b. EventBus
c. ModContainer
d. ItemStack
e. DeferredRegister`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}
	// Labels should be uppercase
	assertOption(t, options, "E", "DeferredRegister")
}

func TestExtractBracketAnswerWithChinesePunctuation(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 下列选项正确的是（A，C，E）
A. a选项
B. b选项
C. c选项
D. d选项
E. e选项`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "ACE" {
		t.Errorf("answer = %q, want ACE (Chinese comma separated)", q.AnswerText)
	}
}

func TestDoesNotRemoveInvalidBracketAnswerWithoutMatchingOption(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 下列说法正确的是（Z）
A. A选项
B. B选项
C. C选项
D. D选项
E. E选项`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	// Z is not one of the options → bracket should NOT be extracted as answer
	if q.AnswerText != "" {
		t.Errorf("answer = %q, want empty (Z does not match any option label)", q.AnswerText)
	}
	// Stem must still contain (Z); we must not silently delete bracket content
	if !strings.Contains(q.StemText, "（Z）") && !strings.Contains(q.StemText, "(Z)") {
		t.Errorf("stem = %q, should still contain （Z）", q.StemText)
	}
	// Options should still be parsed
	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d", len(options))
	}
}

func TestDoesNotSplitOptionContentOnEnglishPunctuation(t *testing.T) {
	svc := newTestExtractionService()
	text := `1. 以下哪个描述正确？（B）
A. Node.js is a JavaScript runtime.
B. Go. language example should stay in option B.
C. Java is also valid.
D. Python is popular.
E. Rust is memory safe.`

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "B" {
		t.Errorf("answer = %q, want B", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}

	// A content must not be cut at "e." in "Node.js is a JavaScript runtime."
	aOpt := getOption(options, "A")
	if aOpt == nil {
		t.Fatal("option A not found")
	}
	if !strings.Contains(aOpt.Content, "Node.js") {
		t.Errorf("A content = %q, should contain 'Node.js'", aOpt.Content)
	}

	// B content must not be cut at "o." in "Go. language example"
	bOpt := getOption(options, "B")
	if bOpt == nil {
		t.Fatal("option B not found")
	}
	if !strings.Contains(bOpt.Content, "Go. language") {
		t.Errorf("B content = %q, should contain 'Go. language'", bOpt.Content)
	}

	// No spurious labels from English punctuation
	for _, opt := range options {
		if opt.Label != "A" && opt.Label != "B" && opt.Label != "C" && opt.Label != "D" && opt.Label != "E" {
			t.Errorf("unexpected option label %q from English punctuation", opt.Label)
		}
	}
}

func TestInlineOptionsRequireBoundaryBeforeMarker(t *testing.T) {
	svc := newTestExtractionService()
	text := "1. 以下哪个描述正确？（E） A. Node.js runtime B. Go. language C. Java D. Python E. Rust"

	items, _, _ := svc.Extract(context.Background(), text, string(types.QuestionTypeShortAnswer), string(types.QuestionDifficultyMedium))
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	q := items[0]
	if q.AnswerText != "E" {
		t.Errorf("answer = %q, want E", q.AnswerText)
	}

	options := getOptionsFromBody(t, q.QuestionBody)
	if len(options) != 5 {
		t.Fatalf("expected 5 options, got %d: %v", len(options), optionLabels(options))
	}

	assertOption(t, options, "A", "Node.js runtime")
	assertOption(t, options, "B", "Go. language")
	assertOption(t, options, "C", "Java")
	assertOption(t, options, "D", "Python")
	assertOption(t, options, "E", "Rust")

	// A content must not be truncated at "e." in "Node.js"
	aOpt := getOption(options, "A")
	if aOpt == nil {
		t.Fatal("option A not found")
	}
	if !strings.Contains(aOpt.Content, "Node.js") {
		t.Errorf("A content = %q, should contain 'Node.js' (not cut at e.)", aOpt.Content)
	}

	// B content must not be truncated at "o." in "Go."
	bOpt := getOption(options, "B")
	if bOpt == nil {
		t.Fatal("option B not found")
	}
	if !strings.Contains(bOpt.Content, "Go. language") {
		t.Errorf("B content = %q, should contain 'Go. language' (not cut at o.)", bOpt.Content)
	}
}

func TestNormalizeMultiChoiceAnswer(t *testing.T) {
	tests := []struct{ in, want string }{
		{"A、C、E", "ACE"},
		{"A,C,E", "ACE"},
		{"A C E", "ACE"},
		{"a、c、e", "ACE"},
		{"B", "B"},
		{"A，C，E", "ACE"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeMultiChoiceAnswer(tt.in)
		if got != tt.want {
			t.Errorf("normalizeMultiChoiceAnswer(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsMultiChoiceAnswer(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"ACE", true},
		{"AC", true},
		{"A", false},
		{"", false},
		{"A C E", false}, // needs normalization first
	}

	for _, tt := range tests {
		got := isMultiChoiceAnswer(tt.in)
		if got != tt.want {
			t.Errorf("isMultiChoiceAnswer(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// --- helpers ---

func getOptionsFromBody(t *testing.T, body types.JSON) []types.QuestionOption {
	t.Helper()
	if len(body) == 0 {
		return nil
	}
	var cb types.ChoiceQuestionBody
	if err := json.Unmarshal([]byte(body), &cb); err != nil {
		t.Fatalf("failed to unmarshal question body: %v", err)
	}
	return cb.Options
}

func getOption(options []types.QuestionOption, label string) *types.QuestionOption {
	for i := range options {
		if options[i].Label == label {
			return &options[i]
		}
	}
	return nil
}

func optionLabels(options []types.QuestionOption) []string {
	labels := make([]string, len(options))
	for i, o := range options {
		labels[i] = o.Label
	}
	return labels
}

func assertOption(t *testing.T, options []types.QuestionOption, label, content string) {
	t.Helper()
	opt := getOption(options, label)
	if opt == nil {
		t.Errorf("option %s not found in options (labels: %v)", label, optionLabels(options))
		return
	}
	if !strings.Contains(opt.Content, content) {
		t.Errorf("option %s content = %q, should contain %q", label, opt.Content, content)
	}
}

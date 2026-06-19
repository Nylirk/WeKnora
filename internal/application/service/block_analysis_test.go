package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestGeneralPreset_BasicPartitioning(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 津液的功能是什么？\nA. 滋润濡养\nB. 化生血液\nC. 调节阴阳\nD. 以上都是\n答案：D"
	blocks, summary := svc.AnalyzeBlocks(text, strategy)

	if summary.TotalBlocks != 1 {
		t.Fatalf("expected 1 block, got %d", summary.TotalBlocks)
	}
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 1 {
		t.Errorf("expected question number 1, got %v", blocks[0].QuestionNumber)
	}
	if !strings.Contains(blocks[0].CurrentText, "津液的功能") {
		t.Errorf("block should contain stem text, got: %s", blocks[0].CurrentText)
	}
	if !strings.Contains(blocks[0].CurrentText, "答案：D") {
		t.Errorf("block should contain answer, got: %s", blocks[0].CurrentText)
	}
}

func TestGeneralPreset_MultipleQuestions(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n\n2. 第二题\n答案：B\n\n3. 第三题\n答案：C"
	_, summary := svc.AnalyzeBlocks(text, strategy)

	if summary.TotalBlocks != 3 {
		t.Fatalf("expected 3 blocks, got %d", summary.TotalBlocks)
	}
	if summary.QuestionNumbers != 3 {
		t.Errorf("expected 3 question numbers, got %d", summary.QuestionNumbers)
	}
}

func TestGeneralPreset_SectionHeadingAsTag(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "第七单元 六腑\n1. 胆的功能是什么？\n答案：贮藏和排泄胆汁"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	// Tags should contain the section heading
	hasSectionTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "第七单元") {
			hasSectionTag = true
		}
	}
	if !hasSectionTag {
		t.Errorf("expected section tag '第七单元 六腑' in block tags, got: %v", blocks[0].Tags)
	}
	// The section heading line should NOT be in the block text
	if strings.Contains(blocks[0].CurrentText, "第七单元") {
		t.Errorf("section heading should be removed from block text, got: %s", blocks[0].CurrentText)
	}
}

func TestGeneralPreset_QuestionTypeHeadingAsTag(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "A1型题\n1. 下列哪项不是津液的功能？\nA. 滋润\nB. 濡养\nC. 化生血液\nD. 运输废物\n答案：D"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	hasTypeTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "A1型题") || strings.Contains(tag, "A1") {
			hasTypeTag = true
		}
	}
	if !hasTypeTag {
		t.Errorf("expected question type tag 'A1型题' in block tags, got: %v", blocks[0].Tags)
	}
	if strings.Contains(blocks[0].CurrentText, "A1型题") {
		t.Errorf("question type heading should be removed from block text")
	}
}

func TestGeneralPreset_BracketedQuestionTypeHeading(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "【A2型题】\n1. 患者近期出现...\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	hasTypeTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "A2型题") {
			hasTypeTag = true
		}
	}
	if !hasTypeTag {
		t.Errorf("expected tag containing 'A2型题', got: %v", blocks[0].Tags)
	}
}

func TestGeneralPreset_PageNumberRemoval(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n第20页，共46页\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	for _, b := range blocks {
		if strings.Contains(b.CurrentText, "第20页") || strings.Contains(b.CurrentText, "共46页") {
			t.Errorf("page number should be removed from block %d: %s", b.Index, b.CurrentText)
		}
	}
}

func TestGeneralPreset_EnglishPageNumberRemoval(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. Question one\nAnswer: A\nPage 20 of 46\n2. Question two\nAnswer: B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	for _, b := range blocks {
		if strings.Contains(strings.ToLower(b.CurrentText), "page") {
			t.Errorf("page number should be removed: %s", b.CurrentText)
		}
	}
}

func TestGeneralPreset_LeadingNoiseRemoval(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// Simulate D/E option debris from previous page
	text := "D. 寒邪\nE. 湿邪\n1. 津液的功能是什么？\n答案：A"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	if strings.Contains(blocks[0].CurrentText, "寒邪") {
		t.Errorf("leading option debris should be removed: %s", blocks[0].CurrentText)
	}
	if !strings.Contains(blocks[0].CurrentText, "津液") {
		t.Errorf("actual question content should be preserved: %s", blocks[0].CurrentText)
	}
}

func TestPDFPreset_MultiQuestionPerLine(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	text := "243. 六腑的功能特点 249. 津液输布的主要途径"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	// Should split into at least 2 blocks
	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 blocks from multi-question line, got %d", len(blocks))
	}
	found243 := false
	found249 := false
	for _, b := range blocks {
		if b.QuestionNumber != nil {
			if *b.QuestionNumber == 243 {
				found243 = true
			}
			if *b.QuestionNumber == 249 {
				found249 = true
			}
		}
	}
	if !found243 || !found249 {
		t.Errorf("expected question numbers 243 and 249, blocks: %+v", blocks)
	}
}

func TestPDFPreset_BareNumberLine(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	text := "249 津液输布的主要途径是\nA. 三焦\nB. 经络\nC. 脏腑\nD. 以上都是\n答案：D"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 249 {
		t.Errorf("expected question number 249 from bare number line, got %v", blocks[0].QuestionNumber)
	}
	if !strings.Contains(blocks[0].CurrentText, "津液输布") {
		t.Errorf("block should contain question text after number repair")
	}
}

func TestPDFPreset_AutoSort(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	text := "3. 第三题\n答案：C\n1. 第一题\n答案：A\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	// After sort, should be 1, 2, 3
	for i, expected := range []int{1, 2, 3} {
		if blocks[i].QuestionNumber == nil || *blocks[i].QuestionNumber != expected {
			t.Errorf("block %d: expected question %d, got %v", i, expected, blocks[i].QuestionNumber)
		}
	}
}

func TestGeneralPreset_NoSort(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "3. 第三题\n答案：C\n1. 第一题\n答案：A\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	// General preset does NOT sort — maintain original order
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	// Order should be as in the document: 3, 1, 2
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 3 {
		t.Errorf("general preset: first block should be 3 (original order), got %v", blocks[0].QuestionNumber)
	}
}

func TestDuplicateQuestionNumber(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n1. 重复题号\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	foundDup := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyDuplicateQuestionNumber {
				foundDup = true
			}
		}
	}
	if !foundDup {
		t.Error("expected DUPLICATE_QUESTION_NUMBER anomaly")
	}
}

func TestQuestionNumberGap(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n5. 第五题（中间有遗漏）\n答案：E"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	foundGap := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyQuestionNumberGap {
				foundGap = true
			}
		}
	}
	if !foundGap {
		t.Error("expected QUESTION_NUMBER_GAP anomaly")
	}
}

func TestNonMonotonicQuestionNumber(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "5. 第五题\n答案：E\n3. 第三题\n答案：C"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	foundNonMon := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyNonMonotonicQuestionNum {
				foundNonMon = true
			}
		}
	}
	if !foundNonMon {
		t.Error("expected NON_MONOTONIC_QUESTION_NUMBER anomaly")
	}
}

func TestOptionOnlyBlock(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "A. 选项A\nB. 选项B\nC. 选项C\nD. 选项D"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block (options without question)")
	}
	foundOptOnly := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyOptionOnlyBlock {
				foundOptOnly = true
			}
		}
	}
	if !foundOptOnly {
		t.Error("expected OPTION_ONLY_BLOCK anomaly")
	}
}

func TestOptionSequenceRestart(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 某题\nA. 选项\nB. 选项\nA. 选项重新开始\nB. 选项"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	foundRestart := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyOptionSequenceRestart {
				foundRestart = true
			}
		}
	}
	if !foundRestart {
		t.Error("expected OPTION_SEQUENCE_RESTART anomaly")
	}
}

func TestFalsePositive_10mg(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "10mg" should NOT be recognized as question 10
	text := "1. 某药物的剂量是10mg，每日两次\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (10mg should NOT split into a new question)", len(blocks))
	}
}

func TestFalsePositive_Year(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "2024年" should NOT be recognized as question 2024
	text := "1. 该政策于2024年实施\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (2024年 should NOT be a question start)", len(blocks))
	}
}

func TestFalsePositive_DecimalHours(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "1.5小时" should NOT be recognized as question 1.5
	text := "1. 操作耗时约1.5小时\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (1.5小时 should NOT split)", len(blocks))
	}
}

func TestFalsePositive_ChapterHeading(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "第2章" should be extracted as tag, NOT as question 2
	text := "第2章 基础理论\n1. 第一题\n答案：A"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (第2章 should be tag not question)", len(blocks))
	}
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 1 {
		t.Errorf("expected first question to be 1, got %v", blocks[0].QuestionNumber)
	}
	hasChapterTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "第2章") {
			hasChapterTag = true
		}
	}
	if !hasChapterTag {
		t.Errorf("expected '第2章 基础理论' in tags, got: %v", blocks[0].Tags)
	}
}

func TestMissingAnswerAnomaly(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\nA. 选项A\nB. 选项B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	foundMissingAnswer := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Type == types.AnomalyMissingAnswer {
				foundMissingAnswer = true
			}
		}
	}
	if !foundMissingAnswer {
		t.Error("expected MISSING_ANSWER anomaly on last block without answer")
	}
}

func TestStemTooShort(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. X"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected 1 block")
	}
	foundShort := false
	for _, a := range blocks[0].Anomalies {
		if a.Type == types.AnomalyStemTooShort {
			foundShort = true
		}
	}
	if !foundShort {
		t.Error("expected STEM_TOO_SHORT anomaly for very short block")
	}
}

func TestChineseQuestionNumbers(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "一、津液的功能\n答案：滋润濡养\n二、六腑的功能\n答案：传化水谷"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks from Chinese numerals, got %d", len(blocks))
	}
}

func TestBracketedQuestionNumbers(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "（1）津液的功能\n答案：滋润\n（2）六腑的功能\n答案：传化"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks from （N） format, got %d", len(blocks))
	}
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 1 {
		t.Errorf("expected question 1, got %v", blocks[0].QuestionNumber)
	}
}

func TestEmptyText(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	blocks, summary := svc.AnalyzeBlocks("", strategy)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for empty text, got %d", len(blocks))
	}
	if summary.TotalBlocks != 0 {
		t.Errorf("expected 0 total blocks in summary")
	}
}

func TestWhitespaceOnlyText(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	blocks, _ := svc.AnalyzeBlocks("   \n\n  \n", strategy)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for whitespace text, got %d", len(blocks))
	}
}

func TestBlockInternalMultiQuestionSplitting(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// A block that somehow contains two question markers (e.g. 1. and 2.)
	text := "1. 第一题\n答案：A\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks split at internal question markers, got %d", len(blocks))
	}
}

func TestMultipleAnomalyTypes(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "5. 第五题\n答案：E\n3. 第三题\n答案：C"
	_, summary := svc.AnalyzeBlocks(text, strategy)

	// Should have gap (1→5) and non-monotonic (5→3) anomalies
	if summary.BlocksWithAnomalies == 0 {
		t.Error("expected blocks with anomalies")
	}
	if summary.AnomalyBreakdown[types.AnomalyQuestionNumberGap] == 0 {
		t.Error("expected gap anomaly")
	}
	if summary.AnomalyBreakdown[types.AnomalyNonMonotonicQuestionNum] == 0 {
		t.Error("expected non-monotonic anomaly")
	}
}

func TestBTypeQuestionTag(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "B型题\n1. 备选答案匹配题\n答案：见解析"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	hasBTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "B型题") {
			hasBTag = true
		}
	}
	if !hasBTag {
		t.Errorf("expected 'B型题' in tags, got: %v", blocks[0].Tags)
	}
}

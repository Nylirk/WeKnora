package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// ===== Basic General Preset =====

func TestGeneralPreset_BasicPartitioning(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 津液的功能是什么？\nA. 滋润濡养\nB. 化生血液\nC. 调节阴阳\nD. 以上都是\n答案：D"
	blocks, summary := svc.AnalyzeBlocks(text, strategy)

	if summary.TotalBlocks != 1 {
		t.Fatalf("expected 1 block, got %d", summary.TotalBlocks)
	}
	b := blocks[0]
	if b.QuestionNumber == nil || *b.QuestionNumber != 1 {
		t.Errorf("expected question number 1, got %v", b.QuestionNumber)
	}
	if !strings.Contains(b.CurrentText, "津液的功能") {
		t.Errorf("block should contain stem text, got: %s", b.CurrentText)
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

// ===== Tag extraction (standalone lines only) =====

func TestGeneralPreset_SectionHeadingAsTag(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "第七单元 六腑\n1. 胆的功能是什么？\n答案：贮藏和排泄胆汁"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	hasSectionTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "第七单元") {
			hasSectionTag = true
		}
	}
	if !hasSectionTag {
		t.Errorf("expected section tag '第七单元 六腑' in block tags, got: %v", blocks[0].Tags)
	}
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
		if strings.Contains(tag, "A1型题") {
			hasTypeTag = true
		}
	}
	if !hasTypeTag {
		t.Errorf("expected question type tag 'A1型题' in block tags, got: %v", blocks[0].Tags)
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

// ===== Standalone heading guard =====

func TestSectionHeading_NotFromOptionLine(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "D.肺肾 第七单元 六腑" should NOT be treated as a section tag
	text := "1. 某题\nD.肺肾 第七单元 六腑\nE.肝肾\n答案：D"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	// Option line containing section text should not be extracted as tag
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "第七单元") {
			t.Errorf("option line 'D.肺肾 第七单元 六腑' should NOT produce section tag, got: %v", blocks[0].Tags)
		}
	}
}

func TestStandaloneSectionHeading_StillWorks(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "第七单元 六腑\n1. 第一题\n答案：A"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	hasTag := false
	for _, tag := range blocks[0].Tags {
		if strings.Contains(tag, "第七单元") {
			hasTag = true
		}
	}
	if !hasTag {
		t.Errorf("standalone '第七单元 六腑' should be extracted as tag, got: %v", blocks[0].Tags)
	}
}

// ===== Page number removal =====

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

// ===== Leading noise removal (all blocks, not just first) =====

func TestGeneralPreset_LeadingNoiseRemoval(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "D. 寒邪\nE. 湿邪\n1. 津液的功能是什么？\n答案：A"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	if strings.Contains(blocks[0].CurrentText, "寒邪") || strings.Contains(blocks[0].CurrentText, "湿邪") {
		t.Errorf("leading option debris should be removed: %s", blocks[0].CurrentText)
	}
	if !strings.Contains(blocks[0].CurrentText, "津液") {
		t.Errorf("actual question content should be preserved: %s", blocks[0].CurrentText)
	}
}

// ===== PDF: multi-question per line with bare number =====

func TestPDFPreset_MultiQuestionPerLine_StrongMarker(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	text := "243. 六腑的功能特点 249. 津液输布的主要途径"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

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
		t.Errorf("expected question numbers 243 and 249")
	}
}

func TestPDFPreset_MultiQuestionPerLine_EmbeddedBareNumber(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	// "243.主睡的是(E) 249 津液输布的主要通道是(D)"
	text := "243.主睡的是(E) 249 津液输布的主要通道是(D)"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 blocks from embedded bare number, got %d", len(blocks))
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
		t.Errorf("expected question numbers 243 and 249 from '243.主睡的是(E) 249 津液...'")
	}
}

// ===== PDF bare number line =====

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
}

// ===== General preset does NOT recognize bare numbers =====

func TestGeneralPreset_NoBareNumber(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// "249 津液..." should NOT be treated as a question start in general preset
	text := "249 津液输布的主要途径是"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	// In general preset, bare numbers are not question starts
	// This should produce 1 block with no question number
	if len(blocks) > 1 {
		t.Errorf("general preset should not split on bare numbers, got %d blocks", len(blocks))
	}
	if len(blocks) == 1 && blocks[0].QuestionNumber != nil {
		t.Errorf("general preset should NOT assign question number to bare number line, got %v", blocks[0].QuestionNumber)
	}
}

// ===== Sort behavior =====

func TestPDFPreset_AutoSort(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.PDFBlockParseStrategy()

	text := "3. 第三题\n答案：C\n1. 第一题\n答案：A\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
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

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	if blocks[0].QuestionNumber == nil || *blocks[0].QuestionNumber != 3 {
		t.Errorf("general preset: first block should be 3 (original order), got %v", blocks[0].QuestionNumber)
	}
}

// ===== Anomalies =====

func TestDuplicateQuestionNumber(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n1. 重复题号\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	foundDup := false
	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Code == types.AnomalyDuplicateQuestionNumber {
				foundDup = true
				if a.Severity != types.SeverityError {
					t.Errorf("DUPLICATE_QUESTION_NUMBER should be error, got %s", a.Severity)
				}
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
			if a.Code == types.AnomalyQuestionNumberGap {
				foundGap = true
				if a.Severity != types.SeverityWarning {
					t.Errorf("QUESTION_NUMBER_GAP should be warning, got %s", a.Severity)
				}
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
			if a.Code == types.AnomalyNonMonotonicQuestionNum {
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
			if a.Code == types.AnomalyOptionOnlyBlock {
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
			if a.Code == types.AnomalyOptionSequenceRestart {
				foundRestart = true
			}
		}
	}
	if !foundRestart {
		t.Error("expected OPTION_SEQUENCE_RESTART anomaly")
	}
}

func TestMissingAnswer_PerBlock(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题（无答案）\nA. 选项A\nB. 选项B\n2. 第二题（也无答案）\nC. 选项C\nD. 选项D"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(blocks))
	}
	for i, b := range blocks {
		hasMissingAnswer := false
		for _, a := range b.Anomalies {
			if a.Code == types.AnomalyMissingAnswer {
				hasMissingAnswer = true
			}
		}
		if !hasMissingAnswer {
			t.Errorf("block %d should have MISSING_ANSWER anomaly", i)
		}
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
		if a.Code == types.AnomalyStemTooShort {
			foundShort = true
		}
	}
	if !foundShort {
		t.Error("expected STEM_TOO_SHORT anomaly for very short block")
	}
}

func TestMultipleAnomalyTypes(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "5. 第五题\n答案：E\n3. 第三题\n答案：C"
	_, summary := svc.AnalyzeBlocks(text, strategy)

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

// ===== False positives =====

func TestFalsePositive_10mg(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 某药物的剂量是10mg，每日两次\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (10mg should NOT split)", len(blocks))
	}
}

func TestFalsePositive_Year(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 该政策于2024年实施\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (2024年 should NOT be a question start)", len(blocks))
	}
}

func TestFalsePositive_DecimalHours(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 操作耗时约1.5小时\n答案：正确"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (1.5小时 should NOT split)", len(blocks))
	}
}

func TestFalsePositive_ChapterHeading(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

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

func TestFalsePositive_Page20(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 参看第20页内容\n答案：A"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d (第20页 should NOT split)", len(blocks))
	}
}

// ===== Chinese / bracketed question numbers =====

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

// ===== Edge cases =====

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

	text := "1. 第一题\n答案：A\n2. 第二题\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks split at internal question markers, got %d", len(blocks))
	}
}

// ===== Anomaly severity check =====

func TestAnomalySeverity(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 第一题\n答案：A\n1. 重复题号\n答案：B"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	for _, b := range blocks {
		for _, a := range b.Anomalies {
			if a.Code == "" {
				t.Error("anomaly Code must not be empty")
			}
			if a.Severity == "" {
				t.Errorf("anomaly %s must have severity", a.Code)
			}
			if a.Severity != types.SeverityError && a.Severity != types.SeverityWarning && a.Severity != types.SeverityInfo {
				t.Errorf("anomaly %s has invalid severity: %s", a.Code, a.Severity)
			}
		}
	}
}

// ===== Answer detection =====

func TestAnswerOutOfOptions(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	// Options A-D, but answer says (F)
	text := "1. 某题\nA. 选项A\nB. 选项B\nC. 选项C\nD. 选项D\n答案：（F）"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	found := false
	for _, a := range blocks[0].Anomalies {
		if a.Code == types.AnomalyAnswerOutOfOptions {
			found = true
			if a.Severity != types.SeverityError {
				t.Errorf("ANSWER_OUT_OF_OPTIONS should be error, got %s", a.Severity)
			}
		}
	}
	if !found {
		t.Error("expected ANSWER_OUT_OF_OPTIONS anomaly when answer (F) exceeds options A-D")
	}
}

func TestAnswerAnalysisMixed(t *testing.T) {
	svc := NewBlockAnalysisService()
	strategy := types.GeneralBlockParseStrategy()

	text := "1. 某题\nA. 选项A\nB. 选项B\n答案：B\n解析很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长的内容"
	blocks, _ := svc.AnalyzeBlocks(text, strategy)

	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
	found := false
	for _, a := range blocks[0].Anomalies {
		if a.Code == types.AnomalyAnswerAnalysisMixed {
			found = true
		}
	}
	if !found {
		t.Error("expected ANSWER_ANALYSIS_MIXED anomaly")
	}
}

// ===== Strategy preset validation =====

func TestValidateStrategyPreset_Valid(t *testing.T) {
	for _, preset := range []string{"", "general", "pdf"} {
		if err := types.ValidateStrategyPreset(preset); err != nil {
			t.Errorf("ValidateStrategyPreset(%q) should succeed, got: %v", preset, err)
		}
	}
}

func TestValidateStrategyPreset_Invalid(t *testing.T) {
	for _, preset := range []string{"xxx", "invalid", "PDF", "GENERAL", "docx"} {
		if err := types.ValidateStrategyPreset(preset); err == nil {
			t.Errorf("ValidateStrategyPreset(%q) should fail", preset)
		}
	}
}

func TestValidateImportMode_Valid(t *testing.T) {
	for _, mode := range []string{"", "single", "batch"} {
		if _, err := types.ValidateImportMode(mode); err != nil {
			t.Errorf("ValidateImportMode(%q) should succeed, got: %v", mode, err)
		}
	}
}

func TestValidateImportMode_Invalid(t *testing.T) {
	for _, mode := range []string{"xxx", "BATCH", "Single"} {
		if _, err := types.ValidateImportMode(mode); err == nil {
			t.Errorf("ValidateImportMode(%q) should fail", mode)
		}
	}
}

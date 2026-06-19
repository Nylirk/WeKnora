package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

func TestCreateDebugExport_Success(t *testing.T) {
	text := "1. test question\nA. option a\nB. option b\n答案：B"
	defaultType := string(types.QuestionTypeShortAnswer)
	defaultDifficulty := string(types.QuestionDifficultyMedium)
	items := []types.ImportQuestionItem{
		{
			LineNumber:   1,
			QuestionType: string(types.QuestionTypeSingleChoice),
			StemText:     "test question",
			AnswerText:   "B",
			Difficulty:   defaultDifficulty,
		},
	}
	var parseErrors []types.ImportQuestionError
	var parseWarnings []string

	debugDir, zipPath, manifest, err := createDebugExport(
		text, defaultType, defaultDifficulty,
		items, parseErrors, parseWarnings,
	)
	if err != nil {
		t.Fatalf("createDebugExport failed: %v", err)
	}
	defer CleanupDebugExport(context.Background(), debugDir, manifest)

	if debugDir == "" {
		t.Fatal("debugDir is empty")
	}
	if zipPath == "" {
		t.Fatal("zipPath is empty")
	}
	if len(manifest) < 7 { // 6 debug files + 1 zip
		t.Fatalf("expected at least 7 manifest entries, got %d", len(manifest))
	}

	// Verify the zip exists and is readable
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("zip file not found: %v", err)
	}

	// Verify each debug file exists
	expectedFiles := []string{
		"01_extracted.md", "02_normalized.md", "03_lines.txt",
		"04_blocks.txt", "05_items.json", "06_summary.json",
	}
	for _, name := range expectedFiles {
		path := filepath.Join(debugDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected debug file %s not found: %v", name, err)
		}
	}
}

func TestCreateDebugExport_EmptyText(t *testing.T) {
	debugDir, _, manifest, err := createDebugExport(
		"", "", "",
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("createDebugExport failed on empty text: %v", err)
	}
	defer CleanupDebugExport(context.Background(), debugDir, manifest)

	// Verify summary exists and contains expected counts.
	// normalizeText("") → "", splitAndCleanLines("") → [""] (1 empty line)
	summaryPath := filepath.Join(debugDir, "06_summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("cannot read summary: %v", err)
	}
	content := string(data)
	for _, want := range []string{`"extracted_len": 0`, `"block_count": 0`, `"item_count": 0`} {
		if !containsStr(content, want) {
			t.Errorf("summary missing expected key %q: %s", want, content)
		}
	}
}

func TestCleanupDebugExport_ManifestDeletion(t *testing.T) {
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug-test-1")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	defer os.RemoveAll(debugDir) // fallback cleanup

	var manifest []string
	for _, entry := range []struct{ name, ext string }{
		{"01_extracted.md", ".md"},
		{"02_normalized.md", ".md"},
		{"03_lines.txt", ".txt"},
		{"05_items.json", ".json"},
		{"debug-export.zip", ".zip"},
	} {
		path := filepath.Join(debugDir, entry.name)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		manifest = append(manifest, path)
	}

	// Also write a manifest file we track
	CleanupDebugExport(context.Background(), debugDir, manifest)

	// All files should be gone
	for _, fp := range manifest {
		if _, err := os.Stat(fp); !os.IsNotExist(err) {
			t.Errorf("file still exists after cleanup: %s", fp)
		}
	}
	// Directory should be gone
	if _, err := os.Stat(debugDir); !os.IsNotExist(err) {
		t.Errorf("directory still exists after cleanup: %s", debugDir)
	}
}

func TestCleanupDebugExport_PathTraversalRejected(t *testing.T) {
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug-test-2")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	defer os.RemoveAll(debugDir) // fallback cleanup

	// Create a legitimate file
	goodFile := filepath.Join(debugDir, "01_extracted.md")
	if err := os.WriteFile(goodFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Create a file outside debug dir that we must NOT delete
	outsideFile := filepath.Join(os.TempDir(), "weknora-debug-test-do-not-delete.txt")
	if err := os.WriteFile(outsideFile, []byte("do not delete"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	defer os.Remove(outsideFile)

	// Manifest includes both the legitimate file and the outside file
	manifest := []string{goodFile, outsideFile}

	CleanupDebugExport(context.Background(), debugDir, manifest)

	// Legitimate file should be gone
	if _, err := os.Stat(goodFile); !os.IsNotExist(err) {
		t.Errorf("legitimate file not cleaned up: %s", goodFile)
	}
	// Outside file should still exist
	if _, err := os.Stat(outsideFile); os.IsNotExist(err) {
		t.Errorf("outside file was incorrectly deleted: %s", outsideFile)
	}
}

func TestCleanupDebugExport_ExtensionWhitelist(t *testing.T) {
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug-test-3")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	defer os.RemoveAll(debugDir) // fallback cleanup

	goodFile := filepath.Join(debugDir, "01_extracted.md")
	badFile := filepath.Join(debugDir, "malicious.exe")

	if err := os.WriteFile(goodFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := os.WriteFile(badFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	manifest := []string{goodFile, badFile}
	CleanupDebugExport(context.Background(), debugDir, manifest)

	// Good file should be gone
	if _, err := os.Stat(goodFile); !os.IsNotExist(err) {
		t.Errorf("whitelisted .md file not cleaned up: %s", goodFile)
	}
	// Bad file should still exist
	if _, err := os.Stat(badFile); os.IsNotExist(err) {
		t.Errorf("non-whitelisted .exe file was incorrectly deleted: %s", badFile)
	}
}

func TestCleanupDebugExport_DirNotRemovedWhenNotEmpty(t *testing.T) {
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug-test-4")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	defer os.RemoveAll(debugDir) // fallback cleanup

	goodFile := filepath.Join(debugDir, "01_extracted.md")
	if err := os.WriteFile(goodFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// Create an orphan file NOT in the manifest
	orphanFile := filepath.Join(debugDir, "orphan.log")
	if err := os.WriteFile(orphanFile, []byte("orphan"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	manifest := []string{goodFile}
	CleanupDebugExport(context.Background(), debugDir, manifest)

	// Manifested file should be gone
	if _, err := os.Stat(goodFile); !os.IsNotExist(err) {
		t.Errorf("manifested file not cleaned up: %s", goodFile)
	}
	// Directory should still exist (orphan file remains)
	if _, err := os.Stat(debugDir); os.IsNotExist(err) {
		t.Errorf("directory should not have been removed (orphan file present)")
	}
}

func TestCleanupDebugExport_SafePathUnderBase_RejectsRelativeTraversal(t *testing.T) {
	debugDir := os.TempDir()
	// Try to sneak past SafePathUnderBase with a path that resolves outside
	traversalPath := filepath.Join(debugDir, "..", "..", "etc", "passwd")
	_, err := secutils.SafePathUnderBase(debugDir, traversalPath)
	if err == nil {
		t.Fatal("SafePathUnderBase should reject relative traversal")
	}
}

func TestCleanupDebugExport_NilManifest(t *testing.T) {
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug-test-5")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	defer os.RemoveAll(debugDir) // fallback cleanup

	// Should not panic
	CleanupDebugExport(context.Background(), debugDir, nil)
	// Should not panic (empty manifest)
	CleanupDebugExport(context.Background(), debugDir, []string{})
}

func TestCleanupDebugExport_EmptyDebugDir(t *testing.T) {
	// Should not panic
	CleanupDebugExport(context.Background(), "", nil)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

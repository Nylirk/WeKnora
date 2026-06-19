package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// createDebugExport captures every intermediate stage of the question extraction
// pipeline into files under os.TempDir()/weknora-question-import-debug/<uuid>/,
// zips them, and returns the temp dir path, zip path, and a manifest of created files.
//
// The caller is responsible for calling CleanupDebugExport after the zip has been
// read. On any error, already-created files are cleaned up before returning.
func createDebugExport(
	extractedText string,
	defaultType string,
	defaultDifficulty string,
	items []types.ImportQuestionItem,
	parseErrors []types.ImportQuestionError,
	parseWarnings []string,
) (debugDir string, zipPath string, manifest []string, err error) {
	requestID := uuid.New().String()
	debugDir = filepath.Join(os.TempDir(), "weknora-question-import-debug", requestID)

	if err := os.MkdirAll(debugDir, 0700); err != nil {
		return "", "", nil, fmt.Errorf("failed to create debug dir: %w", err)
	}

	// Deferred cleanup on any error after directory creation.
	defer func() {
		if err != nil {
			cleanupManifest(debugDir, manifest)
		}
	}()

	// --- Build pipeline intermediates ---
	normalized := normalizeText(extractedText)
	lines := splitAndCleanLines(normalized)
	blocks := partitionIntoBlocks(lines)

	// 01_extracted.md
	p01, err := writeDebugFile(debugDir, "01_extracted.md", []byte(extractedText))
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p01)

	// 02_normalized.md
	p02, err := writeDebugFile(debugDir, "02_normalized.md", []byte(normalized))
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p02)

	// 03_lines.txt — each line prefixed with its 1-based index
	var linesBuf []byte
	for i, ln := range lines {
		linesBuf = fmt.Appendf(linesBuf, "%6d  %s\n", i+1, ln)
	}
	p03, err := writeDebugFile(debugDir, "03_lines.txt", linesBuf)
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p03)

	// 04_blocks.txt — each block preceded by a header
	var blocksBuf []byte
	for i, b := range blocks {
		blocksBuf = fmt.Appendf(blocksBuf, "=== Block %d ===\n", i+1)
		for _, ln := range b {
			blocksBuf = append(blocksBuf, ln...)
			blocksBuf = append(blocksBuf, '\n')
		}
		blocksBuf = append(blocksBuf, '\n')
	}
	p04, err := writeDebugFile(debugDir, "04_blocks.txt", blocksBuf)
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p04)

	// 05_items.json — extracted items, errors, and warnings
	itemsJSON, err := json.MarshalIndent(map[string]interface{}{
		"items":    items,
		"errors":   parseErrors,
		"warnings": parseWarnings,
	}, "", "  ")
	if err != nil {
		return debugDir, "", manifest, fmt.Errorf("failed to marshal items: %w", err)
	}
	p05, err := writeDebugFile(debugDir, "05_items.json", itemsJSON)
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p05)

	// 06_summary.json — pipeline metadata
	summaryJSON, err := json.MarshalIndent(map[string]interface{}{
		"default_type":       defaultType,
		"default_difficulty": defaultDifficulty,
		"extracted_len":      len(extractedText),
		"normalized_len":     len(normalized),
		"line_count":         len(lines),
		"block_count":        len(blocks),
		"item_count":         len(items),
		"error_count":        len(parseErrors),
		"warning_count":      len(parseWarnings),
	}, "", "  ")
	if err != nil {
		return debugDir, "", manifest, fmt.Errorf("failed to marshal summary: %w", err)
	}
	p06, err := writeDebugFile(debugDir, "06_summary.json", summaryJSON)
	if err != nil {
		return debugDir, "", manifest, err
	}
	manifest = append(manifest, p06)

	// --- Zip everything ---
	zipPath = filepath.Join(debugDir, "debug-export.zip")
	if err := createZip(zipPath, debugDir, manifest); err != nil {
		return debugDir, "", manifest, fmt.Errorf("failed to create zip: %w", err)
	}
	manifest = append(manifest, zipPath)

	return debugDir, zipPath, manifest, nil
}

// writeDebugFile writes content to a file whose name has been sanitized
// via SafeFileName and whose path is verified to be under debugDir.
func writeDebugFile(debugDir, name string, content []byte) (string, error) {
	safeName, err := secutils.SafeFileName(name)
	if err != nil {
		return "", fmt.Errorf("unsafe file name %q: %w", name, err)
	}
	path, err := secutils.SafePathUnderBase(debugDir, filepath.Join(debugDir, safeName))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// createZip creates a zip archive at zipPath containing every file in filePaths,
// stored relative to baseDir. A zip-slip guard rejects paths that escape baseDir.
func createZip(zipPath, baseDir string, filePaths []string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, fp := range filePaths {
		rel, err := filepath.Rel(baseDir, fp)
		if err != nil {
			return fmt.Errorf("zip: cannot compute relative path for %s: %w", fp, err)
		}
		// zip-slip guard: reject paths that escape
		if len(rel) >= 2 && rel[:2] == ".." {
			return fmt.Errorf("zip: path traversal detected: %s", rel)
		}
		if filepath.IsAbs(rel) || rel[0] == '/' {
			return fmt.Errorf("zip: absolute path rejected: %s", rel)
		}

		data, err := os.ReadFile(fp)
		if err != nil {
			return fmt.Errorf("zip: cannot read %s: %w", fp, err)
		}

		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// CleanupDebugExport safely deletes files recorded in the manifest, then removes
// the debug directory if it is empty. Only files with extensions in the whitelist
// (.md, .txt, .json, .zip) and verified to be under debugDir are deleted.
//
// All errors are logged; none are returned. This function MUST NOT be called with
// a debugDir derived from user input.
func CleanupDebugExport(ctx context.Context, debugDir string, manifest []string) {
	if debugDir == "" {
		return
	}
	log := logger.GetLogger(ctx)

	for _, fp := range manifest {
		if fp == "" {
			continue
		}
		safePath, err := secutils.SafePathUnderBase(debugDir, fp)
		if err != nil {
			log.Warnf("[debug-export cleanup] path traversal rejected: %s err=%v", fp, err)
			continue
		}
		ext := filepath.Ext(safePath)
		switch ext {
		case ".md", ".txt", ".json", ".zip":
			if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) {
				log.Warnf("[debug-export cleanup] failed to remove %s: %v", safePath, err)
			}
		default:
			log.Warnf("[debug-export cleanup] extension not whitelisted, skipping: %s (ext=%s)", safePath, ext)
		}
	}

	// Remove the directory only if it is now empty.
	if err := os.Remove(debugDir); err != nil && !os.IsNotExist(err) {
		log.Warnf("[debug-export cleanup] failed to remove directory %s: %v", debugDir, err)
	}
}

// cleanupManifest is a non-logging variant used by createDebugExport on partial
// failure; it does the same safe deletion but silently ignores errors.
func cleanupManifest(debugDir string, manifest []string) {
	for _, fp := range manifest {
		if fp == "" {
			continue
		}
		safePath, err := secutils.SafePathUnderBase(debugDir, fp)
		if err != nil {
			continue
		}
		ext := filepath.Ext(safePath)
		switch ext {
		case ".md", ".txt", ".json", ".zip":
			_ = os.Remove(safePath)
		}
	}
	_ = os.Remove(debugDir)
}

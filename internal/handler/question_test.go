package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	servicepkg "github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// stubQuestionServiceForDebugExport embeds the real interface and overrides
// only PreviewImportQuestionsFromFile; any unexpected method call panics.
type stubQuestionServiceForDebugExport struct {
	interfaces.QuestionService
	previewFn func(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error)
}

func (s *stubQuestionServiceForDebugExport) PreviewImportQuestionsFromFile(
	ctx context.Context, kbID, setID string,
	fileData []byte, fileName string,
	req *types.ImportFilePreviewRequest,
) (*types.ImportFilePreviewResponse, error) {
	return s.previewFn(ctx, kbID, setID, fileData, fileName, req)
}

// newQuestionTestRouter creates a gin engine with the preview route.
// Does NOT set gin mode — the caller controls that.
func newQuestionTestRouter(svc interfaces.QuestionService) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), middleware.ErrorHandler())
	h := NewQuestionHandler(svc)
	r.POST("/api/v1/knowledge-bases/:id/question-sets/:set_id/questions/import-file/preview", h.PreviewImportQuestionsFromFile)
	return r
}

// createMultipartRequest creates a multipart/form-data request with a file field.
func createMultipartRequest(t *testing.T, url string, fileName string, fileContent []byte, queryParams map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())

	q := req.URL.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return req
}

func TestPreviewImportDebugExport_ReleaseModeRejects(t *testing.T) {
	// Set release mode explicitly, reset to test mode after
	gin.SetMode(gin.ReleaseMode)
	defer gin.SetMode(gin.TestMode)

	stub := &stubQuestionServiceForDebugExport{
		previewFn: func(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error) {
			t.Fatal("service must not be called in release mode with debug_export=1")
			return nil, nil
		},
	}

	router := newQuestionTestRouter(stub)
	req := createMultipartRequest(t,
		"/api/v1/knowledge-bases/kb-1/question-sets/set-1/questions/import-file/preview",
		"test.docx", []byte("dummy content"),
		map[string]string{"debug_export": "1"},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 in release mode, got %d: body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "debug_export") {
		t.Errorf("response should mention debug_export: %s", rec.Body.String())
	}
}

func TestPreviewImportDebugExport_DebugModeServesZip(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	// Create a real debug export temp directory
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug", "handler-test")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	// Write debug files and a real zip
	var manifest []string
	for _, entry := range []struct{ name, content string }{
		{"01_extracted.md", "# extracted text"},
		{"02_normalized.md", "# normalized"},
		{"03_lines.txt", "1  line one"},
		{"04_blocks.txt", "=== Block 1 ==="},
		{"05_items.json", `{"items":[]}`},
		{"06_summary.json", `{"item_count": 0}`},
	} {
		path := filepath.Join(debugDir, entry.name)
		if err := os.WriteFile(path, []byte(entry.content), 0644); err != nil {
			t.Fatalf("write %s: %v", entry.name, err)
		}
		manifest = append(manifest, path)
	}

	// Create zip
	zipPath := filepath.Join(debugDir, "debug-export.zip")
	if err := createTestZip(zipPath, debugDir, manifest); err != nil {
		t.Fatalf("create test zip: %v", err)
	}
	manifest = append(manifest, zipPath)

	// Deferred cleanup in case test fails before handler cleanup runs
	defer servicepkg.CleanupDebugExport(context.Background(), debugDir, manifest)

	stub := &stubQuestionServiceForDebugExport{
		previewFn: func(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error) {
			return &types.ImportFilePreviewResponse{
				Items:          []types.ImportQuestionItem{},
				Errors:         []types.ImportQuestionError{},
				Warnings:       []string{},
				RawTextPreview: "",
				Stats:          types.ImportFilePreviewStats{},
				DebugExportPath: debugDir,
				DebugManifest:   manifest,
			}, nil
		},
	}

	router := newQuestionTestRouter(stub)
	req := createMultipartRequest(t,
		"/api/v1/knowledge-bases/kb-1/question-sets/set-1/questions/import-file/preview",
		"test.docx", []byte("dummy"),
		map[string]string{"debug_export": "1"},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 in debug mode, got %d: body=%s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/zip") {
		t.Errorf("expected Content-Type application/zip, got %s", contentType)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "question-import-debug.zip") {
		t.Errorf("expected Content-Disposition with question-import-debug.zip, got %s", cd)
	}

	// Verify response body is a valid zip
	zipReader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("response body is not a valid zip: %v", err)
	}
	foundFiles := make(map[string]bool)
	for _, f := range zipReader.File {
		foundFiles[f.Name] = true
	}
	for _, want := range []string{
		"01_extracted.md", "02_normalized.md", "03_lines.txt",
		"04_blocks.txt", "05_items.json", "06_summary.json",
	} {
		if !foundFiles[want] {
			t.Errorf("zip missing expected file: %s", want)
		}
	}
}

func TestPreviewImportDebugExport_NoDebugParam(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	stub := &stubQuestionServiceForDebugExport{
		previewFn: func(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error) {
			// No debug export requested
			return &types.ImportFilePreviewResponse{
				Items:          []types.ImportQuestionItem{},
				Errors:         []types.ImportQuestionError{},
				Warnings:       []string{},
				RawTextPreview: "preview text",
				Stats:          types.ImportFilePreviewStats{DetectedQuestions: 0},
			}, nil
		},
	}

	router := newQuestionTestRouter(stub)
	// No debug_export query param
	req := createMultipartRequest(t,
		"/api/v1/knowledge-bases/kb-1/question-sets/set-1/questions/import-file/preview",
		"test.docx", []byte("dummy"),
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: body=%s", rec.Code, rec.Body.String())
	}

	// Should be JSON, not zip
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/zip") {
		t.Error("expected JSON response, got zip")
	}
	if !strings.Contains(rec.Body.String(), "preview text") {
		t.Errorf("expected JSON with raw_text_preview, got: %s", rec.Body.String())
	}
}

func TestPreviewImportDebugExport_EmptyTextServesZip(t *testing.T) {
	gin.SetMode(gin.DebugMode)
	defer gin.SetMode(gin.TestMode)

	// Simulate: the service produces a debug bundle for empty extracted text
	debugDir := filepath.Join(os.TempDir(), "weknora-question-import-debug", "handler-empty-test")
	if err := os.MkdirAll(debugDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	var manifest []string
	for _, entry := range []struct{ name, content string }{
		{"01_extracted.md", ""},
		{"02_normalized.md", ""},
		{"03_lines.txt", "     1  \n"},
		{"04_blocks.txt", ""},
		{"05_items.json", `{"items":[],"errors":[],"warnings":["未能从文件中抽取文本，请确认文件内容可复制，或等待 OCR 支持。"]}`},
		{"06_summary.json", `{"filename":"empty.docx","file_type":"docx","file_size":0,"extracted_len":0,"item_count":0}`},
	} {
		path := filepath.Join(debugDir, entry.name)
		if err := os.WriteFile(path, []byte(entry.content), 0644); err != nil {
			t.Fatalf("write %s: %v", entry.name, err)
		}
		manifest = append(manifest, path)
	}

	zipPath := filepath.Join(debugDir, "debug-export.zip")
	if err := createTestZip(zipPath, debugDir, manifest); err != nil {
		t.Fatalf("create test zip: %v", err)
	}
	manifest = append(manifest, zipPath)

	defer servicepkg.CleanupDebugExport(context.Background(), debugDir, manifest)

	stub := &stubQuestionServiceForDebugExport{
		previewFn: func(ctx context.Context, kbID, setID string, fileData []byte, fileName string, req *types.ImportFilePreviewRequest) (*types.ImportFilePreviewResponse, error) {
			return &types.ImportFilePreviewResponse{
				Warnings:        []string{"未能从文件中抽取文本，请确认文件内容可复制，或等待 OCR 支持。"},
				DebugExportPath:  debugDir,
				DebugManifest:    manifest,
			}, nil
		},
	}

	router := newQuestionTestRouter(stub)
	req := createMultipartRequest(t,
		"/api/v1/knowledge-bases/kb-1/question-sets/set-1/questions/import-file/preview",
		"empty.docx", []byte{},
		map[string]string{"debug_export": "1"},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for empty text debug export, got %d: body=%s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/zip") {
		t.Errorf("expected Content-Type application/zip for empty text, got %s", rec.Header().Get("Content-Type"))
	}
	// Verify the zip is valid and contains expected files
	zipReader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("empty text debug zip is not valid: %v", err)
	}
	foundFiles := make(map[string]bool)
	for _, f := range zipReader.File {
		foundFiles[f.Name] = true
	}
	for _, want := range []string{"01_extracted.md", "06_summary.json"} {
		if !foundFiles[want] {
			t.Errorf("empty text zip missing expected file: %s", want)
		}
	}
}

// createTestZip creates a zip with the same logic as createZip for test purposes.
func createTestZip(zipPath, baseDir string, filePaths []string) error {
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
			return err
		}
		// Skip the zip itself
		if rel == filepath.Base(zipPath) {
			continue
		}
		data, err := os.ReadFile(fp)
		if err != nil {
			return err
		}
		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, string(data)); err != nil {
			return err
		}
	}
	return nil
}

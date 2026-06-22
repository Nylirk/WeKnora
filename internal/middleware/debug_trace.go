package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sensitiveHeaders are headers whose values are always redacted in traces.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"proxy-authorization": true,
}

// sensitiveBodyKeys are substrings matched case-insensitively against
// JSON object keys. A match means the value is redacted.
var sensitiveBodyKeys = []string{
	"password", "passwd",
	"token", "access_token", "refresh_token",
	"api_key", "apikey",
	"secret", "secret_key", "private_key",
	"credential", "credentials",
	"authorization", "cookie",
	"openai_api_key", "mineru_api_key",
	"access_key", "access_key_id", "secret_access_key",
}

// textContentTypes are MIME types whose body content is safe to capture as text.
func isTextContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	switch {
	case mediaType == "application/json":
		return true
	case mediaType == "text/plain":
		return true
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case mediaType == "application/x-www-form-urlencoded":
		return true
	default:
		return false
	}
}

// shouldSkipTracePath returns true for paths that must not be traced,
// e.g. the debug trace API itself, to avoid recursive inflation.
func shouldSkipTracePath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/system/admin/debug/http-traces")
}

// sanitizeHeader copies a header map, redacting sensitive keys.
func sanitizeHeader(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vals := range h {
		if sensitiveHeaders[strings.ToLower(k)] {
			out[k] = "[REDACTED]"
		} else {
			out[k] = strings.Join(vals, ", ")
		}
	}
	return out
}

// shouldRedactKey returns true if a JSON key contains a sensitive substring.
func shouldRedactKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveBodyKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// redactJSON recursively walks a parsed JSON value and redacts sensitive keys.
func redactJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			if shouldRedactKey(k) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = redactJSON(vv)
			}
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = redactJSON(vv)
		}
		return out
	default:
		return val
	}
}

// captureBody reads and sanitizes a request/response body for tracing.
// Returns the preview string and whether it was truncated.
func captureBody(body io.Reader, contentType string, maxBytes int) (string, bool) {
	if body == nil {
		return "", false
	}
	if !isTextContentType(contentType) {
		return "", false
	}

	// Read up to maxBytes+1 to detect truncation.
	limited := io.LimitReader(body, int64(maxBytes+1))
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false
	}

	truncated := len(raw) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}

	if len(raw) == 0 {
		return "", truncated
	}

	// For JSON: parse, redact, re-serialize.
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "application/json" {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			redacted := redactJSON(parsed)
			if b, err := json.Marshal(redacted); err == nil {
				result := string(b)
				if truncated && len(result) > maxBytes {
					result = result[:maxBytes]
				}
				return result, truncated
			}
		}
		// JSON parse failed — treat as raw text with redaction attempt
	}

	return string(raw), truncated
}

// responseWriter wraps gin.ResponseWriter to capture status and body.
type traceResponseWriter struct {
	gin.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (w *traceResponseWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *traceResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *traceResponseWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// DebugTrace is a gin middleware that records HTTP debug traces into an
// in-memory ring buffer. It must be registered after Auth middleware so
// user/tenant/role context attributes are available.
//
// The middleware reads the enabled/capture_body flags from the provided
// SystemSettingService on every request. When enabled=false, it's a
// near-zero-cost no-op (one setting lookup per request).
func DebugTrace(
	traceSvc interfaces.HTTPDebugTraceService,
	settingSvc interfaces.SystemSettingService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fast path: trace disabled → no-op.
		if !settingSvc.GetBool(c.Request.Context(), "debug.http_trace.enabled", "", false) {
			c.Next()
			return
		}

		// Don't trace the debug trace API itself.
		if shouldSkipTracePath(c.Request.URL.Path) {
			c.Next()
			return
		}

		startedAt := time.Now()
		captureBodyEnabled := settingSvc.GetBool(c.Request.Context(), "debug.http_trace.capture_body", "", false)
		maxBodyBytes := int(settingSvc.GetInt(c.Request.Context(), "debug.http_trace.max_body_bytes", "", 4096))
		if maxBodyBytes <= 0 {
			maxBodyBytes = 4096
		}

		// Collect request metadata.
		trace := &types.HTTPDebugTrace{
			ID:                 uuid.NewString(),
			StartedAt:          startedAt,
			Method:             c.Request.Method,
			Path:               c.Request.URL.Path,
			RawPath:            c.Request.URL.RawPath,
			Query:              c.Request.URL.RawQuery,
			RequestContentType: c.Request.Header.Get("Content-Type"),
			RequestHeaders:     sanitizeHeader(c.Request.Header),
		}

		// Extract user/tenant context from gin keys (set by Auth middleware).
		if v, exists := c.Get(types.UserIDContextKey.String()); exists {
			if uid, ok := v.(string); ok {
				trace.UserID = uid
			}
		}
		if v, exists := c.Get(types.TenantIDContextKey.String()); exists {
			if tid, ok := v.(uint64); ok {
				trace.TenantID = tid
			}
		}
		if v, exists := c.Get(types.TenantRoleContextKey.String()); exists {
			if role, ok := v.(string); ok {
				trace.TenantRole = role
			}
		}
		if v, exists := c.Get(types.SystemAdminContextKey.String()); exists {
			if isSA, ok := v.(bool); ok {
				trace.IsSystemAdmin = isSA
			}
		}

		// Capture request body if enabled and content-type is text.
		if captureBodyEnabled {
			reqCT := c.Request.Header.Get("Content-Type")
			if isTextContentType(reqCT) {
				bodyBytes, err := io.ReadAll(c.Request.Body)
				c.Request.Body.Close()
				if err == nil {
					// Restore body for downstream handlers.
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					preview, truncated := captureBody(
						bytes.NewReader(bodyBytes),
						reqCT,
						maxBodyBytes,
					)
					trace.RequestBodyPreview = preview
					trace.RequestBodyTruncated = truncated
				}
			}
		}

		// Wrap response writer to capture response data.
		rw := &traceResponseWriter{
			ResponseWriter: c.Writer,
			buf:            &bytes.Buffer{},
			status:         http.StatusOK,
		}
		c.Writer = rw

		// Process.
		c.Next()

		// Collect response metadata.
		trace.CompletedAt = time.Now()
		trace.DurationMS = trace.CompletedAt.Sub(startedAt).Milliseconds()
		trace.Status = rw.status
		trace.ResponseContentType = rw.Header().Get("Content-Type")
		trace.ResponseHeaders = sanitizeHeader(rw.Header())

		// Capture response body if enabled.
		if captureBodyEnabled {
			respCT := rw.Header().Get("Content-Type")
			if isTextContentType(respCT) {
				preview, truncated := captureBody(
					bytes.NewReader(rw.buf.Bytes()),
					respCT,
					maxBodyBytes,
				)
				trace.ResponseBodyPreview = preview
				trace.ResponseBodyTruncated = truncated
			}
		}

		// Record errors if present.
		if len(c.Errors) > 0 {
			trace.Error = c.Errors.String()
		}

		traceSvc.Record(c.Request.Context(), trace)

		logger.Debugf(c.Request.Context(),
			"[debug_trace] recorded %s %s status=%d duration=%dms",
			trace.Method, trace.Path, trace.Status, trace.DurationMS,
		)
	}
}

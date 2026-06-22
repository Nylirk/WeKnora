package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
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

// fallbackRedactRaw applies regex-based sensitive-key redaction to raw text.
// Used when JSON parsing fails (e.g. truncated body) or for text/plain content.
// Two independent passes: first JSON-like key-value pairs, then form-encoded pairs.
func fallbackRedactRaw(raw string) string {
	result := fallbackRedactJSONComplete.ReplaceAllString(raw, `"$1": "[REDACTED]"`)
	result = fallbackRedactJSONTruncated.ReplaceAllString(result, `"$1": "[REDACTED]"`)
	result = fallbackRedactFormShape.ReplaceAllString(result, `$1=[REDACTED]`)
	return result
}

// Regexes for fallback redaction.
// Group 1 captures the bare key name (without quotes/colon/equals).
var (
	fallbackRedactJSONComplete  = buildJSONCompleteRegex()  // "key": "value"
	fallbackRedactJSONTruncated = buildJSONTruncatedRegex()  // "key": "value  (no closing quote)
	fallbackRedactFormShape     = buildFormShapeRegex()     // key=value
)

func buildSensitiveKeyAlt() string {
	keys := []string{
		"password", "passwd",
		"token", "access_token", "refresh_token",
		"api_key", "apikey",
		"secret", "secret_key", "private_key",
		"credential", "credentials",
		"authorization", "cookie",
		"openai_api_key", "mineru_api_key",
		"access_key", "access_key_id", "secret_access_key",
	}
	return strings.Join(keys, "|")
}

func buildJSONCompleteRegex() *regexp.Regexp {
	alt := buildSensitiveKeyAlt()
	// "keyContainingSensitiveSubstr": "quoted-value"
	// Group 1: the bare key name (no quotes).
	return regexp.MustCompile(`"(?i)([\w.-]*?(` + alt + `)[\w.-]*?)"\s*:\s*"[^"]*"`)
}

func buildJSONTruncatedRegex() *regexp.Regexp {
	alt := buildSensitiveKeyAlt()
	// "keyContainingSensitiveSubstr": "truncated-value (no closing quote, at EOS)
	// Group 1: the bare key name.
	return regexp.MustCompile(`"(?i)([\w.-]*?(` + alt + `)[\w.-]*?)"\s*:\s*"[^"]*$`)
}

func buildFormShapeRegex() *regexp.Regexp {
	alt := buildSensitiveKeyAlt()
	// keyContainingSensitiveSubstr=nonspace-nonamp-value
	// Group 1: the bare key name.
	return regexp.MustCompile(`(?i)([\w.-]*?(` + alt + `)[\w.-]*?)=[^&\s]+`)
}

// redactURLEncoded parses application/x-www-form-urlencoded data, redacts
// sensitive keys, and re-encodes.
func redactURLEncoded(raw string) string {
	vals, err := url.ParseQuery(raw)
	if err != nil || len(vals) == 0 {
		// Parse failed: fall back to regex redaction.
		return fallbackRedactRaw(raw)
	}
	redacted := make(url.Values, len(vals))
	for k, vs := range vals {
		if shouldRedactKey(k) {
			redacted[k] = []string{"[REDACTED]"}
		} else {
			redacted[k] = vs
		}
	}
	return redacted.Encode()
}

// captureBody reads and sanitizes a request/response body for tracing.
// Returns the preview string and whether it was truncated.
//
// Strategy per content type:
//   - application/json:         parse → recursive-redact → re-serialize → truncate output.
//     On parse failure, fall back to regex-based redaction.
//   - application/x-www-form-urlencoded: url.ParseQuery → redact keys → re-encode.
//   - text/*:                   regex-based fallback redaction.
func captureBody(body io.Reader, contentType string, maxBytes int) (string, bool) {
	if body == nil {
		return "", false
	}
	if !isTextContentType(contentType) {
		return "", false
	}

	// Read maxBytes+1 to detect whether the original body exceeds the limit.
	limited := io.LimitReader(body, int64(maxBytes+1))
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false
	}

	// rawTruncated is true when the original body was longer than maxBytes.
	rawTruncated := len(raw) > maxBytes
	if rawTruncated {
		raw = raw[:maxBytes]
	}

	if len(raw) == 0 {
		return "", false
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)

	switch mediaType {
	case "application/json":
		preview, sanitizedTruncated := captureJSONBody(raw, maxBytes)
		return preview, rawTruncated || sanitizedTruncated

	case "application/x-www-form-urlencoded":
		result := redactURLEncoded(string(raw))
		sanitizedTruncated := len(result) > maxBytes
		if sanitizedTruncated {
			result = result[:maxBytes]
		}
		return result, rawTruncated || sanitizedTruncated

	default:
		// text/plain, text/*, etc.
		result := fallbackRedactRaw(string(raw))
		sanitizedTruncated := len(result) > maxBytes
		if sanitizedTruncated {
			result = result[:maxBytes]
		}
		return result, rawTruncated || sanitizedTruncated
	}
}

// captureJSONBody handles application/json: parse → redact → re-serialize,
// with a regex fallback when parsing fails (e.g. truncated body).
func captureJSONBody(raw []byte, maxBytes int) (string, bool) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err == nil {
		// Successful parse: walk and redact, then re-serialize.
		redacted := redactJSON(parsed)
		b, err := json.Marshal(redacted)
		if err != nil {
			// Marshal failed (shouldn't happen after a successful parse).
			// Fall back to regex redaction on the original bytes.
			result := fallbackRedactRaw(string(raw))
			truncated := len(result) > maxBytes
			if truncated {
				result = result[:maxBytes]
			}
			return result, truncated
		}
		result := string(b)
		truncated := len(result) > maxBytes
		if truncated {
			result = result[:maxBytes]
		}
		return result, truncated
	}

	// JSON parse failed (truncated or malformed). Must NOT return raw text
	// directly — use regex-based fallback redaction to catch partial
	// sensitive fields.
	result := fallbackRedactRaw(string(raw))
	truncated := len(result) > maxBytes
	if truncated {
		result = result[:maxBytes]
	}
	return result, truncated
}

// traceResponseWriter wraps gin.ResponseWriter to capture status and
// conditionally buffer response body bytes. When captureBody is false
// the buffer is nil and all Write/WriteString calls skip the copy —
// large responses pay zero memory cost beyond the original writer.
type traceResponseWriter struct {
	gin.ResponseWriter
	buf         *bytes.Buffer // nil when captureBody is false
	captureBody bool
	maxBytes    int
	bufTruncated bool
	status      int
}

func newTraceResponseWriter(w gin.ResponseWriter, captureBody bool, maxBytes int) *traceResponseWriter {
	rw := &traceResponseWriter{
		ResponseWriter: w,
		captureBody:    captureBody,
		maxBytes:       maxBytes,
		status:         http.StatusOK,
	}
	if captureBody && maxBytes > 0 {
		rw.buf = &bytes.Buffer{}
	}
	return rw
}

func (w *traceResponseWriter) Write(b []byte) (int, error) {
	if w.buf != nil {
		if !w.bufTruncated {
			remain := w.maxBytes - w.buf.Len()
			if remain > 0 {
				if len(b) > remain {
					w.buf.Write(b[:remain])
					w.bufTruncated = true
				} else {
					w.buf.Write(b)
				}
			} else {
				w.bufTruncated = true
			}
		}
	}
	if w.ResponseWriter != nil {
		return w.ResponseWriter.Write(b)
	}
	return len(b), nil
}

func (w *traceResponseWriter) WriteHeader(code int) {
	w.status = code
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *traceResponseWriter) WriteString(s string) (int, error) {
	if w.buf != nil {
		if !w.bufTruncated {
			remain := w.maxBytes - w.buf.Len()
			if remain > 0 {
				if len(s) > remain {
					w.buf.WriteString(s[:remain])
					w.bufTruncated = true
				} else {
					w.buf.WriteString(s)
				}
			} else {
				w.bufTruncated = true
			}
		}
	}
	if w.ResponseWriter != nil {
		return w.ResponseWriter.WriteString(s)
	}
	return len(s), nil
}

// bufferedBody returns the bytes captured in the buffer, or nil when
// captureBody was disabled.
func (w *traceResponseWriter) bufferedBody() []byte {
	if w.buf == nil {
		return nil
	}
	return w.buf.Bytes()
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

		// Wrap response writer. When capture_body is disabled the buffer is
		// nil — zero per-request memory cost for response buffering.
		rw := newTraceResponseWriter(c.Writer, captureBodyEnabled, maxBodyBytes)
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
				respBytes := rw.bufferedBody()
				if len(respBytes) > 0 {
					preview, sanitizedTruncated := captureBody(
						bytes.NewReader(respBytes),
						respCT,
						maxBodyBytes,
					)
					trace.ResponseBodyPreview = preview
					trace.ResponseBodyTruncated = rw.bufTruncated || sanitizedTruncated
				}
			} else if rw.bufTruncated {
				// Non-text response that exceeded the buffer cap.
				trace.ResponseBodyTruncated = true
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

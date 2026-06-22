package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestCaptureJSONBody_RedactsSensitiveKeys(t *testing.T) {
	input := `{"username":"alice","password":"secret123","email":"alice@example.com","nested":{"api_key":"sk-abc123","name":"test"}}`
	result, truncated := captureJSONBody([]byte(input), 4096)
	if truncated {
		t.Error("unexpected truncation")
	}
	// password must be redacted.
	if strings.Contains(result, "secret123") {
		t.Errorf("password value leaked: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output: %s", result)
	}
	// api_key must be redacted.
	if strings.Contains(result, "sk-abc123") {
		t.Errorf("api_key value leaked: %s", result)
	}
	// Non-sensitive fields preserved.
	if !strings.Contains(result, "alice") {
		t.Error("username should be preserved")
	}
	if !strings.Contains(result, "alice@example.com") {
		t.Error("email should be preserved")
	}
}

func TestCaptureJSONBody_TruncatedJSON_NoLeak(t *testing.T) {
	// Simulate a truncated JSON body that ends mid-value in a password field.
	// The raw JSON parse will fail; fallback redaction must catch the partial.
	input := `{"username":"alice","password":"superSecretPwd123`
	result, _ := captureJSONBody([]byte(input), 4096)
	if strings.Contains(result, "superSecretPwd123") {
		t.Errorf("truncated password leaked: %s", result)
	}
	// The fallback should redact the password value.
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected fallback redaction: %s", result)
	}
}

func TestCaptureJSONBody_InvalidJSON_NoLeak(t *testing.T) {
	// Malformed JSON with embedded secrets.
	input := `{"token": "bearer-secret-token", garbage`
	result, _ := captureJSONBody([]byte(input), 4096)
	if strings.Contains(result, "bearer-secret-token") {
		t.Errorf("token leaked in invalid JSON: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected fallback redaction for invalid JSON: %s", result)
	}
}

func TestCaptureJSONBody_PreservesNonSensitiveValues(t *testing.T) {
	input := `{"name":"project-x","description":"A test project","count":42,"enabled":true}`
	result, truncated := captureJSONBody([]byte(input), 4096)
	if truncated {
		t.Error("unexpected truncation")
	}
	if !strings.Contains(result, "project-x") {
		t.Error("non-sensitive value should be preserved")
	}
	if strings.Contains(result, "[REDACTED]") {
		t.Error("no redaction expected for non-sensitive keys")
	}
}

func TestCaptureJSONBody_TruncationFlag(t *testing.T) {
	// Create a large JSON that will be truncated after redaction.
	var sb strings.Builder
	sb.WriteString(`{"data":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"id":`)
		sb.WriteString(strings.Repeat("X", 100))
		sb.WriteString(`,"val":"ok"}`)
	}
	sb.WriteString(`]}`)

	result, truncated := captureJSONBody([]byte(sb.String()), 512)
	if !truncated {
		t.Error("expected truncation for large body")
	}
	if len(result) > 512 {
		t.Errorf("result length %d exceeds maxBytes 512", len(result))
	}
}

func TestRedactURLEncoded_RedactsSensitiveKeys(t *testing.T) {
	input := "username=alice&password=secret123&token=bearer-xyz&name=test&api_key=sk-abc"
	result := redactURLEncoded(input)
	if strings.Contains(result, "secret123") {
		t.Errorf("password leaked in urlencoded: %s", result)
	}
	if strings.Contains(result, "bearer-xyz") {
		t.Errorf("token leaked in urlencoded: %s", result)
	}
	if strings.Contains(result, "sk-abc") {
		t.Errorf("api_key leaked in urlencoded: %s", result)
	}
	if !strings.Contains(result, "%5BREDACTED%5D") {
		t.Errorf("expected URL-encoded [REDACTED] in urlencoded: %s", result)
	}
	// Non-sensitive values preserved.
	if !strings.Contains(result, "alice") {
		t.Error("username should be preserved in urlencoded")
	}
	if !strings.Contains(result, "test") {
		t.Error("name should be preserved in urlencoded")
	}
}

func TestRedactURLEncoded_InvalidEncoding_FallbackRedaction(t *testing.T) {
	// Invalid percent-encoding; ParseQuery fails → fallback redaction applies.
	input := "token=secret123%ZZinvalid"
	result := redactURLEncoded(input)
	if strings.Contains(result, "secret123") {
		t.Errorf("token leaked in invalid urlencoded: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] via fallback: %s", result)
	}
}

func TestFallbackRedactRaw_JSONShape(t *testing.T) {
	// JSON-like fragment that wouldn't parse as valid JSON.
	input := `{"password": "my-secret-pwd", "username": "alice"}`
	result := fallbackRedactRaw(input)
	if strings.Contains(result, "my-secret-pwd") {
		t.Errorf("password leaked in fallback redaction: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] for JSON shape: %s", result)
	}
	if !strings.Contains(result, "alice") {
		t.Error("non-sensitive value should be preserved in fallback")
	}
}

func TestFallbackRedactRaw_FormShape(t *testing.T) {
	input := "access_token=gho_abcdef123456&state=active"
	result := fallbackRedactRaw(input)
	if strings.Contains(result, "gho_abcdef123456") {
		t.Errorf("access_token leaked in form shape: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] for form shape: %s", result)
	}
}

func TestFallbackRedactRaw_MultiplePatterns(t *testing.T) {
	input := `{"client_secret":"sec-xyz"}`
	result := fallbackRedactRaw(input)
	if strings.Contains(result, "sec-xyz") {
		t.Errorf("client_secret value leaked: %s", result)
	}
}

func TestCaptureBody_JSON(t *testing.T) {
	input := `{"password":"pwd123","data":"hello"}`
	result, truncated := captureBody(
		bytes.NewReader([]byte(input)),
		"application/json",
		4096,
	)
	if truncated {
		t.Error("unexpected truncation")
	}
	if strings.Contains(result, "pwd123") {
		t.Errorf("password leaked: %s", result)
	}
	if !strings.Contains(result, "hello") {
		t.Error("non-sensitive value should be preserved")
	}
}

func TestCaptureBody_URLEncoded(t *testing.T) {
	input := `user=alice&password=pwd123&action=login`
	result, truncated := captureBody(
		bytes.NewReader([]byte(input)),
		"application/x-www-form-urlencoded",
		4096,
	)
	if truncated {
		t.Error("unexpected truncation")
	}
	if strings.Contains(result, "pwd123") {
		t.Errorf("password leaked in urlencoded: %s", result)
	}
}

func TestCaptureBody_PlainText(t *testing.T) {
	input := `token=abc123&other=ok`
	result, truncated := captureBody(
		bytes.NewReader([]byte(input)),
		"text/plain",
		4096,
	)
	if truncated {
		t.Error("unexpected truncation")
	}
	// Plain text goes through fallback redaction.
	if strings.Contains(result, "abc123") {
		t.Errorf("token leaked in plain text: %s", result)
	}
}

func TestCaptureBody_NonTextContentType(t *testing.T) {
	// Binary content types should not be captured.
	result, truncated := captureBody(
		bytes.NewReader([]byte("ignored")),
		"application/octet-stream",
		4096,
	)
	if result != "" {
		t.Error("expected empty result for non-text content type")
	}
	if truncated {
		t.Error("expected no truncation flag for non-text content type")
	}
}

func TestCaptureBody_NilReader(t *testing.T) {
	result, truncated := captureBody(nil, "application/json", 4096)
	if result != "" {
		t.Error("expected empty result for nil body")
	}
	if truncated {
		t.Error("expected no truncation for nil body")
	}
}

func TestSanitizeHeader_RedactsSensitiveHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer secret-token")
	h.Set("Content-Type", "application/json")
	h.Set("Cookie", "session=abc123")
	h.Set("X-Api-Key", "sk-12345")

	out := sanitizeHeader(h)
	if out["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %s", out["Authorization"])
	}
	if out["Cookie"] != "[REDACTED]" {
		t.Errorf("Cookie not redacted: %s", out["Cookie"])
	}
	if out["X-Api-Key"] != "[REDACTED]" {
		t.Errorf("X-Api-Key not redacted: %s", out["X-Api-Key"])
	}
	if out["Content-Type"] != "application/json" {
		t.Errorf("Content-Type should not be redacted: %s", out["Content-Type"])
	}
}

func TestSanitizeHeader_EmptyHeaders(t *testing.T) {
	out := sanitizeHeader(nil)
	if out != nil {
		t.Error("expected nil for nil headers")
	}
}

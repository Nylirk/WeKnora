package types

import "time"

// DebugRoute represents a single registered Gin route for the debug console.
type DebugRoute struct {
	Method             string `json:"method"`
	Path               string `json:"path"`
	Handler            string `json:"handler"`
	Module             string `json:"module"`
	AuthRequired       bool   `json:"auth_required"`
	SystemAdminRequired bool  `json:"system_admin_required"`
}

// HTTPDebugTrace is a single recorded HTTP request trace entry.
type HTTPDebugTrace struct {
	ID                   string            `json:"id"`
	StartedAt            time.Time         `json:"started_at"`
	CompletedAt          time.Time         `json:"completed_at"`
	DurationMS           int64             `json:"duration_ms"`
	Method               string            `json:"method"`
	Path                 string            `json:"path"`
	RawPath              string            `json:"raw_path,omitempty"`
	Query                string            `json:"query,omitempty"`
	Status               int               `json:"status"`
	UserID               string            `json:"user_id,omitempty"`
	TenantID             uint64            `json:"tenant_id,omitempty"`
	TenantRole           string            `json:"tenant_role,omitempty"`
	IsSystemAdmin        bool              `json:"is_system_admin"`
	RequestContentType   string            `json:"request_content_type,omitempty"`
	ResponseContentType  string            `json:"response_content_type,omitempty"`
	RequestHeaders       map[string]string `json:"request_headers,omitempty"`
	ResponseHeaders      map[string]string `json:"response_headers,omitempty"`
	RequestBodyPreview   string            `json:"request_body_preview,omitempty"`
	ResponseBodyPreview  string            `json:"response_body_preview,omitempty"`
	RequestBodyTruncated bool              `json:"request_body_truncated"`
	ResponseBodyTruncated bool             `json:"response_body_truncated"`
	Error                string            `json:"error,omitempty"`
}

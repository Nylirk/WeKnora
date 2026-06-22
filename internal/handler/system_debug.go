package handler

import (
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// publicPaths are paths that do not require authentication. Matched exactly
// (method+path) via the noAuthAPI map in middleware/auth.go.
var publicPaths = map[string]bool{
	"/health":                     true,
	"/api/v1/auth/login":          true,
	"/api/v1/auth/register":       true,
	"/api/v1/auth/auto-setup":     true,
	"/api/v1/auth/config":         true,
	"/api/v1/auth/refresh":        true,
	"/api/v1/auth/oidc/config":    true,
	"/api/v1/auth/oidc/url":       true,
	"/api/v1/auth/oidc/callback":  true,
	"/api/v1/auth/invitations/lookup":  true,
	"/api/v1/auth/register-by-invite": true,
	"/api/v1/files/presigned":     true,
}

// inferModule assigns a module label based on the URL path prefix.
func inferModule(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/v1/system"):
		return "system"
	case strings.HasPrefix(path, "/api/v1/knowledge-bases"), strings.HasPrefix(path, "/api/v1/knowledge"):
		return "knowledge"
	case strings.HasPrefix(path, "/api/v1/agents"):
		return "agents"
	case strings.HasPrefix(path, "/api/v1/models"):
		return "models"
	case strings.HasPrefix(path, "/api/v1/tenants"):
		return "tenants"
	case strings.HasPrefix(path, "/api/v1/organizations"):
		return "organizations"
	case strings.HasPrefix(path, "/api/v1/chunks"):
		return "chunks"
	case strings.HasPrefix(path, "/api/v1/questions"):
		return "questions"
	default:
		return "unknown"
	}
}

// isPublicPath returns true for routes that are not behind auth middleware.
func isPublicPath(method, path string) bool {
	// Prefix match for embed and IM callback paths.
	if strings.HasPrefix(path, "/api/v1/embed/") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/im/callback/") {
		return true
	}
	return publicPaths[path]
}

// ListDebugRoutes returns all registered Gin routes with inferred metadata.
func (h *SystemHandler) ListDebugRoutes(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	ginRoutes := h.engine.Routes()
	routes := make([]types.DebugRoute, 0, len(ginRoutes))

	for _, gr := range ginRoutes {
		sysAdmin := strings.HasPrefix(gr.Path, "/api/v1/system/admin")
		authReq := gr.Path != "" && strings.HasPrefix(gr.Path, "/api/v1") && !isPublicPath(gr.Method, gr.Path)

		routes = append(routes, types.DebugRoute{
			Method:             gr.Method,
			Path:               gr.Path,
			Handler:            gr.Handler,
			Module:             inferModule(gr.Path),
			AuthRequired:       authReq,
			SystemAdminRequired: sysAdmin,
		})
	}

	logger.Infof(ctx, "[debug] route registry: %d routes", len(routes))
	c.JSON(http.StatusOK, gin.H{
		"code":   0,
		"msg":    "success",
		"routes": routes,
	})
}

// ListHTTPTraces returns all non-expired HTTP debug traces.
func (h *SystemHandler) ListHTTPTraces(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if h.debugTraceSvc == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "traces": []*types.HTTPDebugTrace{}})
		return
	}

	traces := h.debugTraceSvc.List(ctx)
	if traces == nil {
		traces = []*types.HTTPDebugTrace{}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":   0,
		"msg":    "success",
		"traces": traces,
	})
}

// GetHTTPTrace returns a single debug trace by ID.
func (h *SystemHandler) GetHTTPTrace(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if h.debugTraceSvc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trace service not available"})
		return
	}

	id := c.Param("id")
	trace := h.debugTraceSvc.Get(ctx, id)
	if trace == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  0,
		"msg":   "success",
		"trace": trace,
	})
}

// ClearHTTPTraces removes all debug traces.
func (h *SystemHandler) ClearHTTPTraces(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())

	if h.debugTraceSvc == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success"})
		return
	}

	h.debugTraceSvc.Clear(ctx)
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success"})
}

// Ensure SystemHandler has access to the required services.
// These fields are set in the container wiring.
var _ interfaces.HTTPDebugTraceService = nil

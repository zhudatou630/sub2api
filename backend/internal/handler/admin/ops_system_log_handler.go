package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ListSystemLogs returns indexed system logs.
// GET /api/v1/admin/ops/system-logs
func (h *OpsHandler) ListSystemLogs(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}

	start, end, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsSystemLogFilter{
		Page:            page,
		PageSize:        pageSize,
		StartTime:       &start,
		EndTime:         &end,
		Host:            strings.TrimSpace(c.Query("host")),
		Level:           strings.TrimSpace(c.Query("level")),
		Component:       strings.TrimSpace(c.Query("component")),
		RequestID:       strings.TrimSpace(c.Query("request_id")),
		ClientRequestID: strings.TrimSpace(c.Query("client_request_id")),
		Platform:        strings.TrimSpace(c.Query("platform")),
		Model:           strings.TrimSpace(c.Query("model")),
		Query:           strings.TrimSpace(c.Query("q")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		filter.APIKeyID = &id
	}
	if v := strings.TrimSpace(c.Query("account_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		filter.AccountID = &id
	}

	result, err := h.opsService.ListSystemLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Logs, int64(result.Total), result.Page, result.PageSize)
}

// CleanupSystemLogs is permanently disabled before monitoring or store access.
// POST /api/v1/admin/ops/system-logs/cleanup
func (h *OpsHandler) CleanupSystemLogs(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

// GetSystemLogIngestionHealth returns sink health metrics.
// GET /api/v1/admin/ops/system-logs/health
func (h *OpsHandler) GetSystemLogIngestionHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.opsService.GetSystemLogSinkHealth())
}

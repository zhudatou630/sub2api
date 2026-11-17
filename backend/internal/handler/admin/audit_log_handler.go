package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AuditLogHandler 操作审计日志管理接口。
// 审计日志仅管理员可见；网页/API 全量清空固定禁用。
type AuditLogHandler struct {
	auditService *service.AuditLogService
	totpService  *service.TotpService
}

// NewAuditLogHandler 创建审计日志处理器。
func NewAuditLogHandler(auditService *service.AuditLogService, totpService *service.TotpService) *AuditLogHandler {
	return &AuditLogHandler{
		auditService: auditService,
		totpService:  totpService,
	}
}

// List 分页查询审计日志。
// GET /api/v1/admin/audit-logs
func (h *AuditLogHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}

	filter := &service.AuditLogFilter{
		Page:       page,
		PageSize:   pageSize,
		ActorEmail: strings.TrimSpace(c.Query("actor_email")),
		AuthMethod: strings.TrimSpace(c.Query("auth_method")),
		Action:     strings.TrimSpace(c.Query("action")),
		Method:     strings.TrimSpace(c.Query("method")),
		ClientIP:   strings.TrimSpace(c.Query("client_ip")),
		Query:      strings.TrimSpace(c.Query("q")),
	}

	if v := strings.TrimSpace(c.Query("actor_user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid actor_user_id")
			return
		}
		filter.ActorUserID = &id
	}
	if v := strings.TrimSpace(c.Query("start_time")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.BadRequest(c, "Invalid start_time, expect RFC3339")
			return
		}
		filter.StartTime = &t
	}
	if v := strings.TrimSpace(c.Query("end_time")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.BadRequest(c, "Invalid end_time, expect RFC3339")
			return
		}
		filter.EndTime = &t
	}
	if v := strings.TrimSpace(c.Query("success")); v != "" {
		success := v == "true"
		filter.Success = &success
	}

	result, err := h.auditService.List(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Logs, int64(result.Total), result.Page, result.PageSize)
}

// Get 查询单条审计日志详情（含脱敏后的请求体）。
// GET /api/v1/admin/audit-logs/:id
func (h *AuditLogHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid audit log id")
		return
	}
	item, err := h.auditService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// Clear is permanently disabled, before authentication, TOTP or store access.
// POST /api/v1/admin/audit-logs/clear
func (h *AuditLogHandler) Clear(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

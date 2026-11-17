package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SystemHandler only exposes local build metadata. In-process replacement and
// restart must not be available through the administrative control plane.
type SystemHandler struct {
	updateSvc systemUpdateService
}

type systemUpdateService interface {
	LocalVersionInfo() *service.UpdateInfo
}

// Keep the constructor compatible with existing dependency injection. The lock
// service is deliberately unused: disabled operations must not reach any store.
func NewSystemHandler(updateSvc systemUpdateService, _ *service.SystemOperationLockService) *SystemHandler {
	return &SystemHandler{updateSvc: updateSvc}
}

// GetVersion returns the running build version without release/cache lookups.
func (h *SystemHandler) GetVersion(c *gin.Context) {
	response.Success(c, gin.H{"version": h.updateSvc.LocalVersionInfo().CurrentVersion})
}

// CheckUpdates is retained for older clients, but never probes upstream, even
// with force=true. It reports only local metadata and the disabled policy.
func (h *SystemHandler) CheckUpdates(c *gin.Context) {
	response.Success(c, h.updateSvc.LocalVersionInfo())
}

func (h *SystemHandler) PerformUpdate(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

func (h *SystemHandler) GetRollbackVersions(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

func (h *SystemHandler) Rollback(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

func (h *SystemHandler) RestartService(c *gin.Context) {
	RejectDangerousMaintenance(c)
}

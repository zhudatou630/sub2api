package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// RejectDangerousMaintenance is a fixed fork policy, not a configurable feature.
// Reject before parsing credentials, acquiring locks, replaying idempotent results,
// or touching the database/network. Maintenance requires a separately reviewed
// manual procedure; neither administrator sessions nor API keys can opt in.
func RejectDangerousMaintenance(c *gin.Context) {
	c.Abort()
	response.ErrorWithDetails(c, http.StatusForbidden,
		"Web/API destructive maintenance is disabled in this fork; use a reviewed manual maintenance procedure",
		"DANGEROUS_MAINTENANCE_DISABLED", nil)
}

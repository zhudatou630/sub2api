//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCalmMaintenanceRoutesCannotReachServices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// No dependencies: an accidentally restored handler will panic/fail rather
	// than access any real database/upstream. Authentication is synthetic here;
	// the production routes still inherit the normal admin middleware.
	h := &handler.Handlers{Admin: &handler.AdminHandlers{
		System: adminhandler.NewSystemHandler(service.NewUpdateService(nil, nil, "calm.test", "release"), nil),
		Backup: adminhandler.NewBackupHandler(nil, nil, nil),
	}}
	group := router.Group("/api/v1/admin")
	stepUp := middleware.StepUpAuthMiddleware(func(*gin.Context) {
		t.Fatal("disabled restore must not offer step-up as an opt-in")
	})
	registerSystemRoutes(group, h)
	registerBackupRoutes(group, h, stepUp)
	registerAuditLogRoutes(group, h, stepUp)
	registerUsageRoutes(group, h)
	registerOpsRoutes(group, h)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/system/update"},
		{http.MethodPost, "/system/rollback"},
		{http.MethodPost, "/system/restart"},
		{http.MethodGet, "/system/rollback-versions"},
		{http.MethodPost, "/backups/synthetic/restore"},
		{http.MethodPost, "/audit-logs/clear"},
		{http.MethodPost, "/usage/cleanup-tasks"},
		{http.MethodPost, "/ops/system-logs/cleanup"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/admin"+tc.path,
				strings.NewReader(`{"version":"0.2.21","password":"synthetic","totp_code":"123456"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "synthetic-replay")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), `"reason":"DANGEROUS_MAINTENANCE_DISABLED"`)
		})
	}
}

//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCalmMaintenanceHandlersRejectBeforeDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Nil dependencies deliberately panic if a handler reaches a store, updater
	// or credential check. No database, process restart or network is needed.
	system := NewSystemHandler(nil, nil)
	backup := NewBackupHandler(nil, nil, nil)
	for _, tc := range []struct {
		name   string
		handle gin.HandlerFunc
	}{
		{"update", system.PerformUpdate},
		{"local-rollback", system.Rollback},
		{"versioned-rollback", system.Rollback},
		{"rollback-list", system.GetRollbackVersions},
		{"restart", system.RestartService},
		{"restore", backup.RestoreBackup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range []string{"", `{`, `{"version":"0.2.21","password":"synthetic","totp_code":"123456"}`} {
				router := gin.New()
				router.POST("/operation", tc.handle, func(*gin.Context) {
					t.Fatal("disabled maintenance must abort the handler chain")
				})
				req := httptest.NewRequest(http.MethodPost, "/operation?force=true", strings.NewReader(body))
				req.Header.Set("Idempotency-Key", "synthetic-previous-success")
				req.Header.Set("Authorization", "Bearer synthetic-admin-key")
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req.WithContext(ctx))
				require.Equal(t, http.StatusForbidden, rec.Code)
				var payload struct {
					Reason string `json:"reason"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
				require.Equal(t, "DANGEROUS_MAINTENANCE_DISABLED", payload.Reason)
			}
		})
	}
}

func TestCalmSystemVersionQueriesAreLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Nil Redis and GitHub dependencies prove force cannot trigger a probe.
	svc := service.NewUpdateService(nil, nil, "0.2.22-calm.test", "release")
	h := NewSystemHandler(svc, nil)
	router := gin.New()
	router.GET("/version", h.GetVersion)
	router.GET("/check-updates", h.CheckUpdates)
	for _, path := range []string{"/version", "/check-updates", "/check-updates?force=true"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "0.2.22-calm.test")
		if strings.HasPrefix(path, "/check-updates") {
			require.Contains(t, rec.Body.String(), `"has_update":false`)
			require.Contains(t, rec.Body.String(), "disabled")
			require.NotContains(t, rec.Body.String(), "release_info")
		}
	}
}

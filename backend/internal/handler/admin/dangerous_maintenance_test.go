//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type maintenanceUnreadBody struct{}

func (maintenanceUnreadBody) Read([]byte) (int, error) {
	panic("disabled maintenance must not read the request body")
}
func (maintenanceUnreadBody) Close() error { return nil }

// Call handler methods directly, without the route-level rejection. Nil receivers
// and an unreadable body prove the fixed policy runs before dependencies/parsing.
func TestCalmCleanupHandlersDirectlyRejectBeforeDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var usage *UsageHandler
	var ops *OpsHandler
	var audit *AuditLogHandler
	for _, handler := range []struct {
		name string
		call gin.HandlerFunc
	}{
		{"usage", usage.CreateCleanupTask},
		{"system-logs", ops.CleanupSystemLogs},
		{"audit", audit.Clear},
	} {
		for _, auth := range []string{"", "jwt", "admin_api_key", "mcp_oauth"} {
			t.Run(handler.name+"/"+auth, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/synthetic", maintenanceUnreadBody{}).WithContext(ctx)
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set("Idempotency-Key", "synthetic-previous-success")
				if auth != "" {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99})
					c.Set("auth_method", auth)
				}
				handler.call(c)
				require.True(t, c.IsAborted())
				require.Equal(t, http.StatusForbidden, rec.Code)
				var payload response.Response
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
				require.Equal(t, "DANGEROUS_MAINTENANCE_DISABLED", payload.Reason)
			})
		}
	}
}

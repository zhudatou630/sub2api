package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalmIdentityDiagnosticsNeverContainIdentityOrBody(t *testing.T) {
	const secret = "synthetic-private-identity"
	body := []byte(`{"metadata":{"user_id":"` + secret + `"},"system":"synthetic private conversation"}`)
	req := httptest.NewRequest(http.MethodPost, "https://example.invalid/v1/messages?private="+secret, nil)
	req.Header.Set("User-Agent", secret)
	req.Header.Set("X-Claude-Code-Session-Id", secret)
	req.Header.Set("Cookie", secret)
	req.Header.Set("Authorization", "Bearer "+secret)
	line := buildClaudeMimicDebugLine(req, body, &Account{ID: 1, Name: secret}, "oauth", false)
	lazy := newClaudeMimicDebugLine(req, body, &Account{ID: 1, Name: secret}, "oauth", false)
	require.Equal(t, line, lazy.String(), "upstream lazy diagnostics retain the redacted format")
	require.Equal(t, line, lazy.String(), "repeated lazy formatting is stable")
	require.Nil(t, lazy.req)
	require.Nil(t, lazy.body)
	require.Nil(t, lazy.account)
	require.NotContains(t, line, secret)
	require.NotContains(t, line, "synthetic private conversation")
	require.Contains(t, line, "meta.user_id_present=true")
	require.Contains(t, line, "meta.user_id_parseable=false")
	for k, vs := range req.Header {
		for _, v := range vs {
			require.NotContains(t, safeHeaderValueForLog(k, v), secret)
		}
	}
	f, err := os.CreateTemp(t.TempDir(), "snapshot-")
	require.NoError(t, err)
	defer f.Close()
	svc := &GatewayService{}
	svc.debugGatewayBodyFile.Store(f)
	svc.debugLogGatewaySnapshot("SYNTHETIC", req.Header, body, map[string]string{"url": req.URL.String(), "account": secret})
	snapshot, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.NotContains(t, string(snapshot), secret)
	require.NotContains(t, string(snapshot), "synthetic private conversation")
	require.Contains(t, string(snapshot), "metadata_user_id_present=true")
}

func TestCalmStickyLogsDoNotExposeDeviceOrSession(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	const session = "00000000-0000-4000-8000-000000000001"
	device := strings.Repeat("e", 64)
	uid := FormatMetadataUserID(device, "synthetic-account", session, "2.1.78")
	svc := &GatewayService{}
	require.Equal(t, session, svc.GenerateSessionHash(&ParsedRequest{MetadataUserID: uid}))
	require.NotContains(t, output.String(), device)
	require.NotContains(t, output.String(), session)
	require.Contains(t, output.String(), "session_id_present")
	output.Reset()
	require.Empty(t, svc.GenerateSessionHash(&ParsedRequest{MetadataUserID: "synthetic-malformed-private-metadata"}))
	require.NotContains(t, output.String(), "synthetic-malformed-private-metadata")
}

func TestCalmSetupTokenSharesOAuthIdentityPolicy(t *testing.T) {
	for _, reserved := range []bool{true, false} {
		store := calmStore()
		if reserved {
			store.ids[1] = ""
		}
		svc := &GatewayService{cfg: &config.Config{}, identityService: NewIdentityService(&stubIdentityCache{}, store)}
		account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
		body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
		for _, count := range []bool{false, true} {
			var req *http.Request
			var err error
			if count {
				req, _, err = svc.buildCountTokensRequest(context.Background(), nil, account, body, "synthetic", "oauth", "claude-haiku-4-5", false)
			} else {
				req, _, err = svc.buildUpstreamRequest(context.Background(), nil, account, body, "synthetic", "oauth", "claude-haiku-4-5", false, false)
			}
			if reserved {
				require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
				require.Nil(t, req)
				require.Zero(t, store.initCalls)
			} else {
				require.NoError(t, err)
				require.Equal(t, defaultFingerprint().UserAgent, getHeaderRaw(req.Header, "User-Agent"))
				require.NoError(t, req.Body.Close())
			}
		}
	}
}

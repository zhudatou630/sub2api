//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCalmCountTokensMimicDoesNotForwardClientHeaders(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, mimic := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/mimic=%t", typ, mimic), func(t *testing.T) {
				svc := &GatewayService{cfg: &config.Config{}, identityService: NewIdentityService(&stubIdentityCache{}, calmStore())}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: typ}
				c := calmGatewayContext()
				keys := []string{"x-stainless-helper-method", "accept-language", "sec-fetch-mode", "x-claude-code-session-id"}
				for _, key := range keys {
					c.Request.Header.Set(key, "synthetic-downstream")
				}
				tokenType := "oauth"
				if typ == AccountTypeAPIKey {
					tokenType = "apikey"
				}
				body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
				countReq, _, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "synthetic-token", tokenType, "claude-haiku-4-5", mimic)
				require.NoError(t, err)
				defer countReq.Body.Close()
				messageReq, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "synthetic-token", tokenType, "claude-haiku-4-5", false, mimic)
				require.NoError(t, err)
				defer messageReq.Body.Close()
				for _, key := range keys {
					value := getHeaderRaw(countReq.Header, key)
					require.Equal(t, getHeaderRaw(messageReq.Header, key), value, "count_tokens and messages header policy: %s", key)
					if tokenType == "oauth" && mimic {
						require.Empty(t, value, "mimic must discard client header %s", key)
					} else {
						require.Equal(t, "synthetic-downstream", value, "preserve native/API-key passthrough: %s", key)
					}
				}
			})
		}
	}
}

func TestCalmCountTokensMimicInjectsMissingDurableMetadata(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, metadata := range []string{"", `,"metadata":{"trace":"kept"}`, `,"metadata":{"user_id":"","trace":"kept"}`} {
			t.Run(typ+"/"+metadata, func(t *testing.T) {
				resetGatewayForwardingSettingsCacheForTest(t)
				settings := NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyEnableMetadataPassthrough: "true"}}, &config.Config{})
				cache := &stubIdentityCache{fingerprint: &Fingerprint{ClientID: strings.Repeat("b", 64)}}
				store := calmStore()
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: typ, Credentials: map[string]any{"access_token": "synthetic-token"}, Extra: map[string]any{"claude_user_id": strings.Repeat("c", 64)}}
				// Include generation fields to prove normalization does not let
				// them escape to the count_tokens endpoint when injecting metadata.
				body := []byte(`{"model":"claude-haiku-4-5","max_tokens":32,"stream":true,"temperature":0.5,"top_p":0.8,"top_k":4,"stop_sequences":["stop"],"messages":[{"role":"user","content":"synthetic"}]` + metadata + `}`)
				var previousID string
				for range 2 {
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"input_tokens":7}`))}}
					svc := &GatewayService{cfg: &config.Config{}, settingService: settings, identityService: NewIdentityService(cache, store), httpUpstream: upstream}
					parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, err)
					require.NoError(t, svc.ForwardCountTokens(context.Background(), calmGatewayContext(), account, parsed))
					uid := gjson.GetBytes(upstream.lastBody, "metadata.user_id").String()
					id := ParseMetadataUserID(uid)
					require.NotNil(t, id)
					require.Equal(t, calmTestClientID, id.DeviceID)
					seedSession := generateSessionUUID(buildStableSessionSeed(account.ID, calmTestClientID, "synthetic"))
					require.Equal(t, generateUUIDFromSeed("1::"+seedSession), id.SessionID, "reuse existing synthesis and rewrite formulas")
					if previousID != "" {
						require.Equal(t, previousID, uid)
					}
					previousID = uid
					if metadata != "" {
						require.Equal(t, "kept", gjson.GetBytes(upstream.lastBody, "metadata.trace").String())
					}
					for _, key := range []string{"max_tokens", "temperature", "top_p", "top_k", "stream", "stop_sequences", "stop"} {
						require.False(t, gjson.GetBytes(upstream.lastBody, key).Exists(), "count_tokens must not send generation field %s", key)
					}
				}
				require.Zero(t, store.initCalls)
				require.Zero(t, cache.getCalls)
				require.Zero(t, cache.setCalls)
			})
		}
	}
}

//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func calmGatewayContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "pi/99.0.0")
	c.Request.Header.Set("X-Stainless-Package-Version", "999.0.0")
	return c
}
func TestCalmGatewayIdentityFailureStopsAllConsumers(t *testing.T) {
	for _, state := range []string{"reserved", "read-failure", "init-failure", "invalid", "missing-service"} {
		for _, endpoint := range []string{"messages", "count_tokens", "chat-completions", "responses", "usage"} {
			t.Run(state+"/"+endpoint, func(t *testing.T) {
				store := calmStore()
				switch state {
				case "reserved":
					store.ids[1] = ""
				case "read-failure":
					store.readErr = errors.New("secret DB value")
				case "init-failure":
					delete(store.ids, 1)
					store.initErr = errors.New("secret INSERT value")
				case "invalid":
					store.ids[1] = "malformed"
				}
				identity := NewIdentityService(&stubIdentityCache{fingerprint: &Fingerprint{ClientID: strings.Repeat("b", 64), UserAgent: "pi/99.0.0"}}, store)
				if state == "missing-service" {
					identity = nil
				}
				upstream := &anthropicHTTPUpstreamRecorder{}
				svc := &GatewayService{cfg: &config.Config{}, identityService: identity, httpUpstream: upstream}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "synthetic-token"}}
				c := calmGatewayContext()
				ctx := context.Background()
				body := []byte(`{"model":"claude-haiku-4-5","messages":[{"role":"user","content":"synthetic"}]}`)
				var err error
				switch endpoint {
				case "messages":
					parsed, e := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, e)
					_, err = svc.Forward(ctx, c, account, parsed)
				case "count_tokens":
					parsed, e := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, e)
					err = svc.ForwardCountTokens(ctx, c, account, parsed)
				case "chat-completions":
					_, err = svc.ForwardAsChatCompletions(ctx, c, account, body, nil)
				case "responses":
					_, err = svc.ForwardAsResponses(ctx, c, account, []byte(`{"model":"claude-haiku-4-5","input":"synthetic"}`), nil)
				case "usage":
					fetcher := &calmUsageFetcher{}
					usage := &AccountUsageService{identityService: identity, usageFetcher: fetcher}
					_, err = usage.fetchOAuthUsageRaw(ctx, account)
					require.Zero(t, fetcher.calls)
				}
				require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
				require.NotContains(t, err.Error(), "secret")
				require.Nil(t, upstream.lastReq)
			})
		}
	}
}

type calmUsageFetcher struct {
	calls int
	fp    *Fingerprint
}

func (f *calmUsageFetcher) FetchUsage(context.Context, string, string) (*ClaudeUsageResponse, error) {
	panic("legacy usage must not be used")
}
func (f *calmUsageFetcher) FetchUsageWithOptions(_ context.Context, o *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	f.calls++
	f.fp = o.Fingerprint
	return &ClaudeUsageResponse{}, nil
}
func TestCalmUsageIgnoresFingerprintRedis(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{UserAgent: "pi/99.0.0"}, err: errors.New("invalid JSON")}
	fetcher := &calmUsageFetcher{}
	svc := &AccountUsageService{identityCache: cache, identityService: NewIdentityService(cache, calmStore()), usageFetcher: fetcher}
	_, err := svc.fetchOAuthUsageRaw(context.Background(), &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "synthetic"}})
	require.NoError(t, err)
	require.Equal(t, 1, fetcher.calls)
	require.Equal(t, calmTestClientID, fetcher.fp.ClientID)
	require.Equal(t, defaultFingerprint().UserAgent, fetcher.fp.UserAgent)
	require.Zero(t, cache.getCalls)
}

func TestCalmBuildersStableMetadataAndControlledHeaders(t *testing.T) {
	resetGatewayForwardingSettingsCacheForTest(t)
	for _, endpoint := range []string{"messages", "count_tokens"} {
		for _, mimic := range []bool{true, false} {
			t.Run(endpoint+"/"+map[bool]string{true: "mimic", false: "native"}[mimic], func(t *testing.T) {
				ctx := context.Background()
				store := calmStore()
				svc := &GatewayService{cfg: &config.Config{}, identityService: NewIdentityService(&stubIdentityCache{}, store), settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyEnableFingerprintUnification: "false"}}, &config.Config{})}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"account_uuid": "synthetic-account"}}
				userID := FormatMetadataUserID(strings.Repeat("b", 64), "inbound-account", "00000000-0000-4000-8000-000000000001", "2.1.78")
				body := []byte(`{"model":"claude-haiku-4-5","metadata":{"user_id":` + strconvQuote(userID) + `},"messages":[{"role":"user","content":"synthetic"}]}`)
				var previous string
				for _, ua := range []string{"pi/99.0.0", "claude-cli/99.0.0", "other/1.2.3"} {
					c := calmGatewayContext()
					c.Request.Header.Set("User-Agent", ua)
					for key := range claude.DefaultHeaders() {
						if key != "User-Agent" {
							c.Request.Header.Set(key, "untrusted-downstream")
						}
					}
					var req *http.Request
					var out []byte
					var err error
					if endpoint == "messages" {
						req, out, err = svc.buildUpstreamRequest(ctx, c, account, body, "synthetic", "oauth", "claude-haiku-4-5", false, mimic)
					} else {
						req, out, err = svc.buildCountTokensRequest(ctx, c, account, body, "synthetic", "oauth", "claude-haiku-4-5", mimic)
					}
					require.NoError(t, err)
					require.NoError(t, req.Body.Close())
					for key, value := range claude.DefaultHeaders() {
						require.Equal(t, value, getHeaderRaw(req.Header, key), "server-owned bundle header %s", key)
					}
					uid := gjson.GetBytes(out, "metadata.user_id").String()
					parsed := ParseMetadataUserID(uid)
					require.NotNil(t, parsed)
					require.Equal(t, calmTestClientID, parsed.DeviceID)
					require.Equal(t, generateUUIDFromSeed("1::00000000-0000-4000-8000-000000000001"), parsed.SessionID)
					if previous != "" {
						require.Equal(t, previous, uid)
					}
					previous = uid
				}
			})
		}
	}
}
func TestCalmAPIKeyBypassesClaudeIdentityStore(t *testing.T) {
	store := calmStore()
	store.readErr = errors.New("DB down")
	svc := &GatewayService{cfg: &config.Config{}, identityService: NewIdentityService(&stubIdentityCache{}, store)}
	for _, endpoint := range []string{"messages", "count_tokens"} {
		c := calmGatewayContext()
		account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
		body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
		var req *http.Request
		var err error
		if endpoint == "messages" {
			req, _, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "synthetic", "apikey", "claude-haiku-4-5", false, false)
		} else {
			req, _, err = svc.buildCountTokensRequest(context.Background(), c, account, body, "synthetic", "apikey", "claude-haiku-4-5", false)
		}
		require.NoError(t, err)
		require.NoError(t, req.Body.Close())
		require.Equal(t, "pi/99.0.0", getHeaderRaw(req.Header, "User-Agent"))
		require.Equal(t, "999.0.0", getHeaderRaw(req.Header, "X-Stainless-Package-Version"))
		require.Zero(t, store.initCalls)
	}
}

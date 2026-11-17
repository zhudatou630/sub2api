//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCalmAdminClaudeConsumersFailClosed(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, state := range []string{"reserved", "read-error", "init-error", "invalid", "missing-service"} {
			for _, endpoint := range []string{"connection-test", "models"} {
				t.Run(typ+"/"+state+"/"+endpoint, func(t *testing.T) {
					store := calmStore()
					switch state {
					case "reserved":
						store.ids[1] = ""
					case "read-error":
						store.readErr = errors.New("synthetic-private-storage-error")
					case "init-error":
						delete(store.ids, 1)
						store.initErr = errors.New("synthetic-private-storage-error")
					case "invalid":
						store.ids[1] = "invalid"
					}
					identity := NewIdentityService(&stubIdentityCache{}, store)
					if state == "missing-service" {
						identity = nil
					}
					upstream := &anthropicHTTPUpstreamRecorder{}
					svc := &AccountTestService{cfg: &config.Config{}, identityService: identity, httpUpstream: upstream}
					account := &Account{ID: 1, Platform: PlatformAnthropic, Type: typ, Credentials: map[string]any{"access_token": "synthetic-token"}}
					var err error
					if endpoint == "models" {
						var req *http.Request
						req, err = svc.buildAnthropicUpstreamModelsRequest(context.Background(), account)
						require.Nil(t, req)
						require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
					} else {
						err = svc.testClaudeAccountConnection(calmGatewayContext(), account, "claude-haiku-4-5")
						require.ErrorContains(t, err, ErrClaudeIdentityUnavailable.Error())
					}
					require.NotContains(t, err.Error(), "synthetic-private-storage-error")
					require.Nil(t, upstream.lastReq)
					if state != "init-error" {
						require.Zero(t, store.initCalls, "admin probes must not initialize a reserved identity")
					}
				})
			}
		}
	}
}

func TestCalmAdminClaudeConsumersUseDurableIdentity(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		t.Run(typ, func(t *testing.T) {
			store := calmStore()
			if typ == AccountTypeAPIKey {
				store.readErr = errors.New("DB unavailable: API keys must be unaffected")
			}
			cache := &stubIdentityCache{fingerprint: &Fingerprint{ClientID: strings.Repeat("b", 64), UserAgent: "untrusted"}}
			upstream := &anthropicHTTPUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
			svc := &AccountTestService{cfg: &config.Config{}, identityService: NewIdentityService(cache, store), httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: typ, Credentials: map[string]any{"access_token": "synthetic-token", "api_key": "synthetic-key"}}
			require.NoError(t, svc.testClaudeAccountConnection(calmGatewayContext(), account, "claude-haiku-4-5"))
			require.NotNil(t, upstream.lastReq)
			if account.IsOAuth() {
				metadata := ParseMetadataUserID(gjson.GetBytes(upstream.lastBody, "metadata.user_id").String())
				require.NotNil(t, metadata)
				require.Equal(t, calmTestClientID, metadata.DeviceID)
			}
			for key, value := range claude.DefaultHeaders() {
				require.Equal(t, value, getHeaderRaw(upstream.lastReq.Header, key))
			}
			req, err := svc.buildAnthropicUpstreamModelsRequest(context.Background(), account)
			require.NoError(t, err)
			for key, value := range claude.DefaultHeaders() {
				require.Equal(t, value, getHeaderRaw(req.Header, key))
			}
			require.Zero(t, cache.getCalls)
			require.Zero(t, cache.setCalls)
			require.Zero(t, store.initCalls)
		})
	}
}

func TestCalmRotatedTokenRetryCannotBypassIdentityFailure(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat-completions", "responses"} {
		for _, state := range []string{"reserved", "read-error", "invalid"} {
			t.Run(endpoint+"/"+state, func(t *testing.T) {
				store := calmStore()
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "synthetic-old"}}
				latest := *account
				latest.Credentials = map[string]any{"access_token": "synthetic-new"}
				repo := &rotatedClaudeTokenRepo{account: &latest}
				calls := 0
				upstream := &systemOneHTTPUpstream{do: func(req *http.Request) (*http.Response, error) {
					calls++
					_ = req.Body.Close()
					switch state {
					case "reserved":
						store.ids[1] = ""
					case "read-error":
						store.readErr = errors.New("synthetic-private-storage-error")
					case "invalid":
						store.ids[1] = "invalid"
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"authentication_error"}}`))}, nil
				}}
				svc := &GatewayService{cfg: &config.Config{}, identityService: NewIdentityService(&stubIdentityCache{}, store), accountRepo: repo, httpUpstream: upstream}
				body := []byte(`{"model":"claude-haiku-4-5","messages":[{"role":"user","content":"synthetic"}]}`)
				c := calmGatewayContext()
				var err error
				switch endpoint {
				case "messages":
					parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, parseErr)
					_, err = svc.Forward(context.Background(), c, account, parsed)
				case "chat-completions":
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
				case "responses":
					_, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(`{"model":"claude-haiku-4-5","input":"synthetic"}`), nil)
				}
				require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
				require.NotContains(t, err.Error(), "synthetic-private-storage-error")
				require.Equal(t, 1, calls, "do not send a retry with a fallback identity")
				require.Zero(t, store.initCalls)
			})
		}
	}
}

func TestCalmMissingAccountUUIDCannotPassThroughDownstreamIdentity(t *testing.T) {
	resetGatewayForwardingSettingsCacheForTest(t)
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, endpoint := range []string{"messages", "count_tokens"} {
			t.Run(typ+"/"+endpoint, func(t *testing.T) {
				identity := NewIdentityService(&stubIdentityCache{}, calmStore())
				svc := &GatewayService{cfg: &config.Config{}, identityService: identity}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: typ}
				const session = "00000000-0000-4000-8000-000000000001"
				userID := FormatMetadataUserID(strings.Repeat("b", 64), "downstream-account", session, "2.1.78")
				body := []byte(`{"model":"claude-haiku-4-5","metadata":{"user_id":` + strconvQuote(userID) + `},"messages":[]}`)
				var req *http.Request
				var out []byte
				var err error
				if endpoint == "messages" {
					req, out, err = svc.buildUpstreamRequest(context.Background(), calmGatewayContext(), account, body, "synthetic", "oauth", "claude-haiku-4-5", false, false)
				} else {
					req, out, err = svc.buildCountTokensRequest(context.Background(), calmGatewayContext(), account, body, "synthetic", "oauth", "claude-haiku-4-5", false)
				}
				require.NoError(t, err)
				require.NoError(t, req.Body.Close())
				metadata := ParseMetadataUserID(gjson.GetBytes(out, "metadata.user_id").String())
				require.NotNil(t, metadata)
				require.Equal(t, calmTestClientID, metadata.DeviceID)
				require.Empty(t, metadata.AccountUUID)
				require.Equal(t, generateUUIDFromSeed("1::"+session), metadata.SessionID)
			})
		}
	}
}

func TestCalmMetadataPassthroughCannotBypassDurableDevice(t *testing.T) {
	for _, account := range []*Account{
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeSetupToken},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		{ID: 1, Platform: PlatformTypeSafe, Type: AccountTypeOAuth},
	} {
		for _, endpoint := range []string{"messages", "count_tokens"} {
			t.Run(account.Platform+"/"+account.Type+"/"+endpoint, func(t *testing.T) {
				resetGatewayForwardingSettingsCacheForTest(t)
				settings := NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyEnableMetadataPassthrough: "true"}}, &config.Config{})
				svc := &GatewayService{cfg: &config.Config{}, settingService: settings, identityService: NewIdentityService(&stubIdentityCache{}, calmStore())}
				const session = "00000000-0000-4000-8000-000000000001"
				uid := FormatMetadataUserID(strings.Repeat("b", 64), "downstream-account", session, "2.1.78")
				body := []byte(`{"model":"claude-haiku-4-5","metadata":{"trace":"preserved","user_id":` + strconvQuote(uid) + `},"messages":[]}`)
				var req *http.Request
				var out []byte
				var err error
				tokenType := "oauth"
				if account.Type == AccountTypeAPIKey {
					tokenType = "apikey"
				}
				if endpoint == "messages" {
					req, out, err = svc.buildUpstreamRequest(context.Background(), calmGatewayContext(), account, body, "synthetic", tokenType, "claude-haiku-4-5", false, false)
				} else {
					req, out, err = svc.buildCountTokensRequest(context.Background(), calmGatewayContext(), account, body, "synthetic", tokenType, "claude-haiku-4-5", false)
				}
				require.NoError(t, err)
				require.NoError(t, req.Body.Close())
				if account.Platform == PlatformAnthropic && account.IsOAuth() {
					metadata := ParseMetadataUserID(gjson.GetBytes(out, "metadata.user_id").String())
					require.NotNil(t, metadata)
					require.Equal(t, calmTestClientID, metadata.DeviceID)
					require.Equal(t, generateUUIDFromSeed("1::"+session), metadata.SessionID, "retain the existing account/session derivation formula")
				} else {
					require.Equal(t, uid, gjson.GetBytes(out, "metadata.user_id").String())
				}
				require.Equal(t, "preserved", gjson.GetBytes(out, "metadata.trace").String())
			})
		}
	}
}

func TestCalmRotatedTokenLogsDoNotExposeDependencyErrors(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	const secret = "synthetic-private-token-or-identity"
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "synthetic-new"}}
	repo := &rotatedClaudeTokenRepo{err: errors.New(secret)}
	svc := &GatewayService{accountRepo: repo}
	require.Empty(t, svc.rotatedClaudeAccessToken(context.Background(), account, "synthetic-old"))
	repo.err, repo.account = nil, account
	cache := newClaudeTokenCacheStub()
	cache.deleteErr = errors.New(secret)
	svc.claudeTokenProvider = NewClaudeTokenProvider(repo, cache, nil)
	require.Equal(t, "synthetic-new", svc.rotatedClaudeAccessToken(context.Background(), account, "synthetic-old"))
	require.Contains(t, output.String(), "claude_oauth_401_reload_failed")
	require.Contains(t, output.String(), "claude_oauth_401_invalidate_cache_failed")
	require.NotContains(t, output.String(), secret)
}

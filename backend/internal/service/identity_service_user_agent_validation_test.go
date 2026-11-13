package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const calmTestClientID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// Legacy Redis fixture deliberately remains available to verify it is ignored.
type stubIdentityCache struct {
	fingerprint *Fingerprint
	setCalls    int
	lastSet     *Fingerprint
	getCalls    int
	err         error
}

func (s *stubIdentityCache) GetFingerprint(context.Context, int64) (*Fingerprint, error) {
	s.getCalls++
	return s.fingerprint, s.err
}
func (s *stubIdentityCache) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	s.setCalls++
	s.lastSet = fp
	return s.err
}
func (*stubIdentityCache) GetMaskedSessionID(context.Context, int64) (string, error) { return "", nil }
func (*stubIdentityCache) SetMaskedSessionID(context.Context, int64, string) error   { return nil }
func headersWithUA(ua string) http.Header                                            { h := http.Header{}; h.Set("User-Agent", ua); return h }

type calmMemoryStore struct {
	mu               sync.Mutex
	ids              map[int64]string
	readErr, initErr error
	initCalls        int
}

func calmStore() *calmMemoryStore {
	return &calmMemoryStore{ids: map[int64]string{1: calmTestClientID}}
}
func (s *calmMemoryStore) GetClientID(_ context.Context, id int64) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.ids[id]
	return v, ok, s.readErr
}
func (s *calmMemoryStore) InitializeClientID(_ context.Context, id int64, candidate string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initCalls++
	if s.initErr != nil {
		return "", s.initErr
	}
	if v, ok := s.ids[id]; ok {
		return v, nil
	}
	s.ids[id] = candidate
	return candidate, nil
}

// Replaces the old policy tests that accepted/merged valid inbound UA upgrades.
// Every old malformed/sentinel/valid/newer/missing-UA case now asserts the
// stronger policy: the whole server bundle wins, with no Redis read/write.
func TestCalmFingerprintInboundHeadersNeverControlBundle(t *testing.T) {
	for _, ua := range []string{"", "claude-cli/999.0.0-local (undefined, cli)", "claude-cli/999.0.0 (external, cli)", "claude-cli/2.1.22 (external, cli)", "claude-cli/2.9.0 (external, cli)", "pi/99.0.0 (node)", "Mozilla/5.0", "some-sdk/1.2.3", strings.Repeat("x", 300)} {
		t.Run(ua, func(t *testing.T) {
			cache := &stubIdentityCache{fingerprint: &Fingerprint{ClientID: strings.Repeat("b", 64), UserAgent: "pi/99.0.0"}}
			store := calmStore()
			svc := NewIdentityService(cache, store)
			h := headersWithUA(ua)
			for _, k := range []string{"X-Stainless-Lang", "X-Stainless-Package-Version", "X-Stainless-OS", "X-Stainless-Arch", "X-Stainless-Runtime", "X-Stainless-Runtime-Version"} {
				h.Set(k, "untrusted")
			}
			fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, h)
			require.NoError(t, err)
			want := defaultFingerprint()
			want.ClientID = calmTestClientID
			require.Equal(t, &want, fp)
			require.Zero(t, cache.getCalls)
			require.Zero(t, cache.setCalls)
			require.Zero(t, store.initCalls)
		})
	}
}

func TestCalmFingerprintRedisExpiryReadErrorAndCorruptionIgnored(t *testing.T) {
	for _, tc := range []struct {
		name string
		fp   *Fingerprint
		err  error
	}{
		{name: "expired"}, {name: "read-error", err: errors.New("redis read failed")}, {name: "decode-error", err: errors.New("invalid JSON")},
		{name: "poisoned", fp: &Fingerprint{ClientID: strings.Repeat("b", 64), UserAgent: "claude-cli/999.0.0-local", StainlessPackageVersion: "poison"}},
		{name: "stale", fp: &Fingerprint{ClientID: strings.Repeat("c", 64), UserAgent: "claude-cli/2.1.22", UpdatedAt: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &stubIdentityCache{fingerprint: tc.fp, err: tc.err}
			svc := NewIdentityService(cache, calmStore())
			for i := 0; i < 3; i++ {
				fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, nil)
				require.NoError(t, err)
				require.Equal(t, calmTestClientID, fp.ClientID)
				require.Equal(t, defaultFingerprint().UserAgent, fp.UserAgent)
			}
			require.Zero(t, cache.getCalls)
			require.Zero(t, cache.setCalls)
		})
	}
}

func TestCalmFingerprintFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, value      string
		readErr, initErr error
		missing          bool
	}{
		{name: "reserved-null"}, {name: "malformed", value: "not-a-client-id"},
		{name: "db-read-failure", value: calmTestClientID, readErr: errors.New("secret SQL/client/headers")},
		{name: "db-init-failure", missing: true, initErr: errors.New("secret insert parameter")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &calmMemoryStore{ids: map[int64]string{}, readErr: tc.readErr, initErr: tc.initErr}
			if !tc.missing {
				store.ids[1] = tc.value
			}
			fp, err := NewIdentityService(&stubIdentityCache{}, store).GetOrCreateFingerprint(context.Background(), 1, nil)
			require.Nil(t, fp)
			require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
			require.EqualError(t, err, "claude identity unavailable")
			if !tc.missing {
				require.Zero(t, store.initCalls)
			}
		})
	}
	fp, err := (*IdentityService)(nil).GetOrCreateFingerprint(context.Background(), 1, nil)
	require.Nil(t, fp)
	require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
}

func TestCalmFingerprintConcurrentInitializationReturnsWinner(t *testing.T) {
	store := &calmMemoryStore{ids: map[int64]string{}}
	svc := NewIdentityService(&stubIdentityCache{}, store)
	const n = 64
	ids := make(chan string, n)
	errs := make(chan error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fp, err := svc.GetOrCreateFingerprint(context.Background(), 2, headersWithUA("pi/99.0.0"))
			if err != nil {
				errs <- err
				return
			}
			ids <- fp.ClientID
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	winner := store.ids[2]
	require.True(t, validClaudeClientID(winner))
	count := 0
	for id := range ids {
		require.Equal(t, winner, id)
		count++
	}
	require.Equal(t, n, count)
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 3, nil)
	require.NoError(t, err)
	require.NotEqual(t, winner, fp.ClientID)
}

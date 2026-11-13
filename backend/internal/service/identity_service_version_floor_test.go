package service

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

// The old floor/merge tests expected Redis writeback and trusted higher inbound
// versions. The new policy derives the entire bundle from the runtime server
// version, regardless of whether the cached/inbound version is above or below it.
func TestCalmFingerprintRuntimeVersionOwnsEntireBundle(t *testing.T) {
	version := "9.9.9"
	withCLIVersionResolverForTest(t, func() string { return version })
	cache := &stubIdentityCache{fingerprint: &Fingerprint{ClientID: "old-redis-id", UserAgent: "claude-cli/99.0.0", StainlessPackageVersion: "0.91.1"}}
	svc := NewIdentityService(cache, calmStore())
	for _, inbound := range []string{"claude-cli/1.0.0 (external, desktop)", "claude-cli/9.9.9", "claude-cli/99.0.0", "pi/99.0.0"} {
		fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(inbound))
		require.NoError(t, err)
		require.Equal(t, "claude-cli/9.9.9 (external, cli)", fp.UserAgent)
		require.Equal(t, "0.94.0", fp.StainlessPackageVersion)
		require.Equal(t, calmTestClientID, fp.ClientID)
	}
	version = "9.9.10"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, claude.DefaultUserAgent(), fp.UserAgent)
	require.Contains(t, fp.UserAgent, "9.9.10")
	require.Equal(t, calmTestClientID, fp.ClientID)
	require.Zero(t, cache.getCalls)
	require.Zero(t, cache.setCalls)
}

type calmFailingRandom struct{}

func (calmFailingRandom) Read([]byte) (int, error) { return 0, errors.New("random unavailable") }
func TestCalmFingerprintRandomFailureDoesNotInitialize(t *testing.T) {
	previous := rand.Reader
	rand.Reader = calmFailingRandom{}
	t.Cleanup(func() { rand.Reader = previous })
	store := &calmMemoryStore{ids: map[int64]string{}}
	fp, err := NewIdentityService(&stubIdentityCache{}, store).GetOrCreateFingerprint(context.Background(), 1, nil)
	require.Nil(t, fp)
	require.ErrorIs(t, err, ErrClaudeIdentityUnavailable)
	require.Zero(t, store.initCalls)
	require.Empty(t, store.ids)
}

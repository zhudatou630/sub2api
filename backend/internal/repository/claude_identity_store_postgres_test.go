package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Opt-in, local-only PostgreSQL contract test. Uses synthetic accounts and an
// isolated disposable schema; never consumes application DB configuration.
func TestCalmClaudeIdentityPostgres(t *testing.T) {
	dsn := os.Getenv("CALM_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("set explicit loopback CALM_IDENTITY_TEST_DSN for disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, u.Hostname(), "local test database only")
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.PingContext(ctx))
	schema := fmt.Sprintf("calm_identity_test_%d", time.Now().UnixNano())
	_, err = db.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	defer func() { _, err := db.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); require.NoError(t, err) }()
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	testDB, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	defer testDB.Close()
	testDB.SetMaxOpenConns(64)
	_, err = testDB.ExecContext(ctx, `CREATE TABLE accounts (id BIGINT PRIMARY KEY, platform TEXT, type TEXT, deleted_at TIMESTAMPTZ);
 INSERT INTO accounts VALUES (1,'anthropic','oauth',NULL),(2,'anthropic','oauth',now()),(3,'anthropic','api_key',NULL),(4,'openai','oauth',NULL),(5,'anthropic','setup-token',NULL),(6,'anthropic','setup-token',now());`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("900001_calm_claude_identity.sql")
	require.NoError(t, err)
	_, err = testDB.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	store := NewClaudeIdentityStore(testDB)
	for _, id := range []int64{1, 2, 5, 6} {
		value, found, err := store.GetClientID(ctx, id)
		require.NoError(t, err)
		require.True(t, found)
		require.Empty(t, value)
		_, err = store.InitializeClientID(ctx, id, strings.Repeat("a", 64))
		require.ErrorIs(t, err, service.ErrClaudeIdentityUnavailable)
	}
	for _, id := range []int64{3, 4} {
		_, found, err := store.GetClientID(ctx, id)
		require.NoError(t, err)
		require.False(t, found)
		_, err = store.InitializeClientID(ctx, id, strings.Repeat("a", 64))
		require.ErrorIs(t, err, service.ErrClaudeIdentityUnavailable)
	}
	// Verified import is allowed only for reserved rows. Identity replacement is not.
	_, err = testDB.ExecContext(ctx, `UPDATE calm_claude_identities SET client_id=$1 WHERE account_id=1 AND client_id IS NULL`, strings.Repeat("b", 64))
	require.NoError(t, err)
	_, err = testDB.ExecContext(ctx, `UPDATE calm_claude_identities SET client_id=$1 WHERE account_id=1`, strings.Repeat("c", 64))
	require.Error(t, err)
	_, err = testDB.ExecContext(ctx, `INSERT INTO accounts VALUES (10,'anthropic','oauth',NULL)`)
	require.NoError(t, err)
	const n = 32
	start := make(chan struct{})
	results := make(chan string, n)
	failures := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			candidate := fmt.Sprintf("%064x", i+1)
			id, err := store.InitializeClientID(ctx, 10, candidate)
			if err != nil {
				failures <- err
				return
			}
			results <- id
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	winner, found, err := store.GetClientID(ctx, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, winner, 64)
	count := 0
	for id := range results {
		require.Equal(t, winner, id)
		count++
	}
	require.Equal(t, n, count)
	_, err = testDB.ExecContext(ctx, `INSERT INTO accounts VALUES (11,'anthropic','setup-token',NULL)`)
	require.NoError(t, err)
	setupID, err := store.InitializeClientID(ctx, 11, strings.Repeat("d", 64))
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("d", 64), setupID)

	// Soft deletion preserves identity; hard deletion retains normal cascade cleanup.
	_, err = testDB.ExecContext(ctx, `UPDATE accounts SET deleted_at=now() WHERE id=10`)
	require.NoError(t, err)
	id, found, err := store.GetClientID(ctx, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, winner, id)
	_, err = testDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=10`)
	require.NoError(t, err)
	_, found, err = store.GetClientID(ctx, 10)
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, testDB.Close())
	_, _, err = store.GetClientID(ctx, 1)
	require.ErrorIs(t, err, service.ErrClaudeIdentityUnavailable)
}

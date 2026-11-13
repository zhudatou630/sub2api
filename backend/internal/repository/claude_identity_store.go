package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type claudeIdentityStore struct{ db *sql.DB }

func NewClaudeIdentityStore(db *sql.DB) service.ClaudeIdentityStore {
	return &claudeIdentityStore{db: db}
}

func (r *claudeIdentityStore) GetClientID(ctx context.Context, accountID int64) (string, bool, error) {
	var id sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT client_id FROM calm_claude_identities WHERE account_id = $1`, accountID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, service.ErrClaudeIdentityUnavailable
	}
	return id.String, true, nil
}

func (r *claudeIdentityStore) InitializeClientID(ctx context.Context, accountID int64, candidate string) (string, error) {
	// No UPDATE: an initializer can never overwrite a winner or a reserved NULL.
	// Separate statements are intentional: READ COMMITTED must observe a winner
	// that committed while INSERT ... ON CONFLICT waited on its unique key.
	_, err := r.db.ExecContext(ctx, `INSERT INTO calm_claude_identities (account_id, client_id)
SELECT id, $2 FROM accounts WHERE id = $1 AND platform = 'anthropic' AND type IN ('oauth', 'setup-token')
ON CONFLICT (account_id) DO NOTHING`, accountID, candidate)
	if err != nil {
		return "", service.ErrClaudeIdentityUnavailable
	}
	id, found, err := r.GetClientID(ctx, accountID)
	if err != nil || !found || id == "" {
		return "", service.ErrClaudeIdentityUnavailable
	}
	return id, nil
}

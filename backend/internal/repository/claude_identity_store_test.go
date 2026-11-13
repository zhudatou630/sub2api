package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCalmClaudeIdentityStoreReadStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rows  *sqlmock.Rows
		err   error
		found bool
		id    string
	}{
		{name: "absent", rows: sqlmock.NewRows([]string{"client_id"})},
		{name: "reserved", rows: sqlmock.NewRows([]string{"client_id"}).AddRow(nil), found: true},
		{name: "stored", rows: sqlmock.NewRows([]string{"client_id"}).AddRow("synthetic"), found: true, id: "synthetic"},
		{name: "failure", err: errors.New("sensitive SQL value")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			q := mock.ExpectQuery(regexp.QuoteMeta(`SELECT client_id FROM calm_claude_identities WHERE account_id = $1`)).WithArgs(int64(1))
			if tc.err != nil {
				q.WillReturnError(tc.err)
			} else {
				q.WillReturnRows(tc.rows)
			}
			id, found, err := NewClaudeIdentityStore(db).GetClientID(context.Background(), 1)
			if tc.err != nil {
				require.ErrorIs(t, err, service.ErrClaudeIdentityUnavailable)
				require.NotContains(t, err.Error(), "sensitive")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.found, found)
				require.Equal(t, tc.id, id)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
func TestCalmClaudeIdentityStoreInitializationRereadsWinner(t *testing.T) {
	for _, tc := range []struct {
		name               string
		insertErr, readErr error
		winner             sql.NullString
		wantErr            bool
	}{
		{name: "winner-not-candidate", winner: sql.NullString{String: "synthetic-winner", Valid: true}},
		{name: "reserved-winner", wantErr: true},
		{name: "insert-failure", insertErr: errors.New("secret insert"), wantErr: true},
		{name: "read-failure", readErr: errors.New("secret read"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			q := mock.ExpectExec(`INSERT INTO calm_claude_identities .*`).WithArgs(int64(1), "synthetic-candidate")
			if tc.insertErr != nil {
				q.WillReturnError(tc.insertErr)
			} else {
				q.WillReturnResult(sqlmock.NewResult(0, 0))
				read := mock.ExpectQuery(`SELECT client_id FROM calm_claude_identities`).WithArgs(int64(1))
				if tc.readErr != nil {
					read.WillReturnError(tc.readErr)
				} else {
					read.WillReturnRows(sqlmock.NewRows([]string{"client_id"}).AddRow(tc.winner))
				}
			}
			id, err := NewClaudeIdentityStore(db).InitializeClientID(context.Background(), 1, "synthetic-candidate")
			if tc.wantErr {
				require.ErrorIs(t, err, service.ErrClaudeIdentityUnavailable)
				require.Empty(t, id)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.winner.String, id)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

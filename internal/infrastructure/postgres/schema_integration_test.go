//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const testDatabaseURLEnvironment = "TEST_DATABASE_URL"

func TestWalletMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)

	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
	if !tableExists(t, ctx, connection, "wallets") {
		t.Error("wallets table does not exist after up migration")
	}

	applyMigrationFile(t, ctx, connection, "000001_wallets.down.sql")
	if tableExists(t, ctx, connection, "wallets") {
		t.Error("wallets table still exists after down migration")
	}
}

func TestWalletConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)

	tests := []struct {
		name       string
		seed       string
		invalidSQL string
	}{
		{
			name: "duplicate wallet ID",
			seed: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000102',
				'USD', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "duplicate player and currency",
			seed: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000002',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "negative balance",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', -1, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "nonpositive version",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 0, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "unsupported currency",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'EUR', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "updated before creation",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:01Z', '2026-10-14T12:00:00Z'
			)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPublicSchema(t, ctx, connection)
			applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
			if tt.seed != "" {
				if _, err := connection.Exec(ctx, tt.seed); err != nil {
					t.Fatalf("seed valid wallet: %v", err)
				}
			}
			if _, err := connection.Exec(ctx, tt.invalidSQL); err == nil {
				t.Fatal("invalid wallet insert succeeded")
			}
		})
	}
}

func TestWalletAcceptsValidStates(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name     string
		validSQL string
	}{
		{
			name: "zero BRL opening state",
			validSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "positive USD transitioned state",
			validSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000002',
				'00000000-0000-0000-0000-000000000102',
				'USD', 9223372036854775807, 2,
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
			)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPublicSchema(t, ctx, connection)
			applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
			if _, err := connection.Exec(ctx, tt.validSQL); err != nil {
				t.Fatalf("valid wallet insert failed: %v", err)
			}
		})
	}
}

func TestWagerTransactionMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")

	applyMigrationFile(t, ctx, connection, "000002_wager_transactions.up.sql")
	if !tableExists(t, ctx, connection, "wager_transactions") {
		t.Error("wager_transactions table does not exist after up migration")
	}

	applyMigrationFile(t, ctx, connection, "000002_wager_transactions.down.sql")
	if tableExists(t, ctx, connection, "wager_transactions") {
		t.Error("wager_transactions table still exists after down migration")
	}
	if !tableExists(t, ctx, connection, "wallets") {
		t.Error("wallets table was removed by wager transaction down migration")
	}
}

func TestPendingBetWagerTransactionAcceptsValidState(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePendingBetSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)

	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet: %v", err)
	}
}

func TestPendingBetMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePendingBetSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)

	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet before down migration: %v", err)
	}

	applyMigrationFile(t, ctx, connection, "000003_pending_bet.down.sql")
	_, err := connection.Exec(ctx, `INSERT INTO wager_transactions (
		id, provider_id, external_transaction_id, idempotency_key, payload_hash,
		wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
		status, created_at, updated_at
	) VALUES (
		'00000000-0000-0000-0000-000000000302',
		'provider-a', 'external-bet-2', 'provider-a:external-bet-2', 'hash-bet-2',
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000101',
		'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
		'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
	)`)
	assertPostgreSQLConstraint(t, err, "wager_transactions_kind_supported")

	if _, err := connection.Exec(ctx, validOpeningInsertSQL); err != nil {
		t.Fatalf("insert valid opening after pending bet down migration: %v", err)
	}
}

func TestPendingBetShapeConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		invalidSQL string
		constraint string
	}{
		{name: "missing provider ID", invalidSQL: "UPDATE wager_transactions SET provider_id = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank provider ID", invalidSQL: "UPDATE wager_transactions SET provider_id = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "missing external transaction ID", invalidSQL: "UPDATE wager_transactions SET external_transaction_id = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank external transaction ID", invalidSQL: "UPDATE wager_transactions SET external_transaction_id = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "missing idempotency key", invalidSQL: "UPDATE wager_transactions SET idempotency_key = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank idempotency key", invalidSQL: "UPDATE wager_transactions SET idempotency_key = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "missing payload hash", invalidSQL: "UPDATE wager_transactions SET payload_hash = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank payload hash", invalidSQL: "UPDATE wager_transactions SET payload_hash = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "missing round ID", invalidSQL: "UPDATE wager_transactions SET round_id = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank round ID", invalidSQL: "UPDATE wager_transactions SET round_id = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "missing game ID", invalidSQL: "UPDATE wager_transactions SET game_id = NULL", constraint: "wager_transactions_pending_bet_shape"},
		{name: "blank game ID", invalidSQL: "UPDATE wager_transactions SET game_id = ' '", constraint: "wager_transactions_pending_bet_shape"},
		{name: "external reference", invalidSQL: "UPDATE wager_transactions SET reference_external_transaction_id = 'external-bet'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "resolved reference", invalidSQL: "UPDATE wager_transactions SET resolved_reference_transaction_id = '00000000-0000-0000-0000-000000000901'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "failure code", invalidSQL: "UPDATE wager_transactions SET failure_code = 'INSUFFICIENT_FUNDS'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "result balance amount", invalidSQL: "UPDATE wager_transactions SET result_balance_minor = 100", constraint: "wager_transactions_pending_bet_shape"},
		{name: "result balance currency", invalidSQL: "UPDATE wager_transactions SET result_balance_currency = 'BRL'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "reference attempts", invalidSQL: "UPDATE wager_transactions SET reference_attempts = 1", constraint: "wager_transactions_pending_bet_shape"},
		{name: "next reference attempt", invalidSQL: "UPDATE wager_transactions SET next_reference_attempt_at = '2026-10-14T12:01:00Z'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "reference expiration", invalidSQL: "UPDATE wager_transactions SET reference_expires_at = '2026-10-14T13:00:00Z'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "processed status", invalidSQL: "UPDATE wager_transactions SET status = 'PROCESSED'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "zero amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = 0", constraint: "wager_transactions_pending_bet_shape"},
		{name: "negative amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = -1", constraint: "wager_transactions_pending_bet_shape"},
		{name: "changed update timestamp", invalidSQL: "UPDATE wager_transactions SET updated_at = '2026-10-14T12:00:01Z'", constraint: "wager_transactions_pending_bet_shape"},
		{name: "win before its schema exists", invalidSQL: "UPDATE wager_transactions SET kind = 'WIN'", constraint: "wager_transactions_kind_supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preparePendingBetSchema(t, ctx, connection)
			insertTestWallet(t, ctx, connection)
			if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
				t.Fatalf("insert valid pending bet: %v", err)
			}

			_, err := connection.Exec(ctx, tt.invalidSQL)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

func TestOpeningWagerTransactionAcceptsValidState(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareWagerTransactionSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)

	if _, err := connection.Exec(ctx, validOpeningInsertSQL); err != nil {
		t.Fatalf("insert valid opening transaction: %v", err)
	}
}

func TestOpeningWagerTransactionShapeConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		invalidSQL string
		constraint string
	}{
		{name: "provider ID", invalidSQL: "UPDATE wager_transactions SET provider_id = 'provider-a'", constraint: "wager_transactions_opening_shape"},
		{name: "external transaction ID", invalidSQL: "UPDATE wager_transactions SET external_transaction_id = 'external-1'", constraint: "wager_transactions_opening_shape"},
		{name: "idempotency key", invalidSQL: "UPDATE wager_transactions SET idempotency_key = 'key-1'", constraint: "wager_transactions_opening_shape"},
		{name: "payload hash", invalidSQL: "UPDATE wager_transactions SET payload_hash = 'hash-1'", constraint: "wager_transactions_opening_shape"},
		{name: "round ID", invalidSQL: "UPDATE wager_transactions SET round_id = 'round-1'", constraint: "wager_transactions_opening_shape"},
		{name: "game ID", invalidSQL: "UPDATE wager_transactions SET game_id = 'game-1'", constraint: "wager_transactions_opening_shape"},
		{name: "external reference", invalidSQL: "UPDATE wager_transactions SET reference_external_transaction_id = 'external-bet'", constraint: "wager_transactions_opening_shape"},
		{name: "resolved reference", invalidSQL: "UPDATE wager_transactions SET resolved_reference_transaction_id = '00000000-0000-0000-0000-000000000901'", constraint: "wager_transactions_opening_shape"},
		{name: "failure code", invalidSQL: "UPDATE wager_transactions SET failure_code = 'INVALID_REFERENCE'", constraint: "wager_transactions_opening_shape"},
		{name: "result balance amount", invalidSQL: "UPDATE wager_transactions SET result_balance_minor = 100", constraint: "wager_transactions_opening_shape"},
		{name: "result balance currency", invalidSQL: "UPDATE wager_transactions SET result_balance_currency = 'BRL'", constraint: "wager_transactions_opening_shape"},
		{name: "reference attempts", invalidSQL: "UPDATE wager_transactions SET reference_attempts = 1", constraint: "wager_transactions_opening_shape"},
		{name: "next reference attempt", invalidSQL: "UPDATE wager_transactions SET next_reference_attempt_at = '2026-10-14T12:01:00Z'", constraint: "wager_transactions_opening_shape"},
		{name: "reference expiration", invalidSQL: "UPDATE wager_transactions SET reference_expires_at = '2026-10-14T13:00:00Z'", constraint: "wager_transactions_opening_shape"},
		{name: "external kind before its schema exists", invalidSQL: "UPDATE wager_transactions SET kind = 'BET'", constraint: "wager_transactions_kind_supported"},
		{name: "unsupported status", invalidSQL: "UPDATE wager_transactions SET status = 'UNKNOWN'", constraint: "wager_transactions_opening_shape"},
		{name: "unsupported currency", invalidSQL: "UPDATE wager_transactions SET currency = 'EUR'", constraint: "wager_transactions_currency_supported"},
		{name: "negative reference attempts", invalidSQL: "UPDATE wager_transactions SET reference_attempts = -1", constraint: "wager_transactions_opening_shape"},
		{name: "update timestamp before creation", invalidSQL: "UPDATE wager_transactions SET updated_at = '2026-10-14T11:59:59Z'", constraint: "wager_transactions_opening_shape"},
		{name: "pending status", invalidSQL: "UPDATE wager_transactions SET status = 'PENDING'", constraint: "wager_transactions_opening_shape"},
		{name: "zero amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = 0", constraint: "wager_transactions_opening_shape"},
		{name: "negative amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = -1", constraint: "wager_transactions_opening_shape"},
		{name: "changed update timestamp", invalidSQL: "UPDATE wager_transactions SET updated_at = '2026-10-14T12:00:01Z'", constraint: "wager_transactions_opening_shape"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareWagerTransactionSchema(t, ctx, connection)
			insertTestWallet(t, ctx, connection)
			if _, err := connection.Exec(ctx, validOpeningInsertSQL); err != nil {
				t.Fatalf("insert valid opening transaction: %v", err)
			}

			_, err := connection.Exec(ctx, tt.invalidSQL)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

func TestOpeningWagerTransactionIdentityConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		invalidSQL string
		constraint string
	}{
		{
			name: "duplicate transaction ID",
			invalidSQL: `INSERT INTO wager_transactions (
				id, wallet_id, player_id, kind, amount_minor, currency, status, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000201',
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'OPENING', 100, 'BRL', 'PROCESSED',
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			constraint: "wager_transactions_pkey",
		},
		{
			name: "second opening for wallet",
			invalidSQL: `INSERT INTO wager_transactions (
				id, wallet_id, player_id, kind, amount_minor, currency, status, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000202',
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'OPENING', 100, 'BRL', 'PROCESSED',
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			constraint: "wager_transactions_opening_wallet_unique",
		},
		{
			name:       "different player from wallet",
			invalidSQL: "UPDATE wager_transactions SET player_id = '00000000-0000-0000-0000-000000000102'",
			constraint: "wager_transactions_wallet_identity_fkey",
		},
		{
			name:       "different currency from wallet",
			invalidSQL: "UPDATE wager_transactions SET currency = 'USD'",
			constraint: "wager_transactions_wallet_identity_fkey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareWagerTransactionSchema(t, ctx, connection)
			insertTestWallet(t, ctx, connection)
			if _, err := connection.Exec(ctx, validOpeningInsertSQL); err != nil {
				t.Fatalf("insert valid opening transaction: %v", err)
			}

			_, err := connection.Exec(ctx, tt.invalidSQL)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

const validPendingBetInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	status, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000301',
	'provider-a', 'external-bet-1', 'provider-a:external-bet-1', 'hash-bet-1',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
)`

const validOpeningInsertSQL = `INSERT INTO wager_transactions (
	id, wallet_id, player_id, kind, amount_minor, currency, status, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000201',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'OPENING', 100, 'BRL', 'PROCESSED',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
)`

func preparePendingBetSchema(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
) {
	t.Helper()
	prepareWagerTransactionSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000003_pending_bet.up.sql")
}

func prepareWagerTransactionSchema(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
	applyMigrationFile(t, ctx, connection, "000002_wager_transactions.up.sql")
}

func insertTestWallet(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	if _, err := connection.Exec(ctx, `INSERT INTO wallets (
		id, player_id, currency, balance_minor, version, created_at, updated_at
	) VALUES (
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000101',
		'BRL', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("insert test wallet: %v", err)
	}
}

func assertPostgreSQLConstraint(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("statement succeeded; want constraint %q", expected)
	}
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		t.Fatalf("error = %v; want PostgreSQL constraint %q", err, expected)
	}
	if databaseError.ConstraintName != expected {
		t.Fatalf("constraint = %q; want %q; error = %v", databaseError.ConstraintName, expected, err)
	}
}

func openTestDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	databaseURL := os.Getenv(testDatabaseURLEnvironment)
	if databaseURL == "" {
		t.Fatalf("%s is required for integration tests", testDatabaseURLEnvironment)
	}

	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(ctx); err != nil {
			t.Errorf("close PostgreSQL connection: %v", err)
		}
	})
	return ctx, connection
}

func resetPublicSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	if _, err := connection.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("reset public schema: %v", err)
	}
}

func applyMigrationFile(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
	name string,
) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "migrations", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %q: %v", name, err)
	}
	if _, err := connection.Exec(ctx, string(contents)); err != nil {
		t.Fatalf("apply migration %q: %v", name, err)
	}
}

func tableExists(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
	name string,
) bool {
	t.Helper()
	var exists bool
	if err := connection.QueryRow(
		ctx,
		"SELECT to_regclass($1) IS NOT NULL",
		"public."+name,
	).Scan(&exists); err != nil {
		t.Fatalf("check table %q: %v", name, err)
	}
	return exists
}

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

func TestProcessedBetWagerTransactionAcceptsValidState(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareProcessedBetSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)

	if _, err := connection.Exec(ctx, validProcessedBetInsertSQL); err != nil {
		t.Fatalf("insert valid processed bet: %v", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO wager_transactions (
		id, provider_id, external_transaction_id, idempotency_key, payload_hash,
		wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
		status, result_balance_minor, result_balance_currency, created_at, updated_at
	) VALUES (
		'00000000-0000-0000-0000-000000000312',
		'provider-a', 'external-bet-processed-same-time', 'provider-a:external-bet-processed-same-time', 'hash-bet-processed-same-time',
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000101',
		'round-1', 'game-1', 'BET', 100, 'BRL', 'PROCESSED',
		0, 'BRL',
		'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("insert processed bet with equal timestamps: %v", err)
	}
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet after processed bet schema: %v", err)
	}
}

func TestProcessedReversalMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validProcessedRefundInsertSQL); err != nil {
		t.Fatalf("insert processed refund: %v", err)
	}
	_, err := connection.Exec(ctx, validDuplicateRefundInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_processed_reversal_reference_unique")
	_, err = connection.Exec(ctx, "UPDATE wager_transactions SET amount_minor = 0 WHERE kind = 'REFUND'")
	assertPostgreSQLConstraint(t, err, "wager_transactions_reversal_shape")

	applyMigrationFile(t, ctx, connection, "000011_processed_reversal.down.sql")
	_, err = connection.Exec(ctx, validProcessedRefundInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_kind_supported")
}

func TestPendingReferenceMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validPendingReferenceInsertSQL); err != nil {
		t.Fatalf("insert pending reference: %v", err)
	}
	_, err := connection.Exec(ctx, "UPDATE wager_transactions SET next_reference_attempt_at = updated_at WHERE status = 'PENDING_REFERENCE'")
	assertPostgreSQLConstraint(t, err, "wager_transactions_processed_win_shape")

	applyMigrationFile(t, ctx, connection, "000012_pending_reference.down.sql")
	_, err = connection.Exec(ctx, validPendingReferenceInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_processed_win_shape")
	if _, err := connection.Exec(ctx, validProcessedWinInsertSQL); err != nil {
		t.Fatalf("insert processed win after pending reference down migration: %v", err)
	}
}

func TestProcessedWinMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validProcessedWinInsertSQL); err != nil {
		t.Fatalf("insert processed win: %v", err)
	}
	if _, err := connection.Exec(ctx, validReferencedWinInsertSQL); err != nil {
		t.Fatalf("insert referenced win: %v", err)
	}
	_, err := connection.Exec(ctx, "UPDATE wager_transactions SET amount_minor = 0 WHERE id = '00000000-0000-0000-0000-000000000331'")
	assertPostgreSQLConstraint(t, err, "wager_transactions_processed_win_shape")

	applyMigrationFile(t, ctx, connection, "000010_processed_win.down.sql")
	_, err = connection.Exec(ctx, validProcessedWinInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_kind_supported")
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert pending bet after processed win down migration: %v", err)
	}
}

func TestProcessedLossMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareProcessedBetSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000009_processed_loss.up.sql")
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validProcessedLossInsertSQL); err != nil {
		t.Fatalf("insert processed loss: %v", err)
	}
	_, err := connection.Exec(ctx, "UPDATE wager_transactions SET amount_minor = 1 WHERE kind = 'LOSS'")
	assertPostgreSQLConstraint(t, err, "wager_transactions_processed_loss_shape")

	applyMigrationFile(t, ctx, connection, "000009_processed_loss.down.sql")
	_, err = connection.Exec(ctx, validProcessedLossInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_kind_supported")
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert pending bet after processed loss down migration: %v", err)
	}
}

func TestRejectedBetMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareProcessedBetSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000008_rejected_bet.up.sql")
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validRejectedBetInsertSQL); err != nil {
		t.Fatalf("insert rejected bet: %v", err)
	}
	_, err := connection.Exec(ctx, "UPDATE wager_transactions SET failure_code = NULL WHERE status = 'REJECTED'")
	assertPostgreSQLConstraint(t, err, "wager_transactions_rejected_bet_shape")

	applyMigrationFile(t, ctx, connection, "000008_rejected_bet.down.sql")
	_, err = connection.Exec(ctx, validRejectedBetInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_bet_status_supported")
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert pending bet after rejected bet down migration: %v", err)
	}
}

func TestProcessedBetMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareProcessedBetSchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validProcessedBetInsertSQL); err != nil {
		t.Fatalf("insert valid processed bet before down migration: %v", err)
	}

	applyMigrationFile(t, ctx, connection, "000005_processed_bet.down.sql")
	_, err := connection.Exec(ctx, validProcessedBetInsertSQL)
	assertPostgreSQLConstraint(t, err, "wager_transactions_pending_bet_shape")
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet after processed bet down migration: %v", err)
	}
}

func TestProcessedBetShapeConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		invalidSQL string
		constraint string
	}{
		{name: "missing result balance amount", invalidSQL: "UPDATE wager_transactions SET result_balance_minor = NULL", constraint: "wager_transactions_processed_bet_shape"},
		{name: "negative result balance", invalidSQL: "UPDATE wager_transactions SET result_balance_minor = -1", constraint: "wager_transactions_processed_bet_shape"},
		{name: "missing result balance currency", invalidSQL: "UPDATE wager_transactions SET result_balance_currency = NULL", constraint: "wager_transactions_processed_bet_shape"},
		{name: "result balance currency mismatch", invalidSQL: "UPDATE wager_transactions SET result_balance_currency = 'USD'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "update timestamp before creation", invalidSQL: "UPDATE wager_transactions SET updated_at = '2026-10-14T11:59:59Z'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "external reference", invalidSQL: "UPDATE wager_transactions SET reference_external_transaction_id = 'external-bet'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "resolved reference", invalidSQL: "UPDATE wager_transactions SET resolved_reference_transaction_id = '00000000-0000-0000-0000-000000000901'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "failure code", invalidSQL: "UPDATE wager_transactions SET failure_code = 'INSUFFICIENT_FUNDS'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "reference attempts", invalidSQL: "UPDATE wager_transactions SET reference_attempts = 1", constraint: "wager_transactions_processed_bet_shape"},
		{name: "next reference attempt", invalidSQL: "UPDATE wager_transactions SET next_reference_attempt_at = '2026-10-14T12:01:00Z'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "reference expiration", invalidSQL: "UPDATE wager_transactions SET reference_expires_at = '2026-10-14T13:00:00Z'", constraint: "wager_transactions_processed_bet_shape"},
		{name: "zero amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = 0", constraint: "wager_transactions_processed_bet_shape"},
		{name: "negative amount", invalidSQL: "UPDATE wager_transactions SET amount_minor = -1", constraint: "wager_transactions_processed_bet_shape"},
		{name: "rejected status", invalidSQL: "UPDATE wager_transactions SET status = 'REJECTED'", constraint: "wager_transactions_bet_status_supported"},
		{name: "win before its schema exists", invalidSQL: "UPDATE wager_transactions SET kind = 'WIN'", constraint: "wager_transactions_kind_supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareProcessedBetSchema(t, ctx, connection)
			insertTestWallet(t, ctx, connection)
			if _, err := connection.Exec(ctx, validProcessedBetInsertSQL); err != nil {
				t.Fatalf("insert valid processed bet: %v", err)
			}

			_, err := connection.Exec(ctx, tt.invalidSQL)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
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

func TestPendingBetIdentityIsUnique(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		secondSQL  string
		constraint string
	}{
		{
			name: "same provider and external transaction ID",
			secondSQL: `INSERT INTO wager_transactions (
				id, provider_id, external_transaction_id, idempotency_key, payload_hash,
				wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
				status, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000302',
				'provider-a', 'external-bet-1', 'provider-a:other-key', 'hash-other',
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			constraint: "wager_transactions_provider_external_id_unique",
		},
		{
			name: "same provider and idempotency key",
			secondSQL: `INSERT INTO wager_transactions (
				id, provider_id, external_transaction_id, idempotency_key, payload_hash,
				wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
				status, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000303',
				'provider-a', 'external-bet-2', 'provider-a:external-bet-1', 'hash-other',
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			constraint: "wager_transactions_provider_idempotency_key_unique",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preparePendingBetIdentitySchema(t, ctx, connection)
			insertTestWallet(t, ctx, connection)
			if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
				t.Fatalf("insert valid pending bet: %v", err)
			}

			_, err := connection.Exec(ctx, tt.secondSQL)
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

func TestPendingBetIdentityAllowsAnotherProvider(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePendingBetIdentitySchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet: %v", err)
	}

	if _, err := connection.Exec(ctx, `INSERT INTO wager_transactions (
		id, provider_id, external_transaction_id, idempotency_key, payload_hash,
		wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
		status, created_at, updated_at
	) VALUES (
		'00000000-0000-0000-0000-000000000304',
		'provider-b', 'external-bet-1', 'provider-a:external-bet-1', 'hash-bet-1',
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000101',
		'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
		'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("insert same identities for another provider: %v", err)
	}
}

func TestPendingBetIdentityMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePendingBetIdentitySchema(t, ctx, connection)
	insertTestWallet(t, ctx, connection)
	if _, err := connection.Exec(ctx, validPendingBetInsertSQL); err != nil {
		t.Fatalf("insert valid pending bet: %v", err)
	}

	applyMigrationFile(t, ctx, connection, "000004_external_identity.down.sql")
	if _, err := connection.Exec(ctx, `INSERT INTO wager_transactions (
		id, provider_id, external_transaction_id, idempotency_key, payload_hash,
		wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
		status, created_at, updated_at
	) VALUES (
		'00000000-0000-0000-0000-000000000305',
		'provider-a', 'external-bet-1', 'provider-a:other-key', 'hash-other',
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000101',
		'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING',
		'2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
	)`); err != nil {
		t.Fatalf("duplicate external ID after down migration: %v", err)
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

const validRejectedBetInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	status, failure_code, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000321',
	'provider-a', 'external-bet-rejected', 'provider-a:external-bet-rejected', 'hash-bet-rejected',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'BET', 8000, 'BRL', 'REJECTED',
	'INSUFFICIENT_FUNDS',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validDuplicateRefundInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	reference_external_transaction_id, resolved_reference_transaction_id,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000342',
	'provider-a', 'external-refund-2', 'provider-a:external-refund-2', 'hash-refund-2',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'REFUND', 2500, 'BRL',
	'external-bet-1', '00000000-0000-0000-0000-000000000311',
	'PROCESSED', 10000, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validProcessedRefundInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	reference_external_transaction_id, resolved_reference_transaction_id,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000341',
	'provider-a', 'external-refund-1', 'provider-a:external-refund-1', 'hash-refund-1',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'REFUND', 2500, 'BRL',
	'external-bet-1', '00000000-0000-0000-0000-000000000311',
	'PROCESSED', 10000, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validPendingReferenceInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	reference_external_transaction_id, status, reference_attempts,
	next_reference_attempt_at, reference_expires_at, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000333',
	'provider-a', 'external-win-pending', 'provider-a:external-win-pending', 'hash-win-pending',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'WIN', 4000, 'BRL',
	'external-bet-1', 'PENDING_REFERENCE', 1,
	'2026-10-14T12:00:02Z', '2026-10-14T12:01:01Z',
	'2026-10-14T12:00:01Z', '2026-10-14T12:00:01Z'
)`

const validProcessedWinInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000331',
	'provider-a', 'external-win-1', 'provider-a:external-win-1', 'hash-win-1',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'WIN', 4000, 'BRL', 'PROCESSED',
	14000, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validReferencedWinInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	reference_external_transaction_id, resolved_reference_transaction_id,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000332',
	'provider-a', 'external-win-2', 'provider-a:external-win-2', 'hash-win-2',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'WIN', 4000, 'BRL',
	'external-bet-1', '00000000-0000-0000-0000-000000000311',
	'PROCESSED', 14000, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validProcessedLossInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000321',
	'provider-a', 'external-loss-1', 'provider-a:external-loss-1', 'hash-loss-1',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'LOSS', 0, 'BRL', 'PROCESSED',
	10000, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

const validProcessedBetInsertSQL = `INSERT INTO wager_transactions (
	id, provider_id, external_transaction_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id, kind, amount_minor, currency,
	status, result_balance_minor, result_balance_currency, created_at, updated_at
) VALUES (
	'00000000-0000-0000-0000-000000000311',
	'provider-a', 'external-bet-processed', 'provider-a:external-bet-processed', 'hash-bet-processed',
	'00000000-0000-0000-0000-000000000001',
	'00000000-0000-0000-0000-000000000101',
	'round-1', 'game-1', 'BET', 100, 'BRL', 'PROCESSED',
	0, 'BRL',
	'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
)`

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

func prepareProcessedBetSchema(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
) {
	t.Helper()
	preparePendingBetIdentitySchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000005_processed_bet.up.sql")
}

func preparePendingBetIdentitySchema(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
) {
	t.Helper()
	preparePendingBetSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000004_external_identity.up.sql")
}

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

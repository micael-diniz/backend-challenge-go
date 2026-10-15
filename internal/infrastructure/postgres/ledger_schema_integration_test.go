//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWalletLedgerMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareWagerTransactionSchema(t, ctx, connection)

	applyMigrationFile(t, ctx, connection, "000006_wallet_ledger.up.sql")
	if !tableExists(t, ctx, connection, "wallet_ledger_entries") {
		t.Error("wallet_ledger_entries table does not exist after up migration")
	}

	applyMigrationFile(t, ctx, connection, "000006_wallet_ledger.down.sql")
	if tableExists(t, ctx, connection, "wallet_ledger_entries") {
		t.Error("wallet_ledger_entries table still exists after down migration")
	}
	if !tableExists(t, ctx, connection, "wallets") || !tableExists(t, ctx, connection, "wager_transactions") {
		t.Error("down migration removed a table owned by an earlier migration")
	}
}

func TestWalletLedgerAcceptsCreditAndDebitEquations(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareLedgerSchema(t, ctx, connection)
	insertLedgerOpening(t, ctx, connection, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000101", "00000000-0000-0000-0000-000000000201")
	insertLedgerOpening(t, ctx, connection, "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000102", "00000000-0000-0000-0000-000000000202")

	if _, err := connection.Exec(ctx, ledgerInsertSQL(
		"00000000-0000-0000-0000-000000000301",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000201",
		"CREDIT", 100, 0, 100,
	)); err != nil {
		t.Fatalf("insert credit ledger: %v", err)
	}
	if _, err := connection.Exec(ctx, ledgerInsertSQL(
		"00000000-0000-0000-0000-000000000302",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000202",
		"DEBIT", 40, 100, 60,
	)); err != nil {
		t.Fatalf("insert debit ledger: %v", err)
	}
}

func TestWalletLedgerRejectsInvalidEntries(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name       string
		direction  string
		amount     int
		before     int
		after      int
		constraint string
	}{
		{name: "credit equation", direction: "CREDIT", amount: 100, before: 0, after: 90, constraint: "wallet_ledger_entries_equation"},
		{name: "debit equation", direction: "DEBIT", amount: 40, before: 100, after: 70, constraint: "wallet_ledger_entries_equation"},
		{name: "zero amount", direction: "CREDIT", amount: 0, before: 0, after: 0, constraint: "wallet_ledger_entries_amount_positive"},
		{name: "negative balance before", direction: "CREDIT", amount: 100, before: -1, after: 99, constraint: "wallet_ledger_entries_balances_nonnegative"},
		{name: "unsupported direction", direction: "HOLD", amount: 100, before: 0, after: 100, constraint: "wallet_ledger_entries_direction_supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareLedgerSchema(t, ctx, connection)
			insertLedgerOpening(t, ctx, connection, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000101", "00000000-0000-0000-0000-000000000201")
			_, err := connection.Exec(ctx, ledgerInsertSQL(
				"00000000-0000-0000-0000-000000000301",
				"00000000-0000-0000-0000-000000000001",
				"00000000-0000-0000-0000-000000000201",
				tt.direction, tt.amount, tt.before, tt.after,
			))
			assertPostgreSQLConstraint(t, err, tt.constraint)
		})
	}
}

func TestWalletLedgerRejectsDuplicateWalletTransaction(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareLedgerSchema(t, ctx, connection)
	insertLedgerOpening(t, ctx, connection, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000101", "00000000-0000-0000-0000-000000000201")
	first := ledgerInsertSQL(
		"00000000-0000-0000-0000-000000000301",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000201",
		"CREDIT", 100, 0, 100,
	)
	if _, err := connection.Exec(ctx, first); err != nil {
		t.Fatalf("insert first ledger: %v", err)
	}
	_, err := connection.Exec(ctx, ledgerInsertSQL(
		"00000000-0000-0000-0000-000000000302",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000201",
		"CREDIT", 100, 0, 100,
	))
	assertPostgreSQLConstraint(t, err, "wallet_ledger_entries_wallet_transaction_unique")
}

func TestWalletLedgerRejectsUpdateAndDelete(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareLedgerSchema(t, ctx, connection)
	insertLedgerOpening(t, ctx, connection, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000101", "00000000-0000-0000-0000-000000000201")
	if _, err := connection.Exec(ctx, ledgerInsertSQL(
		"00000000-0000-0000-0000-000000000301",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000201",
		"CREDIT", 100, 0, 100,
	)); err != nil {
		t.Fatalf("insert ledger: %v", err)
	}

	_, updateErr := connection.Exec(ctx, "UPDATE wallet_ledger_entries SET amount_minor = 1")
	assertAppendOnly(t, updateErr)
	_, deleteErr := connection.Exec(ctx, "DELETE FROM wallet_ledger_entries")
	assertAppendOnly(t, deleteErr)

	var count int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallet_ledger_entries").Scan(&count); err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if count != 1 {
		t.Errorf("ledger count = %d; want 1", count)
	}
}

func prepareLedgerSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	prepareWagerTransactionSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000006_wallet_ledger.up.sql")
}

func insertLedgerOpening(t *testing.T, ctx context.Context, connection *pgx.Conn, walletID string, playerID string, transactionID string) {
	t.Helper()
	if _, err := connection.Exec(ctx, `INSERT INTO wallets (
		id, player_id, currency, balance_minor, version, created_at, updated_at
	) VALUES ($1::uuid, $2::uuid, 'BRL', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z')`, walletID, playerID); err != nil {
		t.Fatalf("insert ledger wallet: %v", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO wager_transactions (
		id, wallet_id, player_id, kind, amount_minor, currency, status, created_at, updated_at
	) VALUES ($1::uuid, $2::uuid, $3::uuid, 'OPENING', 100, 'BRL', 'PROCESSED', '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z')`, transactionID, walletID, playerID); err != nil {
		t.Fatalf("insert ledger opening: %v", err)
	}
}

func ledgerInsertSQL(id string, walletID string, transactionID string, direction string, amount int, before int, after int) string {
	return fmt.Sprintf(`INSERT INTO wallet_ledger_entries (
		id, wallet_id, transaction_id, direction, amount_minor, balance_before_minor, balance_after_minor, currency, created_at
	) VALUES (
		'%s', '%s', '%s', '%s', %d, %d, %d, 'BRL', '2026-10-14T12:00:00Z'
	)`, id, walletID, transactionID, direction, amount, before, after)
}

func assertAppendOnly(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("ledger mutation succeeded; want append-only rejection")
	}
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "55000" {
		t.Fatalf("error = %v; want append-only SQLSTATE 55000", err)
	}
}

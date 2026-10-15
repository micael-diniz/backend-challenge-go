package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

const postgresInvalidTextRepresentation = "22P02"

func (store *WalletOpeningStore) ListLedgerEntries(
	ctx context.Context,
	walletID domain.WalletID,
	after time.Time,
	afterID domain.LedgerEntryID,
	limit int,
) ([]domain.WalletLedgerEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var afterAt any
	var afterEntry any
	if !after.IsZero() || afterID != "" {
		afterAt = after
		afterEntry = string(afterID)
	}
	rows, err := store.connection.Query(ctx, `
		SELECT id::text, transaction_id::text, direction, amount_minor,
			balance_before_minor, balance_after_minor, currency, created_at
		FROM wallet_ledger_entries
		WHERE wallet_id = $1::uuid
			AND (
				$2::timestamptz IS NULL
				OR (created_at, id) > ($2::timestamptz, $3::uuid)
			)
		ORDER BY created_at, id
		LIMIT $4`,
		string(walletID),
		afterAt,
		afterEntry,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list ledger: %w", err)
	}
	defer rows.Close()
	var entries []domain.WalletLedgerEntry
	for rows.Next() {
		entry, err := scanLedgerEntry(rows, walletID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ledger: %w", err)
	}
	return entries, nil
}

func (store *WalletOpeningStore) FindTransactionByID(
	ctx context.Context,
	transactionID domain.TransactionID,
) (domain.WagerTransaction, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.WagerTransaction{}, false, err
	}
	transaction, err := scanWagerTransaction(store.connection.QueryRow(ctx, wagerColumnsSQL+`
		WHERE id = $1::uuid`, string(transactionID)))
	if errors.Is(err, pgx.ErrNoRows) || postgresInvalidText(err) {
		return domain.WagerTransaction{}, false, nil
	}
	if err != nil {
		return domain.WagerTransaction{}, false, fmt.Errorf("find transaction: %w", err)
	}
	return transaction, true, nil
}

func scanLedgerEntry(row scanner, walletID domain.WalletID) (domain.WalletLedgerEntry, error) {
	var (
		id, transactionID, direction, currency string
		amountMinor, beforeMinor, afterMinor   int64
		createdAt                              time.Time
	)
	if err := row.Scan(&id, &transactionID, &direction, &amountMinor, &beforeMinor, &afterMinor, &currency, &createdAt); err != nil {
		return domain.WalletLedgerEntry{}, fmt.Errorf("scan ledger: %w", err)
	}
	amount, err := moneyFromMinor(amountMinor, currency)
	if err != nil {
		return domain.WalletLedgerEntry{}, fmt.Errorf("scan ledger amount: %w", err)
	}
	before, err := moneyFromMinor(beforeMinor, currency)
	if err != nil {
		return domain.WalletLedgerEntry{}, fmt.Errorf("scan ledger balance before: %w", err)
	}
	after, err := moneyFromMinor(afterMinor, currency)
	if err != nil {
		return domain.WalletLedgerEntry{}, fmt.Errorf("scan ledger balance after: %w", err)
	}
	entry, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            domain.LedgerEntryID(id),
		WalletID:      walletID,
		TransactionID: domain.TransactionID(transactionID),
		Direction:     domain.LedgerDirection(direction),
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		CreatedAt:     createdAt,
	})
	if err != nil {
		return domain.WalletLedgerEntry{}, fmt.Errorf("rehydrate ledger: %w", err)
	}
	return entry, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func textValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func postgresInvalidText(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == postgresInvalidTextRepresentation
}

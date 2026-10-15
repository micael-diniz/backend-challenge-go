//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
)

func TestPostgresWalletOpeningStorePersistsZeroBalanceWallet(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "0.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	var (
		id           string
		player       string
		currency     string
		balanceMinor int64
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
	)
	if err := connection.QueryRow(ctx, `SELECT id::text, player_id::text, currency, balance_minor, version, created_at, updated_at FROM wallets`).Scan(
		&id, &player, &currency, &balanceMinor, &version, &createdAt, &updatedAt,
	); err != nil {
		t.Fatalf("read persisted wallet: %v", err)
	}
	if id != string(wallet.ID()) || player != string(playerID) || currency != "BRL" || balanceMinor != 0 || version != 1 {
		t.Errorf(
			"row = (%s, %s, %s, %d, %d); want (%s, %s, BRL, 0, 1)",
			id, player, currency, balanceMinor, version, wallet.ID(), playerID,
		)
	}
	if !createdAt.Equal(openedAt) || !updatedAt.Equal(openedAt) {
		t.Errorf("timestamps = (%s, %s); want both %s", createdAt, updatedAt, openedAt)
	}

	_, err = service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "0.00"))
	if !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second OpenWallet() error = %v; want ErrWalletAlreadyExists", err)
	}
	if count := countWallets(t, ctx, connection); count != 1 {
		t.Errorf("wallets count = %d; want 1", count)
	}
}

func TestPostgresWalletOpeningStoreRejectsCanceledContextWithoutInsert(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")

	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), time.Now)
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := service.OpenWallet(canceled, postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "0.00"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenWallet() error = %v; want context.Canceled", err)
	}
	if count := countWallets(t, ctx, connection); count != 0 {
		t.Errorf("wallets count = %d; want 0", count)
	}
}

func TestPostgresWalletOpeningStorePersistsPositiveOpening(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	preparePositiveOpeningSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := postgres.NewWalletOpeningStore(connection)
	service := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	if wallet.Balance().MinorUnits() != 10000 || wallet.Version() != 1 {
		t.Errorf("wallet = %d version %d; want 10000 version 1", wallet.Balance().MinorUnits(), wallet.Version())
	}

	var balanceMinor int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor); err != nil {
		t.Fatalf("read wallet balance: %v", err)
	}
	if balanceMinor != 10000 {
		t.Errorf("stored balance_minor = %d; want 10000", balanceMinor)
	}

	var kind, status string
	var amountMinor int64
	if err := connection.QueryRow(ctx, "SELECT kind, status, amount_minor FROM wager_transactions").Scan(&kind, &status, &amountMinor); err != nil {
		t.Fatalf("read opening: %v", err)
	}
	if kind != "OPENING" || status != "PROCESSED" || amountMinor != 10000 {
		t.Errorf("opening = (%s, %s, %d); want OPENING PROCESSED 10000", kind, status, amountMinor)
	}

	var direction string
	var beforeMinor, afterMinor int64
	if err := connection.QueryRow(ctx, "SELECT direction, balance_before_minor, balance_after_minor FROM wallet_ledger_entries").Scan(&direction, &beforeMinor, &afterMinor); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if direction != "CREDIT" || beforeMinor != 0 || afterMinor != 10000 {
		t.Errorf("ledger = (%s, %d, %d); want CREDIT 0 10000", direction, beforeMinor, afterMinor)
	}

	var outboxCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND attempts = 0").Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if outboxCount != 2 {
		t.Errorf("unpublished outbox events = %d; want 2", outboxCount)
	}

	_, err = service.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if !errors.Is(err, application.ErrWalletAlreadyExists) {
		t.Fatalf("second OpenWallet() error = %v; want ErrWalletAlreadyExists", err)
	}
	if count := countWallets(t, ctx, connection); count != 1 {
		t.Errorf("wallets count = %d; want 1", count)
	}
}

func TestPostgresWalletOpeningStorePersistsProcessedBet(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	openService := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt })
	betService := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	playerID := domain.PlayerID("00000000-0000-0000-0000-000000000101")

	wallet, err := openService.OpenWallet(ctx, postgresOpenWalletCommand(t, playerID, "100.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	updated, bet, _, err := betService.PlaceBet(ctx, application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
		CausationID:           "request-bet-1",
	})
	if err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	if updated.Balance().Amount() != "75.00" || updated.Version() != 2 {
		t.Errorf("wallet = %s version %d; want 75.00 version 2", updated.Balance().Amount(), updated.Version())
	}
	result, present := bet.ResultBalance()
	if bet.Status() != domain.WagerTransactionStatusProcessed || !present || result.Amount() != "75.00" {
		t.Errorf("bet result = (%s, %s, %t); want processed 75.00", bet.Status(), result.Amount(), present)
	}

	var balanceMinor, version int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if balanceMinor != 7500 || version != 2 {
		t.Errorf("stored wallet = (%d, %d); want (7500, 2)", balanceMinor, version)
	}
	var kind, status string
	var resultMinor int64
	if err := connection.QueryRow(ctx, "SELECT kind, status, result_balance_minor FROM wager_transactions WHERE kind = 'BET'").Scan(&kind, &status, &resultMinor); err != nil {
		t.Fatalf("read bet: %v", err)
	}
	if kind != "BET" || status != "PROCESSED" || resultMinor != 7500 {
		t.Errorf("stored bet = (%s, %s, %d); want BET PROCESSED 7500", kind, status, resultMinor)
	}
	var direction string
	var beforeMinor, afterMinor int64
	if err := connection.QueryRow(ctx, "SELECT direction, balance_before_minor, balance_after_minor FROM wallet_ledger_entries WHERE direction = 'DEBIT'").Scan(&direction, &beforeMinor, &afterMinor); err != nil {
		t.Fatalf("read debit: %v", err)
	}
	if beforeMinor != 10000 || afterMinor != 7500 {
		t.Errorf("debit = (%d, %d); want (10000, 7500)", beforeMinor, afterMinor)
	}
	var outboxCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events").Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outboxCount != 4 {
		t.Errorf("outbox events = %d; want 4", outboxCount)
	}
}

func TestPostgresWalletOpeningStoreRejectsStaleBetVersion(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, uuidSequence(), func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	debited, err := wallet.Debit(amount, placedAt)
	if err != nil {
		t.Fatalf("Debit() returned an unexpected error: %v", err)
	}
	pending, err := domain.NewExternalWagerTransaction(domain.ExternalWagerTransactionInput{
		ID:                    "00000000-0000-0000-0000-000000000301",
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-stale",
		IdempotencyKey:        "provider-a:external-bet-stale",
		PayloadHash:           "hash-bet-stale",
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  domain.WagerTransactionKindBet,
		Money:                 amount,
		CreatedAt:             placedAt,
	})
	if err != nil {
		t.Fatalf("NewExternalWagerTransaction() returned an unexpected error: %v", err)
	}
	processed, err := pending.Process(domain.ProcessWagerTransactionInput{ResultBalance: debited.Balance(), OccurredAt: placedAt})
	if err != nil {
		t.Fatalf("Process() returned an unexpected error: %v", err)
	}
	ledger, err := domain.NewWalletLedgerEntry(domain.WalletLedgerEntryInput{
		ID:            "00000000-0000-0000-0000-000000000302",
		WalletID:      wallet.ID(),
		TransactionID: processed.ID(),
		Direction:     domain.LedgerDirectionDebit,
		Amount:        amount,
		BalanceBefore: wallet.Balance(),
		BalanceAfter:  debited.Balance(),
		CreatedAt:     placedAt,
	})
	if err != nil {
		t.Fatalf("NewWalletLedgerEntry() returned an unexpected error: %v", err)
	}
	processedEvent, err := domain.NewWagerTransactionProcessed(domain.DomainEventMetadataInput{
		EventID:       "00000000-0000-0000-0000-000000000303",
		CorrelationID: "correlation-1",
	}, processed)
	if err != nil {
		t.Fatalf("NewWagerTransactionProcessed() returned an unexpected error: %v", err)
	}
	balanceEvent, err := domain.NewWalletBalanceChanged(domain.DomainEventMetadataInput{
		EventID:       "00000000-0000-0000-0000-000000000304",
		CorrelationID: "correlation-1",
	}, ledger, debited)
	if err != nil {
		t.Fatalf("NewWalletBalanceChanged() returned an unexpected error: %v", err)
	}

	err = store.SaveProcessedBet(ctx, wallet, processed, ledger, processedEvent, balanceEvent)
	if !errors.Is(err, application.ErrWalletVersionConflict) {
		t.Fatalf("SaveProcessedBet() error = %v; want version conflict", err)
	}
	var betCount int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET'").Scan(&betCount); err != nil {
		t.Fatalf("count bets: %v", err)
	}
	if betCount != 0 {
		t.Errorf("bet count = %d; want 0", betCount)
	}
	var version int64
	if err := connection.QueryRow(ctx, "SELECT version FROM wallets").Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 1 {
		t.Errorf("wallet version = %d; want 1", version)
	}
}

func TestPostgresWalletOpeningStoreReplaysBetAndRejectsChangedContent(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	betService := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	command := application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
	}
	if _, _, _, err := betService.PlaceBet(ctx, command); err != nil {
		t.Fatalf("first PlaceBet() returned an unexpected error: %v", err)
	}
	_, bet, replay, err := betService.PlaceBet(ctx, command)
	if err != nil {
		t.Fatalf("replay PlaceBet() returned an unexpected error: %v", err)
	}
	if !replay {
		t.Fatal("second PlaceBet() was not a replay")
	}
	result, present := bet.ResultBalance()
	if !present || result.Amount() != "75.00" {
		t.Errorf("replay result = (%s, %t); want 75.00", result.Amount(), present)
	}

	changed := command
	changedAmount, err := domain.NewMoney("10.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	changed.Money = changedAmount
	if _, _, _, err := betService.PlaceBet(ctx, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed content error = %v; want idempotency conflict", err)
	}
	otherKey := command
	otherKey.IdempotencyKey = "provider-a:other-key"
	if _, _, _, err := betService.PlaceBet(ctx, otherKey); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("other key error = %v; want idempotency conflict", err)
	}

	var balanceMinor, version, betCount int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET'").Scan(&betCount); err != nil {
		t.Fatalf("count bets: %v", err)
	}
	if balanceMinor != 7500 || version != 2 || betCount != 1 {
		t.Errorf("stored state = (%d, %d, %d); want (7500, 2, 1)", balanceMinor, version, betCount)
	}
}

func TestPostgresRecordWinCreditsWalletAndReferencedBet(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	service := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	betAmount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	betCommand := application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 betAmount,
		CorrelationID:         "correlation-1",
	}
	_, bet, _, err := service.PlaceBet(ctx, betCommand)
	if err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	winAmount, err := domain.NewMoney("40.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	winCommand := betCommand
	winCommand.ExternalTransactionID = "external-win-1"
	winCommand.IdempotencyKey = "provider-a:external-win-1"
	winCommand.Money = winAmount
	winCommand.ReferenceExternalTransactionID = betCommand.ExternalTransactionID
	_, win, replay, err := service.RecordWin(ctx, winCommand)
	if err != nil {
		t.Fatalf("RecordWin() returned an unexpected error: %v", err)
	}
	resolved, present := win.ResolvedReferenceTransactionID()
	if replay || !present || resolved != bet.ID() {
		t.Fatalf("resolved = (%s, %t) replay %t; want %s", resolved, present, replay, bet.ID())
	}

	var balanceMinor, version, creditCount int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallet_ledger_entries WHERE direction = 'CREDIT'").Scan(&creditCount); err != nil {
		t.Fatalf("count credits: %v", err)
	}
	if balanceMinor != 11500 || version != 3 || creditCount != 2 {
		t.Fatalf("stored state = (%d, %d, %d); want (11500, 3, 2)", balanceMinor, version, creditCount)
	}
	_, replayed, replay, err := service.RecordWin(ctx, winCommand)
	if err != nil {
		t.Fatalf("replay RecordWin() returned an unexpected error: %v", err)
	}
	replayResult, replayPresent := replayed.ResultBalance()
	replayResolved, replayResolvedPresent := replayed.ResolvedReferenceTransactionID()
	if !replay || replayed.ID() != win.ID() || !replayPresent || replayResult.Amount() != "115.00" || !replayResolvedPresent || replayResolved != bet.ID() {
		t.Fatalf("replay = %t id %s result (%s, %t) resolved (%s, %t)", replay, replayed.ID(), replayResult.Amount(), replayPresent, replayResolved, replayResolvedPresent)
	}

	missing := winCommand
	missing.ExternalTransactionID = "external-win-2"
	missing.IdempotencyKey = "provider-a:external-win-2"
	missing.ReferenceExternalTransactionID = "missing-bet"
	_, held, replay, err := service.RecordWin(ctx, missing)
	if err != nil || replay || held.Status() != domain.WagerTransactionStatusPendingReference {
		t.Fatalf("missing reference status = %s replay = %t error = %v", held.Status(), replay, err)
	}
	if err := connection.QueryRow(ctx, "SELECT balance_minor FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor); err != nil {
		t.Fatalf("reread wallet: %v", err)
	}
	if balanceMinor != 11500 {
		t.Fatalf("balance after missing reference = %d; want 11500", balanceMinor)
	}
}

func TestPostgresRecordRefundRestoresBetAndRejectsAnotherReversal(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	service := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	betCommand := application.PlaceBetCommand{
		WalletID: wallet.ID(), PlayerID: wallet.PlayerID(), ProviderID: "provider-a",
		ExternalTransactionID: "external-bet-1", IdempotencyKey: "provider-a:external-bet-1",
		RoundID: "round-1", GameID: "game-1", Money: amount, CorrelationID: "correlation-1",
	}
	if _, _, _, err := service.PlaceBet(ctx, betCommand); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	refund := betCommand
	refund.ExternalTransactionID = "external-refund-1"
	refund.IdempotencyKey = "provider-a:external-refund-1"
	refund.ReferenceExternalTransactionID = betCommand.ExternalTransactionID
	if _, _, _, err := service.RecordRefund(ctx, refund); err != nil {
		t.Fatalf("RecordRefund() returned an unexpected error: %v", err)
	}
	rollback := refund
	rollback.ExternalTransactionID = "external-rollback-1"
	rollback.IdempotencyKey = "provider-a:external-rollback-1"
	if _, _, _, err := service.RecordRollback(ctx, rollback); !errors.Is(err, application.ErrDuplicateReversal) {
		t.Fatalf("rollback after refund error = %v; want duplicate reversal", err)
	}
	var balanceMinor, version int64
	var failureCode string
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT failure_code FROM wager_transactions WHERE kind = 'ROLLBACK'").Scan(&failureCode); err != nil {
		t.Fatalf("read rollback: %v", err)
	}
	if balanceMinor != 10000 || version != 3 || failureCode != "DUPLICATE_REVERSAL" {
		t.Fatalf("stored state = (%d, %d, %s); want (10000, 3, DUPLICATE_REVERSAL)", balanceMinor, version, failureCode)
	}
}

func TestPostgresRecordLossKeepsWalletAndLedger(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	recordedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	service := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return recordedAt })
	amount, err := domain.NewMoney("0.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	command := application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-loss-1",
		IdempotencyKey:        "provider-a:external-loss-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
	}
	_, loss, replay, err := service.RecordLoss(ctx, command)
	if err != nil {
		t.Fatalf("RecordLoss() returned an unexpected error: %v", err)
	}
	if replay || loss.Kind() != domain.WagerTransactionKindLoss || loss.Status() != domain.WagerTransactionStatusProcessed {
		t.Fatalf("loss = %s %s replay %t", loss.Kind(), loss.Status(), replay)
	}

	var balanceMinor, version, ledgerCount, processedEvents, balanceEvents int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallet_ledger_entries").Scan(&ledgerCount); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE event_type = 'WagerTransactionProcessed'").Scan(&processedEvents); err != nil {
		t.Fatalf("count processed events: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE event_type = 'WalletBalanceChanged'").Scan(&balanceEvents); err != nil {
		t.Fatalf("count balance events: %v", err)
	}
	if balanceMinor != 10000 || version != 1 || ledgerCount != 1 || processedEvents != 2 || balanceEvents != 1 {
		t.Fatalf("stored state = (%d, %d, %d, %d, %d); want (10000, 1, 1, 2, 1)", balanceMinor, version, ledgerCount, processedEvents, balanceEvents)
	}
}

func mustPostgresPlaceBetService(
	t *testing.T,
	store *postgres.WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) *application.PlaceBetService {
	t.Helper()
	service, err := application.NewPlaceBetService(store, newID, now)
	if err != nil {
		t.Fatalf("NewPlaceBetService() returned an unexpected error: %v", err)
	}
	return service
}

func TestPostgresReadsWalletLedgerAndTransaction(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	service := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return placedAt })
	amount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	if _, _, _, err := service.PlaceBet(ctx, application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-bet-1",
		IdempotencyKey:        "provider-a:external-bet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 amount,
		CorrelationID:         "correlation-1",
	}); err != nil {
		t.Fatalf("PlaceBet() error = %v", err)
	}
	reads, err := application.NewReadService(store)
	if err != nil {
		t.Fatalf("NewReadService() error = %v", err)
	}
	loaded, err := reads.Wallet(ctx, wallet.ID())
	if err != nil || loaded.Balance().Amount() != "75.00" || loaded.Version() != 2 {
		t.Fatalf("wallet = %s version %d error %v", loaded.Balance().Amount(), loaded.Version(), err)
	}
	first, err := reads.Ledger(ctx, wallet.ID(), "", 1)
	if err != nil || len(first.Entries) != 1 || first.Entries[0].Direction() != domain.LedgerDirectionCredit || first.NextCursor == "" {
		t.Fatalf("first page = %+v error %v", first, err)
	}
	second, err := reads.Ledger(ctx, wallet.ID(), first.NextCursor, 1)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].Direction() != domain.LedgerDirectionDebit || second.NextCursor != "" {
		t.Fatalf("second page = %+v error %v", second, err)
	}
	opening, found, err := reads.Transaction(ctx, first.Entries[0].TransactionID())
	if err != nil || !found || opening.Kind() != domain.WagerTransactionKindOpening {
		t.Fatalf("opening found %t kind %s error %v", found, opening.Kind(), err)
	}
	bet, found, err := reads.ProviderTransaction(ctx, "provider-a", "external-bet-1")
	if err != nil || !found || bet.Status() != domain.WagerTransactionStatusProcessed {
		t.Fatalf("bet found %t status %s error %v", found, bet.Status(), err)
	}
}

func TestPostgresPendingReferenceResolvesAfterTheBetArrives(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	start := openedAt.Add(time.Second)
	clock := start
	ids := uuidSequence()
	store := postgres.NewWalletOpeningStore(connection)
	wallet, err := mustPostgresOpenWalletService(t, store, ids, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, "00000000-0000-0000-0000-000000000101", "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	service := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return clock })
	winAmount, err := domain.NewMoney("40.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	command := application.PlaceBetCommand{
		WalletID:                       wallet.ID(),
		PlayerID:                       wallet.PlayerID(),
		ProviderID:                     "provider-a",
		ExternalTransactionID:          "external-win-1",
		IdempotencyKey:                 "provider-a:external-win-1",
		RoundID:                        "round-1",
		GameID:                         "game-1",
		Money:                          winAmount,
		ReferenceExternalTransactionID: "external-bet-1",
		CorrelationID:                  "correlation-1",
	}
	if _, held, _, err := service.RecordWin(ctx, command); err != nil || held.Status() != domain.WagerTransactionStatusPendingReference || held.ReferenceAttempts() != 1 {
		t.Fatalf("RecordWin() status = %s attempts = %d error = %v", held.Status(), held.ReferenceAttempts(), err)
	}
	if err := service.AdvanceDueReferences(ctx); err != nil {
		t.Fatalf("early AdvanceDueReferences() error = %v", err)
	}

	clock = start.Add(time.Second)
	restarted := mustPostgresPlaceBetService(t, store, ids, func() time.Time { return clock })
	if err := restarted.AdvanceDueReferences(ctx); err != nil {
		t.Fatalf("retry AdvanceDueReferences() error = %v", err)
	}
	betAmount, err := domain.NewMoney("25.00", "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	bet := command
	bet.ExternalTransactionID = "external-bet-1"
	bet.IdempotencyKey = "provider-a:external-bet-1"
	bet.Money = betAmount
	bet.ReferenceExternalTransactionID = ""
	if _, _, _, err := restarted.PlaceBet(ctx, bet); err != nil {
		t.Fatalf("PlaceBet() error = %v", err)
	}
	clock = start.Add(3 * time.Second)
	if err := restarted.AdvanceDueReferences(ctx); err != nil {
		t.Fatalf("resolve AdvanceDueReferences() error = %v", err)
	}
	var balanceMinor, version int64
	var status string
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets WHERE id = $1::uuid", string(wallet.ID())).Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT status FROM wager_transactions WHERE external_transaction_id = 'external-win-1'").Scan(&status); err != nil {
		t.Fatalf("read win: %v", err)
	}
	if balanceMinor != 11500 || version != 3 || status != "PROCESSED" {
		t.Fatalf("stored state = (%d, %d, %s); want (11500, 3, PROCESSED)", balanceMinor, version, status)
	}
}

func prepareBetPersistenceSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	for _, name := range []string{
		"000001_wallets.up.sql",
		"000002_wager_transactions.up.sql",
		"000003_pending_bet.up.sql",
		"000004_external_identity.up.sql",
		"000005_processed_bet.up.sql",
		"000006_wallet_ledger.up.sql",
		"000007_outbox.up.sql",
		"000008_rejected_bet.up.sql",
		"000009_processed_loss.up.sql",
		"000010_processed_win.up.sql",
		"000011_processed_reversal.up.sql",
		"000012_pending_reference.up.sql",
	} {
		applyMigrationFile(t, ctx, connection, name)
	}
}

func preparePositiveOpeningSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	resetPublicSchema(t, ctx, connection)
	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
	applyMigrationFile(t, ctx, connection, "000002_wager_transactions.up.sql")
	applyMigrationFile(t, ctx, connection, "000006_wallet_ledger.up.sql")
	applyMigrationFile(t, ctx, connection, "000007_outbox.up.sql")
}

func mustPostgresOpenWalletService(
	t *testing.T,
	store *postgres.WalletOpeningStore,
	newID func() string,
	now func() time.Time,
) *application.OpenWalletService {
	t.Helper()
	service, err := application.NewOpenWalletService(store, newID, now)
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	return service
}

func postgresOpenWalletCommand(t *testing.T, playerID domain.PlayerID, amount string) application.OpenWalletCommand {
	t.Helper()
	balance, err := domain.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	return application.OpenWalletCommand{
		PlayerID:       playerID,
		InitialBalance: balance,
		CorrelationID:  "correlation-1",
	}
}

func uuidSequence() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", next)
	}
}

func countWallets(t *testing.T, ctx context.Context, connection *pgx.Conn) int {
	t.Helper()
	var count int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallets").Scan(&count); err != nil {
		t.Fatalf("count wallets: %v", err)
	}
	return count
}

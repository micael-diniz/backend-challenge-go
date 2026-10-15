package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestRecordRefundReturnsTheFullBet(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"refund-1", "ledger-2", "event-3", "event-4",
		"refund-2", "event-5",
	), func() time.Time { return placedAt })
	betCommand := placeBetCommand(t, wallet, "25.00")
	if _, _, _, err := service.PlaceBet(context.Background(), betCommand); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	refund := reversalCommand(t, betCommand, "external-refund-1", "25.00")

	credited, reversal, replay, err := service.RecordRefund(context.Background(), refund)
	if err != nil {
		t.Fatalf("RecordRefund() returned an unexpected error: %v", err)
	}
	result, present := reversal.ResultBalance()
	if replay || reversal.Kind() != domain.WagerTransactionKindRefund || !present || result.Amount() != "100.00" || credited.Version() != 3 {
		t.Fatalf("refund result (%s, %t) version %d replay %t", result.Amount(), present, credited.Version(), replay)
	}
	if store.ledger.Direction() != domain.LedgerDirectionCredit {
		t.Fatalf("ledger direction = %s; want CREDIT", store.ledger.Direction())
	}

	partial := reversalCommand(t, betCommand, "external-refund-partial", "10.00")
	if _, _, _, err := service.RecordRefund(context.Background(), partial); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("partial refund error = %v; want invalid reference", err)
	}
	second := reversalCommand(t, betCommand, "external-refund-2", "25.00")
	_, _, _, err = service.RecordRefund(context.Background(), second)
	if !errors.Is(err, ErrDuplicateReversal) {
		t.Fatalf("second refund error = %v; want duplicate reversal", err)
	}
	code, codePresent := store.bet.FailureCode()
	if !codePresent || code != domain.WagerTransactionFailureCodeDuplicateReversal || credited.Balance().Amount() != store.wallet.Balance().Amount() {
		t.Fatalf("stored rejection = (%s, %t) balance %s", code, codePresent, store.wallet.Balance().Amount())
	}
}

func TestRecordRollbackUndoesBetButNotAfterRefund(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"bet-1", "ledger-1", "event-1", "event-2",
		"rollback-1", "ledger-2", "event-3", "event-4",
		"refund-1", "event-5",
	), func() time.Time { return placedAt })
	betCommand := placeBetCommand(t, wallet, "25.00")
	if _, _, _, err := service.PlaceBet(context.Background(), betCommand); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	rollback := reversalCommand(t, betCommand, "external-rollback-1", "25.00")
	_, reversal, _, err := service.RecordRollback(context.Background(), rollback)
	if err != nil {
		t.Fatalf("RecordRollback() returned an unexpected error: %v", err)
	}
	result, present := reversal.ResultBalance()
	if reversal.Kind() != domain.WagerTransactionKindRollback || !present || result.Amount() != "100.00" {
		t.Fatalf("rollback result = (%s, %t)", result.Amount(), present)
	}
	refund := reversalCommand(t, betCommand, "external-refund-1", "25.00")
	if _, _, _, err := service.RecordRefund(context.Background(), refund); !errors.Is(err, ErrDuplicateReversal) {
		t.Fatalf("refund after rollback error = %v; want duplicate reversal", err)
	}
}

func TestRecordRollbackOfWinRejectsWhenBalanceIsShort(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)
	placedAt := openedAt.Add(time.Second)
	wallet := mustPlaceBetWallet(t, "100.00", openedAt)
	store := &betStoreStub{wallet: wallet}
	service := mustNewPlaceBetService(t, store, sequentialIDGenerator(
		"win-1", "ledger-1", "event-1", "event-2",
		"bet-1", "ledger-2", "event-3", "event-4",
		"rollback-1", "event-5",
	), func() time.Time { return placedAt })
	win := placeBetCommand(t, wallet, "80.00")
	win.ExternalTransactionID = "external-win-1"
	win.IdempotencyKey = "provider-a:external-win-1"
	if _, _, _, err := service.RecordWin(context.Background(), win); err != nil {
		t.Fatalf("RecordWin() returned an unexpected error: %v", err)
	}
	bet := placeBetCommand(t, wallet, "150.00")
	if _, _, _, err := service.PlaceBet(context.Background(), bet); err != nil {
		t.Fatalf("PlaceBet() returned an unexpected error: %v", err)
	}
	rollback := reversalCommand(t, win, "external-rollback-win", "80.00")
	_, _, _, err := service.RecordRollback(context.Background(), rollback)
	if !errors.Is(err, ErrReversalInsufficientFunds) {
		t.Fatalf("rollback error = %v; want reversal insufficient funds", err)
	}
	code, present := store.bet.FailureCode()
	if !present || code != domain.WagerTransactionFailureCodeReversalInsufficientFunds || store.wallet.Balance().Amount() != "30.00" {
		t.Fatalf("rejection = (%s, %t) balance %s", code, present, store.wallet.Balance().Amount())
	}
}

func reversalCommand(t *testing.T, reference PlaceBetCommand, externalID string, amount string) PlaceBetCommand {
	t.Helper()
	command := reference
	command.ExternalTransactionID = domain.ExternalTransactionID(externalID)
	command.IdempotencyKey = domain.IdempotencyKey("provider-a:" + externalID)
	command.Money = mustApplicationMoney(t, amount, "BRL")
	command.ReferenceExternalTransactionID = reference.ExternalTransactionID
	return command
}

package application

import (
	"bytes"
	"context"
	"expvar"
	"log/slog"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func TestReconcileReportsAStoredBalanceThatTheLedgerDoesNotExplain(t *testing.T) {
	walletID := domain.WalletID("0192f291-27dd-7d3f-8071-5f8685deef37")
	stored := mustMoney(t, "75.00")
	wallet, err := domain.RehydrateWallet(
		walletID,
		"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
		stored,
		2,
		time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.October, 14, 12, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RehydrateWallet() error = %v", err)
	}
	opening := mustLedgerEntry(t, domain.WalletLedgerEntryInput{
		ID:            "ledger-open-1",
		WalletID:      walletID,
		TransactionID: "opening-1",
		Direction:     domain.LedgerDirectionCredit,
		Amount:        mustMoney(t, "100.00"),
		BalanceBefore: mustMoney(t, "0.00"),
		BalanceAfter:  mustMoney(t, "100.00"),
		CreatedAt:     wallet.CreatedAt(),
	})
	service, err := NewReadService(reconciliationStore{wallet: wallet, entries: []domain.WalletLedgerEntry{opening}})
	if err != nil {
		t.Fatalf("NewReadService() error = %v", err)
	}

	before := expvar.Get("reconciliation_divergences").(*expvar.Int).Value()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	report, err := service.Reconcile(context.Background(), walletID)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.StoredBalance.Amount() != "75.00" || report.CalculatedBalance.Amount() != "100.00" ||
		report.Difference.Amount() != "-25.00" || report.Consistent || report.CheckedEntries != 1 {
		t.Fatalf("report = %+v; want stored 75.00 minus ledger 100.00", report)
	}
	if expvar.Get("reconciliation_divergences").(*expvar.Int).Value() != before+1 {
		t.Fatalf("divergence metric = %d; want %d", expvar.Get("reconciliation_divergences").(*expvar.Int).Value(), before+1)
	}
	if !bytes.Contains(logs.Bytes(), []byte(walletID)) {
		t.Fatalf("log = %s; want the wallet id", logs.String())
	}
	again, err := service.Reconcile(context.Background(), walletID)
	if err != nil || again.StoredBalance.Amount() != "75.00" {
		t.Fatalf("second report = %+v error %v; want the stored balance to stay 75.00", again, err)
	}
}

type reconciliationStore struct {
	wallet  domain.Wallet
	entries []domain.WalletLedgerEntry
}

func (store reconciliationStore) WalletLedger(context.Context, domain.WalletID) (domain.Wallet, []domain.WalletLedgerEntry, error) {
	return store.wallet, store.entries, nil
}

func (store reconciliationStore) LoadWallet(context.Context, domain.WalletID) (domain.Wallet, error) {
	return store.wallet, nil
}

func (store reconciliationStore) ListLedgerEntries(context.Context, domain.WalletID, time.Time, domain.LedgerEntryID, int) ([]domain.WalletLedgerEntry, error) {
	return nil, nil
}

func (store reconciliationStore) FindTransactionByID(context.Context, domain.TransactionID) (domain.WagerTransaction, bool, error) {
	return domain.WagerTransaction{}, false, nil
}

func (store reconciliationStore) FindBetByExternalID(context.Context, domain.ProviderID, domain.ExternalTransactionID) (domain.WagerTransaction, bool, error) {
	return domain.WagerTransaction{}, false, nil
}

func mustMoney(t *testing.T, amount string) domain.Money {
	t.Helper()
	money, err := domain.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney(%s) error = %v", amount, err)
	}
	return money
}

func mustLedgerEntry(t *testing.T, input domain.WalletLedgerEntryInput) domain.WalletLedgerEntry {
	t.Helper()
	entry, err := domain.NewWalletLedgerEntry(input)
	if err != nil {
		t.Fatalf("NewWalletLedgerEntry() error = %v", err)
	}
	return entry
}

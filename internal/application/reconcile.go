package application

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

var reconciliationDivergences = expvar.NewInt("reconciliation_divergences")

type Reconciliation struct {
	WalletID          domain.WalletID
	StoredBalance     domain.Money
	CalculatedBalance domain.Money
	Difference        domain.Money
	Consistent        bool
	CheckedEntries    int
}

func (service *ReadService) Reconcile(ctx context.Context, walletID domain.WalletID) (Reconciliation, error) {
	wallet, entries, err := service.store.WalletLedger(ctx, walletID)
	if err != nil {
		return Reconciliation{}, err
	}
	calculated, err := calculatedBalance(wallet.Currency(), entries)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("reconcile wallet: %w", err)
	}
	difference, err := wallet.Balance().Subtract(calculated)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("reconcile wallet: %w", err)
	}
	report := Reconciliation{
		WalletID:          wallet.ID(),
		StoredBalance:     wallet.Balance(),
		CalculatedBalance: calculated,
		Difference:        difference,
		Consistent:        difference.MinorUnits() == 0,
		CheckedEntries:    len(entries),
	}
	if !report.Consistent {
		reconciliationDivergences.Add(1)
		slog.Warn("wallet reconciliation diverged",
			"walletId", string(wallet.ID()),
			"difference", difference.Amount(),
			"currency", difference.Currency().String(),
			"checkedEntries", report.CheckedEntries,
		)
	}
	return report, nil
}

func calculatedBalance(currency domain.Currency, entries []domain.WalletLedgerEntry) (domain.Money, error) {
	total, err := domain.NewMoney("0.00", currency.String())
	if err != nil {
		return domain.Money{}, err
	}
	for _, entry := range entries {
		amount := entry.Money()
		if entry.Direction() == domain.LedgerDirectionDebit {
			amount, err = amount.Negate()
			if err != nil {
				return domain.Money{}, err
			}
		}
		total, err = total.Add(amount)
		if err != nil {
			return domain.Money{}, err
		}
	}
	return total, nil
}
